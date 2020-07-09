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

type requestType = []string
type responseType = map[string][]byte
type responseCallback = func(responseType)

type serviceType struct {
	mutex            sync.Mutex
	callbacks        map[uint32]responseCallback
	nextID           uint32
	outgoingMessages chan outgoingMessage
}

type outgoingMessage struct {
	bytes   []byte
	isFinal bool
}

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
	service := serviceType{
		callbacks:        make(map[uint32]responseCallback),
		outgoingMessages: make(chan outgoingMessage),
	}
	buffer := make([]byte, 4096)
	stream := []byte{}

	// Write messages on a single goroutine so they aren't interleaved
	waitGroup := &sync.WaitGroup{}
	go func() {
		for {
			message, ok := <-service.outgoingMessages
			if !ok {
				break // No more messages
			}
			os.Stdout.Write(message.bytes)

			// Only signal that this request is done when it has actually been written
			if message.isFinal {
				waitGroup.Done()
			}
		}
	}()

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

		// Process all complete (i.e. not partial) messages
		bytes := stream
		for {
			message, afterMessage, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				break
			}
			bytes = afterMessage

			// Clone the input and run it on another goroutine
			clone := append([]byte{}, message...)
			waitGroup.Add(1)
			go func() {
				if result := service.handleIncomingMessage(clone); result != nil {
					service.outgoingMessages <- outgoingMessage{bytes: result, isFinal: true}
				} else {
					waitGroup.Done()
				}
			}()
		}

		// Move the remaining partial message to the end to avoid reallocating
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

func buildResponse(id uint32, response responseType) []byte {
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

func buildRequest(id uint32, request requestType) []byte {
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

func (service *serviceType) sendRequest(request requestType) responseType {
	result := make(chan responseType)
	var id uint32
	callback := func(response responseType) {
		result <- response
		close(result)
	}
	id = func() uint32 {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		id := service.nextID
		service.nextID++
		service.callbacks[id] = callback
		return id
	}()
	service.outgoingMessages <- outgoingMessage{bytes: buildRequest(id, request)}
	return <-result
}

func (service *serviceType) handleIncomingMessage(bytes []byte) (result []byte) {
	// Read the id
	id, bytes, ok := readUint32(bytes)
	if !ok {
		panic("Invalid message")
	}
	isRequest := (id & 1) == 0
	id >>= 1

	// Read the argument count
	argCount, bytes, ok := readUint32(bytes)
	if !ok {
		panic("Invalid message")
	}

	if isRequest {
		// Read the request
		request := requestType{}
		for i := uint32(0); i < argCount; i++ {
			value, afterValue, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				panic("Invalid request")
			}
			bytes = afterValue
			request = append(request, string(value))
		}
		if len(request) < 1 || len(bytes) != 0 {
			panic("Invalid request")
		}
		command, request := request[0], request[1:]

		// Catch panics in the code below so they get passed to the caller
		defer func() {
			if r := recover(); r != nil {
				result = buildResponse(id, responseType{
					"error": []byte(fmt.Sprintf("Panic: %v\n\n%s", r, debug.Stack())),
				})
			}
		}()

		// Handle the request
		switch command {
		case "build":
			return service.handleBuildRequest(id, request)

		case "transform":
			return service.handleTransformRequest(id, request)

		default:
			return buildResponse(id, responseType{
				"error": []byte(fmt.Sprintf("Invalid command: %s", command)),
			})
		}
	} else {
		// Read the response
		response := responseType{}
		for i := uint32(0); i < argCount; i++ {
			key, afterKey, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				panic("Invalid response")
			}
			value, afterValue, ok := readLengthPrefixedSlice(afterKey)
			if !ok {
				panic("Invalid response")
			}
			bytes = afterValue
			response[string(key)] = value
		}
		if len(bytes) != 0 {
			panic("Invalid response")
		}

		// Handle the response
		callback := func() responseCallback {
			service.mutex.Lock()
			defer service.mutex.Unlock()
			callback := service.callbacks[id]
			delete(service.callbacks, id)
			return callback
		}()
		callback(response)
		return nil
	}
}

func (service *serviceType) handleBuildRequest(id uint32, request requestType) []byte {
	// Special-case the service-only write flag
	write := true
	for i, arg := range request {
		if arg == "--write=false" {
			write = false
			copy(request[i:], request[i+1:])
			request = request[:len(request)-1]
			break
		}
	}

	options, err := cli.ParseBuildOptions(request)
	if err != nil {
		return buildResponse(id, responseType{
			"error": []byte(err.Error()),
		})
	}

	result := api.Build(options)
	response := responseType{
		"errors":   messagesToJSON(result.Errors),
		"warnings": messagesToJSON(result.Warnings),
	}

	if write {
		// Write the output files to disk
		for _, outputFile := range result.OutputFiles {
			if err := os.MkdirAll(filepath.Dir(outputFile.Path), 0755); err != nil {
				result.Errors = append(result.Errors, api.Message{Text: fmt.Sprintf(
					"Failed to create output directory: %s", err.Error())})
			} else if err := ioutil.WriteFile(outputFile.Path, outputFile.Contents, 0644); err != nil {
				result.Errors = append(result.Errors, api.Message{Text: fmt.Sprintf(
					"Failed to write to output file: %s", err.Error())})
			}
		}
	} else {
		// Pass the output files back to the caller
		length := 4
		for _, outputFile := range result.OutputFiles {
			length += 4 + len(outputFile.Path) + 4 + len(outputFile.Contents)
		}
		bytes := make([]byte, 0, length)
		bytes = writeUint32(bytes, uint32(len(result.OutputFiles)))
		for _, outputFile := range result.OutputFiles {
			bytes = writeUint32(bytes, uint32(len(outputFile.Path)))
			bytes = append(bytes, outputFile.Path...)
			bytes = writeUint32(bytes, uint32(len(outputFile.Contents)))
			bytes = append(bytes, outputFile.Contents...)
		}
		response["outputFiles"] = bytes
	}

	return buildResponse(id, response)
}

func (service *serviceType) handleTransformRequest(id uint32, request requestType) []byte {
	if len(request) == 0 {
		return buildResponse(id, responseType{
			"error": []byte("Invalid transform request"),
		})
	}

	options, err := cli.ParseTransformOptions(request[1:])
	if err != nil {
		return buildResponse(id, responseType{
			"error": []byte(err.Error()),
		})
	}

	result := api.Transform(request[0], options)
	return buildResponse(id, responseType{
		"errors":      messagesToJSON(result.Errors),
		"warnings":    messagesToJSON(result.Warnings),
		"js":          result.JS,
		"jsSourceMap": result.JSSourceMap,
	})
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
