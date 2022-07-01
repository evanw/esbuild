package logger_test

import (
	"testing"

	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

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
