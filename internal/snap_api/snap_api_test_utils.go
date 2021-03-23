package snap_api

import (
	"fmt"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/kylelemons/godebug/diff"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/internal/fs"
)

const NO_BUNDLE_GENERATED string = "<no bundle generated>"

type built struct {
	files                map[string]string
	entryPoints          []string
	shouldReplaceRequire api.ShouldReplaceRequirePredicate
	shouldRewriteModule  api.ShouldRewriteModulePredicate
}

type buildResult struct {
	files  map[string]string
	bundle string
}

type suite struct {
	name string
}

func trimFirstLine(s string) string {
	var content string
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			content += line + "\n"
		}
	}
	return content
}

const ProjectBaseDir = "/dev"

func replaceAll(string) bool { return true }

func assertEqual(t *testing.T, key string, a interface{}, b interface{}) {
	t.Helper()
	if a != b {
		stringA := fmt.Sprintf("%v", a)
		stringB := fmt.Sprintf("%v", b)
		if strings.Contains(stringA, "\n") {
			t.Fatal(diff.Diff(stringB, stringA))
		} else {
			t.Fatalf("%s\n\n%s != %s", key, a, b)
		}
	}
}

func extractBuildResult(bundle string) buildResult {
	newFileSectionRx := regexp.MustCompile(`^\/\/ (.+)$`)
	lines := strings.Split(bundle, "\n")
	result := buildResult{
		bundle: bundle,
		files:  make(map[string]string),
	}
	var currentFile string
	var currentContent string
	for _, line := range lines {
		m := newFileSectionRx.FindStringSubmatch(line)
		if len(m) > 0 {
			if currentFile != "" {
				result.files[currentFile] = currentContent
			}
			currentFile = m[1]
			currentContent = ""
		} else {
			if currentFile != "" && line != "" {
				currentContent += line + "\n"
			}
		}
	}

	if currentFile != "" {
		result.files[currentFile] = currentContent
	}
	return result
}

func printBuildResult(result buildResult) {
	fmt.Println("buildResult{\n    files: map[string]string{")
	for k, v := range result.files {
		fmt.Printf("        `%s`: `\n%s`,\n", k, v)
	}
	fmt.Println("    }")
	fmt.Println("}")
	fmt.Printf("\n----------------\n%s\n----------------\n", result.bundle)
}

func verifyBuildResult(t *testing.T, result buildResult, expected buildResult) {
	for k, v := range expected.files {
		act := result.files[strings.TrimLeft(k, "/")]
		exp := trimFirstLine(v)
		assertEqual(t, k, act, exp)
	}
}

func (s *suite) build(args built) buildResult {
	fs := fs.MockFS(args.files)
	shouldReplaceRequire := args.shouldReplaceRequire
	if shouldReplaceRequire == nil {
		shouldReplaceRequire = replaceAll
	}
	result := api.Build(api.BuildOptions{
		LogLevel:    api.LogLevelInfo,
		Target:      api.ES2020,
		Bundle:      true,
		Outfile:     "/out.js",
		EntryPoints: args.entryPoints,
		Platform:    api.PlatformNode,
		Engines: []api.Engine{
			{Name: api.EngineNode, Version: "12.4"},
		},
		Format: api.FormatCommonJS,

		Snapshot: &api.SnapshotOptions{
			CreateSnapshot:       true,
			ShouldReplaceRequire: shouldReplaceRequire,
			ShouldRewriteModule:  args.shouldRewriteModule,
			AbsBasedir:           ProjectBaseDir,
		},
		FS: fs,
	})
	if len(result.OutputFiles) > 0 {
		return extractBuildResult(string(result.OutputFiles[0].Contents))
	} else {
		return buildResult{
			files:  map[string]string{},
			bundle: NO_BUNDLE_GENERATED,
		}
	}
}

func (s *suite) debugBuild(t *testing.T, args built) {
	t.Helper()
	t.Run("", func(t *testing.T) {
		t.Helper()

		result := s.build(args)
		printBuildResult(result)
	})
}

func (s *suite) expectBuild(t *testing.T, args built, expected buildResult) {
	t.Helper()
	t.Run("", func(t *testing.T) {
		t.Helper()
		result := s.build(args)
		verifyBuildResult(t, result, expected)
	})
}
