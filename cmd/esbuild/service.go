// This implements a simple long-running service over stdin/stdout. Each
// incoming request is an array of strings, and each outgoing response is a map
// of strings to byte slices. All values are length-prefixed using 32-bit
// little endian integers.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/evanw/esbuild/pkg/cli"
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
	waitGroup := &sync.WaitGroup{}
	responses := make(chan responseType)
	go writeResponses(responses, waitGroup)

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
			waitGroup.Add(1)
			clone := append([]byte{}, request...)
			go handleRequest(clone, responses, waitGroup)
		}

		// Move the remaining partial request to the end to avoid reallocating
		stream = append(stream[:0], bytes...)
	}

	// Wait for the last response to be written to stdout
	waitGroup.Wait()
}

func writeUint32(bytes []byte, value uint32) []byte {
	bytes = append(bytes, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(bytes[len(bytes)-4:], value)
	return bytes
}

func writeResponses(responses chan responseType, waitGroup *sync.WaitGroup) {
	for {
		response, ok := <-responses
		if !ok {
			break // No more responses
		}

		// Each response is length-prefixed
		length := 4
		for k, v := range response {
			length += 4 + len(k) + 4 + len(v)
		}
		bytes := make([]byte, 0, 4+length)
		bytes = writeUint32(bytes, uint32(length))

		// Each response is formatted as a series of key/value pairs
		bytes = writeUint32(bytes, uint32(len(response)))
		for k, v := range response {
			bytes = writeUint32(bytes, uint32(len(k)))
			bytes = append(bytes, k...)
			bytes = writeUint32(bytes, uint32(len(v)))
			bytes = append(bytes, v...)
		}
		os.Stdout.Write(bytes)

		// Only signal that this request is done when it has actually been written
		waitGroup.Done()
	}
}

func handleRequest(bytes []byte, responses chan responseType, waitGroup *sync.WaitGroup) {
	// Read the argument count
	argCount, bytes, ok := readUint32(bytes)
	if !ok {
		// Invalid request
		waitGroup.Done()
		return
	}

	// Read the arguments
	rawArgs := []string{}
	for i := uint32(0); i < argCount; i++ {
		slice, afterSlice, ok := readLengthPrefixedSlice(bytes)
		if !ok {
			// Invalid request
			waitGroup.Done()
			return
		}
		rawArgs = append(rawArgs, string(slice))
		bytes = afterSlice
	}
	if len(rawArgs) < 2 {
		// Invalid request
		waitGroup.Done()
		return
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

	case "transform":
		handleTransformRequest(responses, id, rawArgs)

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
	options, err := cli.ParseBuildOptions(rawArgs)
	if err != nil {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte(err.Error()),
		}
		return
	}

	result := api.Build(options)
	for _, outputFile := range result.OutputFiles {
		if err := os.MkdirAll(filepath.Dir(outputFile.Path), 0755); err != nil {
			result.Errors = append(result.Errors, api.Message{Text: fmt.Sprintf(
				"Failed to create output directory: %s", err.Error())})
		} else if err := ioutil.WriteFile(outputFile.Path, outputFile.Contents, 0644); err != nil {
			result.Errors = append(result.Errors, api.Message{Text: fmt.Sprintf(
				"Failed to write to output file: %s", err.Error())})
		}
	}

	responses <- responseType{
		"id":       []byte(id),
		"errors":   messagesToJSON(result.Errors),
		"warnings": messagesToJSON(result.Warnings),
	}
}

func handleTransformRequest(responses chan responseType, id string, rawArgs []string) {
	if len(rawArgs) == 0 {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte("Invalid transform request"),
		}
		return
	}

	options, err := cli.ParseTransformOptions(rawArgs[1:])
	if err != nil {
		responses <- responseType{
			"id":    []byte(id),
			"error": []byte(err.Error()),
		}
		return
	}

	result := api.Transform(rawArgs[0], options)
	responses <- responseType{
		"id":          []byte(id),
		"errors":      messagesToJSON(result.Errors),
		"warnings":    messagesToJSON(result.Warnings),
		"js":          result.JS,
		"jsSourceMap": result.JSSourceMap,
	}
}

func messagesToJSON(msgs []api.Message) []byte {
	j := printer.Joiner{}
	j.AddString("[")

	for _, msg := range msgs {
		if j.Length() > 1 {
			j.AddString(",")
		}

		// Some messages won't have a location
		var location api.Location
		if msg.Location != nil {
			location = *msg.Location
		} else {
			location.Length = -1 // Signal that there's no location
		}

		j.AddString(fmt.Sprintf("%s,%d,%s,%d,%d,%s",
			printer.QuoteForJSON(msg.Text),
			location.Length,
			printer.QuoteForJSON(location.File),
			location.Line,
			location.Column,
			printer.QuoteForJSON(location.LineText),
		))
	}

	j.AddString("]")
	return j.Done()
}
