package logger

// Logging is currently designed to look and feel like clang's error format.
// Errors are streamed asynchronously as they happen, each error contains the
// contents of the line with the error, and the error count is limited by
// default.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type Log struct {
	AddMsg    func(Msg)
	HasErrors func() bool
	Done      func() []Msg
}

type LogLevel int8

const (
	LevelNone LogLevel = iota
	LevelInfo
	LevelWarning
	LevelError
	LevelSilent
)

type MsgKind uint8

const (
	Error MsgKind = iota
	Warning
)

type Msg struct {
	Kind     MsgKind
	Text     string
	Location *MsgLocation
}

type MsgLocation struct {
	File     string
	Line     int // 1-based
	Column   int // 0-based, in bytes
	Length   int // in bytes
	LineText string
}

type Loc struct {
	// This is the 0-based index of this location from the start of the file, in bytes
	Start int32
}

type Range struct {
	Loc Loc
	Len int32
}

func (r Range) End() int32 {
	return r.Loc.Start + r.Len
}

// This type is just so we can use Go's native sort function
type msgsArray []Msg

func (a msgsArray) Len() int          { return len(a) }
func (a msgsArray) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a msgsArray) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]

	li := ai.Location
	lj := aj.Location

	// Location
	if li == nil && lj != nil {
		return true
	}
	if li != nil && lj == nil {
		return false
	}

	if li != nil && lj != nil {
		// File
		if li.File < lj.File {
			return true
		}
		if li.File > lj.File {
			return false
		}

		// Line
		if li.Line < lj.Line {
			return true
		}
		if li.Line > lj.Line {
			return false
		}

		// Column
		if li.Column < lj.Column {
			return true
		}
		if li.Column > lj.Column {
			return false
		}

		// Length
		if li.Length < lj.Length {
			return true
		}
		if li.Length > lj.Length {
			return false
		}
	}

	// Kind
	if ai.Kind < aj.Kind {
		return true
	}
	if ai.Kind > aj.Kind {
		return false
	}

	// Text
	return ai.Text < aj.Text
}

// This is used to represent both file system paths (Namespace == "file") and
// abstract module paths (Namespace != "file"). Abstract module paths represent
// "virtual modules" when used for an input file and "package paths" when used
// to represent an external module.
type Path struct {
	Text      string
	Namespace string
}

func (a Path) ComesBeforeInSortedOrder(b Path) bool {
	return a.Namespace > b.Namespace || (a.Namespace == b.Namespace && a.Text < b.Text)
}

type Source struct {
	Index uint32

	// This is used as a unique key to identify this source file. It should never
	// be shown to the user (e.g. never print this to the terminal).
	//
	// If it's marked as an absolute path, it's a platform-dependent path that
	// includes environment-specific things such as Windows backslash path
	// separators and potentially the user's home directory. Only use this for
	// passing to syscalls for reading and writing to the file system. Do not
	// include this in any output data.
	//
	// If it's marked as not an absolute path, it's an opaque string that is used
	// to refer to an automatically-generated module.
	KeyPath Path

	// This is used for error messages and the metadata JSON file.
	//
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

func (s *Source) TextForRange(r Range) string {
	return s.Contents[r.Loc.Start : r.Loc.Start+r.Len]
}

func (s *Source) RangeOfOperatorBefore(loc Loc, op string) Range {
	text := s.Contents[:loc.Start]
	index := strings.LastIndex(text, op)
	if index >= 0 {
		return Range{Loc: Loc{Start: int32(index)}, Len: int32(len(op))}
	}
	return Range{Loc: loc}
}

func (s *Source) RangeOfOperatorAfter(loc Loc, op string) Range {
	text := s.Contents[loc.Start:]
	index := strings.Index(text, op)
	if index >= 0 {
		return Range{Loc: Loc{Start: loc.Start + int32(index)}, Len: int32(len(op))}
	}
	return Range{Loc: loc}
}

func (s *Source) RangeOfString(loc Loc) Range {
	text := s.Contents[loc.Start:]
	if len(text) == 0 {
		return Range{Loc: loc, Len: 0}
	}

	quote := text[0]
	if quote == '"' || quote == '\'' {
		// Search for the matching quote character
		for i := 1; i < len(text); i++ {
			c := text[i]
			if c == quote {
				return Range{Loc: loc, Len: int32(i + 1)}
			} else if c == '\\' {
				i += 1
			}
		}
	}

	return Range{Loc: loc, Len: 0}
}

func (s *Source) RangeOfNumber(loc Loc) (r Range) {
	text := s.Contents[loc.Start:]
	r = Range{Loc: loc, Len: 0}

	if len(text) > 0 {
		if c := text[0]; c >= '0' && c <= '9' {
			r.Len = 1
			for int(r.Len) < len(text) {
				c := text[r.Len]
				if (c < '0' || c > '9') && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '.' && c != '_' {
					break
				}
				r.Len++
			}
		}
	}
	return
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
	var msgs msgsArray
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
		AddMsg: func(msg Msg) {
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
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
				}
			case Warning:
				warnings++
				if options.LogLevel <= LevelWarning {
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
				}
			}

			// Silence further output if we reached the error limit
			if options.ErrorLimit != 0 && errors >= options.ErrorLimit {
				errorLimitWasHit = true
				if options.LogLevel <= LevelError {
					writeStringWithColor(os.Stderr, fmt.Sprintf(
						"%s reached (disable error limit with --error-limit=0)\n", errorAndWarningSummary(errors, warnings)))
				}
			}
		},
		HasErrors: func() bool {
			mutex.Lock()
			defer mutex.Unlock()
			return errors > 0
		},
		Done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()

			// Print out a summary if the error limit wasn't hit
			if !errorLimitWasHit && options.LogLevel <= LevelInfo && (warnings != 0 || errors != 0) {
				writeStringWithColor(os.Stderr, fmt.Sprintf("%s\n", errorAndWarningSummary(errors, warnings)))
			}

			sort.Stable(msgs)
			return msgs
		},
	}
}

func PrintErrorToStderr(osArgs []string, text string) {
	PrintMessageToStderr(osArgs, Msg{Kind: Error, Text: text})
}

func PrintMessageToStderr(osArgs []string, msg Msg) {
	options := StderrOptions{IncludeSource: true}

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
		case "--log-level=silent":
			options.LogLevel = LevelSilent
		}
	}

	log := NewStderrLog(options)
	log.AddMsg(msg)
	log.Done()
}

func NewDeferLog() Log {
	var msgs msgsArray
	var mutex sync.Mutex
	var hasErrors bool

	return Log{
		AddMsg: func(msg Msg) {
			mutex.Lock()
			defer mutex.Unlock()
			if msg.Kind == Error {
				hasErrors = true
			}
			msgs = append(msgs, msg)
		},
		HasErrors: func() bool {
			mutex.Lock()
			defer mutex.Unlock()
			return hasErrors
		},
		Done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()
			sort.Stable(msgs)
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

	if msg.Location == nil {
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
				colorBold, msg.Location.File,
				kindColor, kind,
				colorResetBold, msg.Text,
				colorReset)
		}

		return fmt.Sprintf("%s: %s: %s\n", msg.Location.File, kind, msg.Text)
	}

	d := detailStruct(msg, terminalInfo)

	if terminalInfo.UseColorEscapes {
		return fmt.Sprintf("%s%s:%d:%d: %s%s: %s%s\n%s%s%s%s%s%s\n%s%s%s%s%s\n",
			colorBold, d.Path,
			d.Line,
			d.Column,
			kindColor, d.Kind,
			colorResetBold, d.Message,
			colorReset, d.SourceBefore, colorGreen, d.SourceMarked, colorReset, d.SourceAfter,
			colorGreen, d.Indent, d.Marker,
			colorReset, d.ContentAfter)
	}

	return fmt.Sprintf("%s:%d:%d: %s: %s\n%s\n%s%s%s\n",
		d.Path, d.Line, d.Column, d.Kind, d.Message, d.Source, d.Indent, d.Marker, d.ContentAfter)
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

	ContentAfter string
}

func computeLineAndColumn(contents string, offset int) (lineCount int, columnCount int, lineStart int, lineEnd int) {
	var prevCodePoint rune
	if offset > len(contents) {
		offset = len(contents)
	}

	// Scan up to the offset and count lines
	for i, codePoint := range contents[:offset] {
		switch codePoint {
		case '\n':
			lineStart = i + 1
			if prevCodePoint != '\r' {
				lineCount++
			}
		case '\r':
			lineStart = i + 1
			lineCount++
		case '\u2028', '\u2029':
			lineStart = i + 3 // These take three bytes to encode in UTF-8
			lineCount++
		}
		prevCodePoint = codePoint
	}

	// Scan to the end of the line (or end of file if this is the last line)
	lineEnd = len(contents)
loop:
	for i, codePoint := range contents[offset:] {
		switch codePoint {
		case '\r', '\n', '\u2028', '\u2029':
			lineEnd = offset + i
			break loop
		}
	}

	columnCount = offset - lineStart
	return
}

func locationOrNil(source *Source, r Range) *MsgLocation {
	if source == nil {
		return nil
	}

	// Convert the index into a line and column number
	lineCount, columnCount, lineStart, lineEnd := computeLineAndColumn(source.Contents, int(r.Loc.Start))

	return &MsgLocation{
		File:     source.PrettyPath,
		Line:     lineCount + 1, // 0-based to 1-based
		Column:   columnCount,
		Length:   int(r.Len),
		LineText: source.Contents[lineStart:lineEnd],
	}
}

func detailStruct(msg Msg, terminalInfo TerminalInfo) MsgDetail {
	// Only highlight the first line of the line text
	loc := *msg.Location
	endOfFirstLine := len(loc.LineText)
	for i, c := range loc.LineText {
		if c == '\r' || c == '\n' || c == '\u2028' || c == '\u2029' {
			endOfFirstLine = i
			break
		}
	}
	firstLine := loc.LineText[:endOfFirstLine]
	afterFirstLine := loc.LineText[endOfFirstLine:]

	// Clamp values in range
	if loc.Line < 0 {
		loc.Line = 0
	}
	if loc.Column < 0 {
		loc.Column = 0
	}
	if loc.Length < 0 {
		loc.Length = 0
	}
	if loc.Column > endOfFirstLine {
		loc.Column = endOfFirstLine
	}
	if loc.Length > endOfFirstLine-loc.Column {
		loc.Length = endOfFirstLine - loc.Column
	}

	spacesPerTab := 2
	lineText := renderTabStops(firstLine, spacesPerTab)
	indent := strings.Repeat(" ", len(renderTabStops(firstLine[:loc.Column], spacesPerTab)))
	marker := "^"
	markerStart := len(indent)
	markerEnd := len(indent)

	// Extend markers to cover the full range of the error
	if loc.Length > 0 {
		markerEnd = len(renderTabStops(firstLine[:loc.Column+loc.Length], spacesPerTab))
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
	if loc.Column == endOfFirstLine {
		// If the marker is at the very end of the line, the marker will be a "^"
		// character that extends one column past the end of the line. In this case
		// we should reserve a column at the end so the marker doesn't wrap.
		width -= 1
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
		Path:    loc.File,
		Line:    loc.Line,
		Column:  loc.Column,
		Kind:    kind,
		Message: msg.Text,

		Source:       lineText,
		SourceBefore: lineText[:markerStart],
		SourceMarked: lineText[markerStart:markerEnd],
		SourceAfter:  lineText[markerEnd:],

		Indent: indent,
		Marker: marker,

		ContentAfter: afterFirstLine,
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

func (log Log) AddError(source *Source, loc Loc, text string) {
	log.AddMsg(Msg{
		Kind:     Error,
		Text:     text,
		Location: locationOrNil(source, Range{Loc: loc}),
	})
}

func (log Log) AddWarning(source *Source, loc Loc, text string) {
	log.AddMsg(Msg{
		Kind:     Warning,
		Text:     text,
		Location: locationOrNil(source, Range{Loc: loc}),
	})
}

func (log Log) AddRangeError(source *Source, r Range, text string) {
	log.AddMsg(Msg{
		Kind:     Error,
		Text:     text,
		Location: locationOrNil(source, r),
	})
}

func (log Log) AddRangeWarning(source *Source, r Range, text string) {
	log.AddMsg(Msg{
		Kind:     Warning,
		Text:     text,
		Location: locationOrNil(source, r),
	})
}
