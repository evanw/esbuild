//go:build darwin || freebsd || linux
// +build darwin freebsd linux

package fs

import (
	"time"

	"golang.org/x/sys/unix"
)

func modKey(path string) (ModKey, error) {
	stat := unix.Stat_t{}
	if err := unix.Stat(path, &stat); err != nil {
		return ModKey{}, err
	}

	// We can't detect changes if the file system zeros out the modification time
	if stat.Mtim.Sec == 0 && stat.Mtim.Nsec == 0 {
		return ModKey{}, modKeyUnusable
	}

	// Don't generate a modification key if the file is too new
	now, err := unix.TimeToTimespec(time.Now())
	if err != nil {
		return ModKey{}, err
	}
	mtimeSec := stat.Mtim.Sec + modKeySafetyGap
	if mtimeSec > now.Sec || (mtimeSec == now.Sec && stat.Mtim.Nsec > now.Nsec) {
		return ModKey{}, modKeyUnusable
	}

	return ModKey{
		inode:      stat.Ino,
		size:       stat.Size,
		mtime_sec:  int64(stat.Mtim.Sec),
		mtime_nsec: int64(stat.Mtim.Nsec),
		mode:       uint32(stat.Mode),
		uid:        stat.Uid,
	}, nil
}
