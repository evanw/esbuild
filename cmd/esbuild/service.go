// This implements a simple long-running service over stdin/stdout. Each
// incoming request is an array of strings, and each outgoing response is a map
// of strings to byte slices. All values are length-prefixed using 32-bit
// little endian integers.

package main

import (
	"errors"
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

type responseCallback = func(interface{})

type serviceType struct {
	mutex           sync.Mutex
	callbacks       map[uint32]responseCallback
	nextID          uint32
	outgoingPackets chan outgoingPacket
}

type outgoingPacket struct {
	bytes   []byte
	isFinal bool
}

func runService() {
	service := serviceType{
		callbacks:       make(map[uint32]responseCallback),
		outgoingPackets: make(chan outgoingPacket),
	}
	buffer := make([]byte, 4096)
	stream := []byte{}

	// Write messages on a single goroutine so they aren't interleaved
	waitGroup := &sync.WaitGroup{}
	go func() {
		for {
			message, ok := <-service.outgoingPackets
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
					service.outgoingPackets <- outgoingPacket{bytes: result, isFinal: true}
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

func (service *serviceType) sendRequest(request interface{}) interface{} {
	result := make(chan interface{})
	var id uint32
	callback := func(response interface{}) {
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
	service.outgoingPackets <- outgoingPacket{
		bytes: encodePacket(packet{
			id:        id,
			isRequest: true,
			value:     request,
		}),
	}
	return <-result
}

func (service *serviceType) handleIncomingMessage(bytes []byte) (result []byte) {
	p, ok := decodePacket(bytes)
	if !ok {
		return nil
	}

	if p.isRequest {
		// Catch panics in the code below so they get passed to the caller
		defer func() {
			if r := recover(); r != nil {
				result = encodePacket(packet{
					id: p.id,
					value: map[string]interface{}{
						"error": fmt.Sprintf("Panic: %v\n\n%s", r, debug.Stack()),
					},
				})
			}
		}()

		// Handle the request
		data := p.value.([]interface{})
		switch data[0] {
		case "build":
			return service.handleBuildRequest(p.id, data[1].(map[string]interface{}))

		case "transform":
			return service.handleTransformRequest(p.id, data[1].(map[string]interface{}))

		default:
			return encodePacket(packet{
				id: p.id,
				value: map[string]interface{}{
					"error": fmt.Sprintf("Invalid command: %s", p.value),
				},
			})
		}
	}

	callback := func() responseCallback {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		callback := service.callbacks[p.id]
		delete(service.callbacks, p.id)
		return callback
	}()

	callback(p.value)
	return nil
}

func (service *serviceType) handleBuildRequest(id uint32, request map[string]interface{}) []byte {
	write := request["write"].(bool)
	flags := decodeStringArray(request["flags"].([]interface{}))
	stdin, hasStdin := request["stdin"].(string)
	resolveDir, hasResolveDir := request["resolveDir"].(string)

	options, err := cli.ParseBuildOptions(flags)
	if err == nil && write && options.Outfile == "" && options.Outdir == "" {
		err = errors.New("Either provide \"outfile\" or set \"write\" to false")
	}
	if err != nil {
		return encodePacket(packet{
			id: id,
			value: map[string]interface{}{
				"error": err.Error(),
			},
		})
	}

	// Optionally allow input from the stdin channel
	if hasStdin {
		if options.Stdin == nil {
			options.Stdin = &api.StdinOptions{}
		}
		options.Stdin.Contents = stdin
		if hasResolveDir {
			options.Stdin.ResolveDir = resolveDir
		}
	}

	result := api.Build(options)
	response := map[string]interface{}{
		"errors":   encodeMessages(result.Errors),
		"warnings": encodeMessages(result.Warnings),
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

	return encodePacket(packet{
		id:    id,
		value: response,
	})
}

func (service *serviceType) handleTransformRequest(id uint32, request map[string]interface{}) []byte {
	input := request["input"].(string)
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseTransformOptions(flags)
	if err != nil {
		return encodePacket(packet{
			id: id,
			value: map[string]interface{}{
				"error": err.Error(),
			},
		})
	}

	result := api.Transform(input, options)
	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"errors":      encodeMessages(result.Errors),
			"warnings":    encodeMessages(result.Warnings),
			"js":          string(result.JS),
			"jsSourceMap": string(result.JSSourceMap),
		},
	})
}

func decodeStringArray(values []interface{}) []string {
	strings := make([]string, len(values))
	for i, value := range values {
		strings[i] = value.(string)
	}
	return strings
}

func encodeOutputFiles(outputFiles []api.OutputFile) []interface{} {
	values := make([]interface{}, len(outputFiles))
	for i, outputFile := range outputFiles {
		value := make(map[string]interface{})
		values[i] = value
		value["path"] = outputFile.Path
		value["contents"] = outputFile.Contents
	}
	return values
}

func encodeMessages(msgs []api.Message) []interface{} {
	values := make([]interface{}, len(msgs))
	for i, msg := range msgs {
		value := make(map[string]interface{})
		values[i] = value
		value["text"] = msg.Text

		// Some messages won't have a location
		loc := msg.Location
		if loc == nil {
			value["location"] = nil
		} else {
			value["location"] = map[string]interface{}{
				"file":     loc.File,
				"line":     loc.Line,
				"column":   loc.Column,
				"length":   loc.Length,
				"lineText": loc.LineText,
			}
		}
	}
	return values
}
