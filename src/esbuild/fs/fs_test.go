package fs

import "testing"

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
	if len(src) != 2 || src["index.js"] != FileEntry || src["util.js"] != FileEntry {
		t.Fatalf("Incorrect contents for /src: %v", src)
	}

	// Test the top-level directory
	slash := fs.ReadDirectory("/")
	if slash == nil {
		t.Fatal("Expected to find /")
	}
	if len(slash) != 3 || slash["src"] != DirEntry || slash["README.md"] != FileEntry || slash["package.json"] != FileEntry {
		t.Fatalf("Incorrect contents for /: %v", slash)
	}
}
