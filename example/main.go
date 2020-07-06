package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evanw/esbuild/internal/fs"
	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{os.Args[1]},
		Format:      api.FormatESModule,
		Bundle:      true,
		Loaders:     map[string]api.Loader{".svelte": api.LoaderJS},
		FS:          &SvelteFS{fs.RealFS()},
	})
	for _, warn := range result.Warnings {
		fmt.Println("[WARN] ", warn.Text)
	}
	for _, err := range result.Errors {
		fmt.Println("[ERROR] ", err.Text)
	}
	for _, file := range result.OutputFiles {
		fmt.Println(string(file.Contents))
	}
}

// SvelteFS filesystem
//
// The idea here is to wrap the existing filesytem
// in a filesystem that transforms.
type SvelteFS struct {
	fs.FS
}

var _ fs.FS = (*SvelteFS)(nil)

// ReadFile is transforms any .svelte file
func (fs *SvelteFS) ReadFile(path string) (string, bool) {
	if filepath.Ext(path) == ".svelte" {
		code, ok := fs.FS.ReadFile(path)
		if !ok {
			return "", ok
		}
		// executes ./demo/compile.js
		// could be amortized by running it when initializing SvelteFS
		cmd := exec.Command("node", filepath.Join("example", "compile.js"))
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = bytes.NewBufferString(code)
		err := cmd.Run()
		if err != nil {
			return "", false
		}
		// fmt.Println(string(stdout.String()))
		return stdout.String(), true
	}
	return fs.FS.ReadFile(path)
}
