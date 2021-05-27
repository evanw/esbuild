package helpers

import "strings"

func IsInsideNodeModules(path string) bool {
	for {
		// This is written in a platform-independent manner because it's run on
		// user-specified paths which can be arbitrary non-file-system things. So
		// for example Windows paths may end up being used on Unix or URLs may end
		// up being used on Windows. Be consistently agnostic to which kind of
		// slash is used on all platforms.
		slash := strings.LastIndexAny(path, "/\\")
		if slash == -1 {
			return false
		}
		dir, base := path[:slash], path[slash+1:]
		if base == "node_modules" {
			return true
		}
		path = dir
	}
}
