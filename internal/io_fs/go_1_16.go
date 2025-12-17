//go:build go1.16
// +build go1.16

package io_fs

import "io/fs"

type (
	FS          = fs.FS
	File        = fs.File
	ReadDirFile = fs.ReadDirFile
	DirEntry    = fs.DirEntry
	FileInfo    = fs.FileInfo
	FileMode    = fs.FileMode
)

const (
	ModeDir        = fs.ModeDir
	ModeAppend     = fs.ModeAppend
	ModeExclusive  = fs.ModeExclusive
	ModeTemporary  = fs.ModeTemporary
	ModeSymlink    = fs.ModeSymlink
	ModeDevice     = fs.ModeDevice
	ModeNamedPipe  = fs.ModeNamedPipe
	ModeSocket     = fs.ModeSocket
	ModeSetuid     = fs.ModeSetuid
	ModeSetgid     = fs.ModeSetgid
	ModeCharDevice = fs.ModeCharDevice
	ModeSticky     = fs.ModeSticky
	ModeIrregular  = fs.ModeIrregular
	ModeType       = fs.ModeType
	ModePerm       = fs.ModePerm
)
