// This package contains internal CLI-related code that must be shared with
// other internal code outside of the CLI package.

package cli_helpers

import (
	"fmt"

	"github.com/evanw/esbuild/pkg/api"
)

type ErrorWithNote struct {
	Text string
	Note string
}

func MakeErrorWithNote(text string, note string) *ErrorWithNote {
	return &ErrorWithNote{
		Text: text,
		Note: note,
	}
}

func ParseLoader(text string) (api.Loader, *ErrorWithNote) {
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
	case "copy":
		return api.LoaderCopy, nil
	default:
		return api.LoaderNone, MakeErrorWithNote(
			fmt.Sprintf("Invalid loader value: %q", text),
			"Valid values are \"js\", \"jsx\", \"ts\", \"tsx\", \"css\", \"json\", \"text\", \"base64\", \"dataurl\", \"file\", \"binary\", or \"copy\".",
		)
	}
}
