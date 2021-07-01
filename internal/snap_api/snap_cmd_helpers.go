package snap_api

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/evanw/esbuild/pkg/api"
)

func warningsJSON(result api.BuildResult) string {
	warnings := "[\n"
	for i, x := range result.Warnings {
		/*
		 * interface Location {
		 *   file: string;
		 *   namespace: string;
		 *   line: number; // 1-based
		 *   column: number; // 0-based, in bytes
		 *   length: number; // in bytes
		 *   lineText: string;
		 * }
		 * interface Message {
		 *   text: string;
		 *   location: Location | null;
		 * }
		 */
		warnings += fmt.Sprintf(`{
      "text": %q,
      "location": {
		    "file": %q,
		    "namespace": %q,
		    "line": %d,
		    "column": %d,
        "length": %d,
        "lineText": %q
      }
		}`,
			x.Text,
			x.Location.File,
			x.Location.Namespace,
			x.Location.Line,
			x.Location.Column,
			x.Location.Length,
			x.Location.LineText,
		)

		if i+1 < len(result.Warnings) {
			warnings += ",\n"
		}
	}
	warnings += "\n  ]"
	return warnings
}

func outputFilesToJSON(result api.BuildResult) string {
	includedSourceMap := len(result.OutputFiles) == 2
	if !includedSourceMap && len(result.OutputFiles) != 1 {
		panic(fmt.Sprintf("Expected exactly one Bundle OutputFile and optionally one SourceMap, got %d", len(result.OutputFiles)))
	}
	bundleIdx := 0
	if includedSourceMap {
		bundleIdx = 1
	}

	outputFiles := "["
	outputFiles += fmt.Sprintf(`
    { 
      "path": "<%s>",
      "contents": "%v"
    }`, result.OutputFiles[bundleIdx].Path, hex.EncodeToString(result.OutputFiles[bundleIdx].Contents))
	if includedSourceMap {
		sourcemapIdx := 0
		outputFiles += fmt.Sprintf(`
    ,
    { 
      "path": "<%s>",
      "contents": "%v"
    }`, result.OutputFiles[sourcemapIdx].Path, hex.EncodeToString(result.OutputFiles[sourcemapIdx].Contents))
	}
	outputFiles += "\n  ]"
	return outputFiles
}

// NOTE: esbuild itself doesn't send JSON across the wire like this. Instead it sends binary
// data which it then decodes into an JS object.

/*
 *	interface OutputFile {
 *	  path: string;
 *	  contents: Uint8Array; // "text" as bytes (we actually send a 'hex' string)
 *	  text: string; // "contents" as text  (we don't include that as it transmits data duplicated in contents)
 *	}
 *  interface BuildResult {
 *	  warnings: Message[];
 *	  outputFiles?: OutputFile[]; // Only when "write: false"
 *    metafile?: Metafile;        // Only when "metafile: true"
 *	  rebuild?: BuildInvalidate; // Only when "incremental" is true (not implemented for now)
 *	}
 */

func resultToJSON(result api.BuildResult, write bool) string {
	json := "{\n"
	json += fmt.Sprintf(`  "warnings": %s`, warningsJSON(result))

	if !write {
		json += fmt.Sprintf(`,
  "outfiles": %s`,
			outputFilesToJSON(result))
		json += fmt.Sprintf(`,
  "metafile": { 
    "contents": "%v"
  }`, hex.EncodeToString([]byte(result.Metafile)))
	}
	json += "\n}"
	return json
}

func resultToFile(result api.BuildResult) error {
	bundle := result.OutputFiles[0].Contents
	return ioutil.WriteFile("/tmp/snapshot-bundle.js", bundle, 0644)
}
