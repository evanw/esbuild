// +build !js !wasm

package api

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
)

////////////////////////////////////////////////////////////////////////////////
// Serve API

type apiHandler struct {
	mutex            sync.Mutex
	outdirPathPrefix string
	servedir         string
	options          *config.Options
	onRequest        func(ServeOnRequestArgs)
	rebuild          func() BuildResult
	currentBuild     *runningBuild
	fs               fs.FS
}

type runningBuild struct {
	waitGroup sync.WaitGroup
	result    BuildResult
}

func (h *apiHandler) build() BuildResult {
	build := func() *runningBuild {
		h.mutex.Lock()
		defer h.mutex.Unlock()
		if h.currentBuild == nil {
			build := &runningBuild{}
			build.waitGroup.Add(1)
			h.currentBuild = build

			// Build on another thread
			go func() {
				result := h.rebuild()
				h.rebuild = result.Rebuild
				build.result = result
				build.waitGroup.Done()

				// Build results stay valid for a little bit afterward since a page
				// load may involve multiple requests and don't want to rebuild
				// separately for each of those requests.
				time.Sleep(250 * time.Millisecond)
				h.mutex.Lock()
				defer h.mutex.Unlock()
				h.currentBuild = nil
			}()
		}
		return h.currentBuild
	}()
	build.waitGroup.Wait()
	return build.result
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

	// Handle get requests
	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/") {
		res.Header().Set("Access-Control-Allow-Origin", "*")
		queryPath := path.Clean(req.URL.Path)[1:]
		result := h.build()

		// Requests fail if the build had errors
		if len(result.Errors) > 0 {
			go h.notifyRequest(time.Since(start), req, http.StatusServiceUnavailable)
			res.Header().Set("Content-Type", "text/plain; charset=utf-8")
			res.WriteHeader(http.StatusServiceUnavailable)
			res.Write([]byte(errorsToString(result.Errors)))
			return
		}

		var kind fs.EntryKind
		var fileContents []byte
		dirEntries := make(map[string]bool)
		fileEntries := make(map[string]bool)

		// Check for a match with the results if we're within the output directory
		if strings.HasPrefix(queryPath, h.outdirPathPrefix) {
			outdirQueryPath := queryPath[len(h.outdirPathPrefix):]
			if strings.HasPrefix(outdirQueryPath, "/") {
				outdirQueryPath = outdirQueryPath[1:]
			}
			kind, fileContents = h.matchQueryPathToResult(outdirQueryPath, &result, dirEntries, fileEntries)
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

		// Check for a file in the fallback directory
		if h.servedir != "" && kind != fs.FileEntry {
			absPath := h.fs.Join(h.servedir, queryPath)
			if absDir := h.fs.Dir(absPath); absDir != absPath {
				if entries, err := h.fs.ReadDirectory(absDir); err == nil {
					if entry, _ := entries.Get(h.fs.Base(absPath)); entry != nil && entry.Kind(h.fs) == fs.FileEntry {
						if contents, err := h.fs.ReadFile(absPath); err == nil {
							fileContents = []byte(contents)
							kind = fs.FileEntry
						} else if err != syscall.ENOENT {
							go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
							res.WriteHeader(http.StatusInternalServerError)
							res.Write([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
							return
						}
					}
				}
			}
		}

		// Check for a directory in the fallback directory
		var fallbackIndexName string
		if h.servedir != "" && kind != fs.FileEntry {
			if entries, err := h.fs.ReadDirectory(h.fs.Join(h.servedir, queryPath)); err == nil {
				kind = fs.DirEntry
				for _, name := range entries.UnorderedKeys() {
					entry, _ := entries.Get(name)
					switch entry.Kind(h.fs) {
					case fs.DirEntry:
						dirEntries[name] = true
					case fs.FileEntry:
						fileEntries[name] = true
						if name == "index.html" {
							fallbackIndexName = name
						}
					}
				}
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}
		}

		// Redirect to a trailing slash for directories
		if kind == fs.DirEntry && !strings.HasSuffix(req.URL.Path, "/") {
			res.Header().Set("Location", req.URL.Path+"/")
			go h.notifyRequest(time.Since(start), req, http.StatusFound)
			res.WriteHeader(http.StatusFound)
			res.Write(nil)
			return
		}

		// Serve a "index.html" file if present
		if kind == fs.DirEntry && fallbackIndexName != "" {
			queryPath += "/" + fallbackIndexName
			contents, err := h.fs.ReadFile(h.fs.Join(h.servedir, queryPath))
			if err == nil {
				fileContents = []byte(contents)
				kind = fs.FileEntry
			} else if err != syscall.ENOENT {
				go h.notifyRequest(time.Since(start), req, http.StatusInternalServerError)
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(fmt.Sprintf("500 - Internal server error: %s", err.Error())))
				return
			}
		}

		// Serve a file
		if kind == fs.FileEntry {
			if contentType := mime.TypeByExtension(path.Ext(queryPath)); contentType != "" {
				res.Header().Set("Content-Type", contentType)
			}

			// Handle range requests so that video playback works in Safari
			status := http.StatusOK
			if begin, end, ok := parseRangeHeader(req.Header.Get("Range"), len(fileContents)); ok && begin < end {
				// Note: The content range is inclusive so subtract 1 from the end
				res.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", begin, end-1, len(fileContents)))
				fileContents = fileContents[begin:end]
				status = http.StatusPartialContent
			}

			res.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileContents)))
			go h.notifyRequest(time.Since(start), req, status)
			res.WriteHeader(status)
			res.Write(fileContents)
			return
		}

		// Serve a directory listing
		if kind == fs.DirEntry {
			html := respondWithDirList(queryPath, dirEntries, fileEntries)
			res.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Header().Set("Content-Length", fmt.Sprintf("%d", len(html)))
			go h.notifyRequest(time.Since(start), req, http.StatusOK)
			res.Write(html)
			return
		}
	}

	// Default to a 404
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	go h.notifyRequest(time.Since(start), req, http.StatusNotFound)
	res.WriteHeader(http.StatusNotFound)
	res.Write([]byte("404 - Not Found"))
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
) (fs.EntryKind, []byte) {
	queryIsDir := false
	queryDir := queryPath
	if queryDir != "" {
		queryDir += "/"
	}

	// Check the output files for a match
	for _, file := range result.OutputFiles {
		if relPath, ok := h.fs.Rel(h.options.AbsOutputDir, file.Path); ok {
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			// An exact match
			if relPath == queryPath {
				return fs.FileEntry, file.Contents
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
		return fs.DirEntry, nil
	}

	return 0, nil
}

func respondWithDirList(queryPath string, dirEntries map[string]bool, fileEntries map[string]bool) []byte {
	queryPath = "/" + queryPath
	queryDir := queryPath
	if queryDir != "/" {
		queryDir += "/"
	}
	html := strings.Builder{}
	html.WriteString(`<!doctype html>`)
	html.WriteString(`<meta charset="utf8">`)
	html.WriteString(`<title>Directory: `)
	html.WriteString(escapeForHTML(queryDir))
	html.WriteString(`</title>`)
	html.WriteString(`<h1>Directory: `)
	html.WriteString(escapeForHTML(queryDir))
	html.WriteString(`</h1>`)
	html.WriteString(`<ul>`)

	// Link to the parent directory
	if queryPath != "/" {
		parentDir := path.Dir(queryPath)
		if parentDir != "/" {
			parentDir += "/"
		}
		html.WriteString(fmt.Sprintf(`<li><a href="%s">../</a></li>`, escapeForAttribute(parentDir)))
	}

	// Link to child directories
	strings := make([]string, 0, len(dirEntries)+len(fileEntries))
	for entry := range dirEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf(`<li><a href="%s/">%s/</a></li>`, escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	// Link to files in the directory
	strings = strings[:0]
	for entry := range fileEntries {
		strings = append(strings, entry)
	}
	sort.Strings(strings)
	for _, entry := range strings {
		html.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, escapeForAttribute(path.Join(queryPath, entry)), escapeForHTML(entry)))
	}

	html.WriteString(`</ul>`)
	return []byte(html.String())
}

// This is used to make error messages platform-independent
func prettyPrintPath(fs fs.FS, path string) string {
	if relPath, ok := fs.Rel(fs.Cwd(), path); ok {
		return strings.ReplaceAll(relPath, "\\", "/")
	}
	return path
}

func serveImpl(serveOptions ServeOptions, buildOptions BuildOptions) (ServeResult, error) {
	realFS, err := fs.RealFS(fs.RealFSOptions{
		AbsWorkingDir: buildOptions.AbsWorkingDir,

		// This is a long-lived file system object so do not cache calls to
		// ReadDirectory() (they are normally cached for the duration of a build
		// for performance).
		DoNotCache: true,
	})
	if err != nil {
		return ServeResult{}, err
	}
	buildOptions.Incremental = true
	buildOptions.Write = false

	// Watch and serve are both different ways of rebuilding, and cannot be combined
	if buildOptions.Watch != nil {
		return ServeResult{}, fmt.Errorf("Cannot use \"watch\" with \"serve\"")
	}

	// Validate the fallback path
	if serveOptions.Servedir != "" {
		if absPath, ok := realFS.Abs(serveOptions.Servedir); ok {
			serveOptions.Servedir = absPath
		} else {
			return ServeResult{}, fmt.Errorf("Invalid serve path: %s", serveOptions.Servedir)
		}
	}

	// If there is no output directory, set the output directory to something so
	// the build doesn't try to write to stdout. Make sure not to set this to a
	// path that may contain the user's files in it since we don't want to get
	// errors about overwriting input files.
	outdirPathPrefix := ""
	if buildOptions.Outdir == "" && buildOptions.Outfile == "" {
		buildOptions.Outdir = realFS.Join(realFS.Cwd(), "...")
	} else if serveOptions.Servedir != "" {
		// Compute the output directory
		var outdir string
		if buildOptions.Outdir != "" {
			if absPath, ok := realFS.Abs(buildOptions.Outdir); ok {
				outdir = absPath
			} else {
				return ServeResult{}, fmt.Errorf("Invalid outdir path: %s", buildOptions.Outdir)
			}
		} else {
			if absPath, ok := realFS.Abs(buildOptions.Outfile); ok {
				outdir = realFS.Dir(absPath)
			} else {
				return ServeResult{}, fmt.Errorf("Invalid outdir path: %s", buildOptions.Outfile)
			}
		}

		// Make sure the output directory is contained in the fallback directory
		relPath, ok := realFS.Rel(serveOptions.Servedir, outdir)
		if !ok {
			return ServeResult{}, fmt.Errorf(
				"Cannot compute relative path from %q to %q\n", serveOptions.Servedir, outdir)
		}
		relPath = strings.ReplaceAll(relPath, "\\", "/") // Fix paths on Windows
		if relPath == ".." || strings.HasPrefix(relPath, "../") {
			return ServeResult{}, fmt.Errorf(
				"Output directory %q must be contained in serve directory %q",
				prettyPrintPath(realFS, outdir),
				prettyPrintPath(realFS, serveOptions.Servedir),
			)
		}
		if relPath != "." {
			outdirPathPrefix = relPath
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
		if result, err := net.Listen(network, net.JoinHostPort(host, fmt.Sprintf("%d", serveOptions.Port))); err != nil {
			return ServeResult{}, err
		} else {
			listener = result
		}
	}

	// Try listening on the provided port
	addr := listener.Addr().String()

	// Extract the real port in case we passed a port of "0"
	var result ServeResult
	if host, text, err := net.SplitHostPort(addr); err == nil {
		if port, err := strconv.ParseInt(text, 10, 32); err == nil {
			result.Port = uint16(port)
			result.Host = host
		}
	}

	// The first build will just build normally
	var handler *apiHandler
	handler = &apiHandler{
		onRequest:        serveOptions.OnRequest,
		outdirPathPrefix: outdirPathPrefix,
		servedir:         serveOptions.Servedir,
		rebuild: func() BuildResult {
			build := buildImpl(buildOptions)
			if handler.options == nil {
				handler.options = &build.options
			}
			return build.result
		},
		fs: realFS,
	}

	// Start the server
	server := &http.Server{Addr: addr, Handler: handler}
	wait := make(chan error, 1)
	result.Wait = func() error { return <-wait }
	result.Stop = func() { server.Close() }
	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			wait <- err
		} else {
			wait <- nil
		}
	}()

	// Start the first build shortly after this function returns (but not
	// immediately so that stuff we print right after this will come first)
	go func() {
		time.Sleep(10 * time.Millisecond)
		handler.build()
	}()
	return result, nil
}
