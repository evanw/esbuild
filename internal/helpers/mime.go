package helpers

import "strings"

var builtinTypesLower = map[string]string{
	// Text
	".css":      "text/css; charset=utf-8",
	".htm":      "text/html; charset=utf-8",
	".html":     "text/html; charset=utf-8",
	".js":       "text/javascript; charset=utf-8",
	".json":     "application/json; charset=utf-8",
	".markdown": "text/markdown; charset=utf-8",
	".md":       "text/markdown; charset=utf-8",
	".mjs":      "text/javascript; charset=utf-8",
	".xhtml":    "application/xhtml+xml; charset=utf-8",
	".xml":      "text/xml; charset=utf-8",

	// Images
	".avif": "image/avif",
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".webp": "image/webp",

	// Fonts
	".eot":   "application/vnd.ms-fontobject",
	".otf":   "font/otf",
	".sfnt":  "font/sfnt",
	".ttf":   "font/ttf",
	".woff":  "font/woff",
	".woff2": "font/woff2",

	// Other
	".pdf":         "application/pdf",
	".wasm":        "application/wasm",
	".webmanifest": "application/manifest+json",
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
