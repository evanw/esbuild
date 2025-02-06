package helpers

import (
	"net/url"
	"strings"

	"github.com/evanw/esbuild/internal/fs"
)

func IsInsideNodeModules(path string) bool {
	for {
		// This is written in a platform-independent manner because it's run on
		// user-specified paths which can be arbitrary non-file-system things. So
		// for example Windows paths may end up being used on Unix or URLs may end
		// up being used on Windows. Be consistently agnostic to which kind of
		// slash is used on all platforms.
		slash := strings.LastIndexAny(path, "/\\")
		if slash == -1 {
			return false
		}
		dir, base := path[:slash], path[slash+1:]
		if base == "node_modules" {
			return true
		}
		path = dir
	}
}

func IsFileURL(fileURL *url.URL) bool {
	return fileURL.Scheme == "file" && (fileURL.Host == "" || fileURL.Host == "localhost") && strings.HasPrefix(fileURL.Path, "/")
}

func FileURLFromFilePath(filePath string) *url.URL {
	// Append a trailing slash so that resolving the URL includes the trailing
	// directory, and turn Windows-style paths with volumes into URL-style paths:
	//
	//   "/Users/User/Desktop" => "/Users/User/Desktop/"
	//   "C:\\Users\\User\\Desktop" => "/C:/Users/User/Desktop/"
	//
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	return &url.URL{Scheme: "file", Path: filePath}
}

func FilePathFromFileURL(fs fs.FS, fileURL *url.URL) string {
	path := fileURL.Path

	// Convert URL-style paths back into Windows-style paths if needed:
	//
	//   "/C:/Users/User/foo.js.map" => "C:\\Users\\User\\foo.js.map"
	//
	if !strings.HasPrefix(fs.Cwd(), "/") {
		path = strings.TrimPrefix(path, "/")
		path = strings.ReplaceAll(path, "/", "\\") // This is needed for "filepath.Rel()" to work
	}

	return path
}
