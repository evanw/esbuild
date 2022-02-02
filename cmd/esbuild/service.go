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
type rebuildCallback func(uint32) []byte
type watchStopCallback func()
type serveStopCallback func()
type pluginResolveCallback func(uint32, map[string]interface{}) []byte

type activeBuild struct {
	rebuild       rebuildCallback
	watchStop     watchStopCallback
	serveStop     serveStopCallback
	pluginResolve pluginResolveCallback
	mutex         sync.Mutex
	refCount      int
}

type serviceType struct {
	callbacks          map[uint32]responseCallback
	activeBuilds       map[int]*activeBuild
	outgoingPackets    chan outgoingPacket
	keepAliveWaitGroup sync.WaitGroup
	mutex              sync.Mutex
	nextRequestID      uint32
}

func (service *serviceType) getActiveBuild(key int) *activeBuild {
	service.mutex.Lock()
	activeBuild := service.activeBuilds[key]
	service.mutex.Unlock()
	return activeBuild
}

func (service *serviceType) trackActiveBuild(key int, activeBuild *activeBuild) {
	if activeBuild.refCount > 0 {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		if service.activeBuilds[key] != nil {
			panic("Internal error")
		}
		service.activeBuilds[key] = activeBuild

		// This pairs with "Done()" in "decRefCount"
		service.keepAliveWaitGroup.Add(1)
	}
}

func (service *serviceType) decRefCount(key int, activeBuild *activeBuild) {
	activeBuild.mutex.Lock()
	activeBuild.refCount--
	remaining := activeBuild.refCount
	activeBuild.mutex.Unlock()

	if remaining < 0 {
		panic("Internal error")
	}

	if remaining == 0 {
		service.mutex.Lock()
		delete(service.activeBuilds, key)
		service.mutex.Unlock()

		// This pairs with "Add()" in "trackActiveBuild"
		service.keepAliveWaitGroup.Done()
	}
}

type outgoingPacket struct {
	bytes []byte
}

func runService(sendPings bool) {
	logger.API = logger.JSAPI

	service := serviceType{
		callbacks:       make(map[uint32]responseCallback),
		activeBuilds:    make(map[int]*activeBuild),
		outgoingPackets: make(chan outgoingPacket),
	}
	buffer := make([]byte, 16*1024)
	stream := []byte{}

	// Write packets on a single goroutine so they aren't interleaved
	go func() {
		for {
			packet, ok := <-service.outgoingPackets
			if !ok {
				break // No more packets
			}
			if _, err := os.Stdout.Write(packet.bytes); err != nil {
				os.Exit(1) // I/O error
			}
			service.keepAliveWaitGroup.Done() // This pairs with the "Add()" when putting stuff into "outgoingPackets"
		}
	}()

	// The protocol always starts with the version
	os.Stdout.Write(append(writeUint32(nil, uint32(len(esbuildVersion))), esbuildVersion...))

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

			// Clone the input and run it on another goroutine
			clone := append([]byte{}, packet...)
			service.keepAliveWaitGroup.Add(1)
			go func() {
				out := service.handleIncomingPacket(clone)
				if out.bytes != nil {
					service.keepAliveWaitGroup.Add(1) // The writer thread will call "Done()"
					service.outgoingPackets <- out
				}
				service.keepAliveWaitGroup.Done() // This pairs with the "Add()" in the stdin thread
			}()
		}

		// Move the remaining partial packet to the end to avoid reallocating
		stream = append(stream[:0], bytes...)
	}

	// Wait for the last response to be written to stdout
	service.keepAliveWaitGroup.Wait()
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
		id := service.nextRequestID
		service.nextRequestID++
		service.callbacks[id] = callback
		return id
	}()
	service.keepAliveWaitGroup.Add(1) // The writer thread will call "Done()"
	service.outgoingPackets <- outgoingPacket{
		bytes: encodePacket(packet{
			id:        id,
			isRequest: true,
			value:     request,
		}),
	}
	return <-result
}

func (service *serviceType) handleIncomingPacket(bytes []byte) (result outgoingPacket) {
	p, ok := decodePacket(bytes)
	if !ok {
		return outgoingPacket{}
	}

	if p.isRequest {
		// Catch panics in the code below so they get passed to the caller
		defer func() {
			if r := recover(); r != nil {
				result = outgoingPacket{
					bytes: encodePacket(packet{
						id: p.id,
						value: map[string]interface{}{
							"error": fmt.Sprintf("panic: %v\n\n%s", r, helpers.PrettyPrintedStack()),
						},
					}),
				}
			}
		}()

		// Handle the request
		request := p.value.(map[string]interface{})
		command := request["command"].(string)
		switch command {
		case "build":
			return service.handleBuildRequest(p.id, request)

		case "transform":
			return outgoingPacket{
				bytes: service.handleTransformRequest(p.id, request),
			}

		case "resolve":
			key := request["key"].(int)
			if build := service.getActiveBuild(key); build != nil {
				build.mutex.Lock()
				pluginResolve := build.pluginResolve
				build.mutex.Unlock()
				if pluginResolve != nil {
					return outgoingPacket{
						bytes: pluginResolve(p.id, request),
					}
				}
			}
			return outgoingPacket{
				bytes: encodePacket(packet{
					id: p.id,
					value: map[string]interface{}{
						"error": "Cannot call \"resolve\" on an inactive build",
					},
				}),
			}

		case "rebuild":
			key := request["key"].(int)
			if build := service.getActiveBuild(key); build != nil {
				build.mutex.Lock()
				rebuild := build.rebuild
				build.mutex.Unlock()
				if rebuild != nil {
					return outgoingPacket{
						bytes: rebuild(p.id),
					}
				}
			}
			return outgoingPacket{
				bytes: encodePacket(packet{
					id: p.id,
					value: map[string]interface{}{
						"error": "Cannot rebuild",
					},
				}),
			}

		case "watch-stop":
			key := request["key"].(int)
			if build := service.getActiveBuild(key); build != nil {
				build.mutex.Lock()
				watchStop := build.watchStop
				build.watchStop = nil
				build.mutex.Unlock()

				// Stop watch mode and release this ref count if it was held
				if watchStop != nil {
					watchStop()
					service.decRefCount(key, build)
				}
			}
			return outgoingPacket{
				bytes: encodePacket(packet{
					id:    p.id,
					value: make(map[string]interface{}),
				}),
			}

		case "serve-stop":
			key := request["key"].(int)
			if build := service.getActiveBuild(key); build != nil {
				build.mutex.Lock()
				serveStop := build.serveStop
				build.serveStop = nil
				build.mutex.Unlock()

				// Stop serving but do not release the ref count (that's done elsewhere)
				if serveStop != nil {
					serveStop()
				}
			}
			return outgoingPacket{
				bytes: encodePacket(packet{
					id:    p.id,
					value: make(map[string]interface{}),
				}),
			}

		case "rebuild-dispose":
			key := request["key"].(int)
			if build := service.getActiveBuild(key); build != nil {
				build.mutex.Lock()
				rebuild := build.rebuild
				build.rebuild = nil
				build.mutex.Unlock()

				// Release this ref count if it was held
				if rebuild != nil {
					service.decRefCount(key, build)
				}
			}
			return outgoingPacket{
				bytes: encodePacket(packet{
					id:    p.id,
					value: make(map[string]interface{}),
				}),
			}

		case "error":
			// This just exists so that errors during JavaScript API setup get printed
			// nicely to the console. This matters if the JavaScript API setup code
			// swallows thrown errors. We still want to be able to see the error.
			flags := decodeStringArray(request["flags"].([]interface{}))
			msg := decodeMessageToPrivate(request["error"].(map[string]interface{}))
			logger.PrintMessageToStderr(flags, msg)
			return outgoingPacket{
				bytes: encodePacket(packet{
					id:    p.id,
					value: make(map[string]interface{}),
				}),
			}

		case "format-msgs":
			return outgoingPacket{
				bytes: service.handleFormatMessagesRequest(p.id, request),
			}

		case "analyze-metafile":
			return outgoingPacket{
				bytes: service.handleAnalyzeMetafileRequest(p.id, request),
			}

		default:
			return outgoingPacket{
				bytes: encodePacket(packet{
					id: p.id,
					value: map[string]interface{}{
						"error": fmt.Sprintf("Invalid command: %s", command),
					},
				}),
			}
		}
	}

	callback := func() responseCallback {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		callback := service.callbacks[p.id]
		delete(service.callbacks, p.id)
		return callback
	}()

	if callback == nil {
		panic(fmt.Sprintf("callback nil for id %d, value %v", p.id, p.value))
	}

	callback(p.value)
	return outgoingPacket{}
}

func encodeErrorPacket(id uint32, err error) []byte {
	return encodePacket(packet{
		id: id,
		value: map[string]interface{}{
			"error": err.Error(),
		},
	})
}

func (service *serviceType) handleBuildRequest(id uint32, request map[string]interface{}) outgoingPacket {
	key := request["key"].(int)
	write := request["write"].(bool)
	incremental := request["incremental"].(bool)
	serveObj, isServe := request["serve"].(interface{})
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
	writeToStdout := err == nil && !isServe && write && options.Outfile == "" && options.Outdir == ""

	if err != nil {
		return outgoingPacket{bytes: encodeErrorPacket(id, err)}
	}

	// Optionally allow input from the stdin channel
	if stdin, ok := request["stdinContents"].(string); ok {
		if options.Stdin == nil {
			options.Stdin = &api.StdinOptions{}
		}
		options.Stdin.Contents = stdin
		if resolveDir, ok := request["stdinResolveDir"].(string); ok {
			options.Stdin.ResolveDir = resolveDir
		}
	}

	activeBuild := &activeBuild{refCount: 1}
	service.trackActiveBuild(key, activeBuild)
	defer service.decRefCount(key, activeBuild)

	if plugins, ok := request["plugins"]; ok {
		if plugins, err := service.convertPlugins(key, plugins, activeBuild); err != nil {
			return outgoingPacket{bytes: encodeErrorPacket(id, err)}
		} else {
			options.Plugins = plugins
		}
	}

	if isServe {
		return service.handleServeRequest(id, options, serveObj, key, activeBuild)
	}

	resultToResponse := func(result api.BuildResult) map[string]interface{} {
		response := map[string]interface{}{
			"errors":   encodeMessages(result.Errors),
			"warnings": encodeMessages(result.Warnings),
			"rebuild":  incremental,
			"watch":    options.Watch != nil,
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

	if options.Watch != nil {
		options.Watch.OnRebuild = func(result api.BuildResult) {
			service.sendRequest(map[string]interface{}{
				"command": "watch-rebuild",
				"key":     key,
				"args":    resultToResponse(result),
			})
		}
	}

	if !writeToStdout {
		options.Write = write
	}
	options.Incremental = incremental
	result := api.Build(options)
	response := resultToResponse(result)

	if incremental {
		activeBuild.rebuild = func(id uint32) []byte {
			result := result.Rebuild()
			response := resultToResponse(result)
			return encodePacket(packet{
				id:    id,
				value: response,
			})
		}

		// Make sure the build doesn't finish until "dispose" has been called
		activeBuild.refCount++
	}

	if options.Watch != nil {
		activeBuild.watchStop = func() {
			result.Stop()
		}

		// Make sure the build doesn't finish until "stop" has been called
		activeBuild.refCount++
	}

	return outgoingPacket{
		bytes: encodePacket(packet{
			id:    id,
			value: response,
		}),
	}
}

func (service *serviceType) handleServeRequest(id uint32, options api.BuildOptions, serveObj interface{}, key int, activeBuild *activeBuild) outgoingPacket {
	var serveOptions api.ServeOptions
	serve := serveObj.(map[string]interface{})
	if port, ok := serve["port"]; ok {
		serveOptions.Port = uint16(port.(int))
	}
	if host, ok := serve["host"]; ok {
		serveOptions.Host = host.(string)
	}
	if servedir, ok := serve["servedir"]; ok {
		serveOptions.Servedir = servedir.(string)
	}
	serveOptions.OnRequest = func(args api.ServeOnRequestArgs) {
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
	}
	result, err := api.Serve(serveOptions, options)
	if err != nil {
		return outgoingPacket{bytes: encodeErrorPacket(id, err)}
	}
	response := map[string]interface{}{
		"port": int(result.Port),
		"host": result.Host,
	}

	activeBuild.refCount++ // Make sure the serve doesn't finish until "Wait" finishes
	activeBuild.serveStop = result.Stop

	// Asynchronously wait for the server to stop, then fulfil the "wait" promise
	go func() {
		request := map[string]interface{}{
			"command": "serve-wait",
			"key":     key,
		}
		if err := result.Wait(); err != nil {
			request["error"] = err.Error()
		} else {
			request["error"] = nil
		}
		service.sendRequest(request)

		// Release the ref count now that the server has shut down
		service.decRefCount(key, activeBuild)
	}()

	return outgoingPacket{
		bytes: encodePacket(packet{
			id:    id,
			value: response,
		}),
	}
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

	return api.ResolveEntryPoint, false
}

func (service *serviceType) convertPlugins(key int, jsPlugins interface{}, activeBuild *activeBuild) ([]api.Plugin, error) {
	type filteredCallback struct {
		filter     *regexp.Regexp
		pluginName string
		namespace  string
		id         int
	}

	var onResolveCallbacks []filteredCallback
	var onLoadCallbacks []filteredCallback

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

		if callbacks, err := filteredCallbacks(pluginName, "onResolve", p["onResolve"].([]interface{})); err != nil {
			return nil, err
		} else {
			onResolveCallbacks = append(onResolveCallbacks, callbacks...)
		}

		if callbacks, err := filteredCallbacks(pluginName, "onLoad", p["onLoad"].([]interface{})); err != nil {
			return nil, err
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

			build.OnStart(func() (api.OnStartResult, error) {
				result := api.OnStartResult{}

				response := service.sendRequest(map[string]interface{}{
					"command": "on-start",
					"key":     key,
				}).(map[string]interface{})

				if value, ok := response["errors"]; ok {
					result.Errors = decodeMessages(value.([]interface{}))
				}
				if value, ok := response["warnings"]; ok {
					result.Warnings = decodeMessages(value.([]interface{}))
				}

				return result, nil
			})

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

				response := service.sendRequest(map[string]interface{}{
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

				response := service.sendRequest(map[string]interface{}{
					"command":    "on-load",
					"key":        key,
					"ids":        ids,
					"path":       args.Path,
					"namespace":  args.Namespace,
					"suffix":     args.Suffix,
					"pluginData": args.PluginData,
				}).(map[string]interface{})

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
		},
	}}, nil
}

func (service *serviceType) handleTransformRequest(id uint32, request map[string]interface{}) []byte {
	inputFS := request["inputFS"].(bool)
	input := request["input"].(string)
	flags := decodeStringArray(request["flags"].([]interface{}))

	options, err := cli.ParseTransformOptions(flags)
	if err != nil {
		return encodeErrorPacket(id, err)
	}
	options.MangleCache, _ = request["mangleCache"].(map[string]interface{})

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

	response := map[string]interface{}{
		"errors":   encodeMessages(result.Errors),
		"warnings": encodeMessages(result.Warnings),

		"codeFS": codeFS,
		"code":   string(result.Code),

		"mapFS": mapFS,
		"map":   string(result.Map),
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
