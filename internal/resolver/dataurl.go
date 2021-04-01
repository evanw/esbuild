package resolver

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

type DataURL struct {
	mimeType string
	data     string
	isBase64 bool
}

func ParseDataURL(url string) (parsed DataURL, ok bool) {
	if strings.HasPrefix(url, "data:") {
		if comma := strings.IndexByte(url, ','); comma != -1 {
			parsed.mimeType = url[len("data:"):comma]
			parsed.data = url[comma+1:]
			if strings.HasSuffix(parsed.mimeType, ";base64") {
				parsed.mimeType = parsed.mimeType[:len(parsed.mimeType)-len(";base64")]
				parsed.isBase64 = true
			}
			ok = true
		}
	}
	return
}

type MIMEType uint8

const (
	MIMETypeUnsupported MIMEType = iota
	MIMETypeTextCSS
	MIMETypeTextJavaScript
	MIMETypeApplicationJSON
)

func (parsed DataURL) DecodeMIMEType() MIMEType {
	// Remove things like ";charset=utf-8"
	mimeType := parsed.mimeType
	if semicolon := strings.IndexByte(mimeType, ';'); semicolon != -1 {
		mimeType = mimeType[:semicolon]
	}

	// Hard-code a few supported types
	switch mimeType {
	case "text/css":
		return MIMETypeTextCSS
	case "text/javascript":
		return MIMETypeTextJavaScript
	case "application/json":
		return MIMETypeApplicationJSON
	default:
		return MIMETypeUnsupported
	}
}

func (parsed DataURL) DecodeData() (string, error) {
	// Try to read base64 data
	if parsed.isBase64 {
		bytes, err := base64.StdEncoding.DecodeString(parsed.data)
		if err != nil {
			return "", fmt.Errorf("could not decode base64 data: %s", err.Error())
		}
		return string(bytes), nil
	}

	// Try to read percent-escaped data
	content, err := url.PathUnescape(parsed.data)
	if err != nil {
		return "", fmt.Errorf("could not decode percent-escaped data: %s", err.Error())
	}
	return content, nil
}
