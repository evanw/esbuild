//go:build !js || !wasm
// +build !js !wasm

package api

// This file implements the "Serve()" function in esbuild's public API. It
// provides a basic web server that can serve a directory tree over HTTP. When
// a directory is visited the "index.html" will be served if present, otherwise
// esbuild will automatically generate a directory listing page with links for
// each file in the directory. If there is a build configured that generates
// output files, those output files are not written to disk but are instead
// "overlayed" virtually on top of the real file system. The server responds to
// HTTP requests for output files from the build with the latest in-memory
// build results.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

////////////////////////////////////////////////////////////////////////////////
// Serve API

type apiHandler struct {
	onRequest        func(ServeOnRequestArgs)
	rebuild          func() BuildResult
	stop             func()
	fs               fs.FS
	absOutputDir     string
	outdirPathPrefix string
	publicPath       string
	servedir         string
	keyfileToLower   string
	certfileToLower  string
	fallback         string
	hosts            []string
	serveWaitGroup   sync.WaitGroup
	activeStreams    []chan serverSentEvent
	currentHashes    map[string]string
	mutex            sync.Mutex
}

type serverSentEvent struct {
	event string
	data  string
}

func escapeForHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func escapeForAttribute(text string) string {
	text = escapeForHTML(text)
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&apos;")
	return text
}

func (h *apiHandler) notifyRequest(duration time.Duration, req *http.Request, status int) {
	if h.onRequest != nil {
		h.onRequest(ServeOnRequestArgs{
			RemoteAddress: req.RemoteAddr,
			Method:        req.Method,
			Path:          req.URL.Path,
			Status:        status,
			TimeInMS:      int(duration.Milliseconds()),
		})
	}
}

func errorsToString(errors []Message) string {
	stderrOptions := logger.OutputOptions{IncludeSource: true}
	terminalOptions := logger.TerminalInfo{}
	sb := strings.Builder{}
	limit := 5
	for i, msg := range convertMessagesToInternal(nil, logger.Error, errors) {
		if i == limit {
			sb.WriteString(fmt.Sprintf("%d out of %d errors shown\n", limit, len(errors)))
			break
		}
		sb.WriteString(msg.String(stderrOptions, terminalOptions))
	}
	return sb.String()
}

func (h *apiHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	start := time.Now()

	// HEAD requests omit the body
	maybeWriteResponseBody := func(bytes []byte) { res.Write(bytes) }
	isHEAD := req.Method == "HEAD"
	if isHEAD {
		maybeWriteResponseBody = func([]byte) { res.Write(nil) }
	}

	// Check the "Host" header to prevent DNS rebinding attacks
	if strings.ContainsRune(req.Host, ':') {
		// Try to strip off the port number
		if host, _, err := net.SplitHostPort(req.Host); err == nil {
			req.Host = host
		}
	}
	if req.Host != "localhost" {
		ok := false
		for _, allowed := range h.hosts {
			if req.Host == allowed {
				ok = true
				break
			}
		}
		if !ok {
			go h.notifyRequest(time.Since(start), req, http.StatusForbidden)
			res.WriteHeader(http.StatusForbidden)
			maybeWriteResponseBody([]byte(fmt.Sprintf("403 - Forbidden: The host %q is not allowed", req.Host)))
			return
		}
	}

	// Special-case the esbuild event stream
	if req.Method == "GET" && req.URL.Path == "/esbuild" && req.Header.Get("Accept") == "text/event-stream" {
		h.serveEventStream(start, req, res)
		return
	}

	// Handle GET and HEAD requests
	if (isHEAD || req.Method == "GET") && strings.HasPrefix(req.URL.Path, "/") {
		queryPath := path.Clean(req.URL.Path)[1:]
		result := h.rebuild()

		// Requests fail if the build had errors
		if len(result.Errors) > 0 {
			res.Header().Set("Content-Type", "text/plain; charset=utf-8")
			go h.notifyRequest(time.Since(start), req, http.StatusServiceUnavailable)
			res.WriteHeader(http.StatusServiceUnavailable)
			maybeWriteResponseBody([]byte(errorsToString(result.Errors)))
			return
		}

		type fileToServe struct {
			absPath  string
			contents fs.OpenedFile
		}

		var kind fs.EntryKind
		var file fileToServe
		dirEntries := make(map[string]bool)
		fileEntries := make(map[string]bool)

		// Check for a match with the results if we're within the output directory
		if outdirQueryPath, ok := stripDirPrefix(queryPath, h.outdirPathPrefix, "/"); ok {
			resultKind, inMemoryBytes, absPath, isImplicitIndexHTML := h.matchQueryPathToResult(outdirQueryPath, &result, dirEntries, fileEntries)
			kind = resultKind
			file = fileToServe{
				absPath:  absPath,
				contents: &fs.InMemoryOpenedFile{Contents: inMemoryBytes},
			}
			if isImplicitIndexHTML {
				queryPath = path.Join(queryPath, "index.html")
			}
		} else {
			// Create a fake directory entry for the output path so that it appears to be a real directory
			p := h.outdirPathPrefix
			for p != "" {
				var dir string
				var base string
				if slash := strings.IndexByte(p, '/'); slash == -1 {
					base = p
				} else {
					dir = p[:slash]
					base = p[slash+1:]
				}
				if dir == queryPath {
					kind = fs.DirEntry
					dirEntries[base] = true
					break
				}
				p = dir
			}
		}

		// Check for a file in the "servedir" directory
		if h.servedir != "" && kind != fs.FileEntry {
			absPath := h.fs.Join(h.servedir, queryPath)
			if absDir := h.fs.Dir(absPath); absDir != absPath {
				if entries, err, _ := h.fs.ReadDirectory(absDir); err == nil {
					if entry, _ := entries.Get(h.fs.Base(absPath)); entry != nil && entry.Kind(h.fs) == fs.FileEntry {
						if h.keyfileToLower != "" || h.certfileToLower != "" {
							if toLower := strings.ToLower(absPath); toLower == h.keyfileToLower || toLower == h.certfileToLower {
								// Don't serve the HTTPS key or certificate. This uses a case-
								// insensitive check because some file systems are case-sensitive.
								go h.notifyRequest(time.Since(start), req, http.StatusForbidden)
								res.WriteHeader(http.StatusForbidden)
								maybeWriteResponseBody([]byte("403 - Forbidden"))
								return
							}
						}
						if contents, err, _ := h.fs.OpenFile(absPath); err == nil {
							defer contents.Close()
							file = fileToServe{absPath: absPath, contents: contents}
							kind = fs.FileEntry
						} else if err != syscall.ENOENT {
							go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
							res.WriteHeader(http.StatusInternalServerError)
							maybeWriteResponseBody([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
							return
						}
					}
				}
			}
		}

		// Check for a directory in the "servedir" directory
		var servedirIndexName string
		if h.servedir != "" && kind != fs.FileEntry {
			if entries, err, _ := h.fs.ReadDirectory(h.fs.Join(h.servedir, queryPath)); err == nil {
				kind = fs.DirEntry
				for _, name := range entries.SortedKeys() {
					entry, _ := entries.Get(name)
					switch entry.Kind(h.fs) {
					case fs.DirEntry:
						dirEntries[name] = true
					case fs.FileEntry:
						fileEntries[name] = true
						if name == "index.html" {
							servedirIndexName = name
						}
					}
				}
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				maybeWriteResponseBody([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}
		}

		// Redirect to a trailing slash for directories
		if kind == fs.DirEntry && !strings.HasSuffix(req.URL.Path, "/") {
			res.Header().Set("Location", path.Clean(req.URL.Path)+"/")
			go h.notifyRequest(time.Since(start), req, http.StatusFound)
			res.WriteHeader(http.StatusFound)
			maybeWriteResponseBody(nil)
			return
		}

		// Serve an "index.html" file if present
		if kind == fs.DirEntry && servedirIndexName != "" {
			queryPath += "/" + servedirIndexName
			absPath := h.fs.Join(h.servedir, queryPath)
			if contents, err, _ := h.fs.OpenFile(absPath); err == nil {
				defer contents.Close()
				file = fileToServe{absPath: absPath, contents: contents}
				kind = fs.FileEntry
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				maybeWriteResponseBody([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}
		}

		// Serve the fallback HTML page if one was provided
		if kind != fs.FileEntry && h.fallback != "" {
			if contents, err, _ := h.fs.OpenFile(h.fallback); err == nil {
				defer contents.Close()
				file = fileToServe{absPath: h.fallback, contents: contents}
				kind = fs.FileEntry
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				maybeWriteResponseBody([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}
		}

		// Serve a file
		if kind == fs.FileEntry {
			// Default to serving the whole file
			status := http.StatusOK
			fileContentsLen := file.contents.Len()
			begin := 0
			end := fileContentsLen
			isRange := false

			// Handle range requests so that video playback works in Safari
			if rangeBegin, rangeEnd, ok := parseRangeHeader(req.Header.Get("Range"), fileContentsLen); ok && rangeBegin < rangeEnd {
				// Note: The content range is inclusive so subtract 1 from the end
				isRange = true
				begin = rangeBegin
				end = rangeEnd
				status = http.StatusPartialContent
			}

			// Try to read the range from the file, which may fail
			fileBytes, err := file.contents.Read(begin, end)
			if err != nil {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				maybeWriteResponseBody([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}

			// If we get here, the request was successful
			if contentType := helpers.MimeTypeByExtension(h.fs.Ext(file.absPath)); contentType != "" {
				res.Header().Set("Content-Type", contentType)
			} else {
				res.Header().Set("Content-Type", "application/octet-stream")
			}
			if isRange {
				res.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", begin, end-1, fileContentsLen))
			}
			res.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileBytes)))
			go h.notifyRequest(time.Since(start), req, status)
			res.WriteHeader(status)
			maybeWriteResponseBody(fileBytes)
			return
		}

		// Serve a directory listing
		if kind == fs.DirEntry {
			html := respondWithDirList(queryPath, dirEntries, fileEntries)
			res.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Header().Set("Content-Length", fmt.Sprintf("%d", len(html)))
			go h.notifyRequest(time.Since(start), req, http.StatusOK)
			maybeWriteResponseBody(html)
			return
		}
	}

	// Satisfy requests for "favicon.ico" to avoid errors in Firefox developer tools
	if req.Method == "GET" && req.URL.Path == "/favicon.ico" {
		for _, encoding := range strings.Split(req.Header.Get("Accept-Encoding"), ",") {
			if semi := strings.IndexByte(encoding, ';'); semi >= 0 {
				encoding = encoding[:semi]
			}
			if strings.TrimSpace(encoding) == "gzip" {
				res.Header().Set("Content-Encoding", "gzip")
				res.Header().Set("Content-Type", "image/vnd.microsoft.icon")
				go h.notifyRequest(time.Since(start), req, http.StatusOK)
				maybeWriteResponseBody(favicon_ico_gz)
				return
			}
		}
	}

	// Default to a 404
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	go h.notifyRequest(time.Since(start), req, http.StatusNotFound)
	res.WriteHeader(http.StatusNotFound)
	maybeWriteResponseBody([]byte("404 - Not Found"))
}

// This exposes an event stream to clients using server-sent events:
// https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
func (h *apiHandler) serveEventStream(start time.Time, req *http.Request, res http.ResponseWriter) {
	if flusher, ok := res.(http.Flusher); ok {
		if closer, ok := res.(http.CloseNotifier); ok {
			// Add a new stream to the array of active streams
			stream := make(chan serverSentEvent)
			h.mutex.Lock()
			h.activeStreams = append(h.activeStreams, stream)
			h.mutex.Unlock()

			// Start the event stream
			res.Header().Set("Content-Type", "text/event-stream")
			res.Header().Set("Connection", "keep-alive")
			res.Header().Set("Cache-Control", "no-cache")
			go h.notifyRequest(time.Since(start), req, http.StatusOK)
			res.WriteHeader(http.StatusOK)
			res.Write([]byte("retry: 500\n"))
			flusher.Flush()

			// Send incoming messages over the stream
			streamWasClosed := make(chan struct{}, 1)
			go func() {
				for {
					var msg []byte
					select {
					case next, ok := <-stream:
						if !ok {
							streamWasClosed <- struct{}{}
							return
						}
						msg = []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", next.event, next.data))
					case <-time.After(30 * time.Second):
						// Send an occasional keep-alive
						msg = []byte(":\n\n")
					}
					if _, err := res.Write(msg); err != nil {
						return
					}
					flusher.Flush()
				}
			}()

			// When the stream is closed (either by them or by us), remove it
			// from the array and end the response body to clean up resources
			select {
			case <-closer.CloseNotify():
			case <-streamWasClosed:
			}
			h.mutex.Lock()
			for i := range h.activeStreams {
				if h.activeStreams[i] == stream {
					end := len(h.activeStreams) - 1
					h.activeStreams[i] = h.activeStreams[end]
					h.activeStreams = h.activeStreams[:end]

					// Only close the stream if it's present in the list of active
					// streams. Stopping the server can also call close on this
					// stream and Go only lets you close a channel once before
					// panicking, so we don't want to close it twice.
					close(stream)
					break
				}
			}
			h.mutex.Unlock()
			return
		}
	}

	// If we get here, then event streaming isn't possible
	go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
	res.WriteHeader(http.StatusInternalServerError)
	res.Write([]byte("500 - Event stream error"))
}

func (h *apiHandler) broadcastBuildResult(result BuildResult, newHashes map[string]string) {
	h.mutex.Lock()

	var added []string
	var removed []string
	var updated []string

	urlForPath := func(absPath string) (string, bool) {
		if relPath, ok := stripDirPrefix(absPath, h.absOutputDir, "\\/"); ok {
			relPath = strings.ReplaceAll(relPath, "\\", "/")
			relPath = path.Join(h.outdirPathPrefix, relPath)
			publicPath := h.publicPath
			slash := "/"
			if publicPath != "" && strings.HasSuffix(h.publicPath, "/") {
				slash = ""
			}
			return fmt.Sprintf("%s%s%s", publicPath, slash, relPath), true
		}
		return "", false
	}

	// Diff the old and new states, but only if the build succeeded. We shouldn't
	// make it appear as if all files were removed when there is a build error.
	if len(result.Errors) == 0 {
		oldHashes := h.currentHashes
		h.currentHashes = newHashes

		for absPath, newHash := range newHashes {
			if oldHash, ok := oldHashes[absPath]; !ok {
				if url, ok := urlForPath(absPath); ok {
					added = append(added, url)
				}
			} else if newHash != oldHash {
				if url, ok := urlForPath(absPath); ok {
					updated = append(updated, url)
				}
			}
		}

		for absPath := range oldHashes {
			if _, ok := newHashes[absPath]; !ok {
				if url, ok := urlForPath(absPath); ok {
					removed = append(removed, url)
				}
			}
		}
	}

	// Only notify listeners if there's a change that's worth sending. That way
	// you can implement a simple "reload on any change" script without having
	// to do this check in the script.
	if len(added) > 0 || len(removed) > 0 || len(updated) > 0 {
		sort.Strings(added)
		sort.Strings(removed)
		sort.Strings(updated)

		// Assemble the diff
		var sb strings.Builder
		sb.WriteString("{\"added\":[")
		for i, path := range added {
			if i > 0 {
				sb.WriteRune(',')
			}
			sb.Write(helpers.QuoteForJSON(path, false))
		}
		sb.WriteString("],\"removed\":[")
		for i, path := range removed {
			if i > 0 {
				sb.WriteRune(',')
			}
			sb.Write(helpers.QuoteForJSON(path, false))
		}
		sb.WriteString("],\"updated\":[")
		for i, path := range updated {
			if i > 0 {
				sb.WriteRune(',')
			}
			sb.Write(helpers.QuoteForJSON(path, false))
		}
		sb.WriteString("]}")
		json := sb.String()

		// Broadcast the diff to all streams
		for _, stream := range h.activeStreams {
			stream <- serverSentEvent{event: "change", data: json}
		}
	}

	h.mutex.Unlock()
}

// Handle enough of the range specification so that video playback works in Safari
func parseRangeHeader(r string, contentLength int) (int, int, bool) {
	if strings.HasPrefix(r, "bytes=") {
		r = r[len("bytes="):]
		if dash := strings.IndexByte(r, '-'); dash != -1 {
			// Note: The range is inclusive so the limit is deliberately "length - 1"
			if begin, ok := parseRangeInt(r[:dash], contentLength-1); ok {
				if end, ok := parseRangeInt(r[dash+1:], contentLength-1); ok {
					// Note: The range is inclusive so a range of "0-1" is two bytes long
					return begin, end + 1, true
				}
			}
		}
	}
	return 0, 0, false
}

func parseRangeInt(text string, maxValue int) (int, bool) {
	if text == "" {
		return 0, false
	}
	value := 0
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, false
		}
		value = value*10 + int(c-'0')
		if value > maxValue {
			return 0, false
		}
	}
	return value, true
}

func (h *apiHandler) matchQueryPathToResult(
	queryPath string,
	result *BuildResult,
	dirEntries map[string]bool,
	fileEntries map[string]bool,
) (fs.EntryKind, []byte, string, bool) {
	queryIsDir := false
	queryDir := queryPath
	if queryDir != "" {
		queryDir += "/"
	}

	// Check the output files for a match
	for _, file := range result.OutputFiles {
		if relPath, ok := h.fs.Rel(h.absOutputDir, file.Path); ok {
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			// An exact match
			if relPath == queryPath {
				return fs.FileEntry, file.Contents, file.Path, false
			}

			// Serve an "index.html" file if present
			if dir, base := path.Split(relPath); base == "index.html" && queryDir == dir {
				return fs.FileEntry, file.Contents, file.Path, true
			}

			// A match inside this directory
			if strings.HasPrefix(relPath, queryDir) {
				entry := relPath[len(queryDir):]
				queryIsDir = true
				if slash := strings.IndexByte(entry, '/'); slash == -1 {
					fileEntries[entry] = true
				} else if dir := entry[:slash]; !dirEntries[dir] {
					dirEntries[dir] = true
				}
			}
		}
	}

	// Treat this as a directory if it's non-empty
	if queryIsDir {
		return fs.DirEntry, nil, "", false
	}

	return 0, nil, "", false
}

func respondWithDirList(queryPath string, dirEntries map[string]bool, fileEntries map[string]bool) []byte {
	queryPath = "/" + queryPath
	queryDir := queryPath
	if queryDir != "/" {
		queryDir += "/"
	}
	html := strings.Builder{}
	html.WriteString("<!doctype html>\n")
	html.WriteString("<meta charset=\"utf8\">\n")
	html.WriteString("<style>\n")
	html.WriteString("body { margin: 30px; color: #222; background: #fff; font: 16px/22px sans-serif; }\n")
	html.WriteString("a { color: inherit; text-decoration: none; }\n")
	html.WriteString("a:hover { text-decoration: underline; }\n")
	html.WriteString("a:visited { color: #777; }\n")
	html.WriteString("@media (prefers-color-scheme: dark) {\n")
	html.WriteString("  body { color: #fff; background: #222; }\n")
	html.WriteString("  a:visited { color: #aaa; }\n")
	html.WriteString("}\n")
	html.WriteString("</style>\n")
	html.WriteString("<title>Directory: ")
	html.WriteString(escapeForHTML(queryDir))
	html.WriteString("</title>\n")
	html.WriteString("<h1>Directory: ")
	var parts []string
	if queryPath == "/" {
		parts = []string{""}
	} else {
		parts = strings.Split(queryPath, "/")
	}
	for i, part := range parts {
		if i+1 < len(parts) {
			html.WriteString("<a href=\"")
			html.WriteString(escapeForHTML(strings.Join(parts[:i+1], "/")))
			html.WriteString("/\">")
		}
		html.WriteString(escapeForHTML(part))
		html.WriteString("/")
		if i+1 < len(parts) {
			html.WriteString("</a>")
		}
	}
	html.WriteString("</h1>\n")

	// Link to the parent directory
	if queryPath != "/" {
		parentDir := path.Dir(queryPath)
		if parentDir != "/" {
			parentDir += "/"
		}
		html.WriteString(fmt.Sprintf("<div>📁 <a href=\"%s\">../</a></div>\n", escapeForAttribute(parentDir)))
	}

	// Link to child directories
	strings := make([]string, 0, len(dirEntries)+len(fileEntries))
	for entry := range dirEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf("<div>📁 <a href=\"%s/\">%s/</a></div>\n", escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	// Link to files in the directory
	strings = strings[:0]
	for entry := range fileEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf("<div>📄 <a href=\"%s\">%s</a></div>\n", escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	return []byte(html.String())
}

// This is used to make error messages platform-independent
func prettyPrintPath(fs fs.FS, path string) string {
	if relPath, ok := fs.Rel(fs.Cwd(), path); ok {
		return strings.ReplaceAll(relPath, "\\", "/")
	}
	return path
}

func (ctx *internalContext) Serve(serveOptions ServeOptions) (ServeResult, error) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()

	// Ignore disposed contexts
	if ctx.didDispose {
		return ServeResult{}, errors.New("Cannot serve a disposed context")
	}

	// Don't allow starting serve mode multiple times
	if ctx.handler != nil {
		return ServeResult{}, errors.New("Serve mode has already been enabled")
	}

	// Don't allow starting serve mode multiple times
	if (serveOptions.Keyfile != "") != (serveOptions.Certfile != "") {
		return ServeResult{}, errors.New("Must specify both key and certificate for HTTPS")
	}

	// Validate the "servedir" path
	if serveOptions.Servedir != "" {
		if absPath, ok := ctx.realFS.Abs(serveOptions.Servedir); ok {
			serveOptions.Servedir = absPath
		} else {
			return ServeResult{}, fmt.Errorf("Invalid serve path: %s", serveOptions.Servedir)
		}
	}

	// Validate the "fallback" path
	if serveOptions.Fallback != "" {
		if absPath, ok := ctx.realFS.Abs(serveOptions.Fallback); ok {
			serveOptions.Fallback = absPath
		} else {
			return ServeResult{}, fmt.Errorf("Invalid fallback path: %s", serveOptions.Fallback)
		}
	}

	// Stuff related to the output directory only matters if there are entry points
	outdirPathPrefix := ""
	if len(ctx.args.entryPoints) > 0 {
		// Don't allow serving when builds are written to stdout
		if ctx.args.options.WriteToStdout {
			what := "entry points"
			if len(ctx.args.entryPoints) == 1 {
				what = "an entry point"
			}
			return ServeResult{}, fmt.Errorf("Cannot serve %s without an output path", what)
		}

		// Compute the output path prefix
		if serveOptions.Servedir != "" && ctx.args.options.AbsOutputDir != "" {
			// Make sure the output directory is contained in the "servedir" directory
			relPath, ok := ctx.realFS.Rel(serveOptions.Servedir, ctx.args.options.AbsOutputDir)
			if !ok {
				return ServeResult{}, fmt.Errorf(
					"Cannot compute relative path from %q to %q\n", serveOptions.Servedir, ctx.args.options.AbsOutputDir)
			}
			relPath = strings.ReplaceAll(relPath, "\\", "/") // Fix paths on Windows
			if relPath == ".." || strings.HasPrefix(relPath, "../") {
				return ServeResult{}, fmt.Errorf(
					"Output directory %q must be contained in serve directory %q",
					prettyPrintPath(ctx.realFS, ctx.args.options.AbsOutputDir),
					prettyPrintPath(ctx.realFS, serveOptions.Servedir),
				)
			}
			if relPath != "." {
				outdirPathPrefix = relPath
			}
		}
	}

	// Determine the host
	var listener net.Listener
	network := "tcp4"
	host := "0.0.0.0"
	if serveOptions.Host != "" {
		host = serveOptions.Host

		// Only use "tcp4" if this is an IPv4 address, otherwise use "tcp"
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			network = "tcp"
		}
	}

	// Pick the port
	if serveOptions.Port == 0 {
		// Default to picking a "800X" port
		for port := 8000; port <= 8009; port++ {
			if result, err := net.Listen(network, net.JoinHostPort(host, fmt.Sprintf("%d", port))); err == nil {
				listener = result
				break
			}
		}
	}
	if listener == nil {
		// Otherwise pick the provided port
		port := serveOptions.Port
		if port < 0 || port > 0xFFFF {
			port = 0 // Pick a random port if the provided port is out of range
		}
		if result, err := net.Listen(network, net.JoinHostPort(host, fmt.Sprintf("%d", port))); err != nil {
			return ServeResult{}, err
		} else {
			listener = result
		}
	}

	// Try listening on the provided port
	addr := listener.Addr().String()

	// Extract the real port in case we passed a port of "0"
	var result ServeResult
	var boundHost string
	if host, text, err := net.SplitHostPort(addr); err == nil {
		if port, err := strconv.ParseInt(text, 10, 32); err == nil {
			result.Port = uint16(port)
			boundHost = host
		}
	}

	// Build up a list of all hosts we use
	if ip := net.ParseIP(boundHost); ip != nil && ip.IsUnspecified() {
		// If this is "0.0.0.0" or "::", list all relevant IP addresses
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, addr := range addrs {
				if addr, ok := addr.(*net.IPNet); ok && (addr.IP.To4() != nil) == (ip.To4() != nil) && !addr.IP.IsLinkLocalUnicast() {
					result.Hosts = append(result.Hosts, addr.IP.String())
				}
			}
		}
	} else {
		result.Hosts = append(result.Hosts, boundHost)
	}

	// HTTPS-related files should be absolute paths
	isHTTPS := serveOptions.Keyfile != "" && serveOptions.Certfile != ""
	if isHTTPS {
		serveOptions.Keyfile, _ = ctx.realFS.Abs(serveOptions.Keyfile)
		serveOptions.Certfile, _ = ctx.realFS.Abs(serveOptions.Certfile)
	}

	var shouldStop int32

	// The first build will just build normally
	handler := &apiHandler{
		onRequest:        serveOptions.OnRequest,
		outdirPathPrefix: outdirPathPrefix,
		absOutputDir:     ctx.args.options.AbsOutputDir,
		publicPath:       ctx.args.options.PublicPath,
		servedir:         serveOptions.Servedir,
		keyfileToLower:   strings.ToLower(serveOptions.Keyfile),
		certfileToLower:  strings.ToLower(serveOptions.Certfile),
		fallback:         serveOptions.Fallback,
		hosts:            append([]string{}, result.Hosts...),
		rebuild: func() BuildResult {
			if atomic.LoadInt32(&shouldStop) != 0 {
				// Don't start more rebuilds if we were told to stop
				return BuildResult{}
			} else {
				return ctx.activeBuildOrRecentBuildOrRebuild()
			}
		},
		fs: ctx.realFS,
	}

	// Create the server
	server := &http.Server{Addr: addr, Handler: handler}

	// When stop is called, block further rebuilds and then close the server
	handler.stop = func() {
		atomic.StoreInt32(&shouldStop, 1)

		// Close the server and wait for it to close
		server.Close()

		// Close all open event streams
		handler.mutex.Lock()
		for _, stream := range handler.activeStreams {
			close(stream)
		}
		handler.activeStreams = nil
		handler.mutex.Unlock()

		handler.serveWaitGroup.Wait()
	}

	// HACK: Go's HTTP API doesn't appear to provide a way to separate argument
	// validation errors from eventual network errors. Specifically "ServeTLS"
	// blocks for an arbitrarily long time before returning an error. So we
	// intercept the first call to "Accept" on the listener and say that the
	// serve call succeeded without an error if we get to that point.
	hack := &hackListener{Listener: listener}
	hack.waitGroup.Add(1)

	// Start the server and signal on "serveWaitGroup" when it stops
	handler.serveWaitGroup.Add(1)
	go func() {
		var err error
		if isHTTPS {
			err = server.ServeTLS(hack, serveOptions.Certfile, serveOptions.Keyfile)
		} else {
			err = server.Serve(hack)
		}
		if err != http.ErrServerClosed {
			hack.mutex.Lock()
			if !hack.done {
				hack.done = true
				hack.err = err
				hack.waitGroup.Done()
			}
			hack.mutex.Unlock()
		}
		handler.serveWaitGroup.Done()
	}()

	// Return an error if the server failed to start accepting connections
	hack.waitGroup.Wait()
	if hack.err != nil {
		return ServeResult{}, hack.err
	}

	// There appears to be some issue with Linux (but not with macOS) where
	// destroying and recreating a server with the same port as the previous
	// server had sometimes causes subsequent connections to fail with
	// ECONNRESET (shows up in node as "Error: socket hang up").
	//
	// I think the problem is sort of that Go sets SO_REUSEADDR to 1 for listener
	// sockets (specifically in "setDefaultListenerSockopts"). In some ways this
	// is good, because it's more convenient for the user if the port is the
	// same. However, I believe this sends a TCP RST packet to kill any previous
	// connections. That can then be received by clients attempting to connect
	// to the new server.
	//
	// As a hack to work around this problem, we wait for an additional short
	// amount of time before returning. I observed this problem even with a 5ms
	// timeout but I did not observe this problem with a 10ms timeout. So I'm
	// setting this timeout to 50ms to be extra safe.
	time.Sleep(50 * time.Millisecond)

	// Only set the context handler if the server started successfully
	ctx.handler = handler

	// Print the URL(s) that the server can be reached at
	if ctx.args.logOptions.LogLevel <= logger.LevelInfo {
		printURLs(handler.hosts, result.Port, isHTTPS, ctx.args.logOptions.Color)
	}

	// Start the first build shortly after this function returns (but not
	// immediately so that stuff we print right after this will come first).
	//
	// This also helps the CLI not do two builds when serve and watch mode
	// are enabled together. Watch mode is enabled after serve mode because
	// we want the stderr output for watch to come after the stderr output for
	// serve, but watch mode will do another build if the current build is
	// not a watch mode build.
	go func() {
		time.Sleep(10 * time.Millisecond)
		handler.rebuild()
	}()
	return result, nil
}

type hackListener struct {
	net.Listener
	mutex     sync.Mutex
	waitGroup sync.WaitGroup
	err       error
	done      bool
}

func (hack *hackListener) Accept() (net.Conn, error) {
	hack.mutex.Lock()
	if !hack.done {
		hack.done = true
		hack.waitGroup.Done()
	}
	hack.mutex.Unlock()
	return hack.Listener.Accept()
}

func printURLs(hosts []string, port uint16, https bool, useColor logger.UseColor) {
	logger.PrintTextWithColor(os.Stderr, useColor, func(colors logger.Colors) string {
		sb := strings.Builder{}
		sb.WriteString(colors.Reset)

		// Determine the host kinds
		kinds := make([]string, len(hosts))
		maxLen := 0
		for i, host := range hosts {
			kind := "Network"
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				kind = "Local"
			}
			kinds[i] = kind
			if len(kind) > maxLen {
				maxLen = len(kind)
			}
		}

		// Pretty-print the host list
		protocol := "http"
		if https {
			protocol = "https"
		}
		for i, kind := range kinds {
			sb.WriteString(fmt.Sprintf("\n > %s:%s %s%s://%s/%s",
				kind, strings.Repeat(" ", maxLen-len(kind)), colors.Underline, protocol,
				net.JoinHostPort(hosts[i], fmt.Sprintf("%d", port)), colors.Reset))
		}

		sb.WriteString("\n\n")
		return sb.String()
	})
}
