// This implements a simple long-running service over stdin/stdout. Each
// incoming request is an array of strings, and each outgoing response is a map
// of strings to byte slices. All values are length-prefixed using 32-bit
// little endian integers.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/evanw/esbuild/pkg/cli"
)

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
	service.outgoingMessages <- outgoingMessage{bytes: encodeRequest(id, request)}
	return <-result
}

func (service *serviceType) handleIncomingMessage(bytes []byte) (result []byte) {
	id, request, response := decodeRequestOrResponse(bytes)

	if request != nil {
		// Catch panics in the code below so they get passed to the caller
		defer func() {
			if r := recover(); r != nil {
				result = encodeResponse(id, responseType{
					"error": []byte(fmt.Sprintf("Panic: %v\n\n%s", r, debug.Stack())),
				})
			}
		}()

		// Handle the request
		switch request[0] {
		case "build":
			return service.handleBuildRequest(id, request[1:])

		case "transform":
			return service.handleTransformRequest(id, request[1:])

		default:
			return encodeResponse(id, responseType{
				"error": []byte(fmt.Sprintf("Invalid command: %s", request[0])),
			})
		}
	}

	if response != nil {
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

	return nil
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
		return encodeResponse(id, responseType{
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
		response["outputFiles"] = encodeOutputFiles(result.OutputFiles)
	}

	return encodeResponse(id, response)
}

func (service *serviceType) handleTransformRequest(id uint32, request requestType) []byte {
	if len(request) == 0 {
		return encodeResponse(id, responseType{
			"error": []byte("Invalid transform request"),
		})
	}

	options, err := cli.ParseTransformOptions(request[1:])
	if err != nil {
		return encodeResponse(id, responseType{
			"error": []byte(err.Error()),
		})
	}

	result := api.Transform(request[0], options)
	return encodeResponse(id, responseType{
		"errors":      messagesToJSON(result.Errors),
		"warnings":    messagesToJSON(result.Warnings),
		"js":          result.JS,
		"jsSourceMap": result.JSSourceMap,
	})
}
