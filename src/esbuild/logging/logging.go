package logging

// Logging is currently designed to look and feel like clang's error format.
// Errors are streamed asynchronously as they happen, each error contains the
// contents of the line with the error, and the error count is limited by
// default.

import (
	"fmt"
	"github.com/evanw/esbuild/src/esbuild/ast"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Log struct {
	msgs chan Msg
}

type MsgKind int

const (
	Error MsgKind = iota
	Warning
)

type Msg struct {
	source Source
	start  int32
	length int32
	text   string
	kind   MsgKind
}

type Source struct {
	Index        uint32
	AbsolutePath string
	PrettyPath   string
	Contents     string
}

func (s *Source) TextForRange(r ast.Range) string {
	return s.Contents[r.Loc.Start : r.Loc.Start+r.Len]
}

func (s *Source) RangeOfString(loc ast.Loc) ast.Range {
	text := s.Contents[loc.Start:]
	if len(text) == 0 {
		return ast.Range{loc, 0}
	}

	quote := text[0]
	if quote == '"' || quote == '\'' {
		// Search for the matching quote character
		for i := 1; i < len(text); i++ {
			c := text[i]
			if c == quote {
				return ast.Range{loc, int32(i + 1)}
			} else if c == '\\' {
				i += 1
			}
		}
	}

	return ast.Range{loc, 0}
}

func NewLog(msgs chan Msg) Log {
	return Log{msgs}
}

type MsgCounts struct {
	Errors   int
	Warnings int
}

func plural(prefix string, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, prefix)
	}
	return fmt.Sprintf("%d %ss", count, prefix)
}

func (counts MsgCounts) String() string {
	if counts.Errors == 0 {
		if counts.Warnings == 0 {
			return "no errors"
		} else {
			return plural("warning", counts.Warnings)
		}
	} else {
		if counts.Warnings == 0 {
			return plural("error", counts.Errors)
		} else {
			return fmt.Sprintf("%s and %s",
				plural("warning", counts.Warnings),
				plural("error", counts.Errors))
		}
	}
}

type TerminalInfo struct {
	UseColor bool
	Width    int
}

func StdinTerminalInfo() TerminalInfo {
	// This approach doesn't work on Windows. Just don't use color on Windows.
	if runtime.GOOS == "windows" {
		return TerminalInfo{}
	}

	// This approach works on macOS and Linux without using syscalls that are
	// hard to cross-compile. Worst case we just don't use color, which isn't bad.
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	if out, err := cmd.Output(); err == nil {
		if space := strings.IndexByte(string(out), ' '); space != -1 {
			if newline := strings.IndexByte(string(out), '\n'); newline > space {
				if width, err := strconv.Atoi(string(out[space+1 : newline])); err == nil && width > 0 {
					return TerminalInfo{true, width}
				}
			}
		}
	}

	return TerminalInfo{}
}

func asyncStdinTerminalInfo() func() TerminalInfo {
	channel := make(chan *TerminalInfo)
	var result *TerminalInfo

	// Run the command asynchronously
	go func() {
		info := StdinTerminalInfo()
		channel <- &info
	}()

	// Return a function that blocks on the result of the asynchronous command
	return func() TerminalInfo {
		if result == nil {
			result = <-channel
		}
		return *result
	}
}

func NewStderrLog(options StderrOptions) (Log, func() MsgCounts) {
	msgs := make(chan Msg)
	done := make(chan MsgCounts)
	log := NewLog(msgs)

	// Get the terminal info asynchronously to avoid blocking this thread
	asyncTerminalInfo := asyncStdinTerminalInfo()

	go func(msgs chan Msg, done chan MsgCounts) {
		counts := MsgCounts{}
		for msg := range msgs {
			os.Stderr.WriteString(msg.String(options, asyncTerminalInfo()))
			switch msg.kind {
			case Error:
				counts.Errors++
			case Warning:
				counts.Warnings++
			}
			if options.ExitWhenLimitIsHit && options.ErrorLimit != 0 && counts.Errors >= options.ErrorLimit {
				fmt.Fprintf(os.Stderr, "%s reached (disable error limit with --error-limit=0)\n", counts.String())
				os.Exit(1)
			}
		}
		done <- counts
	}(msgs, done)

	return log, func() MsgCounts {
		close(log.msgs)
		counts := <-done
		if counts.Warnings != 0 || counts.Errors != 0 {
			fmt.Fprintf(os.Stderr, "%s\n", counts.String())
		}
		return counts
	}
}

func NewDeferLog() (Log, func() []Msg) {
	msgs := make(chan Msg)
	done := make(chan []Msg)
	log := NewLog(msgs)

	go func(msgs chan Msg, done chan []Msg) {
		result := []Msg{}
		for msg := range msgs {
			result = append(result, msg)
		}
		done <- result
	}(msgs, done)

	return log, func() []Msg {
		close(log.msgs)
		return <-done
	}
}

const colorReset = "\033[0m"
const colorRed = "\033[31m"
const colorGreen = "\033[32m"
const colorMagenta = "\033[35m"
const colorBold = "\033[1m"
const colorResetBold = "\033[0;1m"

type StderrOptions struct {
	IncludeSource      bool
	ErrorLimit         int
	ExitWhenLimitIsHit bool
}

func (msg Msg) String(options StderrOptions, terminalInfo TerminalInfo) string {
	kind := "error"
	kindColor := colorRed

	if msg.kind == Warning {
		kind = "warning"
		kindColor = colorMagenta
	}

	if msg.source.PrettyPath == "" {
		if terminalInfo.UseColor {
			return fmt.Sprintf("%s%s%s: %s%s%s\n",
				colorBold, kindColor, kind,
				colorResetBold, msg.text,
				colorReset)
		}

		return fmt.Sprintf("%s: %s\n", kind, msg.text)
	}

	if !options.IncludeSource {
		if terminalInfo.UseColor {
			return fmt.Sprintf("%s%s: %s%s: %s%s%s\n",
				colorBold, msg.source.PrettyPath,
				kindColor, kind,
				colorResetBold, msg.text,
				colorReset)
		}

		return fmt.Sprintf("%s: %s: %s\n", msg.source.PrettyPath, kind, msg.text)
	}

	d := detailStruct(msg, terminalInfo)

	if terminalInfo.UseColor {
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

func detailStruct(msg Msg, terminalInfo TerminalInfo) MsgDetail {
	var prevCodePoint rune
	contents := msg.source.Contents
	lineStart := 0
	lineCount := 0

	for i, codePoint := range contents[0:msg.start] {
		switch codePoint {
		case '\n':
			lineStart = i + 1
			if prevCodePoint != '\r' {
				lineCount++
			}
		case '\r', '\u2028', '\u2029':
			lineStart = i + 1
		}
		prevCodePoint = codePoint
	}

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
	columnCount := int(msg.start) - lineStart
	lineText := renderTabStops(contents[lineStart:lineEnd], spacesPerTab)
	indent := strings.Repeat(" ", len(renderTabStops(contents[lineStart:msg.start], spacesPerTab)))
	marker := "^"
	markerStart := len(indent)
	markerEnd := len(indent)

	// Extend markers to cover the full range of the error
	if msg.length > 0 {
		markerEnd = len(renderTabStops(contents[lineStart:msg.start+msg.length], spacesPerTab))
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
	if terminalInfo.Width > 0 && len(lineText) > terminalInfo.Width {
		// Try to center the error
		sliceStart := (markerStart + markerEnd - terminalInfo.Width) / 2
		if sliceStart > markerStart-terminalInfo.Width/5 {
			sliceStart = markerStart - terminalInfo.Width/5
		}
		if sliceStart < 0 {
			sliceStart = 0
		}
		if sliceStart > len(lineText)-terminalInfo.Width {
			sliceStart = len(lineText) - terminalInfo.Width
		}
		sliceEnd := sliceStart + terminalInfo.Width

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
	if msg.kind == Warning {
		kind = "warning"
	}

	return MsgDetail{
		Path:    msg.source.PrettyPath,
		Line:    lineCount + 1,
		Column:  columnCount,
		Kind:    kind,
		Message: msg.text,

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

func (log Log) AddError(source Source, loc ast.Loc, text string) {
	log.msgs <- Msg{source, loc.Start, 0, text, Error}
}

func (log Log) AddWarning(source Source, loc ast.Loc, text string) {
	log.msgs <- Msg{source, loc.Start, 0, text, Warning}
}

func (log Log) AddRangeError(source Source, r ast.Range, text string) {
	log.msgs <- Msg{source, r.Loc.Start, r.Len, text, Error}
}

func (log Log) AddRangeWarning(source Source, r ast.Range, text string) {
	log.msgs <- Msg{source, r.Loc.Start, r.Len, text, Warning}
}
