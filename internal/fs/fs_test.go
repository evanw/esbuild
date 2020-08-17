package fs

import (
	"fmt"
	"testing"
)

func TestBasic(t *testing.T) {
	fs := MockFS(map[string]string{
		"/README.md":    "// README.md",
		"/package.json": "// package.json",
		"/src/index.js": "// src/index.js",
		"/src/util.js":  "// src/util.js",
	})

	// Test a missing file
	_, ok := fs.ReadFile("/missing.txt")
	if ok {
		t.Fatal("Unexpectedly found /missing.txt")
	}

	// Test an existing file
	readme, ok := fs.ReadFile("/README.md")
	if !ok {
		t.Fatal("Expected to find /README.md")
	}
	if readme != "// README.md" {
		t.Fatalf("Incorrect contents for /README.md: %q", readme)
	}

	// Test an existing nested file
	index, ok := fs.ReadFile("/src/index.js")
	if !ok {
		t.Fatal("Expected to find /src/index.js")
	}
	if index != "// src/index.js" {
		t.Fatalf("Incorrect contents for /src/index.js: %q", index)
	}

	// Test a missing directory
	missing := fs.ReadDirectory("/missing")
	if missing != nil {
		t.Fatal("Unexpectedly found /missing")
	}

	// Test a nested directory
	src := fs.ReadDirectory("/src")
	if src == nil {
		t.Fatal("Expected to find /src")
	}
	if len(src) != 2 || src["index.js"].Kind() != FileEntry || src["util.js"].Kind() != FileEntry {
		t.Fatalf("Incorrect contents for /src: %v", src)
	}

	// Test the top-level directory
	slash := fs.ReadDirectory("/")
	if slash == nil {
		t.Fatal("Expected to find /")
	}
	if len(slash) != 3 || slash["src"].Kind() != DirEntry || slash["README.md"].Kind() != FileEntry || slash["package.json"].Kind() != FileEntry {
		t.Fatalf("Incorrect contents for /: %v", slash)
	}
}

func TestRel(t *testing.T) {
	fs := MockFS(map[string]string{})

	expect := func(a string, b string, c string) {
		t.Run(fmt.Sprintf("Rel(%q, %q) == %q", a, b, c), func(t *testing.T) {
			rel, ok := fs.Rel(a, b)
			if !ok {
				t.Fatalf("!ok")
			}
			if rel != c {
				t.Fatalf("Expected %q, got %q", c, rel)
			}
		})
	}

	expect("/a/b", "/a/b", ".")
	expect("/a/b", "/a/b/c", "c")
	expect("/a/b", "/a/b/c/d", "c/d")
	expect("/a/b/c", "/a/b", "..")
	expect("/a/b/c/d", "/a/b", "../..")
	expect("/a/b/c", "/a/b/x", "../x")
	expect("/a/b/c/d", "/a/b/x", "../../x")
	expect("/a/b/c", "/a/b/x/y", "../x/y")
	expect("/a/b/c/d", "/a/b/x/y", "../../x/y")
}
