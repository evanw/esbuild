// This implements a simple long-running service over stdin/stdout. Each
// incoming request is an array of strings, and each outgoing response is a map
// of strings to byte slices. All values are length-prefixed using 32-bit
// little endian integers.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/evanw/esbuild/internal/cli_helpers"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/evanw/esbuild/pkg/cli"
)

type responseCallback func(interface{})
type pluginResolveCallback func(uint32, map[string]interface{}) []byte

type activeBuild struct {
	ctx              api.BuildContext
	pluginResolve    pluginResolveCallback
	mutex            sync.Mutex
	disposeWaitGroup sync.WaitGroup // Allows "dispose" to wait for all active tasks

	// These are guarded by the mutex
	rebuildWaitGroup   *sync.WaitGroup // Allows "cancel" to wait for all active rebuilds (within mutex because "sync.WaitGroup" isn't thread-safe)
	withinRebuildCount int
	didGetCancel       bool
}

type serviceType struct {
	callbacks          map[uint32]responseCallback
	activeBuilds       map[int]*activeBuild
	outgoingPackets    chan []byte // Always use "sendPacket" instead of sending on this channel
	keepAliveWaitGroup *helpers.ThreadSafeWaitGroup
	mutex              sync.Mutex
	nextRequestID      uint32
}

func (service *serviceType) getActiveBuild(key int) *activeBuild {
	service.mutex.Lock()
	activeBuild := service.activeBuilds[key]
	service.mutex.Unlock()
	return activeBuild
}

func (service *serviceType) createActiveBuild(key int) *activeBuild {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.activeBuilds[key] != nil {
		panic("Internal error")
	}
	activeBuild := &activeBuild{}
	service.activeBuilds[key] = activeBuild

	// This pairs with "Done()" in "decRefCount"
	service.keepAliveWaitGroup.Add(1)
	return activeBuild
}

func (service *serviceType) destroyActiveBuild(key int) {
	service.mutex.Lock()
	if service.activeBuilds[key] == nil {
		panic("Internal error")
	}
	delete(service.activeBuilds, key)
	service.mutex.Unlock()

	// This pairs with "Add()" in "trackActiveBuild"
	service.keepAliveWaitGroup.Done()
}

func runService(sendPings bool) {
	logger.API = logger.JSAPI

	service := serviceType{
		callbacks:          make(map[uint32]responseCallback),
		activeBuilds:       make(map[int]*activeBuild),
		outgoingPackets:    make(chan []byte),
		keepAliveWaitGroup: helpers.MakeThreadSafeWaitGroup(),
	}
	buffer := make([]byte, 16*1024)
	stream := []byte{}

	// Write packets on a single goroutine so they aren't interleaved
	go func() {
		for packet := range service.outgoingPackets {
			if _, err := os.Stdout.Write(packet); err != nil {
				os.Exit(1) // I/O error
			}
			service.keepAliveWaitGroup.Done() // This pairs with the "Add()" when putting stuff into "outgoingPackets"
		}
	}()

	// The protocol always starts with the version
	os.Stdout.Write(append(writeUint32(nil, uint32(len(esbuildVersion))), esbuildVersion...))

	// Wait for the last response to be written to stdout before returning from
	// the enclosing function, which will return from "main()" and exit.
	service.keepAliveWaitGroup.Add(1)
	defer func() {
		service.keepAliveWaitGroup.Done()
		service.keepAliveWaitGroup.Wait()
	}()

	// Periodically ping the host even when we're idle. This will catch cases
	// where the host has disappeared and will never send us anything else but
	// we incorrectly think we are still needed. In that case we will now try
	// to write to stdout and fail, and then know that we should exit.
	if sendPings {
		go func() {
			for {
				time.Sleep(1 * time.Second)
				service.sendRequest(map[string]interface{}{
					"command": "ping",
				})
			}
		}()
	}

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

			// Clone the input since slices into it may be used on another goroutine
			clone := append([]byte{}, packet...)
			service.handleIncomingPacket(clone)
		}

		// Move the remaining partial packet to the end to avoid reallocating
		stream = append(stream[:0], bytes...)
	}
}

// Each packet added to "outgoingPackets" must also add to the wait group
func (service *serviceType) sendPacket(packet []byte) {
	service.keepAliveWaitGroup.Add(1) // The writer thread will call "Done()"
	service.outgoingPackets <- packet
}

// This will either block until the request has been sent and a response has
// been received, or it will return nil to indicate failure to send due to
// stdin being closed.
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
		id := service.nextRequestID
		service.nextRequestID++
		service.callbacks[id] = callback
		return id
	}()

	service.sendPacket(encodePacket(packet{
		id:        id,
		isRequest: true,
		value:     request,
	}))
	return <-result
}

// This function deliberately processes incoming packets sequentially on the
// same goroutine as the caller. We want calling "dispose" on a context to take
// effect immediately and to fail all future calls on that context. We don't
// want "dispose" to accidentally be reordered after any future calls on that
// context, since those future calls are supposed to fail.
//
// If processing a packet could potentially take a while, then the remainder of
// the work should be run on another goroutine after decoding the command.
func (service *serviceType) handleIncomingPacket(bytes []byte) {
	p, ok := decodePacket(bytes)
	if !ok {
		return
	}

	if !p.isRequest {
		service.mutex.Lock()
		callback := service.callbacks[p.id]
		delete(service.callbacks, p.id)
		service.mutex.Unlock()

		if callback == nil {
			panic(fmt.Sprintf("callback nil for id %d, value %v", p.id, p.value))
		}

		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()
			callback(p.value)
		}()
		return
	}

	// Handle the request
	request := p.value.(map[string]interface{})
	command := request["command"].(string)
	switch command {
	case "build":
		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()
			service.sendPacket(service.handleBuildRequest(p.id, request))
		}()

	case "transform":
		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()
			service.sendPacket(service.handleTransformRequest(p.id, request))
		}()

	case "resolve":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			pluginResolve := build.pluginResolve
			if ctx != nil && pluginResolve != nil {
				build.disposeWaitGroup.Add(1)
			}
			build.mutex.Unlock()
			if pluginResolve != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()
					if ctx != nil {
						defer build.disposeWaitGroup.Done()
					}
					service.sendPacket(pluginResolve(p.id, request))
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id: p.id,
			value: map[string]interface{}{
				"error": "Cannot call \"resolve\" on an inactive build",
			},
		}))

	case "rebuild":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			if ctx != nil {
				build.withinRebuildCount++
				if build.rebuildWaitGroup == nil {
					build.rebuildWaitGroup = &sync.WaitGroup{}
				}
				build.rebuildWaitGroup.Add(1)
				build.disposeWaitGroup.Add(1)
			}
			build.mutex.Unlock()
			if ctx != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()
					defer build.disposeWaitGroup.Done()
					result := ctx.Rebuild()
					build.mutex.Lock()
					build.withinRebuildCount--
					build.rebuildWaitGroup.Done()
					if build.withinRebuildCount == 0 {
						// Clear the cancel flag now that the last rebuild has finished
						build.didGetCancel = false

						// Clear this to avoid confusion with the next group of rebuilds
						build.rebuildWaitGroup = nil
					}
					build.mutex.Unlock()
					service.sendPacket(encodePacket(packet{
						id: p.id,
						value: map[string]interface{}{
							"errors":   encodeMessages(result.Errors),
							"warnings": encodeMessages(result.Warnings),
						},
					}))
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id: p.id,
			value: map[string]interface{}{
				"error": "Cannot rebuild",
			},
		}))

	case "watch":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			if ctx != nil {
				build.disposeWaitGroup.Add(1)
			}
			build.mutex.Unlock()
			if ctx != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()
					defer build.disposeWaitGroup.Done()
					if err := ctx.Watch(api.WatchOptions{}); err != nil {
						service.sendPacket(encodeErrorPacket(p.id, err))
					} else {
						service.sendPacket(encodePacket(packet{
							id:    p.id,
							value: make(map[string]interface{}),
						}))
					}
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id: p.id,
			value: map[string]interface{}{
				"error": "Cannot watch",
			},
		}))

	case "serve":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			if ctx != nil {
				build.disposeWaitGroup.Add(1)
			}
			build.mutex.Unlock()
			if ctx != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()
					defer build.disposeWaitGroup.Done()
					var options api.ServeOptions
					if value, ok := request["host"]; ok {
						options.Host = value.(string)
					}
					if value, ok := request["port"]; ok {
						options.Port = uint16(value.(int))
					}
					if value, ok := request["servedir"]; ok {
						options.Servedir = value.(string)
					}
					if value, ok := request["keyfile"]; ok {
						options.Keyfile = value.(string)
					}
					if value, ok := request["certfile"]; ok {
						options.Certfile = value.(string)
					}
					if request["onRequest"].(bool) {
						options.OnRequest = func(args api.ServeOnRequestArgs) {
							// This could potentially be called after we return from
							// "Dispose()". If it does, then make sure we don't call into
							// JavaScript because we'll get an error. Also make sure that
							// if we do call into JavaScript, we wait to call "Dispose()"
							// until JavaScript has returned back to us.
							build.mutex.Lock()
							ctx := build.ctx
							if ctx != nil {
								build.disposeWaitGroup.Add(1)
							}
							build.mutex.Unlock()
							if ctx != nil {
								service.sendRequest(map[string]interface{}{
									"command": "serve-request",
									"key":     key,
									"args": map[string]interface{}{
										"remoteAddress": args.RemoteAddress,
										"method":        args.Method,
										"path":          args.Path,
										"status":        args.Status,
										"timeInMS":      args.TimeInMS,
									},
								})
								build.disposeWaitGroup.Done()
							}
						}
					}
					if result, err := ctx.Serve(options); err != nil {
						service.sendPacket(encodeErrorPacket(p.id, err))
					} else {
						service.sendPacket(encodePacket(packet{
							id: p.id,
							value: map[string]interface{}{
								"port": int(result.Port),
								"host": result.Host,
							},
						}))
					}
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id: p.id,
			value: map[string]interface{}{
				"error": "Cannot serve",
			},
		}))

	case "cancel":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			rebuildWaitGroup := build.rebuildWaitGroup
			if build.withinRebuildCount > 0 {
				// If Go got a "rebuild" message from JS before this, there's a chance
				// that Go hasn't run "ctx.Rebuild()" by the time our "ctx.Cancel()"
				// runs below because both of them are on separate goroutines. To
				// handle this, we set this flag to tell our "OnStart" plugin to cancel
				// the build in case things happen in that order.
				build.didGetCancel = true
			}
			build.mutex.Unlock()
			if ctx != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()
					ctx.Cancel()

					// Block until all manual rebuilds that were active at the time the
					// "cancel" packet was originally processed have finished. That way
					// JS can wait for "cancel" to end and be assured that it can call
					// "rebuild" and have it not merge with any other ongoing rebuilds.
					if rebuildWaitGroup != nil {
						rebuildWaitGroup.Wait()
					}

					// Only return control to JavaScript once the cancel operation has succeeded
					service.sendPacket(encodePacket(packet{
						id:    p.id,
						value: make(map[string]interface{}),
					}))
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id:    p.id,
			value: make(map[string]interface{}),
		}))

	case "dispose":
		key := request["key"].(int)
		if build := service.getActiveBuild(key); build != nil {
			build.mutex.Lock()
			ctx := build.ctx
			build.ctx = nil
			build.mutex.Unlock()

			// Release this ref count if it was held
			if ctx != nil {
				service.keepAliveWaitGroup.Add(1)
				go func() {
					defer service.keepAliveWaitGroup.Done()

					// While "Dispose()" will wait for any existing operations on the
					// context to finish, we also don't want to start any new operations.
					// That can happen because operations (e.g. "Rebuild()") are started
					// from a separate goroutine without locking the build mutex. This
					// uses a WaitGroup to handle this case. If that happened, then we'll
					// wait for it here before disposing. Once the wait is over, no more
					// operations can happen on the context because we have already
					// zeroed out the shared context pointer above.
					build.disposeWaitGroup.Done()
					build.disposeWaitGroup.Wait()

					ctx.Dispose()
					service.destroyActiveBuild(key)

					// Only return control to JavaScript once everything relating to this
					// build has gracefully ended. Otherwise JavaScript will unregister
					// everything related to this build and any calls an ongoing build
					// makes into JavaScript will cause errors, which may be observable.
					service.sendPacket(encodePacket(packet{
						id:    p.id,
						value: make(map[string]interface{}),
					}))
				}()
				return
			}
		}
		service.sendPacket(encodePacket(packet{
			id:    p.id,
			value: make(map[string]interface{}),
		}))

	case "error":
		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()

			// This just exists so that errors during JavaScript API setup get printed
			// nicely to the console. This matters if the JavaScript API setup code
			// swallows thrown errors. We still want to be able to see the error.
			flags := decodeStringArray(request["flags"].([]interface{}))
			msg := decodeMessageToPrivate(request["error"].(map[string]interface{}))
			logger.PrintMessageToStderr(flags, msg)
			service.sendPacket(encodePacket(packet{
				id:    p.id,
				value: make(map[string]interface{}),
			}))
		}()

	case "format-msgs":
		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()
			service.sendPacket(service.handleFormatMessagesRequest(p.id, request))
		}()

	case "analyze-metafile":
		service.keepAliveWaitGroup.Add(1)
		go func() {
			defer service.keepAliveWaitGroup.Done()
			service.sendPacket(service.handleAnalyzeMetafileRequest(p.id, request))
		}()

	default:
		service.sendPacket(encodePacket(packet{
			id: p.id,
			value: map[string]interface{}{
				"error": fmt.Sprintf("Invalid command: %s", command),
			},
		}))
	}
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
	isContext := request["context"].(bool)
	key := request["key"].(int)
	write := request["write"].(bool)
	entries := request["entries"].([]interface{})
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseBuildOptions(flags)
	options.AbsWorkingDir = request["absWorkingDir"].(string)
	options.NodePaths = decodeStringArray(request["nodePaths"].([]interface{}))
	options.MangleCache, _ = request["mangleCache"].(map[string]interface{})

	for _, entry := range entries {
		entry := entry.([]interface{})
		key := entry[0].(string)
		value := entry[1].(string)
		options.EntryPointsAdvanced = append(options.EntryPointsAdvanced, api.EntryPoint{
			OutputPath: key,
			InputPath:  value,
		})
	}

	// Normally when "write" is true and there is no output file/directory then
	// the output is written to stdout instead. However, we're currently using
	// stdout as a communication channel and writing the build output to stdout
	// would corrupt our protocol. Special-case this to channel this back to the
	// host process and write it to stdout there.
	writeToStdout := err == nil && write && options.Outfile == "" && options.Outdir == ""

	if err != nil {
		return encodeErrorPacket(id, err)
	}

	// Optionally allow input from the stdin channel
	if stdin, ok := request["stdinContents"].([]byte); ok {
		if options.Stdin == nil {
			options.Stdin = &api.StdinOptions{}
		}
		options.Stdin.Contents = string(stdin)
		if resolveDir, ok := request["stdinResolveDir"].(string); ok {
			options.Stdin.ResolveDir = resolveDir
		}
	}

	activeBuild := service.createActiveBuild(key)

	hasOnEndCallbacks := false
	if plugins, ok := request["plugins"]; ok {
		if plugins, hasOnEnd, err := service.convertPlugins(key, plugins, activeBuild); err != nil {
			return encodeErrorPacket(id, err)
		} else {
			options.Plugins = plugins
			hasOnEndCallbacks = hasOnEnd
		}
	}

	resultToResponse := func(result api.BuildResult) map[string]interface{} {
		response := map[string]interface{}{
			"errors":   encodeMessages(result.Errors),
			"warnings": encodeMessages(result.Warnings),
		}
		if !write {
			// Pass the output files back to the caller
			response["outputFiles"] = encodeOutputFiles(result.OutputFiles)
		}
		if options.Metafile {
			response["metafile"] = result.Metafile
		}
		if options.MangleCache != nil {
			response["mangleCache"] = result.MangleCache
		}
		if writeToStdout && len(result.OutputFiles) == 1 {
			response["writeToStdout"] = result.OutputFiles[0].Contents
		}
		return response
	}

	if !writeToStdout {
		options.Write = write
	}

	if isContext {
		options.Plugins = append(options.Plugins, api.Plugin{
			Name: "onEnd",
			Setup: func(build api.PluginBuild) {
				build.OnStart(func() (api.OnStartResult, error) {
					activeBuild.mutex.Lock()
					if currentWaitGroup := activeBuild.rebuildWaitGroup; currentWaitGroup != nil && activeBuild.didGetCancel {
						// Cancel the current build now that the current build is active.
						// This catches the case where JS does "rebuild()" then "cancel()"
						// but Go's scheduler runs the original "ctx.Cancel()" goroutine
						// before it runs the "ctx.Rebuild()" goroutine.
						//
						// This adds to the rebuild wait group that other cancel operations
						// are waiting on because we also want those other cancel operations
						// to wait on this cancel operation. Go might schedule this new
						// goroutine after all currently-active rebuilds end. We don't want
						// the user's cancel operation to return to the user and for them
						// to start another rebuild before our "ctx.Cancel" below runs
						// because our cancel is supposed to cancel the current build, not
						// some independent future build.
						activeBuild.rebuildWaitGroup.Add(1)
						go func() {
							activeBuild.ctx.Cancel()

							// Lock the mutex because "sync.WaitGroup" isn't thread-safe.
							// But use the wait group that was active at the time the
							// "OnStart" callback ran instead of the latest one on the
							// active build in case this goroutine is delayed.
							activeBuild.mutex.Lock()
							currentWaitGroup.Done()
							activeBuild.mutex.Unlock()
						}()
					}
					activeBuild.mutex.Unlock()
					return api.OnStartResult{}, nil
				})

				build.OnEnd(func(result *api.BuildResult) (api.OnEndResult, error) {
					// For performance, we only send JavaScript an "onEnd" message if
					// it's needed. It's only needed if one of the following is true:
					//
					// - There are any "onEnd" callbacks registered
					// - JavaScript has called our "rebuild()" function
					// - We are writing build output to JavaScript's stdout
					//
					// This is especially important if "write" is false since otherwise
					// we'd unnecessarily send the entire contents of all output files!
					//
					//          "If a tree falls in a forest and no one is
					//           around to hear it, does it make a sound?"
					//
					activeBuild.mutex.Lock()
					isWithinRebuild := activeBuild.withinRebuildCount > 0
					activeBuild.mutex.Unlock()
					if !hasOnEndCallbacks && !isWithinRebuild && !writeToStdout {
						return api.OnEndResult{}, nil
					}
					request := resultToResponse(*result)
					request["command"] = "on-end"
					request["key"] = key
					response, ok := service.sendRequest(request).(map[string]interface{})
					if !ok {
						return api.OnEndResult{}, errors.New("The service was stopped")
					}
					var errors []api.Message
					var warnings []api.Message
					if value, ok := response["errors"].([]interface{}); ok {
						errors = decodeMessages(value)
					}
					if value, ok := response["warnings"].([]interface{}); ok {
						warnings = decodeMessages(value)
					}
					return api.OnEndResult{
						Errors:   errors,
						Warnings: warnings,
					}, nil
				})
			},
		})

		ctx, err := api.Context(options)
		if err != nil {
			return encodePacket(packet{
				id: id,
				value: map[string]interface{}{
					"errors":   encodeMessages(err.Errors),
					"warnings": []interface{}{},
				},
			})
		}

		// Keep the build alive until "dispose" has been called
		activeBuild.disposeWaitGroup.Add(1)
		activeBuild.ctx = ctx

		return encodePacket(packet{
			id: id,
			value: map[string]interface{}{
				"errors":   []interface{}{},
				"warnings": []interface{}{},
			},
		})
	}

	result := api.Build(options)
	response := resultToResponse(result)

	service.destroyActiveBuild(key)

	return encodePacket(packet{
		id:    id,
		value: response,
	})
}

func resolveKindToString(kind api.ResolveKind) string {
	switch kind {
	case api.ResolveEntryPoint:
		return "entry-point"

	// JS
	case api.ResolveJSImportStatement:
		return "import-statement"
	case api.ResolveJSRequireCall:
		return "require-call"
	case api.ResolveJSDynamicImport:
		return "dynamic-import"
	case api.ResolveJSRequireResolve:
		return "require-resolve"

	// CSS
	case api.ResolveCSSImportRule:
		return "import-rule"
	case api.ResolveCSSURLToken:
		return "url-token"

	default:
		panic("Internal error")
	}
}

func stringToResolveKind(kind string) (api.ResolveKind, bool) {
	switch kind {
	case "entry-point":
		return api.ResolveEntryPoint, true

	// JS
	case "import-statement":
		return api.ResolveJSImportStatement, true
	case "require-call":
		return api.ResolveJSRequireCall, true
	case "dynamic-import":
		return api.ResolveJSDynamicImport, true
	case "require-resolve":
		return api.ResolveJSRequireResolve, true

	// CSS
	case "import-rule":
		return api.ResolveCSSImportRule, true
	case "url-token":
		return api.ResolveCSSURLToken, true
	}

	return api.ResolveNone, false
}

func (service *serviceType) convertPlugins(key int, jsPlugins interface{}, activeBuild *activeBuild) ([]api.Plugin, bool, error) {
	type filteredCallback struct {
		filter     *regexp.Regexp
		pluginName string
		namespace  string
		id         int
	}

	var onResolveCallbacks []filteredCallback
	var onLoadCallbacks []filteredCallback
	hasOnStart := false
	hasOnEnd := false

	filteredCallbacks := func(pluginName string, kind string, items []interface{}) (result []filteredCallback, err error) {
		for _, item := range items {
			item := item.(map[string]interface{})
			filter, err := config.CompileFilterForPlugin(pluginName, kind, item["filter"].(string))
			if err != nil {
				return nil, err
			}
			result = append(result, filteredCallback{
				pluginName: pluginName,
				id:         item["id"].(int),
				filter:     filter,
				namespace:  item["namespace"].(string),
			})
		}
		return
	}

	for _, p := range jsPlugins.([]interface{}) {
		p := p.(map[string]interface{})
		pluginName := p["name"].(string)

		if p["onStart"].(bool) {
			hasOnStart = true
		}

		if p["onEnd"].(bool) {
			hasOnEnd = true
		}

		if callbacks, err := filteredCallbacks(pluginName, "onResolve", p["onResolve"].([]interface{})); err != nil {
			return nil, false, err
		} else {
			onResolveCallbacks = append(onResolveCallbacks, callbacks...)
		}

		if callbacks, err := filteredCallbacks(pluginName, "onLoad", p["onLoad"].([]interface{})); err != nil {
			return nil, false, err
		} else {
			onLoadCallbacks = append(onLoadCallbacks, callbacks...)
		}
	}

	// We want to minimize the amount of IPC traffic. Instead of adding one Go
	// plugin for every JavaScript plugin, we just add a single Go plugin that
	// proxies the plugin queries to the list of JavaScript plugins in the host.
	return []api.Plugin{{
		Name: "JavaScript plugins",
		Setup: func(build api.PluginBuild) {
			activeBuild.mutex.Lock()
			activeBuild.pluginResolve = func(id uint32, request map[string]interface{}) []byte {
				path := request["path"].(string)
				var options api.ResolveOptions
				if value, ok := request["pluginName"]; ok {
					options.PluginName = value.(string)
				}
				if value, ok := request["importer"]; ok {
					options.Importer = value.(string)
				}
				if value, ok := request["namespace"]; ok {
					options.Namespace = value.(string)
				}
				if value, ok := request["resolveDir"]; ok {
					options.ResolveDir = value.(string)
				}
				if value, ok := request["kind"]; ok {
					str := value.(string)
					kind, ok := stringToResolveKind(str)
					if !ok {
						return encodePacket(packet{
							id: id,
							value: map[string]interface{}{
								"error": fmt.Sprintf("Invalid kind: %q", str),
							},
						})
					}
					options.Kind = kind
				}
				if value, ok := request["pluginData"]; ok {
					options.PluginData = value.(int)
				}

				result := build.Resolve(path, options)
				return encodePacket(packet{
					id: id,
					value: map[string]interface{}{
						"errors":      encodeMessages(result.Errors),
						"warnings":    encodeMessages(result.Warnings),
						"path":        result.Path,
						"external":    result.External,
						"sideEffects": result.SideEffects,
						"namespace":   result.Namespace,
						"suffix":      result.Suffix,
						"pluginData":  result.PluginData,
					},
				})
			}
			activeBuild.mutex.Unlock()

			// Only register "OnStart" if needed
			if hasOnStart {
				build.OnStart(func() (api.OnStartResult, error) {
					response, ok := service.sendRequest(map[string]interface{}{
						"command": "on-start",
						"key":     key,
					}).(map[string]interface{})
					if !ok {
						return api.OnStartResult{}, errors.New("The service was stopped")
					}
					return api.OnStartResult{
						Errors:   decodeMessages(response["errors"].([]interface{})),
						Warnings: decodeMessages(response["warnings"].([]interface{})),
					}, nil
				})
			}

			// Only register "OnResolve" if needed
			if len(onResolveCallbacks) > 0 {
				build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					var ids []interface{}
					applyPath := logger.Path{Text: args.Path, Namespace: args.Namespace}
					for _, item := range onResolveCallbacks {
						if config.PluginAppliesToPath(applyPath, item.filter, item.namespace) {
							ids = append(ids, item.id)
						}
					}

					result := api.OnResolveResult{}
					if len(ids) == 0 {
						return result, nil
					}

					response, ok := service.sendRequest(map[string]interface{}{
						"command":    "on-resolve",
						"key":        key,
						"ids":        ids,
						"path":       args.Path,
						"importer":   args.Importer,
						"namespace":  args.Namespace,
						"resolveDir": args.ResolveDir,
						"kind":       resolveKindToString(args.Kind),
						"pluginData": args.PluginData,
					}).(map[string]interface{})
					if !ok {
						return result, errors.New("The service was stopped")
					}

					if value, ok := response["id"]; ok {
						id := value.(int)
						for _, item := range onResolveCallbacks {
							if item.id == id {
								result.PluginName = item.pluginName
								break
							}
						}
					}
					if value, ok := response["error"]; ok {
						return result, errors.New(value.(string))
					}
					if value, ok := response["pluginName"]; ok {
						result.PluginName = value.(string)
					}
					if value, ok := response["path"]; ok {
						result.Path = value.(string)
					}
					if value, ok := response["namespace"]; ok {
						result.Namespace = value.(string)
					}
					if value, ok := response["suffix"]; ok {
						result.Suffix = value.(string)
					}
					if value, ok := response["external"]; ok {
						result.External = value.(bool)
					}
					if value, ok := response["sideEffects"]; ok {
						if value.(bool) {
							result.SideEffects = api.SideEffectsTrue
						} else {
							result.SideEffects = api.SideEffectsFalse
						}
					}
					if value, ok := response["pluginData"]; ok {
						result.PluginData = value.(int)
					}
					if value, ok := response["errors"]; ok {
						result.Errors = decodeMessages(value.([]interface{}))
					}
					if value, ok := response["warnings"]; ok {
						result.Warnings = decodeMessages(value.([]interface{}))
					}
					if value, ok := response["watchFiles"]; ok {
						result.WatchFiles = decodeStringArray(value.([]interface{}))
					}
					if value, ok := response["watchDirs"]; ok {
						result.WatchDirs = decodeStringArray(value.([]interface{}))
					}

					return result, nil
				})
			}

			// Only register "OnLoad" if needed
			if len(onLoadCallbacks) > 0 {
				build.OnLoad(api.OnLoadOptions{Filter: ".*"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					var ids []interface{}
					applyPath := logger.Path{Text: args.Path, Namespace: args.Namespace}
					for _, item := range onLoadCallbacks {
						if config.PluginAppliesToPath(applyPath, item.filter, item.namespace) {
							ids = append(ids, item.id)
						}
					}

					result := api.OnLoadResult{}
					if len(ids) == 0 {
						return result, nil
					}

					response, ok := service.sendRequest(map[string]interface{}{
						"command":    "on-load",
						"key":        key,
						"ids":        ids,
						"path":       args.Path,
						"namespace":  args.Namespace,
						"suffix":     args.Suffix,
						"pluginData": args.PluginData,
					}).(map[string]interface{})
					if !ok {
						return result, errors.New("The service was stopped")
					}

					if value, ok := response["id"]; ok {
						id := value.(int)
						for _, item := range onLoadCallbacks {
							if item.id == id {
								result.PluginName = item.pluginName
								break
							}
						}
					}
					if value, ok := response["error"]; ok {
						return result, errors.New(value.(string))
					}
					if value, ok := response["pluginName"]; ok {
						result.PluginName = value.(string)
					}
					if value, ok := response["loader"]; ok {
						loader, err := cli_helpers.ParseLoader(value.(string))
						if err != nil {
							return result, errors.New(err.Text)
						}
						result.Loader = loader
					}
					if value, ok := response["contents"]; ok {
						contents := string(value.([]byte))
						result.Contents = &contents
					}
					if value, ok := response["resolveDir"]; ok {
						result.ResolveDir = value.(string)
					}
					if value, ok := response["pluginData"]; ok {
						result.PluginData = value.(int)
					}
					if value, ok := response["errors"]; ok {
						result.Errors = decodeMessages(value.([]interface{}))
					}
					if value, ok := response["warnings"]; ok {
						result.Warnings = decodeMessages(value.([]interface{}))
					}
					if value, ok := response["watchFiles"]; ok {
						result.WatchFiles = decodeStringArray(value.([]interface{}))
					}
					if value, ok := response["watchDirs"]; ok {
						result.WatchDirs = decodeStringArray(value.([]interface{}))
					}

					return result, nil
				})
			}
		},
	}}, hasOnEnd, nil
}

func (service *serviceType) handleTransformRequest(id uint32, request map[string]interface{}) []byte {
	inputFS := request["inputFS"].(bool)
	input := string(request["input"].([]byte))
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseTransformOptions(flags)
	if err != nil {
		return encodeErrorPacket(id, err)
	}
	options.MangleCache, _ = request["mangleCache"].(map[string]interface{})

	transformInput := input
	if inputFS {
		fs.BeforeFileOpen()
		bytes, err := os.ReadFile(input)
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
		if err := os.WriteFile(file, result.Code, 0644); err == nil {
			result.Code = []byte(file)
			codeFS = true
		}
		fs.AfterFileClose()
	}

	if inputFS && len(result.Map) > 0 {
		file := input + ".map"
		fs.BeforeFileOpen()
		if err := os.WriteFile(file, result.Map, 0644); err == nil {
			result.Map = []byte(file)
			mapFS = true
		}
		fs.AfterFileClose()
	}

	response := map[string]interface{}{
		"errors":   encodeMessages(result.Errors),
		"warnings": encodeMessages(result.Warnings),

		"codeFS": codeFS,
		"code":   string(result.Code),

		"mapFS": mapFS,
		"map":   string(result.Map),
	}

	if result.LegalComments != nil {
		response["legalComments"] = string(result.LegalComments)
	}

	if result.MangleCache != nil {
		response["mangleCache"] = result.MangleCache
	}

	return encodePacket(packet{
		id:    id,
		value: response,
	})
}

func (service *serviceType) handleFormatMessagesRequest(id uint32, request map[string]interface{}) []byte {
	msgs := decodeMessages(request["messages"].([]interface{}))

	options := api.FormatMessagesOptions{
		Kind: api.ErrorMessage,
	}
	if request["isWarning"].(bool) {
		options.Kind = api.WarningMessage
	}
	if value, ok := request["color"].(bool); ok {
		options.Color = value
	}
	if value, ok := request["terminalWidth"].(int); ok {
		options.TerminalWidth = value
	}

	result := api.FormatMessages(msgs, options)

	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"messages": encodeStringArray(result),
		},
	})
}

func (service *serviceType) handleAnalyzeMetafileRequest(id uint32, request map[string]interface{}) []byte {
	metafile := request["metafile"].(string)

	options := api.AnalyzeMetafileOptions{}
	if value, ok := request["color"].(bool); ok {
		options.Color = value
	}
	if value, ok := request["verbose"].(bool); ok {
		options.Verbose = value
	}

	result := api.AnalyzeMetafile(metafile, options)

	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"result": result,
		},
	})
}

func encodeStringArray(strings []string) []interface{} {
	values := make([]interface{}, len(strings))
	for i, value := range strings {
		values[i] = value
	}
	return values
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

func encodeLocation(loc *api.Location) interface{} {
	if loc == nil {
		return nil
	}
	return map[string]interface{}{
		"file":       loc.File,
		"namespace":  loc.Namespace,
		"line":       loc.Line,
		"column":     loc.Column,
		"length":     loc.Length,
		"lineText":   loc.LineText,
		"suggestion": loc.Suggestion,
	}
}

func encodeMessages(msgs []api.Message) []interface{} {
	values := make([]interface{}, len(msgs))
	for i, msg := range msgs {
		value := map[string]interface{}{
			"id":         msg.ID,
			"pluginName": msg.PluginName,
			"text":       msg.Text,
			"location":   encodeLocation(msg.Location),
		}
		values[i] = value

		notes := make([]interface{}, len(msg.Notes))
		for j, note := range msg.Notes {
			notes[j] = map[string]interface{}{
				"text":     note.Text,
				"location": encodeLocation(note.Location),
			}
		}
		value["notes"] = notes

		// Send "-1" to mean "undefined"
		detail, ok := msg.Detail.(int)
		if !ok {
			detail = -1
		}
		value["detail"] = detail
	}
	return values
}

func decodeLocation(value interface{}) *api.Location {
	if value == nil {
		return nil
	}
	loc := value.(map[string]interface{})
	namespace := loc["namespace"].(string)
	if namespace == "" {
		namespace = "file"
	}
	return &api.Location{
		File:       loc["file"].(string),
		Namespace:  namespace,
		Line:       loc["line"].(int),
		Column:     loc["column"].(int),
		Length:     loc["length"].(int),
		LineText:   loc["lineText"].(string),
		Suggestion: loc["suggestion"].(string),
	}
}

func decodeMessages(values []interface{}) []api.Message {
	msgs := make([]api.Message, len(values))
	for i, value := range values {
		obj := value.(map[string]interface{})
		msg := api.Message{
			ID:         obj["id"].(string),
			PluginName: obj["pluginName"].(string),
			Text:       obj["text"].(string),
			Location:   decodeLocation(obj["location"]),
			Detail:     obj["detail"].(int),
		}
		for _, note := range obj["notes"].([]interface{}) {
			noteObj := note.(map[string]interface{})
			msg.Notes = append(msg.Notes, api.Note{
				Text:     noteObj["text"].(string),
				Location: decodeLocation(noteObj["location"]),
			})
		}
		msgs[i] = msg
	}
	return msgs
}

func decodeLocationToPrivate(value interface{}) *logger.MsgLocation {
	if value == nil {
		return nil
	}
	loc := value.(map[string]interface{})
	namespace := loc["namespace"].(string)
	if namespace == "" {
		namespace = "file"
	}
	return &logger.MsgLocation{
		File:       loc["file"].(string),
		Namespace:  namespace,
		Line:       loc["line"].(int),
		Column:     loc["column"].(int),
		Length:     loc["length"].(int),
		LineText:   loc["lineText"].(string),
		Suggestion: loc["suggestion"].(string),
	}
}

func decodeMessageToPrivate(obj map[string]interface{}) logger.Msg {
	msg := logger.Msg{
		ID:         logger.StringToMaximumMsgID(obj["id"].(string)),
		PluginName: obj["pluginName"].(string),
		Data: logger.MsgData{
			Text:       obj["text"].(string),
			Location:   decodeLocationToPrivate(obj["location"]),
			UserDetail: obj["detail"].(int),
		},
	}
	for _, note := range obj["notes"].([]interface{}) {
		noteObj := note.(map[string]interface{})
		msg.Notes = append(msg.Notes, logger.MsgData{
			Text:     noteObj["text"].(string),
			Location: decodeLocationToPrivate(noteObj["location"]),
		})
	}
	return msg
}
