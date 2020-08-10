package bundler

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logging"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/kylelemons/godebug/diff"
)

func es(version int) compat.Feature {
	return compat.UnsupportedFeatures(map[compat.Engine][]int{
		compat.ES: {version},
	})
}

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		stringA := fmt.Sprintf("%v", a)
		stringB := fmt.Sprintf("%v", b)
		if strings.Contains(stringA, "\n") {
			t.Fatal(diff.Diff(stringB, stringA))
		} else {
			t.Fatalf("%s != %s", a, b)
		}
	}
}

func assertLog(t *testing.T, msgs []logging.Msg, expected string) {
	text := ""
	for _, msg := range msgs {
		text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
	}
	assertEqual(t, text, expected)
}

func hasErrors(msgs []logging.Msg) bool {
	for _, msg := range msgs {
		if msg.Kind == logging.Error {
			return true
		}
	}
	return false
}

type bundled struct {
	files              map[string]string
	entryPaths         []string
	expected           map[string]string
	expectedScanLog    string
	expectedCompileLog string
	options            config.Options
}

type suite struct {
	name string
}

func (s *suite) expectBundled(t *testing.T, args bundled) {
	t.Run("", func(t *testing.T) {
		fs := fs.MockFS(args.files)
		args.options.ExtensionOrder = []string{".tsx", ".ts", ".jsx", ".js", ".json"}
		if args.options.AbsOutputFile != "" {
			args.options.AbsOutputDir = path.Dir(args.options.AbsOutputFile)
		}
		log := logging.NewDeferLog()
		resolver := resolver.NewResolver(fs, log, args.options)
		bundle := ScanBundle(log, fs, resolver, args.entryPaths, args.options)
		msgs := log.Done()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if hasErrors(msgs) {
			return
		}

		log = logging.NewDeferLog()
		args.options.OmitRuntimeForTests = true
		results := bundle.Compile(log, args.options)
		msgs = log.Done()
		assertLog(t, msgs, args.expectedCompileLog)

		// Stop now if there were any errors during the compile
		if hasErrors(msgs) {
			return
		}

		// Don't include source maps in results since they are just noise. Source
		// map validity is tested separately in a test that uses Mozilla's source
		// map parsing library.
		resultsWithoutSourceMaps := []OutputFile{}
		for _, result := range results {
			if !strings.HasSuffix(result.AbsPath, ".map") {
				resultsWithoutSourceMaps = append(resultsWithoutSourceMaps, result)
			}
		}

		assertEqual(t, len(resultsWithoutSourceMaps), len(args.expected))
		for _, result := range resultsWithoutSourceMaps {
			file := args.expected[result.AbsPath]
			path := "[" + result.AbsPath + "]\n"
			assertEqual(t, path+string(result.Contents), path+file)
		}
	})
}
