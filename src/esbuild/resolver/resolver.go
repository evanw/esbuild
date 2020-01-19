package resolver

import (
	"esbuild/ast"
	"esbuild/fs"
	"esbuild/lexer"
	"esbuild/logging"
	"esbuild/parser"
	"strings"
)

type Resolver interface {
	Resolve(sourcePath string, importPath string) (string, bool)
	Read(path string) (string, bool)
	FileExists(path string) bool
	PrettyPath(path string) string
}

type resolver struct {
	fs fs.FS
}

func NewResolver(fs fs.FS) Resolver {
	return &resolver{fs}
}

func (r *resolver) Resolve(sourcePath string, importPath string) (string, bool) {
	// This implements the module resolution algorithm from node.js, which is
	// described here: https://nodejs.org/api/modules.html#modules_all_together

	sourceDir := r.fs.Dir(sourcePath)
	startsWithSlash := strings.HasPrefix(importPath, "/")

	if startsWithSlash {
		sourceDir = "/"
	}

	if startsWithSlash || strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return r.loadAsFileOrDirectory(r.fs.Join(sourceDir, importPath))
	}

	absolute, ok := r.loadNodeModules(importPath, sourceDir)
	if ok {
		return absolute, true
	}

	return r.loadSelfReference(importPath, sourceDir)
}

func (r *resolver) Read(path string) (string, bool) {
	contents, ok := r.fs.ReadFile(path)
	return contents, ok
}

func (r *resolver) FileExists(path string) bool {
	entries := r.fs.ReadDirectory(r.fs.Dir(path))
	return entries != nil && entries[r.fs.Base(path)]
}

func (r *resolver) PrettyPath(path string) string {
	if rel, ok := r.fs.RelativeToCwd(path); ok {
		return rel
	}
	return path
}

////////////////////////////////////////////////////////////////////////////////

var extensionOrder = []string{".jsx", ".js", ".json"}

func (r *resolver) loadAsFile(path string) (string, bool) {
	if r.FileExists(path) {
		return path, true
	}

	for _, ext := range extensionOrder {
		extPath := path + ext
		if r.FileExists(extPath) {
			return extPath, true
		}
	}

	return "", false
}

func (r *resolver) loadAsIndex(path string) (string, bool) {
	jsPath := r.fs.Join(path, "index.js")
	if r.FileExists(jsPath) {
		return jsPath, true
	}
	jsonPath := r.fs.Join(path, "index.json")
	if r.FileExists(jsonPath) {
		return jsonPath, true
	}
	return "", false
}

func (r *resolver) parseMainFromJson(path string) (result string, found bool) {
	// Read the file
	contents, ok := r.Read(path)
	if ok {
		// Parse the JSON
		log, _ := logging.NewDeferLog()
		source := logging.Source{Contents: contents}
		parsed, ok := parser.ParseJson(log, source)
		if ok {
			// Check for a top-level object
			if obj, ok := parsed.Data.(*ast.EObject); ok {
				for _, prop := range obj.Properties {
					// Find the key that says "main"
					if key, ok := prop.Key.Data.(*ast.EString); ok && len(key.Value) == 4 && lexer.UTF16ToString(key.Value) == "main" {
						if value, ok := prop.Value.Data.(*ast.EString); ok {
							// Return the value for this key if it's a string
							result = lexer.UTF16ToString(value.Value)
							found = true
						}
					}
				}
			}
		}
	}
	return
}

func (r *resolver) loadAsFileOrDirectory(path string) (string, bool) {
	absolute, ok := r.loadAsFile(path)
	if ok {
		return absolute, true
	}

	packageJson := r.fs.Join(path, "package.json")
	if r.FileExists(packageJson) {
		if main, ok := r.parseMainFromJson(packageJson); ok {
			mainPath := r.fs.Join(path, main)

			absolute, ok := r.loadAsFile(mainPath)
			if ok {
				return absolute, true
			}

			absolute, ok = r.loadAsIndex(mainPath)
			if ok {
				return absolute, true
			}
		}
	}

	return r.loadAsIndex(path)
}

func (r *resolver) loadNodeModules(path string, start string) (string, bool) {
	for {
		// Skip "node_modules" folders
		if r.fs.Base(start) != "node_modules" {
			absolute, ok := r.loadAsFileOrDirectory(r.fs.Join(start, "node_modules", path))
			if ok {
				return absolute, true
			}
		}

		// Go to the parent directory, stopping at the file system root
		dir := r.fs.Dir(start)
		if start == dir {
			break
		}
		start = dir
	}

	return "", false
}

func (r *resolver) loadSelfReference(path string, start string) (string, bool) {
	// Note: this is modified from how node's resolution algorithm works. Instead
	// of just checking the closest enclosing directory with a "package.json"
	// file, it checks all enclosing directories.

	for {
		// Check this directory
		absolute, ok := r.loadAsFileOrDirectory(r.fs.Join(start, path))
		if ok {
			return absolute, true
		}

		// Go to the parent directory, stopping at the file system root
		dir := r.fs.Dir(start)
		if start == dir {
			break
		}
		start = dir
	}

	return "", false
}
