package helpers

import "strings"

type GlobWildcard uint8

const (
	GlobNone GlobWildcard = iota
	GlobAllExceptSlash
	GlobAllIncludingSlash
)

type GlobPart struct {
	Prefix   string
	Wildcard GlobWildcard
}

// The returned array will always be at least one element. If there are no
// wildcards then it will be exactly one element, and if there are wildcards
// then it will be more than one element.
func ParseGlobPattern(text string) (pattern []GlobPart) {
	for {
		star := strings.IndexByte(text, '*')
		if star < 0 {
			pattern = append(pattern, GlobPart{Prefix: text})
			break
		}
		count := 1
		for star+count < len(text) && text[star+count] == '*' {
			count++
		}
		wildcard := GlobAllExceptSlash

		// Allow both "/" and "\" as slashes
		if count > 1 && (star == 0 || text[star-1] == '/' || text[star-1] == '\\') &&
			(star+count == len(text) || text[star+count] == '/' || text[star+count] == '\\') {
			wildcard = GlobAllIncludingSlash // A "globstar" path segment
		}

		pattern = append(pattern, GlobPart{Prefix: text[:star], Wildcard: wildcard})
		text = text[star+count:]
	}
	return
}

func GlobPatternToString(pattern []GlobPart) string {
	sb := strings.Builder{}
	for _, part := range pattern {
		sb.WriteString(part.Prefix)
		switch part.Wildcard {
		case GlobAllExceptSlash:
			sb.WriteByte('*')
		case GlobAllIncludingSlash:
			sb.WriteString("**")
		}
	}
	return sb.String()
}
