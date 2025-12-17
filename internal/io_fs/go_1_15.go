//go:build !go1.16
// +build !go1.16

package io_fs

import (
	"fmt"
	"time"
)

// These are shims to allow esbuild to compile with older versions of Go. The
// caller must use Go 1.16 or newer to be able to use FS.

type FS interface {
	Open(name string) (File, error)
}

type File interface {
	Stat() (FileInfo, error)
	Read([]byte) (int, error)
	Close() error
}

type ReadDirFile interface {
	File
	ReadDir(n int) ([]DirEntry, error)
}

type DirEntry interface {
	Name() string
	IsDir() bool
	Type() FileMode
	Info() (FileInfo, error)
}

type FileInfo interface {
	Name() string
	Size() int64
	Mode() FileMode
	ModTime() time.Time
	IsDir() bool
	Sys() any
}

type FileMode uint32

const (
	ModeDir FileMode = 1 << (32 - 1 - iota)
	ModeAppend
	ModeExclusive
	ModeTemporary
	ModeSymlink
	ModeDevice
	ModeNamedPipe
	ModeSocket
	ModeSetuid
	ModeSetgid
	ModeCharDevice
	ModeSticky
	ModeIrregular

	ModeType = ModeDir | ModeSymlink | ModeNamedPipe | ModeSocket | ModeDevice | ModeCharDevice | ModeIrregular

	ModePerm FileMode = 0777
)

func (m FileMode) IsDir() bool {
	return m&ModeDir != 0
}

func (m FileMode) IsRegular() bool {
	return m&ModeType == 0
}

func (m FileMode) Perm() FileMode {
	return m & ModePerm
}

func (m FileMode) Type() FileMode {
	return m & ModeType
}

func (m FileMode) String() string {
	return fmt.Sprint(m)
}
