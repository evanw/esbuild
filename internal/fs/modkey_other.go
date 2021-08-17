//go:build !darwin && !freebsd && !linux
// +build !darwin,!freebsd,!linux

package fs

import (
	"os"
	"time"
)

var zeroTime time.Time

func modKey(path string) (ModKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ModKey{}, err
	}

	// We can't detect changes if the file system zeros out the modification time
	mtime := info.ModTime()
	if mtime == zeroTime || mtime.Unix() == 0 {
		return ModKey{}, modKeyUnusable
	}

	// Don't generate a modification key if the file is too new
	if mtime.Add(modKeySafetyGap * time.Second).After(time.Now()) {
		return ModKey{}, modKeyUnusable
	}

	return ModKey{
		size:      info.Size(),
		mtime_sec: mtime.Unix(),
		mode:      uint32(info.Mode()),
	}, nil
}
