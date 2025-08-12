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
	}, MockUnix, "/")

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
		"C:\\README.md":       "// README.md",
		"C:\\package.json":    "// package.json",
		"C:\\src\\index.js":   "// src/index.js",
		"C:\\src\\util.js":    "// src/util.js",
		"D:\\other\\file.txt": "// other/file.txt",
	}, MockWindows, "C:\\")

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

	// Test an existing nested file on another drive
	file, err, _ := fs.ReadFile("D:\\other\\file.txt")
	if err != nil {
		t.Fatal("Expected to find D:\\other\\file.txt")
	}
	if file != "// other/file.txt" {
		t.Fatalf("Incorrect contents for D:\\other/file.txt: %q", file)
	}

	// Should not find a file on another drive
	_, err, _ = fs.ReadFile("C:\\other\\file.txt")
	if err == nil {
		t.Fatal("Unexpectedly found C:\\other\\file.txt")
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

	// Test a nested directory on another drive
	other, err, _ := fs.ReadDirectory("D:\\other")
	if err != nil {
		t.Fatal("Expected to find D:\\other")
	}
	fileEntry, _ := other.Get("file.txt")
	if len(other.data) != 1 ||
		fileEntry == nil || fileEntry.Kind(fs) != FileEntry {
		t.Fatalf("Incorrect contents for D:\\other: %v", other)
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
	fs := MockFS(map[string]string{}, MockUnix, "/")

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
	fs := MockFS(map[string]string{}, MockWindows, "C:\\")

	expect := func(a string, b string, works bool, c string) {
		t.Helper()
		t.Run(fmt.Sprintf("Rel(%q, %q) == %q", a, b, c), func(t *testing.T) {
			t.Helper()
			rel, ok := fs.Rel(a, b)
			if works {
				if !ok {
					t.Fatalf("!ok")
				}
				if rel != c {
					t.Fatalf("Expected %q, got %q", c, rel)
				}
			} else {
				if ok {
					t.Fatalf("ok")
				}
			}
		})
	}

	expect("C:\\a\\b", "C:\\a\\b", true, ".")
	expect("C:\\a\\b", "C:\\a\\b\\c", true, "c")
	expect("C:\\a\\b", "C:\\a\\b\\c\\d", true, "c\\d")
	expect("C:\\a\\b\\c", "C:\\a\\b", true, "..")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b", true, "..\\..")
	expect("C:\\a\\b\\c", "C:\\a\\b\\x", true, "..\\x")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b\\x", true, "..\\..\\x")
	expect("C:\\a\\b\\c", "C:\\a\\b\\x\\y", true, "..\\x\\y")
	expect("C:\\a\\b\\c\\d", "C:\\a\\b\\x\\y", true, "..\\..\\x\\y")

	expect("a\\b", "a\\c", true, "..\\c")
	expect(".\\a\\b", ".\\a\\c", true, "..\\c")
	expect(".", ".\\a\\b", true, "a\\b")
	expect(".", ".\\\\a\\b", true, "a\\b")
	expect(".", ".\\.\\a\\b", true, "a\\b")
	expect(".", ".\\.\\\\a\\b", true, "a\\b")

	expect("C:\\a\\b", "\\a\\b", true, ".")
	expect("\\a", "\\b", true, "..\\b")
	expect("C:\\a", "D:\\a", false, "")
}
