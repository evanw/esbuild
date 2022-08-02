// Code in this file has been forked from the "filepath" module in the Go
// source code to work around bugs with the WebAssembly build target. More
// information about why here: https://github.com/golang/go/issues/43768.

////////////////////////////////////////////////////////////////////////////////

// Copyright (c) 2009 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package fs

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

type goFilepath struct {
	cwd           string
	isWindows     bool
	pathSeparator byte
}

func isSlash(c uint8) bool {
	return c == '\\' || c == '/'
}

// reservedNames lists reserved Windows names. Search for PRN in
// https://docs.microsoft.com/en-us/windows/desktop/fileio/naming-a-file
// for details.
var reservedNames = []string{
	"CON", "PRN", "AUX", "NUL",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}

// isReservedName returns true, if path is Windows reserved name.
// See reservedNames for the full list.
func isReservedName(path string) bool {
	if len(path) == 0 {
		return false
	}
	for _, reserved := range reservedNames {
		if strings.EqualFold(path, reserved) {
			return true
		}
	}
	return false
}

// IsAbs reports whether the path is absolute.
func (fp goFilepath) isAbs(path string) bool {
	if !fp.isWindows {
		return strings.HasPrefix(path, "/")
	}
	if isReservedName(path) {
		return true
	}
	l := fp.volumeNameLen(path)
	if l == 0 {
		return false
	}
	path = path[l:]
	if path == "" {
		return false
	}
	return isSlash(path[0])
}

// Abs returns an absolute representation of path.
// If the path is not absolute it will be joined with the current
// working directory to turn it into an absolute path. The absolute
// path name for a given file is not guaranteed to be unique.
// Abs calls Clean on the result.
func (fp goFilepath) abs(path string) (string, error) {
	if fp.isAbs(path) {
		return fp.clean(path), nil
	}
	return fp.join([]string{fp.cwd, path}), nil
}

// IsPathSeparator reports whether c is a directory separator character.
func (fp goFilepath) isPathSeparator(c uint8) bool {
	return c == '/' || (fp.isWindows && c == '\\')
}

// volumeNameLen returns length of the leading volume name on Windows.
// It returns 0 elsewhere.
func (fp goFilepath) volumeNameLen(path string) int {
	if !fp.isWindows {
		return 0
	}
	if len(path) < 2 {
		return 0
	}
	// with drive letter
	c := path[0]
	if path[1] == ':' && ('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z') {
		return 2
	}
	// is it UNC? https://msdn.microsoft.com/en-us/library/windows/desktop/aa365247(v=vs.85).aspx
	if l := len(path); l >= 5 && isSlash(path[0]) && isSlash(path[1]) &&
		!isSlash(path[2]) && path[2] != '.' {
		// first, leading `\\` and next shouldn't be `\`. its server name.
		for n := 3; n < l-1; n++ {
			// second, next '\' shouldn't be repeated.
			if isSlash(path[n]) {
				n++
				// third, following something characters. its share name.
				if !isSlash(path[n]) {
					if path[n] == '.' {
						break
					}
					for ; n < l; n++ {
						if isSlash(path[n]) {
							break
						}
					}
					return n
				}
				break
			}
		}
	}
	return 0
}

// EvalSymlinks returns the path name after the evaluation of any symbolic
// links.
// If path is relative the result will be relative to the current directory,
// unless one of the components is an absolute symbolic link.
// EvalSymlinks calls Clean on the result.
func (fp goFilepath) evalSymlinks(path string) (string, error) {
	volLen := fp.volumeNameLen(path)
	pathSeparator := string(fp.pathSeparator)

	if volLen < len(path) && fp.isPathSeparator(path[volLen]) {
		volLen++
	}
	vol := path[:volLen]
	dest := vol
	linksWalked := 0
	for start, end := volLen, volLen; start < len(path); start = end {
		for start < len(path) && fp.isPathSeparator(path[start]) {
			start++
		}
		end = start
		for end < len(path) && !fp.isPathSeparator(path[end]) {
			end++
		}

		// On Windows, "." can be a symlink.
		// We look it up, and use the value if it is absolute.
		// If not, we just return ".".
		isWindowsDot := fp.isWindows && path[fp.volumeNameLen(path):] == "."

		// The next path component is in path[start:end].
		if end == start {
			// No more path components.
			break
		} else if path[start:end] == "." && !isWindowsDot {
			// Ignore path component ".".
			continue
		} else if path[start:end] == ".." {
			// Back up to previous component if possible.
			// Note that volLen includes any leading slash.

			// Set r to the index of the last slash in dest,
			// after the volume.
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if fp.isPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen || dest[r+1:] == ".." {
				// Either path has no slashes
				// (it's empty or just "C:")
				// or it ends in a ".." we had to keep.
				// Either way, keep this "..".
				if len(dest) > volLen {
					dest += pathSeparator
				}
				dest += ".."
			} else {
				// Discard everything since the last slash.
				dest = dest[:r]
			}
			continue
		}

		// Ordinary path component. Add it to result.

		if len(dest) > fp.volumeNameLen(dest) && !fp.isPathSeparator(dest[len(dest)-1]) {
			dest += pathSeparator
		}

		dest += path[start:end]

		// Resolve symlink.

		fi, err := os.Lstat(dest)
		if err != nil {
			return "", err
		}

		if fi.Mode()&os.ModeSymlink == 0 {
			if !fi.Mode().IsDir() && end < len(path) {
				return "", syscall.ENOTDIR
			}
			continue
		}

		// Found symlink.

		linksWalked++
		if linksWalked > 255 {
			return "", errors.New("EvalSymlinks: too many links")
		}

		link, err := os.Readlink(dest)
		if err != nil {
			return "", err
		}

		if isWindowsDot && !fp.isAbs(link) {
			// On Windows, if "." is a relative symlink,
			// just return ".".
			break
		}

		path = link + path[end:]

		v := fp.volumeNameLen(link)
		if v > 0 {
			// Symlink to drive name is an absolute path.
			if v < len(link) && fp.isPathSeparator(link[v]) {
				v++
			}
			vol = link[:v]
			dest = vol
			end = len(vol)
		} else if len(link) > 0 && fp.isPathSeparator(link[0]) {
			// Symlink to absolute path.
			dest = link[:1]
			end = 1
		} else {
			// Symlink to relative path; replace last
			// path component in dest.
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if fp.isPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen {
				dest = vol
			} else {
				dest = dest[:r]
			}
			end = 0
		}
	}
	return fp.clean(dest), nil
}

// A lazybuf is a lazily constructed path buffer.
// It supports append, reading previously appended bytes,
// and retrieving the final string. It does not allocate a buffer
// to hold the output until that output diverges from s.
type lazybuf struct {
	path       string
	volAndPath string
	buf        []byte
	w          int
	volLen     int
}

func (b *lazybuf) index(i int) byte {
	if b.buf != nil {
		return b.buf[i]
	}
	return b.path[i]
}

func (b *lazybuf) append(c byte) {
	if b.buf == nil {
		if b.w < len(b.path) && b.path[b.w] == c {
			b.w++
			return
		}
		b.buf = make([]byte, len(b.path))
		copy(b.buf, b.path[:b.w])
	}
	b.buf[b.w] = c
	b.w++
}

func (b *lazybuf) string() string {
	if b.buf == nil {
		return b.volAndPath[:b.volLen+b.w]
	}
	return b.volAndPath[:b.volLen] + string(b.buf[:b.w])
}

// FromSlash returns the result of replacing each slash ('/') character
// in path with a separator character. Multiple slashes are replaced
// by multiple separators.
func (fp goFilepath) fromSlash(path string) string {
	if !fp.isWindows {
		return path
	}
	return strings.ReplaceAll(path, "/", "\\")
}

// Clean returns the shortest path name equivalent to path
// by purely lexical processing. It applies the following rules
// iteratively until no further processing can be done:
//
//  1. Replace multiple Separator elements with a single one.
//  2. Eliminate each . path name element (the current directory).
//  3. Eliminate each inner .. path name element (the parent directory)
//     along with the non-.. element that precedes it.
//  4. Eliminate .. elements that begin a rooted path:
//     that is, replace "/.." by "/" at the beginning of a path,
//     assuming Separator is '/'.
//
// The returned path ends in a slash only if it represents a root directory,
// such as "/" on Unix or `C:\` on Windows.
//
// Finally, any occurrences of slash are replaced by Separator.
//
// If the result of this process is an empty string, Clean
// returns the string ".".
//
// See also Rob Pike, "Lexical File Names in Plan 9 or
// Getting Dot-Dot Right,"
// https://9p.io/sys/doc/lexnames.html
func (fp goFilepath) clean(path string) string {
	originalPath := path
	volLen := fp.volumeNameLen(path)
	path = path[volLen:]
	if path == "" {
		if volLen > 1 && originalPath[1] != ':' {
			// should be UNC
			return fp.fromSlash(originalPath)
		}
		return originalPath + "."
	}
	rooted := fp.isPathSeparator(path[0])

	// Invariants:
	//	reading from path; r is index of next byte to process.
	//	writing to buf; w is index of next byte to write.
	//	dotdot is index in buf where .. must stop, either because
	//		it is the leading slash or it is a leading ../../.. prefix.
	n := len(path)
	out := lazybuf{path: path, volAndPath: originalPath, volLen: volLen}
	r, dotdot := 0, 0
	if rooted {
		out.append(fp.pathSeparator)
		r, dotdot = 1, 1
	}

	for r < n {
		switch {
		case fp.isPathSeparator(path[r]):
			// empty path element
			r++
		case path[r] == '.' && (r+1 == n || fp.isPathSeparator(path[r+1])):
			// . element
			r++
		case path[r] == '.' && path[r+1] == '.' && (r+2 == n || fp.isPathSeparator(path[r+2])):
			// .. element: remove to last separator
			r += 2
			switch {
			case out.w > dotdot:
				// can backtrack
				out.w--
				for out.w > dotdot && !fp.isPathSeparator(out.index(out.w)) {
					out.w--
				}
			case !rooted:
				// cannot backtrack, but not rooted, so append .. element.
				if out.w > 0 {
					out.append(fp.pathSeparator)
				}
				out.append('.')
				out.append('.')
				dotdot = out.w
			}
		default:
			// real path element.
			// add slash if needed
			if rooted && out.w != 1 || !rooted && out.w != 0 {
				out.append(fp.pathSeparator)
			}
			// copy element
			for ; r < n && !fp.isPathSeparator(path[r]); r++ {
				out.append(path[r])
			}
		}
	}

	// Turn empty string into "."
	if out.w == 0 {
		out.append('.')
	}

	return fp.fromSlash(out.string())
}

// VolumeName returns leading volume name.
// Given "C:\foo\bar" it returns "C:" on Windows.
// Given "\\host\share\foo" it returns "\\host\share".
// On other platforms it returns "".
func (fp goFilepath) volumeName(path string) string {
	return path[:fp.volumeNameLen(path)]
}

// Base returns the last element of path.
// Trailing path separators are removed before extracting the last element.
// If the path is empty, Base returns ".".
// If the path consists entirely of separators, Base returns a single separator.
func (fp goFilepath) base(path string) string {
	if path == "" {
		return "."
	}
	// Strip trailing slashes.
	for len(path) > 0 && fp.isPathSeparator(path[len(path)-1]) {
		path = path[0 : len(path)-1]
	}
	// Throw away volume name
	path = path[len(fp.volumeName(path)):]
	// Find the last element
	i := len(path) - 1
	for i >= 0 && !fp.isPathSeparator(path[i]) {
		i--
	}
	if i >= 0 {
		path = path[i+1:]
	}
	// If empty now, it had only slashes.
	if path == "" {
		return string(fp.pathSeparator)
	}
	return path
}

// Dir returns all but the last element of path, typically the path's directory.
// After dropping the final element, Dir calls Clean on the path and trailing
// slashes are removed.
// If the path is empty, Dir returns ".".
// If the path consists entirely of separators, Dir returns a single separator.
// The returned path does not end in a separator unless it is the root directory.
func (fp goFilepath) dir(path string) string {
	vol := fp.volumeName(path)
	i := len(path) - 1
	for i >= len(vol) && !fp.isPathSeparator(path[i]) {
		i--
	}
	dir := fp.clean(path[len(vol) : i+1])
	if dir == "." && len(vol) > 2 {
		// must be UNC
		return vol
	}
	return vol + dir
}

// Ext returns the file name extension used by path.
// The extension is the suffix beginning at the final dot
// in the final element of path; it is empty if there is
// no dot.
func (fp goFilepath) ext(path string) string {
	for i := len(path) - 1; i >= 0 && !fp.isPathSeparator(path[i]); i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

// Join joins any number of path elements into a single path,
// separating them with an OS specific Separator. Empty elements
// are ignored. The result is Cleaned. However, if the argument
// list is empty or all its elements are empty, Join returns
// an empty string.
// On Windows, the result will only be a UNC path if the first
// non-empty element is a UNC path.
func (fp goFilepath) join(elem []string) string {
	for i, e := range elem {
		if e != "" {
			if fp.isWindows {
				return fp.joinNonEmpty(elem[i:])
			}
			return fp.clean(strings.Join(elem[i:], string(fp.pathSeparator)))
		}
	}
	return ""
}

// joinNonEmpty is like join, but it assumes that the first element is non-empty.
func (fp goFilepath) joinNonEmpty(elem []string) string {
	if len(elem[0]) == 2 && elem[0][1] == ':' {
		// First element is drive letter without terminating slash.
		// Keep path relative to current directory on that drive.
		// Skip empty elements.
		i := 1
		for ; i < len(elem); i++ {
			if elem[i] != "" {
				break
			}
		}
		return fp.clean(elem[0] + strings.Join(elem[i:], string(fp.pathSeparator)))
	}
	// The following logic prevents Join from inadvertently creating a
	// UNC path on Windows. Unless the first element is a UNC path, Join
	// shouldn't create a UNC path. See golang.org/issue/9167.
	p := fp.clean(strings.Join(elem, string(fp.pathSeparator)))
	if !fp.isUNC(p) {
		return p
	}
	// p == UNC only allowed when the first element is a UNC path.
	head := fp.clean(elem[0])
	if fp.isUNC(head) {
		return p
	}
	// head + tail == UNC, but joining two non-UNC paths should not result
	// in a UNC path. Undo creation of UNC path.
	tail := fp.clean(strings.Join(elem[1:], string(fp.pathSeparator)))
	if head[len(head)-1] == fp.pathSeparator {
		return head + tail
	}
	return head + string(fp.pathSeparator) + tail
}

// isUNC reports whether path is a UNC path.
func (fp goFilepath) isUNC(path string) bool {
	return fp.volumeNameLen(path) > 2
}

// Rel returns a relative path that is lexically equivalent to targpath when
// joined to basepath with an intervening separator. That is,
// Join(basepath, Rel(basepath, targpath)) is equivalent to targpath itself.
// On success, the returned path will always be relative to basepath,
// even if basepath and targpath share no elements.
// An error is returned if targpath can't be made relative to basepath or if
// knowing the current working directory would be necessary to compute it.
// Rel calls Clean on the result.
func (fp goFilepath) rel(basepath, targpath string) (string, error) {
	baseVol := fp.volumeName(basepath)
	targVol := fp.volumeName(targpath)
	base := fp.clean(basepath)
	targ := fp.clean(targpath)
	if fp.sameWord(targ, base) {
		return ".", nil
	}
	base = base[len(baseVol):]
	targ = targ[len(targVol):]
	if base == "." {
		base = ""
	}
	// Can't use IsAbs - `\a` and `a` are both relative in Windows.
	baseSlashed := len(base) > 0 && base[0] == fp.pathSeparator
	targSlashed := len(targ) > 0 && targ[0] == fp.pathSeparator
	if baseSlashed != targSlashed || !fp.sameWord(baseVol, targVol) {
		return "", errors.New("Rel: can't make " + targpath + " relative to " + basepath)
	}
	// Position base[b0:bi] and targ[t0:ti] at the first differing elements.
	bl := len(base)
	tl := len(targ)
	var b0, bi, t0, ti int
	for {
		for bi < bl && base[bi] != fp.pathSeparator {
			bi++
		}
		for ti < tl && targ[ti] != fp.pathSeparator {
			ti++
		}
		if !fp.sameWord(targ[t0:ti], base[b0:bi]) {
			break
		}
		if bi < bl {
			bi++
		}
		if ti < tl {
			ti++
		}
		b0 = bi
		t0 = ti
	}
	if base[b0:bi] == ".." {
		return "", errors.New("Rel: can't make " + targpath + " relative to " + basepath)
	}
	if b0 != bl {
		// Base elements left. Must go up before going down.
		seps := strings.Count(base[b0:bl], string(fp.pathSeparator))
		size := 2 + seps*3
		if tl != t0 {
			size += 1 + tl - t0
		}
		buf := make([]byte, size)
		n := copy(buf, "..")
		for i := 0; i < seps; i++ {
			buf[n] = fp.pathSeparator
			copy(buf[n+1:], "..")
			n += 3
		}
		if t0 != tl {
			buf[n] = fp.pathSeparator
			copy(buf[n+1:], targ[t0:])
		}
		return string(buf), nil
	}
	return targ[t0:], nil
}

func (fp goFilepath) sameWord(a, b string) bool {
	if !fp.isWindows {
		return a == b
	}
	return strings.EqualFold(a, b)
}
