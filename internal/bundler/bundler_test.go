package bundler

// Bundling test results are stored in snapshot files, located in the
// "snapshots" directory. This allows test results to be updated easily without
// manually rewriting all of the expected values. To update the tests run
// "UPDATE_SNAPSHOTS=1 make test" and commit the updated values. Make sure to
// inspect the diff to ensure the expected values are valid.

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/evanw/esbuild/internal/cache"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/internal/test"
)

func es(version int) compat.JSFeature {
	return compat.UnsupportedJSFeatures(map[compat.Engine][]int{
		compat.ES: {version},
	})
}

func assertLog(t *testing.T, msgs []logger.Msg, expected string) {
	t.Helper()
	text := ""
	for _, msg := range msgs {
		text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
	}
	test.AssertEqualWithDiff(t, text, expected)
}

func hasErrors(msgs []logger.Msg) bool {
	for _, msg := range msgs {
		if msg.Kind == logger.Error {
			return true
		}
	}
	return false
}

type bundled struct {
	files              map[string]string
	entryPaths         []string
	entryPathsAdvanced []EntryPoint
	expectedScanLog    string
	expectedCompileLog string
	options            config.Options
	debugLogs          bool
}

type suite struct {
	expectedSnapshots  map[string]string
	generatedSnapshots map[string]string
	name               string
	path               string
	mutex              sync.Mutex
}

func (s *suite) expectBundled(t *testing.T, args bundled) {
	t.Helper()
	s.__expectBundledImpl(t, args, fs.MockUnix)
	s.__expectBundledImpl(t, args, fs.MockWindows)
}

func (s *suite) expectBundledUnix(t *testing.T, args bundled) {
	t.Helper()
	s.__expectBundledImpl(t, args, fs.MockUnix)
}

func (s *suite) expectBundledWindows(t *testing.T, args bundled) {
	t.Helper()
	s.__expectBundledImpl(t, args, fs.MockWindows)
}

// Don't call this directly. Call the helpers above instead.
func (s *suite) __expectBundledImpl(t *testing.T, args bundled, fsKind fs.MockKind) {
	t.Helper()

	testName := t.Name()
	subName := "Unix"
	if fsKind == fs.MockWindows {
		subName = "Windows"
	}

	t.Run(subName, func(t *testing.T) {
		t.Helper()

		// Prepare the options
		if args.options.ExtensionOrder == nil {
			args.options.ExtensionOrder = []string{".tsx", ".ts", ".jsx", ".js", ".css", ".json"}
		}
		if args.options.AbsOutputFile != "" {
			args.options.AbsOutputDir = path.Dir(args.options.AbsOutputFile)
		}
		if args.options.Mode == config.ModeBundle || (args.options.Mode == config.ModeConvertFormat && args.options.OutputFormat == config.FormatIIFE) {
			// Apply this default to all tests since it was not configurable when the tests were written
			args.options.TreeShaking = true
		}
		logKind := logger.DeferLogNoVerboseOrDebug
		if args.debugLogs {
			logKind = logger.DeferLogAll
		}
		entryPoints := make([]EntryPoint, 0, len(args.entryPaths)+len(args.entryPathsAdvanced))
		for _, path := range args.entryPaths {
			entryPoints = append(entryPoints, EntryPoint{InputPath: path})
		}
		entryPoints = append(entryPoints, args.entryPathsAdvanced...)

		// Handle conversion to Windows-style paths
		if fsKind == fs.MockWindows {
			for i, entry := range entryPoints {
				entry.InputPath = unix2win(entry.InputPath)
				entryPoints[i] = entry
			}

			for i, absPath := range args.options.InjectAbsPaths {
				args.options.InjectAbsPaths[i] = unix2win(absPath)
			}

			replace := make(map[string]bool)
			for k, v := range args.options.ExternalSettings.PostResolve.Exact {
				replace[unix2win(k)] = v
			}
			args.options.ExternalSettings.PostResolve.Exact = replace

			args.options.AbsOutputFile = unix2win(args.options.AbsOutputFile)
			args.options.AbsOutputDir = unix2win(args.options.AbsOutputDir)
			args.options.TsConfigOverride = unix2win(args.options.TsConfigOverride)
		}

		// Run the bundler
		log := logger.NewDeferLog(logKind, nil)
		caches := cache.MakeCacheSet()
		mockFS := fs.MockFS(args.files, fsKind)
		resolver := resolver.NewResolver(mockFS, log, caches, args.options)
		bundle := ScanBundle(log, mockFS, resolver, caches, entryPoints, args.options, nil)
		msgs := log.Done()
		assertLog(t, msgs, args.expectedScanLog)

		// Stop now if there were any errors during the scan
		if hasErrors(msgs) {
			return
		}

		log = logger.NewDeferLog(logKind, nil)
		args.options.OmitRuntimeForTests = true
		results, _ := bundle.Compile(log, args.options, nil, nil)
		msgs = log.Done()
		assertLog(t, msgs, args.expectedCompileLog)

		// Stop now if there were any errors during the compile
		if hasErrors(msgs) {
			return
		}

		// Don't include source maps in results since they are just noise. Source
		// map validity is tested separately in a test that uses Mozilla's source
		// map parsing library.
		generated := ""
		for _, result := range results {
			if !strings.HasSuffix(result.AbsPath, ".map") {
				if generated != "" {
					generated += "\n"
				}
				if fsKind == fs.MockWindows {
					result.AbsPath = win2unix(result.AbsPath)
				}
				generated += fmt.Sprintf("---------- %s ----------\n%s", result.AbsPath, string(result.Contents))
			}
		}
		s.compareSnapshot(t, testName, generated)
	})
}

const snapshotsDir = "snapshots"
const snapshotSplitter = "\n================================================================================\n"

var globalTestMutex sync.Mutex
var globalSuites map[*suite]bool
var globalUpdateSnapshots bool

func (s *suite) compareSnapshot(t *testing.T, testName string, generated string) {
	t.Helper()
	// Initialize the test suite during the first test
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.path == "" {
		s.path = snapshotsDir + "/snapshots_" + s.name + ".txt"
		s.generatedSnapshots = make(map[string]string)
		s.expectedSnapshots = make(map[string]string)
		if contents, err := ioutil.ReadFile(s.path); err == nil {
			// Replacing CRLF with LF is necessary to fix tests in GitHub actions,
			// which for some reason check out the source code in CLRF mode
			for _, part := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), snapshotSplitter) {
				if newline := strings.IndexByte(part, '\n'); newline != -1 {
					key := part[:newline]
					value := part[newline+1:]
					s.expectedSnapshots[key] = value
				} else {
					s.expectedSnapshots[part] = ""
				}
			}
		}
		globalTestMutex.Lock()
		defer globalTestMutex.Unlock()
		if globalSuites == nil {
			globalSuites = make(map[*suite]bool)
		}
		globalSuites[s] = true
		_, globalUpdateSnapshots = os.LookupEnv("UPDATE_SNAPSHOTS")
	}

	// Check against the stored snapshot if present
	s.generatedSnapshots[testName] = generated
	if !globalUpdateSnapshots {
		if expected, ok := s.expectedSnapshots[testName]; ok {
			test.AssertEqualWithDiff(t, generated, expected)
		} else {
			t.Fatalf("No snapshot saved for %s\n%s%s%s",
				testName,
				logger.TerminalColors.Green,
				generated,
				logger.TerminalColors.Reset,
			)
		}
	}
}

func (s *suite) updateSnapshots() {
	os.Mkdir(snapshotsDir, 0755)
	keys := make([]string, 0, len(s.generatedSnapshots))
	for key := range s.generatedSnapshots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	contents := ""
	for i, key := range keys {
		if i > 0 {
			contents += snapshotSplitter
		}
		contents += fmt.Sprintf("%s\n%s", key, s.generatedSnapshots[key])
	}
	if err := ioutil.WriteFile(s.path, []byte(contents), 0644); err != nil {
		panic(err)
	}
}

func (s *suite) validateSnapshots() bool {
	isValid := true
	for key := range s.expectedSnapshots {
		if _, ok := s.generatedSnapshots[key]; !ok {
			if isValid {
				fmt.Printf("%s\n", s.path)
			}
			fmt.Printf("    No test found for snapshot %s\n", key)
			isValid = false
		}
	}
	return isValid
}

func TestMain(m *testing.M) {
	code := m.Run()
	if globalSuites != nil {
		if globalUpdateSnapshots {
			for s := range globalSuites {
				s.updateSnapshots()
			}
		} else {
			for s := range globalSuites {
				if !s.validateSnapshots() {
					code = 1
				}
			}
		}
	}
	os.Exit(code)
}

func win2unix(p string) string {
	if strings.HasPrefix(p, "C:\\") {
		p = p[2:]
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return p
}

func unix2win(p string) string {
	p = strings.ReplaceAll(p, "/", "\\")
	if strings.HasPrefix(p, "\\") {
		p = "C:" + p
	}
	return p
}
