package fs

import (
	"fmt"
	"testing"
)

func TestMockFSBasicUnix(t *testing.T) {
	fs := MockFS(map[string]string{
		"/README.md":    "// README.md",
		"/package.json": "// package.json",
		"/src/index.js": "// src/index.js",
		"/src/util.js":  "// src/util.js",
	}, MockUnix)

	// Test a missing file
	_, err, _ := fs.ReadFile("/missing.txt")
	if err == nil {
		t.Fatal("Unexpectedly found /missing.txt")
	}

	// Test an existing file
	readme, err, _ := fs.ReadFile("/README.md")
	if err != nil {
		t.Fatal("Expected to find /README.md")
	}
	if readme != "// README.md" {
		t.Fatalf("Incorrect contents for /README.md: %q", readme)
	}

	// Test an existing nested file
	index, err, _ := fs.ReadFile("/src/index.js")
	if err != nil {
		t.Fatal("Expected to find /src/index.js")
	}
	if index != "// src/index.js" {
		t.Fatalf("Incorrect contents for /src/index.js: %q", index)
	}

	// Test a missing directory
	_, err, _ = fs.ReadDirectory("/missing")
	if err == nil {
		t.Fatal("Unexpectedly found /missing")
	}

	// Test a nested directory
	src, err, _ := fs.ReadDirectory("/src")
	if err != nil {
		t.Fatal("Expected to find /src")
	}
	indexEntry, _ := src.Get("index.js")
	utilEntry, _ := src.Get("util.js")
	if len(src.data) != 2 ||
		indexEntry == nil || indexEntry.Kind(fs) != FileEntry ||
		utilEntry == nil || utilEntry.Kind(fs) != FileEntry {
		t.Fatalf("Incorrect contents for /src: %v", src)
	}

	// Test the top-level directory
	slash, err, _ := fs.ReadDirectory("/")
	if err != nil {
		t.Fatal("Expected to find /")
	}
	srcEntry, _ := slash.Get("src")
	readmeEntry, _ := slash.Get("README.md")
	packageEntry, _ := slash.Get("package.json")
	if len(slash.data) != 3 ||
		srcEntry == nil || srcEntry.Kind(fs) != DirEntry ||
		readmeEntry == nil || readmeEntry.Kind(fs) != FileEntry ||
		packageEntry == nil || packageEntry.Kind(fs) != FileEntry {
		t.Fatalf("Incorrect contents for /: %v", slash)
	}
}

func TestMockFSBasicWindows(t *testing.T) {
	fs := MockFS(map[string]string{
		"/README.md":    "// README.md",
		"/package.json": "// package.json",
		"/src/index.js": "// src/index.js",
		"/src/util.js":  "// src/util.js",
	}, MockWindows)

	// Test a missing file
	_, err, _ := fs.ReadFile("C:\\missing.txt")
	if err == nil {
		t.Fatal("Unexpectedly found C:\\missing.txt")
	}

	// Test an existing file
	readme, err, _ := fs.ReadFile("C:\\README.md")
	if err != nil {
		t.Fatal("Expected to find C:\\README.md")
	}
	if readme != "// README.md" {
		t.Fatalf("Incorrect contents for C:\\README.md: %q", readme)
	}

	// Test an existing nested file
	index, err, _ := fs.ReadFile("C:\\src\\index.js")
	if err != nil {
		t.Fatal("Expected to find C:\\src\\index.js")
	}
	if index != "// src/index.js" {
		t.Fatalf("Incorrect contents for C:\\src\\index.js: %q", index)
	}

	// Test a missing directory
	_, err, _ = fs.ReadDirectory("C:\\missing")
	if err == nil {
		t.Fatal("Unexpectedly found C:\\missing")
	}

	// Test a nested directory
	src, err, _ := fs.ReadDirectory("C:\\src")
	if err != nil {
		t.Fatal("Expected to find C:\\src")
	}
	indexEntry, _ := src.Get("index.js")
	utilEntry, _ := src.Get("util.js")
	if len(src.data) != 2 ||
		indexEntry == nil || indexEntry.Kind(fs) != FileEntry ||
		utilEntry == nil || utilEntry.Kind(fs) != FileEntry {
		t.Fatalf("Incorrect contents for C:\\src: %v", src)
	}

	// Test the top-level directory
	slash, err, _ := fs.ReadDirectory("C:\\")
	if err != nil {
		t.Fatal("Expected to find C:\\")
	}
	srcEntry, _ := slash.Get("src")
	readmeEntry, _ := slash.Get("README.md")
	packageEntry, _ := slash.Get("package.json")
	if len(slash.data) != 3 ||
		srcEntry == nil || srcEntry.Kind(fs) != DirEntry ||
		readmeEntry == nil || readmeEntry.Kind(fs) != FileEntry ||
		packageEntry == nil || packageEntry.Kind(fs) != FileEntry {
		t.Fatalf("Incorrect contents for C:\\: %v", slash)
	}
}

func TestMockFSRelUnix(t *testing.T) {
	fs := MockFS(map[string]string{}, MockUnix)

	expect := func(a string, b string, c string) {
		t.Helper()
		t.Run(fmt.Sprintf("Rel(%q, %q) == %q", a, b, c), func(t *testing.T) {
			t.Helper()
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

	expect("a/b", "a/c", "../c")
	expect("./a/b", "./a/c", "../c")
	expect(".", "./a/b", "a/b")
	expect(".", ".//a/b", "a/b")
	expect(".", "././a/b", "a/b")
	expect(".", "././/a/b", "a/b")
}

func TestMockFSRelWindows(t *testing.T) {
	fs := MockFS(map[string]string{}, MockWindows)

	expect := func(a string, b string, c string) {
		t.Helper()
		t.Run(fmt.Sprintf("Rel(%q, %q) == %q", a, b, c), func(t *testing.T) {
			t.Helper()
			rel, ok := fs.Rel(a, b)
			if !ok {
				t.Fatalf("!ok")
			}
			if rel != c {
				t.Fatalf("Expected %q, got %q", c, rel)
			}
		})
	}

	expect("C:\\a\\b", "C:\\a\\b", ".")
	expect("C:\\a\\b", "C:\\a\\b\\c", "c")
	expect("C:\\a\\b", "C:\\a\\b\\c\\d", "c\\d")
	expect("C:\\a\\b\\c", "C:\\a\\b", "..")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b", "..\\..")
	expect("C:\\a\\b\\c", "C:\\a\\b\\x", "..\\x")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b\\x", "..\\..\\x")
	expect("C:\\a\\b\\c", "C:\\a\\b\\x\\y", "..\\x\\y")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b\\x\\y", "..\\..\\x\\y")

	expect("a\\b", "a\\c", "..\\c")
	expect(".\\a\\b", ".\\a\\c", "..\\c")
	expect(".", ".\\a\\b", "a\\b")
	expect(".", ".\\\\a\\b", "a\\b")
	expect(".", ".\\.\\a\\b", "a\\b")
	expect(".", ".\\.\\\\a\\b", "a\\b")
}
