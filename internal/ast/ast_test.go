package ast

import "testing"

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestGenerateNonUniqueNameFromPath(t *testing.T) {
	assertEqual(t, GenerateNonUniqueNameFromPath("<stdin>"), "stdin")
	assertEqual(t, GenerateNonUniqueNameFromPath("foo/bar"), "bar")
	assertEqual(t, GenerateNonUniqueNameFromPath("foo/bar.js"), "bar")
	assertEqual(t, GenerateNonUniqueNameFromPath("foo/bar.min.js"), "bar_min")
	assertEqual(t, GenerateNonUniqueNameFromPath("trailing//slashes//"), "slashes")
	assertEqual(t, GenerateNonUniqueNameFromPath("path/with/spaces in name.js"), "spaces_in_name")
	assertEqual(t, GenerateNonUniqueNameFromPath("path\\on\\windows.js"), "windows")
	assertEqual(t, GenerateNonUniqueNameFromPath("node_modules/demo-pkg/index.js"), "demo_pkg")
	assertEqual(t, GenerateNonUniqueNameFromPath("node_modules\\demo-pkg\\index.js"), "demo_pkg")
	assertEqual(t, GenerateNonUniqueNameFromPath("123_invalid_identifier.js"), "invalid_identifier")
	assertEqual(t, GenerateNonUniqueNameFromPath("emoji 🍕 name.js"), "emoji_name")
}
