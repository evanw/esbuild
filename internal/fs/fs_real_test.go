package fs

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestRealFsReadDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Skipf("got error when calling Getwd() : %s", err)
	}

	// tmpdir will hold the elements for this test
	var tmpdir string
	var (
		dirname        string // a regular directory
		filename       string // a file (not a directory)
		restrictedname string // a directory, with no access (read or write)
	)

	var fp goFilepath
	if CheckIfWindows() {
		fp.isWindows = true
		fp.pathSeparator = '\\'
	} else {
		fp.isWindows = false
		fp.pathSeparator = '/'
	}

	setup := func(t *testing.T) {
		t.Helper()

		var err error
		tmpdir, err = os.MkdirTemp("", "testRealFs")
		if err != nil {
			t.Fatalf("can't create temp directory: %s", err)
		}

		dirname = fp.join([]string{tmpdir, "testdir"})
		err = os.Mkdir(dirname, 0775)
		if err != nil {
			t.Fatalf("can't create directory: %s", err)
		}

		filename = fp.join([]string{tmpdir, "file.txt"})
		f, err := os.Create(filename)
		if err != nil {
			t.Fatalf("can't create file: %s", err)
		}
		f.Close()

		restrictedname = fp.join([]string{tmpdir, "restricted"})
		err = os.Mkdir(restrictedname, 0)
		if err != nil {
			t.Fatalf("can't create restricted directory: %s", err)
		}
	}
	teardown := func(t *testing.T) {
		if tmpdir != "" {
			os.RemoveAll(tmpdir)
		}
	}

	setup(t)
	defer teardown(t)

	testCases := []struct {
		name string
		path string

		hasErr   bool
		matchErr error
	}{
		{
			name: "regular directory",
			path: dirname,

			hasErr: false,
		},
		{
			name: "path does not exist",
			path: "no_directory_here",

			hasErr:   true,
			matchErr: syscall.ENOENT,
		},
		{
			name: "file",
			path: filename,

			hasErr:   true,
			matchErr: syscall.ENOTDIR,
		},
		{
			name: "no access",
			path: restrictedname,

			hasErr:   !fp.isWindows, // seen on github.com/evanw/esbuild repo : EACCES isn't triggered on Windows
			matchErr: syscall.EACCES,
		},
	}

	fsys, err := RealFS(RealFSOptions{
		AbsWorkingDir: wd,
		DoNotCache:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testCases {
		_, canonical, err := fsys.ReadDirectory(tc.path)

		gotErr := err != nil
		if tc.hasErr != gotErr {
			if tc.hasErr {
				t.Fatalf("%s: expected an error, got none", tc.name)
			} else {
				t.Fatalf("%s: expected no error, got '%s'", tc.name, err)
			}
		}

		gotCanonicalErr := canonical != nil
		if tc.hasErr != gotCanonicalErr {
			if tc.hasErr {
				t.Fatalf("%s: expected an error, got none", tc.name)
			} else {
				t.Fatalf("%s: expected no error, got '%s'", tc.name, canonical)
			}
		}

		if tc.hasErr && tc.matchErr != nil {
			if !errors.Is(canonical, tc.matchErr) {
				t.Fatalf("%s: canonical error does not match expected error\n\texpected: %T '%s'\n\tgot %T '%s'", tc.name, tc.matchErr, tc.matchErr, canonical, canonical)
			}
		}
	}
}
