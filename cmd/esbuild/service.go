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
	"runtime/debug"
	"sync"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
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
	buffer := make([]byte, 16*1024)
	stream := []byte{}

	// Write packets on a single goroutine so they aren't interleaved
	waitGroup := &sync.WaitGroup{}
	go func() {
		for {
			packet, ok := <-service.outgoingPackets
			if !ok {
				break // No more packets
			}
			os.Stdout.Write(packet.bytes)

			// Only signal that this request is done when it has actually been written
			if packet.isFinal {
				waitGroup.Done()
			}
		}
	}()

	// The protocol always starts with the version
	os.Stdout.Write(append(writeUint32(nil, uint32(len(esbuildVersion))), esbuildVersion...))

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

		// Process all complete (i.e. not partial) packets
		bytes := stream
		for {
			packet, afterPacket, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				break
			}
			bytes = afterPacket

			// Clone the input and run it on another goroutine
			clone := append([]byte{}, packet...)
			waitGroup.Add(1)
			go func() {
				if result := service.handleIncomingPacket(clone); result != nil {
					service.outgoingPackets <- outgoingPacket{bytes: result, isFinal: true}
				} else {
					waitGroup.Done()
				}
			}()
		}

		// Move the remaining partial packet to the end to avoid reallocating
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

func (service *serviceType) handleIncomingPacket(bytes []byte) (result []byte) {
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
		request := p.value.(map[string]interface{})
		command := request["command"].(string)
		switch command {
		case "build":
			return service.handleBuildRequest(p.id, request)

		case "transform":
			return service.handleTransformRequest(p.id, request)

		case "error":
			// This just exists so that errors during JavaScript API setup get printed
			// nicely to the console. This matters if the JavaScript API setup code
			// swallows thrown errors. We still want to be able to see the error.
			flags := decodeStringArray(request["flags"].([]interface{}))
			msg := decodeMessageToPrivate(request["error"].(map[string]interface{}))
			logger.PrintMessageToStderr(flags, msg)
			return encodePacket(packet{
				id:    p.id,
				value: make(map[string]interface{}),
			})

		default:
			return encodePacket(packet{
				id: p.id,
				value: map[string]interface{}{
					"error": fmt.Sprintf("Invalid command: %s", command),
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

func encodeErrorPacket(id uint32, err error) []byte {
	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"error": err.Error(),
		},
	})
}

func (service *serviceType) handleBuildRequest(id uint32, request map[string]interface{}) []byte {
	key := request["key"].(int)
	write := request["write"].(bool)
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseBuildOptions(flags)

	// Normally when "write" is true and there is no output file/directory then
	// the output is written to stdout instead. However, we're currently using
	// stdout as a communication channel and writing the build output to stdout
	// would corrupt our protocol.
	//
	// While we could channel this back to the host process and write it to
	// stdout there, the public Go API we're about to call doesn't have an option
	// for "write to stdout but don't actually write" and I don't think it should.
	// For now let's just forbid this case because it's not even that useful.
	if err == nil && write && options.Outfile == "" && options.Outdir == "" {
		err = errors.New("Either provide \"outfile\" or set \"write\" to false")
	}

	if err != nil {
		return encodeErrorPacket(id, err)
	}

	// Optionally allow input from the stdin channel
	if stdin, ok := request["stdin"].(string); ok {
		if options.Stdin == nil {
			options.Stdin = &api.StdinOptions{}
		}
		options.Stdin.Contents = stdin
		if resolveDir, ok := request["resolveDir"].(string); ok {
			options.Stdin.ResolveDir = resolveDir
		}
	}

	if plugins, ok := request["plugins"]; ok {
		for _, p := range plugins.([]interface{}) {
			func(p map[string]interface{}) {
				options.Plugins = append(options.Plugins, func(plugin api.Plugin) {
					plugin.SetName(p["name"].(string))

					for _, resolver := range p["resolvers"].([]interface{}) {
						resolver := resolver.(map[string]interface{})
						plugin.AddResolver(api.ResolverOptions{
							Filter:    resolver["filter"].(string),
							Namespace: resolver["namespace"].(string),
						}, func(args api.ResolverArgs) (api.ResolverResult, error) {
							result := api.ResolverResult{}
							response := service.sendRequest(map[string]interface{}{
								"command":    "resolver",
								"key":        key,
								"id":         resolver["id"].(int),
								"path":       args.Path,
								"importer":   args.Importer,
								"namespace":  args.Namespace,
								"resolveDir": args.ResolveDir,
							}).(map[string]interface{})
							if value, ok := response["error"]; ok {
								return api.ResolverResult{}, errors.New(value.(string))
							}
							if value, ok := response["path"]; ok {
								result.Path = value.(string)
							}
							if value, ok := response["namespace"]; ok {
								result.Namespace = value.(string)
							}
							if value, ok := response["external"]; ok {
								result.External = value.(bool)
							}
							if value, ok := response["errors"]; ok {
								result.Errors = decodeMessages(value.([]interface{}))
							}
							if value, ok := response["warnings"]; ok {
								result.Warnings = decodeMessages(value.([]interface{}))
							}
							return result, nil
						})
					}

					for _, loader := range p["loaders"].([]interface{}) {
						loader := loader.(map[string]interface{})
						plugin.AddLoader(api.LoaderOptions{
							Filter:    loader["filter"].(string),
							Namespace: loader["namespace"].(string),
						}, func(args api.LoaderArgs) (api.LoaderResult, error) {
							result := api.LoaderResult{}
							response := service.sendRequest(map[string]interface{}{
								"command": "loader",
								"key":     key,
								"id":      loader["id"].(int),
								"path":    args.Path,
							}).(map[string]interface{})
							if value, ok := response["error"]; ok {
								return api.LoaderResult{}, errors.New(value.(string))
							}
							if value, ok := response["contents"]; ok {
								contents := string(value.([]byte))
								result.Contents = &contents
							}
							if value, ok := response["resolveDir"]; ok {
								result.ResolveDir = value.(string)
							}
							if value, ok := response["errors"]; ok {
								result.Errors = decodeMessages(value.([]interface{}))
							}
							if value, ok := response["warnings"]; ok {
								result.Warnings = decodeMessages(value.([]interface{}))
							}
							if value, ok := response["loader"]; ok {
								loader, err := helpers.ParseLoader(value.(string))
								if err != nil {
									return api.LoaderResult{}, err
								}
								result.Loader = loader
							}
							return result, nil
						})
					}
				})
			}(p.(map[string]interface{}))
		}
	}

	options.Write = write
	result := api.Build(options)
	response := map[string]interface{}{
		"errors":   encodeMessages(result.Errors),
		"warnings": encodeMessages(result.Warnings),
	}

	if !write {
		// Pass the output files back to the caller
		response["outputFiles"] = encodeOutputFiles(result.OutputFiles)
	}

	return encodePacket(packet{
		id:    id,
		value: response,
	})
}

func (service *serviceType) handleTransformRequest(id uint32, request map[string]interface{}) []byte {
	inputFS := request["inputFS"].(bool)
	input := request["input"].(string)
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseTransformOptions(flags)
	if err != nil {
		return encodeErrorPacket(id, err)
	}

	transformInput := input
	if inputFS {
		fs.BeforeFileOpen()
		bytes, err := ioutil.ReadFile(input)
		fs.AfterFileClose()
		if err == nil {
			err = os.Remove(input)
		}
		if err != nil {
			return encodeErrorPacket(id, err)
		}
		transformInput = string(bytes)
	}

	result := api.Transform(transformInput, options)
	codeFS := false
	mapFS := false

	if inputFS && len(result.Code) > 0 {
		file := input + ".code"
		fs.BeforeFileOpen()
		if err := ioutil.WriteFile(file, result.Code, 0644); err == nil {
			result.Code = []byte(file)
			codeFS = true
		}
		fs.AfterFileClose()
	}

	if inputFS && len(result.Map) > 0 {
		file := input + ".map"
		fs.BeforeFileOpen()
		if err := ioutil.WriteFile(file, result.Map, 0644); err == nil {
			result.Map = []byte(file)
			mapFS = true
		}
		fs.AfterFileClose()
	}

	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"errors":   encodeMessages(result.Errors),
			"warnings": encodeMessages(result.Warnings),

			"codeFS": codeFS,
			"code":   string(result.Code),

			"mapFS": mapFS,
			"map":   string(result.Map),
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

func decodeMessages(values []interface{}) []api.Message {
	msgs := make([]api.Message, len(values))
	for i, value := range values {
		obj := value.(map[string]interface{})
		msg := api.Message{Text: obj["text"].(string)}

		// Some messages won't have a location
		loc := obj["location"]
		if loc != nil {
			loc := loc.(map[string]interface{})
			namespace := loc["namespace"].(string)
			if namespace == "" {
				namespace = "file"
			}
			msg.Location = &api.Location{
				File:      loc["file"].(string),
				Namespace: namespace,
				Line:      loc["line"].(int),
				Column:    loc["column"].(int),
				Length:    loc["length"].(int),
				LineText:  loc["lineText"].(string),
			}
		}

		msgs[i] = msg
	}
	return msgs
}

func decodeMessageToPrivate(obj map[string]interface{}) logger.Msg {
	msg := logger.Msg{Text: obj["text"].(string)}

	// Some messages won't have a location
	loc := obj["location"]
	if loc != nil {
		loc := loc.(map[string]interface{})
		namespace := loc["namespace"].(string)
		if namespace == "" {
			namespace = "file"
		}
		msg.Location = &logger.MsgLocation{
			File:      loc["file"].(string),
			Namespace: namespace,
			Line:      loc["line"].(int),
			Column:    loc["column"].(int),
			Length:    loc["length"].(int),
			LineText:  loc["lineText"].(string),
		}
	}

	return msg
}
