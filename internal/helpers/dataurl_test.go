package helpers_test

import (
	"fmt"
	"testing"

	"github.com/evanw/esbuild/internal/helpers"
)

func TestEncodeDataURL(t *testing.T) {
	check := func(raw string, expected string) {
		url, ok := helpers.EncodeStringAsPercentEscapedDataURL("text/plain", raw)
		if !ok {
			t.Fatalf("Failed to encode %q", raw)
		} else if url != expected {
			t.Fatalf("Got %q but expected %q", url, expected)
		}
	}

	for i := 0; i <= 0xFF; i++ {
		alwaysEscape := i == '\t' || i == '\r' || i == '\n' || i == '#'
		trailingEscape := i <= 0x20 || i == '#'

		if trailingEscape {
			check(string(rune(i)), fmt.Sprintf("data:text/plain,%%%02X", i))
			check("foo"+string(rune(i)), fmt.Sprintf("data:text/plain,foo%%%02X", i))
		} else {
			check(string(rune(i)), fmt.Sprintf("data:text/plain,%c", i))
			check("foo"+string(rune(i)), fmt.Sprintf("data:text/plain,foo%c", i))
		}

		if alwaysEscape {
			check(string(rune(i))+"foo", fmt.Sprintf("data:text/plain,%%%02Xfoo", i))
		} else {
			check(string(rune(i))+"foo", fmt.Sprintf("data:text/plain,%cfoo", i))
		}
	}

	// Test leading vs. trailing
	check(" \t ", "data:text/plain, %09%20")
	check(" \n ", "data:text/plain, %0A%20")
	check(" \r ", "data:text/plain, %0D%20")
	check(" # ", "data:text/plain, %23%20")
	check("\x08#\x08", "data:text/plain,\x08%23%08")

	// Only "%" symbols that could form an escape need to be escaped
	check("%, %3, %33, %333", "data:text/plain,%, %3, %2533, %25333")
}
