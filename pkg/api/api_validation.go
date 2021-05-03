package api

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/snap_printer"
)

// Denotes that rewriting the code causes problems and thus that particular module
// should not be rewritten in order to obtain a valid bundle.
const SNAPSHOT_REWRITE_FAILURE = "[SNAPSHOT_REWRITE_FAILURE]"

// Denotes that a module needs to be deferred, i.e. if it accesses constants like
// __dirname that it shouldn't.
const SNAPSHOT_CACHE_FAILURE = "[SNAPSHOT_CACHE_FAILURE]"

func ErrorToWarningLogger(log *logger.Log, failureType string) logger.Log {
	forgivingLog := logger.Log{
		AddMsg: func(msg logger.Msg) {
			if msg.Kind == logger.Error {
				msg.Data.Text = fmt.Sprintf("%s %s", failureType, msg.Data.Text)
				msg.Kind = logger.Warning
			}
			log.AddMsg(msg)
		},
		HasErrors: func() bool {
			return log.HasErrors()
		},
		Done: func() []logger.Msg {
			return log.Done()
		},
	}
	return forgivingLog
}

func fileLoggerSource(filePath string, js []byte) logger.Source {
	path := logger.Path{Text: filePath, Namespace: "file"}
	return logger.Source{
		Index:          0,
		KeyPath:        path,
		PrettyPath:     filePath,
		Contents:       string(js),
		IdentifierName: filePath,
	}
}

func reportValidationErrors(result *snap_printer.PrintResult, log *logger.Log, filePath string) bool {
	if result.ValidationErrors == nil || len(result.ValidationErrors) == 0 {
		return false
	}
	reportedError := false
	rewriteLog := ErrorToWarningLogger(log, SNAPSHOT_REWRITE_FAILURE)
	deferLog := ErrorToWarningLogger(log, SNAPSHOT_CACHE_FAILURE)
	source := fileLoggerSource(filePath, result.JS)
	for _, err := range result.ValidationErrors {
		switch err.Kind {
		case snap_printer.NoRewrite:
			rewriteLog.AddError(&source, logger.Loc{Start: int32(err.Idx)}, err.Msg)
			break
		case snap_printer.Defer:
			deferLog.AddError(&source, logger.Loc{Start: int32(err.Idx)}, err.Msg)
			break

		}
		reportedError = true
	}
	return reportedError
}

func verifyPrint(result *snap_printer.PrintResult, log *logger.Log, filePath string, shouldPanic bool) {
	// Cannot use printer logger since that would add any issues as error messages which causes the
	// entire process to fail. What we want instead is to provide an indicator of what error
	// occurred in which file so that the caller can process it.
	vlog := ErrorToWarningLogger(log, SNAPSHOT_REWRITE_FAILURE)
	source := fileLoggerSource(filePath, result.JS)
	js_parser.Parse(vlog, source, js_parser.OptionsFromConfig(&config.Options{}))
}

func reportWarning(
	result *snap_printer.PrintResult,
	log *logger.Log,
	filePath string,
	error string,
	errorStart int32) {
	loc := logger.Loc{Start: errorStart}
	path := logger.Path{Text: filePath, Namespace: "file"}
	source := logger.Source{
		Index:          0,
		KeyPath:        path,
		PrettyPath:     filePath,
		Contents:       string(result.JS),
		IdentifierName: filePath,
	}
	vlog := ErrorToWarningLogger(log, SNAPSHOT_CACHE_FAILURE)
	s := fmt.Sprintf("Encountered a problem inside '%s'\n  %s", filePath, error)
	vlog.AddError(&source, loc, s)
}

// Tries to find the needle in the code and normalizes the result to `0` if not found
func tryFindLocInside(js *[]byte, needle string, skip int) int32 {
	// Here we do a cheap search in the code to guess where the use of the needle occurred
	loc := 0
	needleLen := len(needle)
	offset := 0
	s := string(*js)
	for n := 0; n <= skip; n++ {
		loc := strings.Index(s[offset:], needle)
		if loc < 0 {
			return 0
		}
		offset = offset + loc + needleLen
	}
	return int32(offset + loc - needleLen)
}
