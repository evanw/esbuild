package snap_api

import (
	"fmt"
	"encoding/hex"

	"github.com/evanw/esbuild/pkg/api"
)

/*
 *	interface OutputFile {
 *	  path: string;
 *	  contents: Uint8Array; // "text" as bytes (we actually send a 'hex' string)
 *	  text: string; // "contents" as text  (we don't include that as it transmits data duplicated in contents)
 *	}
 *  interface BuildResult {
 *	  warnings: Message[];
 *	  outputFiles?: OutputFile[]; // Only when "write: false"
 *	  rebuild?: BuildInvalidate; // Only when "incremental" is true (not implemented for now)
 *	}
 */

func warningsJSON(result api.BuildResult) string {
	warnings := "[\n"
	for i, x := range result.Warnings {
		// TODO(thlorenz): include location related data (see encodeMessages in service.go)
		warnings += fmt.Sprintf("    %q", x.Text)
		if i+1 < len(result.Warnings) {
			warnings += ",\n"
		}
	}
	warnings += "\n  ]"
	return warnings
}

func outputFilesToJSON(result api.BuildResult) string {
	outputFiles := "["

	for i, x := range result.OutputFiles {
    contents := x.Contents
		outputFiles += fmt.Sprintf(`
    { 
      "path": "<stdout>",
      "contents": "%v"
    }`,  hex.EncodeToString(contents))

		if i+1 < len(result.OutputFiles) {
			outputFiles += ","
		}
	}
	outputFiles += "\n  ]"
	return outputFiles
}

// NOTE: esbuild itself doesn't send JSON across the wire like this. Instead it sends binary
// data which it then decodes into an JS object.
func resultToJSON(result api.BuildResult, write bool) string {
	json := "{\n"
	// TODO(thlorenz): warnings need proper escaping
	json += fmt.Sprintf(`  "warnings": %s`, warningsJSON(result))

	if !write {
		json += fmt.Sprintf(`,
  "outfiles": %s`,
    outputFilesToJSON(result))
	}
	json += "\n}"
	return json
}
