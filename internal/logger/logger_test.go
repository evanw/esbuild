package logger_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func TestPrintTextWithColor(t *testing.T) {
	// Include SGR sequences both in and outside of esbuild's color palette, as
	// well as other control characters. Redirecting output must preserve them.
	text := "\x1b[4m\x1b[2mhttps://example.com\x1b[22m\x1b[24m" +
		"\x1b[91merror\x1b[39m\x1b[0m\x00\x07\x08\t\r\n\x1b[2J"
	for _, color := range []logger.UseColor{logger.ColorAlways, logger.ColorNever, logger.ColorIfTerminal} {
		file, err := ioutil.TempFile("", "esbuild-logger-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file.Name())
		defer file.Close()

		logger.PrintTextWithColor(file, color, func(colors logger.Colors) string {
			return colors.Red + text + colors.Reset
		})
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		bytes, err := ioutil.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		expected := text
		if color == logger.ColorAlways && logger.SupportsColorEscapes {
			expected = logger.TerminalColors.Red + text + logger.TerminalColors.Reset
		}
		test.AssertEqual(t, string(bytes), expected)
	}
}

func TestMsgIDs(t *testing.T) {
	for id := logger.MsgID_None; id <= logger.MsgID_END; id++ {
		str := logger.MsgIDToString(id)
		if str == "" {
			continue
		}

		overrides := make(map[logger.MsgID]logger.LogLevel)
		logger.StringToMsgIDs(str, logger.LevelError, overrides)
		if len(overrides) == 0 {
			t.Fatalf("Failed to find message id(s) for the string %q", str)
		}

		for k, v := range overrides {
			test.AssertEqual(t, logger.MsgIDToString(k), str)
			test.AssertEqual(t, v, logger.LevelError)
		}
	}
}
