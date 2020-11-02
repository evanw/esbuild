// This package contains internal API-related code that must be shared with
// other internal code outside of the API package.

package helpers

import (
	"fmt"

	"github.com/evanw/esbuild/pkg/api"
)

func ParseLoader(text string) (api.Loader, error) {
	switch text {
	case "js":
		return api.LoaderJS, nil
	case "jsx":
		return api.LoaderJSX, nil
	case "ts":
		return api.LoaderTS, nil
	case "tsx":
		return api.LoaderTSX, nil
	case "css":
		return api.LoaderCSS, nil
	case "json":
		return api.LoaderJSON, nil
	case "text":
		return api.LoaderText, nil
	case "base64":
		return api.LoaderBase64, nil
	case "dataurl":
		return api.LoaderDataURL, nil
	case "file":
		return api.LoaderFile, nil
	case "binary":
		return api.LoaderBinary, nil
	case "default":
		return api.LoaderDefault, nil
	default:
		return api.LoaderNone, fmt.Errorf("Invalid loader: %q (valid: "+
			"js, jsx, ts, tsx, css, json, text, base64, dataurl, file, binary)", text)
	}
}
