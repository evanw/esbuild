// This implements a simple long-running service over stdin/stdout. Each
// incoming request is an array of strings, and each outgoing response is a map
// of strings to byte slices. All values are length-prefixed using 32-bit
// little endian integers.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/evanw/esbuild/internal/bundler"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/internal/resolver"
)

type responseType = map[string][]byte

func readUint32(bytes []byte) (value uint32, leftOver []byte, ok bool) {
	if len(bytes) >= 4 {
		return binary.LittleEndian.Uint32(bytes), bytes[4:], true
	}

	return 0, bytes, false
}

func readLengthPrefixedSlice(bytes []byte) (slice []byte, leftOver []byte, ok bool) {
	if length, afterLength, ok := readUint32(bytes); ok && uint(len(afterLength)) >= uint(length) {
		return afterLength[:length], afterLength[length:], true
	}

	return []byte{}, bytes, false
}

func runService() {
	buffer := make([]byte, 4096)
	stream := []byte{}

	// Write responses on a single goroutine so they aren't interleaved
	responses := make(chan responseType)
	go writeResponses(responses)

	for {
		// Read more data from stdin
		n, err := os.Stdin.Read(buffer)
		if n == 0 || err == io.EOF {
			break // End of stdin
		}
		if err != nil {
			panic(err)
		}
		stream = append(stream, buffer[:n]...)

		// Process all complete (i.e. not partial) requests
		bytes := stream
		for {
			request, afterRequest, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				break
			}
			bytes = afterRequest

			// Clone the input and run it on another goroutine
			clone := append([]byte{}, request...)
			go handleRequest(clone, responses)
		}

		// Move the remaining partial request to the end to avoid reallocating
		stream = append(stream[:0], bytes...)
	}
}

func writeUint32(value uint32) {
	bytes := []byte{0, 0, 0, 0}
	binary.LittleEndian.PutUint32(bytes, value)
	os.Stdout.Write(bytes)
}

func writeResponses(responses chan responseType) {
	for {
		response := <-responses

		// Each response is length-prefixed
		length := 4
		for k, v := range response {
			length += 4 + len(k) + 4 + len(v)
		}
		writeUint32(uint32(length))

		// Each response is formatted as a series of key/value pairs
		writeUint32(uint32(len(response)))
		for k, v := range response {
			writeUint32(uint32(len(k)))
			os.Stdout.Write([]byte(k))
			writeUint32(uint32(len(v)))
			os.Stdout.Write(v)
		}
	}
}

func handleRequest(bytes []byte, responses chan responseType) {
	// Read the argument count
	argCount, bytes, ok := readUint32(bytes)
	if !ok {
		return // Invalid request
	}

	// Read the arguments
	rawArgs := []string{}
	for i := uint32(0); i < argCount; i++ {
		slice, afterSlice, ok := readLengthPrefixedSlice(bytes)
		if !ok {
			return // Invalid request
		}
		rawArgs = append(rawArgs, string(slice))
		bytes = afterSlice
	}
	if len(rawArgs) < 2 {
		return // Invalid request
	}

	// Requests have the format "id command [args...]"
	id, command, rawArgs := rawArgs[0], rawArgs[1], rawArgs[2:]

	// Catch panics in the code below so they get passed to the caller
	defer func() {
		if r := recover(); r != nil {
			responses <- responseType{
				"id":    []byte(id),
				"error": []byte(fmt.Sprintf("%v\n\n%s", r, debug.Stack())),
			}
		}
	}()

	// Dispatch the command
	switch command {
	case "ping":
		handlePingRequest(responses, id, rawArgs)

	case "build":
		handleBuildRequest(responses, id, rawArgs)

	default:
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte(fmt.Sprintf("Invalid command: %s", command)),
		}
	}
}

func handlePingRequest(responses chan responseType, id string, rawArgs []string) {
	responses <- responseType{
		"id": []byte(id),
	}
}

func handleBuildRequest(responses chan responseType, id string, rawArgs []string) {
	files, rawArgs := stripFilesFromBuildArgs(rawArgs)
	if files == nil {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte("Invalid build request"),
		}
		return
	}

	mockFS := fs.MockFS(nil)
	args, err := parseArgs(mockFS, rawArgs)
	if err != nil {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte(err.Error()),
		}
		return
	}

	// Make sure we don't accidentally try to read from stdin here
	if args.bundleOptions.LoaderForStdin != bundler.LoaderNone {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte("Cannot read from stdin in service mode"),
		}
		return
	}

	// Make sure we don't accidentally try to write to stdout here
	if args.bundleOptions.WriteToStdout {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte("Cannot write to stdout in service mode"),
		}
		return
	}

	mockFS = fs.MockFS(files)
	log, join := logging.NewDeferLog()
	resolver := resolver.NewResolver(mockFS, log, args.resolveOptions)
	bundle := bundler.ScanBundle(log, mockFS, resolver, args.entryPaths, args.parseOptions, args.bundleOptions)

	// Stop now if there were errors
	msgs := join()
	errors := messagesOfKind(logging.Error, msgs)
	if len(errors) != 0 {
		responses <- responseType{
			"id":       []byte(id),
			"errors":   messagesToJSON(errors),
			"warnings": messagesToJSON(messagesOfKind(logging.Warning, msgs)),
		}
		return
	}

	// Generate the results
	log, join = logging.NewDeferLog()
	results := bundle.Compile(log, args.bundleOptions)

	// Return the results
	msgs2 := join()
	errors = messagesOfKind(logging.Error, msgs2)
	response := responseType{
		"id":     []byte(id),
		"errors": messagesToJSON(errors),
		"warnings": messagesToJSON(append(
			messagesOfKind(logging.Warning, msgs),
			messagesOfKind(logging.Warning, msgs2)...)),
	}
	for _, result := range results {
		response[result.AbsPath] = result.Contents
	}
	responses <- response
}

func stripFilesFromBuildArgs(args []string) (map[string]string, []string) {
	for i, arg := range args {
		if arg == "--" && i%2 == 0 {
			files := make(map[string]string)
			for j := 0; j < i; j += 2 {
				files[args[j]] = args[j+1]
			}
			return files, args[i+1:]
		}
	}
	return nil, []string{}
}

func messagesOfKind(kind logging.MsgKind, msgs []logging.Msg) []logging.Msg {
	filtered := []logging.Msg{}
	for _, msg := range msgs {
		if msg.Kind == kind {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

func messagesToJSON(msgs []logging.Msg) []byte {
	bytes := []byte{'['}

	for _, msg := range msgs {
		if len(bytes) > 1 {
			bytes = append(bytes, ',')
		}
		lineCount := 0
		columnCount := 0

		// Some errors won't have a location
		if msg.Source.PrettyPath != "" {
			lineCount, columnCount, _ = logging.ComputeLineAndColumn(msg.Source.Contents[0:msg.Start])
			lineCount++
		}

		bytes = append(bytes, fmt.Sprintf("%s,%s,%d,%d",
			printer.QuoteForJSON(msg.Text),
			printer.QuoteForJSON(msg.Source.PrettyPath),
			lineCount,
			columnCount)...)
	}

	bytes = append(bytes, ']')
	return bytes
}
