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
