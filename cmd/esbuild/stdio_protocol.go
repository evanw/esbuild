// The JavaScript API communicates with the Go child process over stdin/stdout
// using this protocol. It's a very simple binary protocol that uses length-
// prefixed strings. Every message has a 32-bit integer id and is either a
// request or a response. You must send a response after receiving a request
// because the other end is blocking on the response coming back. Requests are
// arrays of strings and responses are maps of strings to byte arrays.

package main

import (
	"encoding/binary"
	"fmt"

	"github.com/evanw/esbuild/internal/printer"
	"github.com/evanw/esbuild/pkg/api"
)

type requestType = []string
type responseType = map[string][]byte

func readUint32(bytes []byte) (value uint32, leftOver []byte, ok bool) {
	if len(bytes) >= 4 {
		return binary.LittleEndian.Uint32(bytes), bytes[4:], true
	}

	return 0, bytes, false
}

func writeUint32(bytes []byte, value uint32) []byte {
	bytes = append(bytes, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(bytes[len(bytes)-4:], value)
	return bytes
}

func readLengthPrefixedSlice(bytes []byte) (slice []byte, leftOver []byte, ok bool) {
	if length, afterLength, ok := readUint32(bytes); ok && uint(len(afterLength)) >= uint(length) {
		return afterLength[:length], afterLength[length:], true
	}

	return []byte{}, bytes, false
}

func encodeRequest(id uint32, request requestType) []byte {
	// Each request is length-prefixed
	length := 12
	for _, v := range request {
		length += 4 + len(v)
	}
	bytes := make([]byte, 0, length)
	bytes = writeUint32(bytes, uint32(length-4))
	bytes = writeUint32(bytes, id<<1)
	bytes = writeUint32(bytes, uint32(len(request)))

	// Each request is formatted as a series of values
	for _, v := range request {
		bytes = writeUint32(bytes, uint32(len(v)))
		bytes = append(bytes, v...)
	}

	return bytes
}

func encodeResponse(id uint32, response responseType) []byte {
	// Each response is length-prefixed
	length := 12
	for k, v := range response {
		length += 4 + len(k) + 4 + len(v)
	}
	bytes := make([]byte, 0, length)
	bytes = writeUint32(bytes, uint32(length-4))
	bytes = writeUint32(bytes, (id<<1)|1)
	bytes = writeUint32(bytes, uint32(len(response)))

	// Each response is formatted as a series of key/value pairs
	for k, v := range response {
		bytes = writeUint32(bytes, uint32(len(k)))
		bytes = append(bytes, k...)
		bytes = writeUint32(bytes, uint32(len(v)))
		bytes = append(bytes, v...)
	}

	return bytes
}

func decodeRequestOrResponse(bytes []byte) (uint32, requestType, responseType) {
	// Read the id
	id, bytes, ok := readUint32(bytes)
	if !ok {
		return 0, nil, nil
	}
	isRequest := (id & 1) == 0
	id >>= 1

	// Read the argument count
	argCount, bytes, ok := readUint32(bytes)
	if !ok {
		return 0, nil, nil
	}

	if isRequest {
		// Read the request
		request := requestType{}
		for i := uint32(0); i < argCount; i++ {
			value, afterValue, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				return 0, nil, nil
			}
			bytes = afterValue
			request = append(request, string(value))
		}
		if len(bytes) != 0 {
			return 0, nil, nil
		}
		return id, request, nil
	} else {
		// Read the response
		response := responseType{}
		for i := uint32(0); i < argCount; i++ {
			key, afterKey, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				return 0, nil, nil
			}
			value, afterValue, ok := readLengthPrefixedSlice(afterKey)
			if !ok {
				return 0, nil, nil
			}
			bytes = afterValue
			response[string(key)] = value
		}
		if len(bytes) != 0 {
			return 0, nil, nil
		}
		return id, nil, response
	}
}

func encodeOutputFiles(outputFiles []api.OutputFile) []byte {
	length := 4
	for _, outputFile := range outputFiles {
		length += 4 + len(outputFile.Path) + 4 + len(outputFile.Contents)
	}
	bytes := make([]byte, 0, length)
	bytes = writeUint32(bytes, uint32(len(outputFiles)))
	for _, outputFile := range outputFiles {
		bytes = writeUint32(bytes, uint32(len(outputFile.Path)))
		bytes = append(bytes, outputFile.Path...)
		bytes = writeUint32(bytes, uint32(len(outputFile.Contents)))
		bytes = append(bytes, outputFile.Contents...)
	}
	return bytes
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
