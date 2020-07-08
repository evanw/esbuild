package logging

// Logging is currently designed to look and feel like clang's error format.
// Errors are streamed asynchronously as they happen, each error contains the
// contents of the line with the error, and the error count is limited by
// default.

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/evanw/esbuild/internal/ast"
)

type Log struct {
	addMsg    func(Msg)
	hasErrors func() bool
	done      func() []Msg
}

type LogLevel int8

const (
	LevelNone LogLevel = iota
	LevelInfo
	LevelWarning
	LevelError
)

type MsgKind uint8

const (
	Error MsgKind = iota
	Warning
)

type Msg struct {
	Source *Source
	Start  int32
	Length int32
	Text   string
	Kind   MsgKind
}

type Source struct {
	Index uint32

	// This is a platform-dependent path that includes environment-specific things
	// such as Windows backslash path separators and potentially the user's home
	// directory. Only use this for passing to syscalls for reading and writing to
	// the file system. Do not include this in any output data.
	KeyPath ast.Path

	// This is a mostly platform-independent path. It's relative to the current
	// working directory and always uses standard path separators. Use this for
	// referencing a file in all output data. These paths still use the original
	// case of the path so they may still work differently on file systems that
	// are case-insensitive vs. case-sensitive.
	PrettyPath string

	// An identifier that is mixed in to automatically-generated symbol names to
	// improve readability. For example, if the identifier is "util" then the
	// symbol for an "export default" statement will be called "util_default".
	IdentifierName string

	Contents string
}

func (s *Source) TextForRange(r ast.Range) string {
	return s.Contents[r.Loc.Start : r.Loc.Start+r.Len]
}

func (s *Source) RangeOfString(loc ast.Loc) ast.Range {
	text := s.Contents[loc.Start:]
	if len(text) == 0 {
		return ast.Range{Loc: loc, Len: 0}
	}

	quote := text[0]
	if quote == '"' || quote == '\'' {
		// Search for the matching quote character
		for i := 1; i < len(text); i++ {
			c := text[i]
			if c == quote {
				return ast.Range{Loc: loc, Len: int32(i + 1)}
			} else if c == '\\' {
				i += 1
			}
		}
	}

	return ast.Range{Loc: loc, Len: 0}
}

func plural(prefix string, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, prefix)
	}
	return fmt.Sprintf("%d %ss", count, prefix)
}

func errorAndWarningSummary(errors int, warnings int) string {
	switch {
	case errors == 0:
		return plural("warning", warnings)
	case warnings == 0:
		return plural("error", errors)
	default:
		return fmt.Sprintf("%s and %s",
			plural("warning", warnings),
			plural("error", errors))
	}
}

type TerminalInfo struct {
	IsTTY           bool
	UseColorEscapes bool
	Width           int
}

func NewStderrLog(options StderrOptions) Log {
	var mutex sync.Mutex
	var msgs []Msg
	terminalInfo := GetTerminalInfo(os.Stderr)
	errors := 0
	warnings := 0
	errorLimitWasHit := false

	switch options.Color {
	case ColorNever:
		terminalInfo.UseColorEscapes = false
	case ColorAlways:
		terminalInfo.UseColorEscapes = SupportsColorEscapes
	}

	return Log{
		addMsg: func(msg Msg) {
			mutex.Lock()
			defer mutex.Unlock()
			msgs = append(msgs, msg)

			// Be silent if we're past the limit so we don't flood the terminal
			if errorLimitWasHit {
				return
			}

			switch msg.Kind {
			case Error:
				errors++
				if options.LogLevel <= LevelError {
					os.Stderr.WriteString(msg.String(options, terminalInfo))
				}
			case Warning:
				warnings++
				if options.LogLevel <= LevelWarning {
					os.Stderr.WriteString(msg.String(options, terminalInfo))
				}
			}

			// Silence further output if we reached the error limit
			if options.ErrorLimit != 0 && errors >= options.ErrorLimit {
				errorLimitWasHit = true
				if options.LogLevel <= LevelError {
					fmt.Fprintf(os.Stderr, "%s reached (disable error limit with --error-limit=0)\n", errorAndWarningSummary(errors, warnings))
				}
			}
		},
		hasErrors: func() bool {
			mutex.Lock()
			defer mutex.Unlock()
			return errors > 0
		},
		done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()

			// Print out a summary if the error limit wasn't hit
			if !errorLimitWasHit && options.LogLevel <= LevelInfo && (warnings != 0 || errors != 0) {
				fmt.Fprintf(os.Stderr, "%s\n", errorAndWarningSummary(errors, warnings))
			}

			return msgs
		},
	}
}

func PrintErrorToStderr(osArgs []string, text string) {
	options := StderrOptions{}

	// Implement a mini argument parser so these options always work even if we
	// haven't yet gotten to the general-purpose argument parsing code
	for _, arg := range osArgs {
		switch arg {
		case "--color=false":
			options.Color = ColorNever
		case "--color=true":
			options.Color = ColorAlways
		case "--log-level=info":
			options.LogLevel = LevelInfo
		case "--log-level=warning":
			options.LogLevel = LevelWarning
		case "--log-level=error":
			options.LogLevel = LevelError
		}
	}

	log := NewStderrLog(options)
	log.AddError(nil, ast.Loc{}, text)
	log.Done()
}

func NewDeferLog() Log {
	var msgs []Msg
	var mutex sync.Mutex
	var hasErrors bool

	return Log{
		addMsg: func(msg Msg) {
			mutex.Lock()
			defer mutex.Unlock()
			if msg.Kind == Error {
				hasErrors = true
			}
			msgs = append(msgs, msg)
		},
		hasErrors: func() bool {
			mutex.Lock()
			defer mutex.Unlock()
			return hasErrors
		},
		done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()
			return msgs
		},
	}
}

const colorReset = "\033[0m"
const colorRed = "\033[31m"
const colorGreen = "\033[32m"
const colorMagenta = "\033[35m"
const colorBold = "\033[1m"
const colorResetBold = "\033[0;1m"

type StderrColor uint8

const (
	ColorIfTerminal StderrColor = iota
	ColorNever
	ColorAlways
)

type StderrOptions struct {
	IncludeSource bool
	ErrorLimit    int
	Color         StderrColor
	LogLevel      LogLevel
}

func (msg Msg) String(options StderrOptions, terminalInfo TerminalInfo) string {
	kind := "error"
	kindColor := colorRed

	if msg.Kind == Warning {
		kind = "warning"
		kindColor = colorMagenta
	}

	if msg.Source == nil {
		if terminalInfo.UseColorEscapes {
			return fmt.Sprintf("%s%s%s: %s%s%s\n",
				colorBold, kindColor, kind,
				colorResetBold, msg.Text,
				colorReset)
		}

		return fmt.Sprintf("%s: %s\n", kind, msg.Text)
	}

	if !options.IncludeSource {
		if terminalInfo.UseColorEscapes {
			return fmt.Sprintf("%s%s: %s%s: %s%s%s\n",
				colorBold, msg.Source.PrettyPath,
				kindColor, kind,
				colorResetBold, msg.Text,
				colorReset)
		}

		return fmt.Sprintf("%s: %s: %s\n", msg.Source.PrettyPath, kind, msg.Text)
	}

	d := detailStruct(msg, terminalInfo)

	if terminalInfo.UseColorEscapes {
		return fmt.Sprintf("%s%s:%d:%d: %s%s: %s%s\n%s%s%s%s%s%s\n%s%s%s%s\n",
			colorBold, d.Path,
			d.Line,
			d.Column,
			kindColor, d.Kind,
			colorResetBold, d.Message,
			colorReset, d.SourceBefore, colorGreen, d.SourceMarked, colorReset, d.SourceAfter,
			colorGreen, d.Indent, d.Marker,
			colorReset)
	}

	return fmt.Sprintf("%s:%d:%d: %s: %s\n%s\n%s%s\n",
		d.Path, d.Line, d.Column, d.Kind, d.Message, d.Source, d.Indent, d.Marker)
}

type MsgDetail struct {
	Path    string
	Line    int
	Column  int
	Kind    string
	Message string

	// Source == SourceBefore + SourceMarked + SourceAfter
	Source       string
	SourceBefore string
	SourceMarked string
	SourceAfter  string

	Indent string
	Marker string
}

func ComputeLineAndColumn(text string) (lineCount int, columnCount, lastLineStart int) {
	var prevCodePoint rune

	for i, codePoint := range text {
		switch codePoint {
		case '\n':
			lastLineStart = i + 1
			if prevCodePoint != '\r' {
				lineCount++
			}
		case '\r', '\u2028', '\u2029':
			lastLineStart = i + 1
		}
		prevCodePoint = codePoint
	}

	columnCount = len(text) - lastLineStart
	return
}

func detailStruct(msg Msg, terminalInfo TerminalInfo) MsgDetail {
	contents := msg.Source.Contents
	lineCount, columnCount, lineStart := ComputeLineAndColumn(contents[0:msg.Start])
	lineEnd := len(contents)

loop:
	for i, codePoint := range contents[lineStart:] {
		switch codePoint {
		case '\r', '\n', '\u2028', '\u2029':
			lineEnd = lineStart + i
			break loop
		}
	}

	spacesPerTab := 2
	lineText := renderTabStops(contents[lineStart:lineEnd], spacesPerTab)
	indent := strings.Repeat(" ", len(renderTabStops(contents[lineStart:msg.Start], spacesPerTab)))
	marker := "^"
	markerStart := len(indent)
	markerEnd := len(indent)

	// Extend markers to cover the full range of the error
	if msg.Length > 0 {
		markerEnd = len(renderTabStops(contents[lineStart:msg.Start+msg.Length], spacesPerTab))
	}

	// Clip the marker to the bounds of the line
	if markerStart > len(lineText) {
		markerStart = len(lineText)
	}
	if markerEnd > len(lineText) {
		markerEnd = len(lineText)
	}
	if markerEnd < markerStart {
		markerEnd = markerStart
	}

	// Trim the line to fit the terminal width
	width := terminalInfo.Width
	if width < 1 {
		width = 80
	}
	if len(lineText) > width {
		// Try to center the error
		sliceStart := (markerStart + markerEnd - width) / 2
		if sliceStart > markerStart-width/5 {
			sliceStart = markerStart - width/5
		}
		if sliceStart < 0 {
			sliceStart = 0
		}
		if sliceStart > len(lineText)-width {
			sliceStart = len(lineText) - width
		}
		sliceEnd := sliceStart + width

		// Slice the line
		slicedLine := lineText[sliceStart:sliceEnd]
		markerStart -= sliceStart
		markerEnd -= sliceStart
		if markerStart < 0 {
			markerStart = 0
		}
		if markerEnd > len(slicedLine) {
			markerEnd = len(slicedLine)
		}

		// Truncate the ends with "..."
		if len(slicedLine) > 3 && sliceStart > 0 {
			slicedLine = "..." + slicedLine[3:]
			if markerStart < 3 {
				markerStart = 3
			}
		}
		if len(slicedLine) > 3 && sliceEnd < len(lineText) {
			slicedLine = slicedLine[:len(slicedLine)-3] + "..."
			if markerEnd > len(slicedLine)-3 {
				markerEnd = len(slicedLine) - 3
			}
			if markerEnd < markerStart {
				markerEnd = markerStart
			}
		}

		// Now we can compute the indent
		indent = strings.Repeat(" ", markerStart)
		lineText = slicedLine
	}

	// If marker is still multi-character after clipping, make the marker wider
	if markerEnd-markerStart > 1 {
		marker = strings.Repeat("~", markerEnd-markerStart)
	}

	kind := "error"
	if msg.Kind == Warning {
		kind = "warning"
	}

	return MsgDetail{
		Path:    msg.Source.PrettyPath,
		Line:    lineCount + 1,
		Column:  columnCount,
		Kind:    kind,
		Message: msg.Text,

		Source:       lineText,
		SourceBefore: lineText[:markerStart],
		SourceMarked: lineText[markerStart:markerEnd],
		SourceAfter:  lineText[markerEnd:],

		Indent: indent,
		Marker: marker,
	}
}

func renderTabStops(withTabs string, spacesPerTab int) string {
	if !strings.ContainsRune(withTabs, '\t') {
		return withTabs
	}

	withoutTabs := strings.Builder{}
	count := 0

	for _, c := range withTabs {
		if c == '\t' {
			spaces := spacesPerTab - count%spacesPerTab
			for i := 0; i < spaces; i++ {
				withoutTabs.WriteRune(' ')
				count++
			}
		} else {
			withoutTabs.WriteRune(c)
			count++
		}
	}

	return withoutTabs.String()
}

func (log Log) HasErrors() bool {
	return log.hasErrors()
}

func (log Log) Done() []Msg {
	return log.done()
}

func (log Log) AddError(source *Source, loc ast.Loc, text string) {
	log.addMsg(Msg{source, loc.Start, 0, text, Error})
}

func (log Log) AddWarning(source *Source, loc ast.Loc, text string) {
	log.addMsg(Msg{source, loc.Start, 0, text, Warning})
}

func (log Log) AddRangeError(source *Source, r ast.Range, text string) {
	log.addMsg(Msg{source, r.Loc.Start, r.Len, text, Error})
}

func (log Log) AddRangeWarning(source *Source, r ast.Range, text string) {
	log.addMsg(Msg{source, r.Loc.Start, r.Len, text, Warning})
}
