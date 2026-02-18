//go:build js && wasm
// +build js,wasm

package bundler

// Lightweight MIME sniffing for WebAssembly builds to avoid pulling in net/http.
// This checks common binary magic bytes and falls back to text/plain or
// application/octet-stream based on whether the content looks like ASCII text.

func sniffMimeType(contents []byte) string {
	if len(contents) == 0 {
		return "text/plain; charset=utf-8"
	}

	// Check magic bytes for common binary formats
	if len(contents) >= 8 {
		// PNG
		if contents[0] == 0x89 && contents[1] == 'P' && contents[2] == 'N' && contents[3] == 'G' {
			return "image/png"
		}
		// GIF87a or GIF89a
		if contents[0] == 'G' && contents[1] == 'I' && contents[2] == 'F' && contents[3] == '8' {
			return "image/gif"
		}
		// WebP (RIFF....WEBP)
		if contents[0] == 'R' && contents[1] == 'I' && contents[2] == 'F' && contents[3] == 'F' &&
			contents[8] == 'W' && contents[9] == 'E' && contents[10] == 'B' && contents[11] == 'P' {
			return "image/webp"
		}
	}

	// JPEG
	if len(contents) >= 3 && contents[0] == 0xFF && contents[1] == 0xD8 && contents[2] == 0xFF {
		return "image/jpeg"
	}

	// PDF
	if len(contents) >= 5 && contents[0] == '%' && contents[1] == 'P' && contents[2] == 'D' && contents[3] == 'F' && contents[4] == '-' {
		return "application/pdf"
	}

	// ZIP (also covers XLSX, DOCX, JAR, etc.)
	if len(contents) >= 4 && contents[0] == 'P' && contents[1] == 'K' && contents[2] == 0x03 && contents[3] == 0x04 {
		return "application/zip"
	}

	// GZIP
	if len(contents) >= 2 && contents[0] == 0x1F && contents[1] == 0x8B {
		return "application/x-gzip"
	}

	// Check if content looks like text (first 512 bytes, matching net/http behavior)
	n := len(contents)
	if n > 512 {
		n = 512
	}
	for _, b := range contents[:n] {
		if b < 0x08 || (b > 0x0D && b < 0x20 && b != 0x1B) {
			return "application/octet-stream"
		}
	}

	return "text/plain; charset=utf-8"
}
