package helpers

import "strings"

var builtinTypesLower = map[string]string{
	".css":  "text/css; charset=utf-8",
	".gif":  "image/gif",
	".htm":  "text/html; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".js":   "text/javascript; charset=utf-8",
	".json": "application/json",
	".mjs":  "text/javascript; charset=utf-8",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".wasm": "application/wasm",
	".webp": "image/webp",
	".xml":  "text/xml; charset=utf-8",
}

// This is used instead of Go's built-in "mime.TypeByExtension" function because
// that function is broken on Windows: https://github.com/golang/go/issues/32350.
func MimeTypeByExtension(ext string) string {
	contentType := builtinTypesLower[ext]
	if contentType == "" {
		contentType = builtinTypesLower[strings.ToLower(ext)]
	}
	return contentType
}
