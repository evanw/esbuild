package helpers

import (
	"runtime/debug"
	"strings"
)

func PrettyPrintedStack() string {
	lines := strings.Split(strings.TrimSpace(string(debug.Stack())), "\n")

	// Strip the first "goroutine" line
	if len(lines) > 0 {
		if first := lines[0]; strings.HasPrefix(first, "goroutine ") && strings.HasSuffix(first, ":") {
			lines = lines[1:]
		}
	}

	sb := strings.Builder{}

	for _, line := range lines {
		// Indented lines are source locations
		if strings.HasPrefix(line, "\t") {
			line = line[1:]
			line = strings.TrimPrefix(line, "github.com/evanw/esbuild/")
			if offset := strings.LastIndex(line, " +0x"); offset != -1 {
				line = line[:offset]
			}
			sb.WriteString(" (")
			sb.WriteString(line)
			sb.WriteString(")")
			continue
		}

		// Other lines are function calls
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		if strings.HasSuffix(line, ")") {
			if paren := strings.LastIndexByte(line, '('); paren != -1 {
				line = line[:paren]
			}
		}
		if slash := strings.LastIndexByte(line, '/'); slash != -1 {
			line = line[slash+1:]
		}
		sb.WriteString(line)
	}

	return sb.String()
}
