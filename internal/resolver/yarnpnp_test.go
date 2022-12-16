package resolver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

type pnpTestExpectation struct {
	Manifest interface{}
	Tests    []pnpTest
}

type pnpTest struct {
	It       string
	Imported string
	Importer string
	Expected string
}

func TestYarnPnP(t *testing.T) {
	t.Helper()
	contents, err := ioutil.ReadFile("testExpectations.json")
	if err != nil {
		t.Fatalf("Failed to read testExpectations.json: %s", err.Error())
	}

	var expectations []pnpTestExpectation
	err = json.Unmarshal(contents, &expectations)
	if err != nil {
		t.Fatalf("Failed to parse testExpectations.json: %s", err.Error())
	}

	for i, expectation := range expectations {
		path := fmt.Sprintf("testExpectations[%d].manifest", i)
		contents, err := json.Marshal(expectation.Manifest)
		if err != nil {
			t.Fatalf("Failed to generate JSON: %s", err.Error())
		}

		source := logger.Source{
			KeyPath:    logger.Path{Text: path},
			PrettyPath: path,
			Contents:   string(contents),
		}
		tempLog := logger.NewDeferLog(logger.DeferLogAll, nil)
		expr, ok := js_parser.ParseJSON(tempLog, source, js_parser.JSONOptions{})
		if !ok {
			t.Fatalf("Failed to re-parse JSON: %s", path)
		}

		msgs := tempLog.Done()
		if len(msgs) != 0 {
			t.Fatalf("Log not empty after re-parsing JSON: %s", path)
		}

		manifest := compileYarnPnPData(path, "/path/to/project/", expr, source)

		for _, current := range expectation.Tests {
			func(current pnpTest) {
				t.Run(current.It, func(t *testing.T) {
					fs := fs.MockFS(nil, fs.MockUnix)
					r := resolverQuery{Resolver: NewResolver(fs, logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil), nil, config.Options{})}
					result := r.resolveToUnqualified(current.Imported, current.Importer, manifest)

					var observed string
					switch result.status {
					case pnpSuccess:
						observed = fs.Join(result.pkgDirPath, result.pkgSubpath)
					case pnpSkipped:
						observed = current.Imported
					default:
						observed = "error!"
					}

					// If a we aren't going through PnP, then we should just run the
					// normal node module resolution rules instead of throwing an error.
					// However, this test requires us to throw an error, which seems
					// incorrect. So we change the expected value of the test instead.
					expected := current.Expected
					if current.It == `shouldn't go through PnP when trying to resolve dependencies from packages covered by ignorePatternData` {
						expected = current.Imported
					} else if observed != "error!" && !strings.HasSuffix(observed, "/") {
						// This is important for matching Yarn PnP's expectations in tests,
						// but it's important for esbuild that the slash isn't present.
						// Otherwise esbuild's implementation of node module resolution
						// (which runs after Yarn PnP resolution) will fail. Specifically
						// "foo/" will look for "foo/foo.js" instead of "foo/index.js".
						observed += "/"
					}

					test.AssertEqualWithDiff(t, observed, expected)
				})
			}(current)
		}
	}
}
