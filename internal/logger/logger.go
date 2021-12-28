package logger

// Logging is currently designed to look and feel like clang's error format.
// Errors are streamed asynchronously as they happen, each error contains the
// contents of the line with the error, and the error count is limited by
// default.

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const defaultTerminalWidth = 80

type Log struct {
	Level LogLevel

	AddMsg    func(Msg)
	HasErrors func() bool

	// This is called after the build has finished but before writing to stdout.
	// It exists to ensure that deferred warning messages end up in the terminal
	// before the data written to stdout.
	AlmostDone func()

	Done func() []Msg
}

type LogLevel int8

const (
	LevelNone LogLevel = iota
	LevelVerbose
	LevelDebug
	LevelInfo
	LevelWarning
	LevelError
	LevelSilent
)

type MsgKind uint8

const (
	Error MsgKind = iota
	Warning
	Info
	Note
	Debug
	Verbose
)

func (kind MsgKind) String() string {
	switch kind {
	case Error:
		return "ERROR"
	case Warning:
		return "WARNING"
	case Info:
		return "INFO"
	case Note:
		return "NOTE"
	case Debug:
		return "DEBUG"
	case Verbose:
		return "VERBOSE"
	default:
		panic("Internal error")
	}
}

func (kind MsgKind) Icon() string {
	// Special-case Windows command prompt, which only supports a few characters
	if isProbablyWindowsCommandPrompt() {
		switch kind {
		case Error:
			return "X"
		case Warning:
			return "▲"
		case Info:
			return "►"
		case Note:
			return "→"
		case Debug:
			return "●"
		case Verbose:
			return "♦"
		default:
			panic("Internal error")
		}
	}

	switch kind {
	case Error:
		return "✘"
	case Warning:
		return "▲"
	case Info:
		return "▶"
	case Note:
		return "→"
	case Debug:
		return "●"
	case Verbose:
		return "⬥"
	default:
		panic("Internal error")
	}
}

var windowsCommandPrompt struct {
	mutex         sync.Mutex
	once          bool
	isProbablyCMD bool
}

func isProbablyWindowsCommandPrompt() bool {
	windowsCommandPrompt.mutex.Lock()
	defer windowsCommandPrompt.mutex.Unlock()

	if !windowsCommandPrompt.once {
		windowsCommandPrompt.once = true

		// Assume we are running in Windows Command Prompt if we're on Windows. If
		// so, we can't use emoji because it won't be supported. Except we can
		// still use emoji if the WT_SESSION environment variable is present
		// because that means we're running in the new Windows Terminal instead.
		if runtime.GOOS == "windows" {
			windowsCommandPrompt.isProbablyCMD = true
			for _, env := range os.Environ() {
				if strings.HasPrefix(env, "WT_SESSION=") {
					windowsCommandPrompt.isProbablyCMD = false
					break
				}
			}
		}
	}

	return windowsCommandPrompt.isProbablyCMD
}

type Msg struct {
	PluginName string
	Kind       MsgKind
	Data       MsgData
	Notes      []MsgData
}

type MsgData struct {
	Text                string
	Location            *MsgLocation
	DisableMaximumWidth bool

	// Optional user-specified data that is passed through unmodified
	UserDetail interface{}
}

type MsgLocation struct {
	File       string
	Namespace  string
	Line       int // 1-based
	Column     int // 0-based, in bytes
	Length     int // in bytes
	LineText   string
	Suggestion string
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

type Span struct {
	Text  string
	Range Range
}

// This type is just so we can use Go's native sort function
type SortableMsgs []Msg

func (a SortableMsgs) Len() int          { return len(a) }
func (a SortableMsgs) Swap(i int, j int) { a[i], a[j] = a[j], a[i] }

func (a SortableMsgs) Less(i int, j int) bool {
	ai := a[i]
	aj := a[j]
	aiLoc := ai.Data.Location
	ajLoc := aj.Data.Location
	if aiLoc == nil || ajLoc == nil {
		return aiLoc == nil && ajLoc != nil
	}
	if aiLoc.File != ajLoc.File {
		return aiLoc.File < ajLoc.File
	}
	if aiLoc.Line != ajLoc.Line {
		return aiLoc.Line < ajLoc.Line
	}
	if aiLoc.Column != ajLoc.Column {
		return aiLoc.Column < ajLoc.Column
	}
	if ai.Kind != aj.Kind {
		return ai.Kind < aj.Kind
	}
	return ai.Data.Text < aj.Data.Text
}

// This is used to represent both file system paths (Namespace == "file") and
// abstract module paths (Namespace != "file"). Abstract module paths represent
// "virtual modules" when used for an input file and "package paths" when used
// to represent an external module.
type Path struct {
	Text      string
	Namespace string

	// This feature was added to support ancient CSS libraries that append things
	// like "?#iefix" and "#icons" to some of their import paths as a hack for IE6.
	// The intent is for these suffix parts to be ignored but passed through to
	// the output. This is supported by other bundlers, so we also support this.
	IgnoredSuffix string

	Flags PathFlags
}

type PathFlags uint8

const (
	// This corresponds to a value of "false' in the "browser" package.json field
	PathDisabled PathFlags = 1 << iota
)

func (p Path) IsDisabled() bool {
	return (p.Flags & PathDisabled) != 0
}

func (a Path) ComesBeforeInSortedOrder(b Path) bool {
	return a.Namespace > b.Namespace ||
		(a.Namespace == b.Namespace && (a.Text < b.Text ||
			(a.Text == b.Text && (a.Flags < b.Flags ||
				(a.Flags == b.Flags && a.IgnoredSuffix < b.IgnoredSuffix)))))
}

var noColorResult bool
var noColorOnce sync.Once

func hasNoColorEnvironmentVariable() bool {
	noColorOnce.Do(func() {
		for _, key := range os.Environ() {
			// Read "NO_COLOR" from the environment. This is a convention that some
			// software follows. See https://no-color.org/ for more information.
			if strings.HasPrefix(key, "NO_COLOR=") {
				noColorResult = true
			}
		}
	})
	return noColorResult
}

// This has a custom implementation instead of using "filepath.Dir/Base/Ext"
// because it should work the same on Unix and Windows. These names end up in
// the generated output and the generated output should not depend on the OS.
func PlatformIndependentPathDirBaseExt(path string) (dir string, base string, ext string) {
	for {
		i := strings.LastIndexAny(path, "/\\")

		// Stop if there are no more slashes
		if i < 0 {
			base = path
			break
		}

		// Stop if we found a non-trailing slash
		if i+1 != len(path) {
			dir, base = path[:i], path[i+1:]
			break
		}

		// Ignore trailing slashes
		path = path[:i]
	}

	// Strip off the extension
	if dot := strings.LastIndexByte(base, '.'); dot >= 0 {
		base, ext = base[:dot], base[dot:]
	}

	return
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

func (s *Source) LocBeforeWhitespace(loc Loc) Loc {
	for loc.Start > 0 {
		c, width := utf8.DecodeLastRuneInString(s.Contents[:loc.Start])
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		loc.Start -= int32(width)
	}
	return loc
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

func (s *Source) RangeOfLegacyOctalEscape(loc Loc) (r Range) {
	text := s.Contents[loc.Start:]
	r = Range{Loc: loc, Len: 0}

	if len(text) >= 2 && text[0] == '\\' {
		r.Len = 2
		for r.Len < 4 && int(r.Len) < len(text) {
			c := text[r.Len]
			if c < '0' || c > '9' {
				break
			}
			r.Len++
		}
	}
	return
}

func plural(prefix string, count int, shown int, someAreMissing bool) string {
	var text string
	if count == 1 {
		text = fmt.Sprintf("%d %s", count, prefix)
	} else {
		text = fmt.Sprintf("%d %ss", count, prefix)
	}
	if shown < count {
		text = fmt.Sprintf("%d of %s", shown, text)
	} else if someAreMissing && count > 1 {
		text = "all " + text
	}
	return text
}

func errorAndWarningSummary(errors int, warnings int, shownErrors int, shownWarnings int) string {
	someAreMissing := shownWarnings < warnings || shownErrors < errors
	switch {
	case errors == 0:
		return plural("warning", warnings, shownWarnings, someAreMissing)
	case warnings == 0:
		return plural("error", errors, shownErrors, someAreMissing)
	default:
		return fmt.Sprintf("%s and %s",
			plural("warning", warnings, shownWarnings, someAreMissing),
			plural("error", errors, shownErrors, someAreMissing))
	}
}

type APIKind uint8

const (
	GoAPI APIKind = iota
	CLIAPI
	JSAPI
)

// This can be used to customize error messages for the current API kind
var API APIKind

type TerminalInfo struct {
	IsTTY           bool
	UseColorEscapes bool
	Width           int
	Height          int
}

func NewStderrLog(options OutputOptions) Log {
	var mutex sync.Mutex
	var msgs SortableMsgs
	terminalInfo := GetTerminalInfo(os.Stderr)
	errors := 0
	warnings := 0
	shownErrors := 0
	shownWarnings := 0
	hasErrors := false
	remainingMessagesBeforeLimit := options.MessageLimit
	if remainingMessagesBeforeLimit == 0 {
		remainingMessagesBeforeLimit = 0x7FFFFFFF
	}
	var deferredWarnings []Msg
	didFinalizeLog := false

	finalizeLog := func() {
		if didFinalizeLog {
			return
		}
		didFinalizeLog = true

		// Print the deferred warning now if there was no error after all
		for remainingMessagesBeforeLimit > 0 && len(deferredWarnings) > 0 {
			shownWarnings++
			writeStringWithColor(os.Stderr, deferredWarnings[0].String(options, terminalInfo))
			deferredWarnings = deferredWarnings[1:]
			remainingMessagesBeforeLimit--
		}

		// Print out a summary
		if options.MessageLimit > 0 && errors+warnings > options.MessageLimit {
			writeStringWithColor(os.Stderr, fmt.Sprintf("%s shown (disable the message limit with --log-limit=0)\n",
				errorAndWarningSummary(errors, warnings, shownErrors, shownWarnings)))
		} else if options.LogLevel <= LevelInfo && (warnings != 0 || errors != 0) {
			writeStringWithColor(os.Stderr, fmt.Sprintf("%s\n",
				errorAndWarningSummary(errors, warnings, shownErrors, shownWarnings)))
		}
	}

	switch options.Color {
	case ColorNever:
		terminalInfo.UseColorEscapes = false
	case ColorAlways:
		terminalInfo.UseColorEscapes = SupportsColorEscapes
	}

	return Log{
		Level: options.LogLevel,

		AddMsg: func(msg Msg) {
			mutex.Lock()
			defer mutex.Unlock()
			msgs = append(msgs, msg)

			switch msg.Kind {
			case Verbose:
				if options.LogLevel <= LevelVerbose {
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
				}

			case Debug:
				if options.LogLevel <= LevelDebug {
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
				}

			case Info:
				if options.LogLevel <= LevelInfo {
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
				}

			case Error:
				hasErrors = true
				if options.LogLevel <= LevelError {
					errors++
				}

			case Warning:
				if options.LogLevel <= LevelWarning {
					warnings++
				}
			}

			// Be silent if we're past the limit so we don't flood the terminal
			if remainingMessagesBeforeLimit == 0 {
				return
			}

			switch msg.Kind {
			case Error:
				if options.LogLevel <= LevelError {
					shownErrors++
					writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
					remainingMessagesBeforeLimit--
				}

			case Warning:
				if options.LogLevel <= LevelWarning {
					if remainingMessagesBeforeLimit > (options.MessageLimit+1)/2 {
						shownWarnings++
						writeStringWithColor(os.Stderr, msg.String(options, terminalInfo))
						remainingMessagesBeforeLimit--
					} else {
						// If we have less than half of the slots left, wait for potential
						// future errors instead of using up all of the slots with warnings.
						// We want the log for a failed build to always have at least one
						// error in it.
						deferredWarnings = append(deferredWarnings, msg)
					}
				}
			}
		},

		HasErrors: func() bool {
			mutex.Lock()
			defer mutex.Unlock()
			return hasErrors
		},

		AlmostDone: func() {
			mutex.Lock()
			defer mutex.Unlock()

			finalizeLog()
		},

		Done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()

			finalizeLog()
			sort.Stable(msgs)
			return msgs
		},
	}
}

func PrintErrorToStderr(osArgs []string, text string) {
	PrintMessageToStderr(osArgs, Msg{Kind: Error, Data: MsgData{Text: text}})
}

func OutputOptionsForArgs(osArgs []string) OutputOptions {
	options := OutputOptions{IncludeSource: true}

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

	return options
}

func PrintMessageToStderr(osArgs []string, msg Msg) {
	log := NewStderrLog(OutputOptionsForArgs(osArgs))
	log.AddMsg(msg)
	log.Done()
}

type Colors struct {
	Reset     string
	Bold      string
	Dim       string
	Underline string

	Red   string
	Green string
	Blue  string

	Cyan    string
	Magenta string
	Yellow  string

	RedBgRed     string
	RedBgWhite   string
	GreenBgGreen string
	GreenBgWhite string
	BlueBgBlue   string
	BlueBgWhite  string

	CyanBgCyan       string
	CyanBgBlack      string
	MagentaBgMagenta string
	MagentaBgBlack   string
	YellowBgYellow   string
	YellowBgBlack    string
}

var TerminalColors = Colors{
	Reset:     "\033[0m",
	Bold:      "\033[1m",
	Dim:       "\033[37m",
	Underline: "\033[4m",

	Red:   "\033[31m",
	Green: "\033[32m",
	Blue:  "\033[34m",

	Cyan:    "\033[36m",
	Magenta: "\033[35m",
	Yellow:  "\033[33m",

	RedBgRed:     "\033[41;31m",
	RedBgWhite:   "\033[41;97m",
	GreenBgGreen: "\033[42;32m",
	GreenBgWhite: "\033[42;97m",
	BlueBgBlue:   "\033[44;34m",
	BlueBgWhite:  "\033[44;97m",

	CyanBgCyan:       "\033[46;36m",
	CyanBgBlack:      "\033[46;30m",
	MagentaBgMagenta: "\033[45;35m",
	MagentaBgBlack:   "\033[45;30m",
	YellowBgYellow:   "\033[43;33m",
	YellowBgBlack:    "\033[43;30m",
}

func PrintText(file *os.File, level LogLevel, osArgs []string, callback func(Colors) string) {
	options := OutputOptionsForArgs(osArgs)

	// Skip logging these if these logs are disabled
	if options.LogLevel > level {
		return
	}

	PrintTextWithColor(file, options.Color, callback)
}

func PrintTextWithColor(file *os.File, useColor UseColor, callback func(Colors) string) {
	var useColorEscapes bool
	switch useColor {
	case ColorNever:
		useColorEscapes = false
	case ColorAlways:
		useColorEscapes = SupportsColorEscapes
	case ColorIfTerminal:
		useColorEscapes = GetTerminalInfo(file).UseColorEscapes
	}

	var colors Colors
	if useColorEscapes {
		colors = TerminalColors
	}
	writeStringWithColor(file, callback(colors))
}

type SummaryTableEntry struct {
	Dir         string
	Base        string
	Size        string
	Bytes       int
	IsSourceMap bool
}

// This type is just so we can use Go's native sort function
type SummaryTable []SummaryTableEntry

func (t SummaryTable) Len() int          { return len(t) }
func (t SummaryTable) Swap(i int, j int) { t[i], t[j] = t[j], t[i] }

func (t SummaryTable) Less(i int, j int) bool {
	ti := t[i]
	tj := t[j]

	// Sort source maps last
	if !ti.IsSourceMap && tj.IsSourceMap {
		return true
	}
	if ti.IsSourceMap && !tj.IsSourceMap {
		return false
	}

	// Sort by size first
	if ti.Bytes > tj.Bytes {
		return true
	}
	if ti.Bytes < tj.Bytes {
		return false
	}

	// Sort alphabetically by directory first
	if ti.Dir < tj.Dir {
		return true
	}
	if ti.Dir > tj.Dir {
		return false
	}

	// Then sort alphabetically by file name
	return ti.Base < tj.Base
}

// Show a warning icon next to output files that are 1mb or larger
const sizeWarningThreshold = 1024 * 1024

func PrintSummary(useColor UseColor, table SummaryTable, start *time.Time) {
	PrintTextWithColor(os.Stderr, useColor, func(colors Colors) string {
		isProbablyWindowsCommandPrompt := isProbablyWindowsCommandPrompt()
		sb := strings.Builder{}

		if len(table) > 0 {
			info := GetTerminalInfo(os.Stderr)

			// Truncate the table in case it's really long
			maxLength := info.Height / 2
			if info.Height == 0 {
				maxLength = 20
			} else if maxLength < 5 {
				maxLength = 5
			}
			length := len(table)
			sort.Sort(table)
			if length > maxLength {
				table = table[:maxLength]
			}

			// Compute the maximum width of the size column
			spacingBetweenColumns := 2
			hasSizeWarning := false
			maxPath := 0
			maxSize := 0
			for _, entry := range table {
				path := len(entry.Dir) + len(entry.Base)
				size := len(entry.Size) + spacingBetweenColumns
				if path > maxPath {
					maxPath = path
				}
				if size > maxSize {
					maxSize = size
				}
				if !entry.IsSourceMap && entry.Bytes >= sizeWarningThreshold {
					hasSizeWarning = true
				}
			}

			margin := "  "
			layoutWidth := info.Width
			if layoutWidth < 1 {
				layoutWidth = defaultTerminalWidth
			}
			layoutWidth -= 2 * len(margin)
			if hasSizeWarning {
				// Add space for the warning icon
				layoutWidth -= 2
			}
			if layoutWidth > maxPath+maxSize {
				layoutWidth = maxPath + maxSize
			}
			sb.WriteByte('\n')

			for _, entry := range table {
				dir, base := entry.Dir, entry.Base
				pathWidth := layoutWidth - maxSize

				// Truncate the path with "..." to fit on one line
				if len(dir)+len(base) > pathWidth {
					// Trim the directory from the front, leaving the trailing slash
					if len(dir) > 0 {
						n := pathWidth - len(base) - 3
						if n < 1 {
							n = 1
						}
						dir = "..." + dir[len(dir)-n:]
					}

					// Trim the file name from the back
					if len(dir)+len(base) > pathWidth {
						n := pathWidth - len(dir) - 3
						if n < 0 {
							n = 0
						}
						base = base[:n] + "..."
					}
				}

				spacer := layoutWidth - len(entry.Size) - len(dir) - len(base)
				if spacer < 0 {
					spacer = 0
				}

				// Put a warning next to the size if it's above a certain threshold
				sizeColor := colors.Cyan
				sizeWarning := ""
				if !entry.IsSourceMap && entry.Bytes >= sizeWarningThreshold {
					sizeColor = colors.Yellow

					// Emoji don't work in Windows Command Prompt
					if !isProbablyWindowsCommandPrompt {
						sizeWarning = " ⚠️"
					}
				}

				sb.WriteString(fmt.Sprintf("%s%s%s%s%s%s%s%s%s%s%s%s\n",
					margin,
					colors.Dim,
					dir,
					colors.Reset,
					colors.Bold,
					base,
					colors.Reset,
					strings.Repeat(" ", spacer),
					sizeColor,
					entry.Size,
					sizeWarning,
					colors.Reset,
				))
			}

			// Say how many remaining files are not shown
			if length > maxLength {
				plural := "s"
				if length == maxLength+1 {
					plural = ""
				}
				sb.WriteString(fmt.Sprintf("%s%s...and %d more output file%s...%s\n", margin, colors.Dim, length-maxLength, plural, colors.Reset))
			}
		}
		sb.WriteByte('\n')

		lightningSymbol := "⚡ "

		// Emoji don't work in Windows Command Prompt
		if isProbablyWindowsCommandPrompt {
			lightningSymbol = ""
		}

		// Printing the time taken is optional
		if start != nil {
			sb.WriteString(fmt.Sprintf("%s%sDone in %dms%s\n",
				lightningSymbol,
				colors.Green,
				time.Since(*start).Milliseconds(),
				colors.Reset,
			))
		}

		return sb.String()
	})
}

type DeferLogKind uint8

const (
	DeferLogAll DeferLogKind = iota
	DeferLogNoVerboseOrDebug
)

func NewDeferLog(kind DeferLogKind) Log {
	var msgs SortableMsgs
	var mutex sync.Mutex
	var hasErrors bool

	return Log{
		Level: LevelInfo,

		AddMsg: func(msg Msg) {
			if kind == DeferLogNoVerboseOrDebug && (msg.Kind == Verbose || msg.Kind == Debug) {
				return
			}
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

		AlmostDone: func() {
		},

		Done: func() []Msg {
			mutex.Lock()
			defer mutex.Unlock()
			sort.Stable(msgs)
			return msgs
		},
	}
}

type UseColor uint8

const (
	ColorIfTerminal UseColor = iota
	ColorNever
	ColorAlways
)

type OutputOptions struct {
	IncludeSource bool
	MessageLimit  int
	Color         UseColor
	LogLevel      LogLevel
}

func (msg Msg) String(options OutputOptions, terminalInfo TerminalInfo) string {
	// Format the message
	text := msgString(options.IncludeSource, terminalInfo, msg.Kind, msg.Data, msg.PluginName)

	// Format the notes
	var oldData MsgData
	for i, note := range msg.Notes {
		if options.IncludeSource && (i == 0 || strings.IndexByte(oldData.Text, '\n') >= 0 || oldData.Location != nil) {
			text += "\n"
		}
		text += msgString(options.IncludeSource, terminalInfo, Note, note, "")
		oldData = note
	}

	// Add extra spacing between messages if source code is present
	if options.IncludeSource {
		text += "\n"
	}
	return text
}

// The number of margin characters in addition to the line number
const extraMarginChars = 9

func marginWithLineText(maxMargin int, line int) string {
	number := fmt.Sprintf("%d", line)
	return fmt.Sprintf("      %s%s │ ", strings.Repeat(" ", maxMargin-len(number)), number)
}

func emptyMarginText(maxMargin int, isLast bool) string {
	space := strings.Repeat(" ", maxMargin)
	if isLast {
		return fmt.Sprintf("      %s ╵ ", space)
	}
	return fmt.Sprintf("      %s │ ", space)
}

func msgString(includeSource bool, terminalInfo TerminalInfo, kind MsgKind, data MsgData, pluginName string) string {
	if !includeSource {
		if loc := data.Location; loc != nil {
			return fmt.Sprintf("%s: %s: %s\n", loc.File, kind.String(), data.Text)
		}
		return fmt.Sprintf("%s: %s\n", kind.String(), data.Text)
	}

	var colors Colors
	if terminalInfo.UseColorEscapes {
		colors = TerminalColors
	}

	var iconColor string
	var kindColorBrackets string
	var kindColorText string

	location := ""

	if data.Location != nil {
		maxMargin := len(fmt.Sprintf("%d", data.Location.Line))
		d := detailStruct(data, terminalInfo, maxMargin)

		if d.Suggestion != "" {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s\n%s%s%s%s%s\n%s%s%s%s%s\n%s",
				d.Path, d.Line, d.Column,
				colors.Dim, d.SourceBefore, colors.Green, d.SourceMarked, colors.Dim, d.SourceAfter,
				emptyMarginText(maxMargin, false), d.Indent, colors.Green, d.Marker, colors.Dim,
				emptyMarginText(maxMargin, true), d.Indent, colors.Green, d.Suggestion, colors.Reset,
				d.ContentAfter,
			)
		} else {
			location = fmt.Sprintf("\n    %s:%d:%d:\n%s%s%s%s%s%s\n%s%s%s%s%s\n%s",
				d.Path, d.Line, d.Column,
				colors.Dim, d.SourceBefore, colors.Green, d.SourceMarked, colors.Dim, d.SourceAfter,
				emptyMarginText(maxMargin, true), d.Indent, colors.Green, d.Marker, colors.Reset,
				d.ContentAfter,
			)
		}
	}

	switch kind {
	case Verbose:
		iconColor = colors.Cyan
		kindColorBrackets = colors.CyanBgCyan
		kindColorText = colors.CyanBgBlack

	case Debug:
		iconColor = colors.Green
		kindColorBrackets = colors.GreenBgGreen
		kindColorText = colors.GreenBgWhite

	case Info:
		iconColor = colors.Blue
		kindColorBrackets = colors.BlueBgBlue
		kindColorText = colors.BlueBgWhite

	case Error:
		iconColor = colors.Red
		kindColorBrackets = colors.RedBgRed
		kindColorText = colors.RedBgWhite

	case Warning:
		iconColor = colors.Yellow
		kindColorBrackets = colors.YellowBgYellow
		kindColorText = colors.YellowBgBlack

	case Note:
		sb := strings.Builder{}

		for _, line := range strings.Split(data.Text, "\n") {
			// Special-case word wrapping
			if wrapWidth := terminalInfo.Width; wrapWidth > 2 {
				if !data.DisableMaximumWidth && wrapWidth > 100 {
					wrapWidth = 100 // Enforce a maximum paragraph width for readability
				}
				for _, run := range wrapWordsInString(line, wrapWidth-2) {
					sb.WriteString("  ")
					sb.WriteString(linkifyText(run, colors.Underline, colors.Reset))
					sb.WriteByte('\n')
				}
				continue
			}

			// Otherwise, just write an indented line
			sb.WriteString("  ")
			sb.WriteString(linkifyText(line, colors.Underline, colors.Reset))
			sb.WriteByte('\n')
		}

		sb.WriteString(location)
		return sb.String()
	}

	if pluginName != "" {
		pluginName = fmt.Sprintf("%s%s[plugin %s]%s ", colors.Bold, colors.Magenta, pluginName, colors.Reset)
	}

	return fmt.Sprintf("%s%s %s[%s%s%s]%s %s%s%s%s\n%s",
		iconColor, kind.Icon(),
		kindColorBrackets, kindColorText, kind.String(), kindColorBrackets, colors.Reset,
		pluginName,
		colors.Bold, data.Text, colors.Reset,
		location,
	)
}

func linkifyText(text string, underline string, reset string) string {
	if underline == "" {
		return text
	}

	https := strings.Index(text, "https://")
	if https == -1 {
		return text
	}

	sb := strings.Builder{}
	for {
		https := strings.Index(text, "https://")
		if https == -1 {
			break
		}

		end := strings.IndexByte(text[https:], ' ')
		if end == -1 {
			end = len(text)
		} else {
			end += https
		}

		// Remove trailing punctuation
		if end > https {
			switch text[end-1] {
			case '.', ',', '?', '!', ')', ']', '}':
				end--
			}
		}

		sb.WriteString(text[:https])
		sb.WriteString(underline)
		sb.WriteString(text[https:end])
		sb.WriteString(reset)
		text = text[end:]
	}

	sb.WriteString(text)
	return sb.String()
}

func wrapWordsInString(text string, width int) []string {
	runs := []string{}

outer:
	for text != "" {
		i := 0
		x := 0
		wordEndI := 0

		// Skip over any leading spaces
		for i < len(text) && text[i] == ' ' {
			i++
			x++
		}

		// Find out how many words will fit in this run
		for i < len(text) {
			oldWordEndI := wordEndI
			wordStartI := i

			// Find the end of the word
			for i < len(text) {
				c, width := utf8.DecodeRuneInString(text[i:])
				if c == ' ' {
					break
				}
				i += width
				x += 1 // Naively assume that each unicode code point is a single column
			}
			wordEndI = i

			// Split into a new run if this isn't the first word in the run and the end is past the width
			if wordStartI > 0 && x > width {
				runs = append(runs, text[:oldWordEndI])
				text = text[wordStartI:]
				continue outer
			}

			// Skip over any spaces after the word
			for i < len(text) && text[i] == ' ' {
				i++
				x++
			}
		}

		// If we get here, this is the last run (i.e. everything fits)
		break
	}

	// Remove any trailing spaces on the last run
	for len(text) > 0 && text[len(text)-1] == ' ' {
		text = text[:len(text)-1]
	}
	runs = append(runs, text)
	return runs
}

type MsgDetail struct {
	Path   string
	Line   int
	Column int

	SourceBefore string
	SourceMarked string
	SourceAfter  string

	Indent     string
	Marker     string
	Suggestion string

	ContentAfter string
}

// It's not common for large files to have many warnings. But when it happens,
// we want to make sure that it's not too slow. Source code locations are
// represented as byte offsets for compactness but transforming these to
// line/column locations for warning messages requires scanning through the
// file. A naive approach for this would cause O(n^2) scanning time for n
// warnings distributed throughout the file.
//
// Warnings are typically generated sequentially as the file is scanned. So
// one way of optimizing this is to just start scanning from where we left
// off last time instead of always starting from the beginning of the file.
// That's what this object does.
//
// Another option could be to eagerly populate an array of line/column offsets
// and then use binary search for each query. This might slow down the common
// case of a file with only at most a few warnings though, so think before
// optimizing too much. Performance in the zero or one warning case is by far
// the most important.
type LineColumnTracker struct {
	contents     string
	prettyPath   string
	offset       int32
	line         int32
	lineStart    int32
	lineEnd      int32
	hasLineStart bool
	hasLineEnd   bool
	hasSource    bool
}

func MakeLineColumnTracker(source *Source) LineColumnTracker {
	if source == nil {
		return LineColumnTracker{
			hasSource: false,
		}
	}

	return LineColumnTracker{
		contents:     source.Contents,
		prettyPath:   source.PrettyPath,
		hasLineStart: true,
		hasSource:    true,
	}
}

func (tracker *LineColumnTracker) MsgData(r Range, text string) MsgData {
	return MsgData{
		Text:     text,
		Location: tracker.MsgLocationOrNil(r),
	}
}

func (t *LineColumnTracker) scanTo(offset int32) {
	contents := t.contents
	i := t.offset

	// Scan forward
	if i < offset {
		for {
			r, size := utf8.DecodeRuneInString(contents[i:])
			i += int32(size)

			switch r {
			case '\n':
				t.hasLineStart = true
				t.hasLineEnd = false
				t.lineStart = i
				if i == int32(size) || contents[i-int32(size)-1] != '\r' {
					t.line++
				}

			case '\r', '\u2028', '\u2029':
				t.hasLineStart = true
				t.hasLineEnd = false
				t.lineStart = i
				t.line++
			}

			if i >= offset {
				t.offset = i
				return
			}
		}
	}

	// Scan backward
	if i > offset {
		for {
			r, size := utf8.DecodeLastRuneInString(contents[:i])
			i -= int32(size)

			switch r {
			case '\n':
				t.hasLineStart = false
				t.hasLineEnd = true
				t.lineEnd = i
				if i == 0 || contents[i-1] != '\r' {
					t.line--
				}

			case '\r', '\u2028', '\u2029':
				t.hasLineStart = false
				t.hasLineEnd = true
				t.lineEnd = i
				t.line--
			}

			if i <= offset {
				t.offset = i
				return
			}
		}
	}
}

func (t *LineColumnTracker) computeLineAndColumn(offset int) (lineCount int, columnCount int, lineStart int, lineEnd int) {
	t.scanTo(int32(offset))

	// Scan for the start of the line
	if !t.hasLineStart {
		contents := t.contents
		i := t.offset
		for i > 0 {
			r, size := utf8.DecodeLastRuneInString(contents[:i])
			if r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
				break
			}
			i -= int32(size)
		}
		t.hasLineStart = true
		t.lineStart = i
	}

	// Scan for the end of the line
	if !t.hasLineEnd {
		contents := t.contents
		i := t.offset
		n := int32(len(contents))
		for i < n {
			r, size := utf8.DecodeRuneInString(contents[i:])
			if r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
				break
			}
			i += int32(size)
		}
		t.hasLineEnd = true
		t.lineEnd = i
	}

	return int(t.line), offset - int(t.lineStart), int(t.lineStart), int(t.lineEnd)
}

func (tracker *LineColumnTracker) MsgLocationOrNil(r Range) *MsgLocation {
	if tracker == nil || !tracker.hasSource {
		return nil
	}

	// Convert the index into a line and column number
	lineCount, columnCount, lineStart, lineEnd := tracker.computeLineAndColumn(int(r.Loc.Start))

	return &MsgLocation{
		File:     tracker.prettyPath,
		Line:     lineCount + 1, // 0-based to 1-based
		Column:   columnCount,
		Length:   int(r.Len),
		LineText: tracker.contents[lineStart:lineEnd],
	}
}

func detailStruct(data MsgData, terminalInfo TerminalInfo, maxMargin int) MsgDetail {
	// Only highlight the first line of the line text
	loc := *data.Location
	endOfFirstLine := len(loc.LineText)
	for i, c := range loc.LineText {
		if c == '\r' || c == '\n' || c == '\u2028' || c == '\u2029' {
			endOfFirstLine = i
			break
		}
	}
	firstLine := loc.LineText[:endOfFirstLine]
	afterFirstLine := loc.LineText[endOfFirstLine:]
	if afterFirstLine != "" && !strings.HasSuffix(afterFirstLine, "\n") {
		afterFirstLine += "\n"
	}

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
	textUpToLoc := renderTabStops(firstLine[:loc.Column], spacesPerTab)
	markerStart := len(textUpToLoc)
	markerEnd := markerStart
	indent := strings.Repeat(" ", estimateWidthInTerminal(textUpToLoc))
	marker := "^"

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
		width = defaultTerminalWidth
	}
	width -= maxMargin + extraMarginChars
	if width < 1 {
		width = 1
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
		lineText = slicedLine
		indent = strings.Repeat(" ", estimateWidthInTerminal(lineText[:markerStart]))
	}

	// If marker is still multi-character after clipping, make the marker wider
	if markerEnd-markerStart > 1 {
		marker = strings.Repeat("~", estimateWidthInTerminal(lineText[markerStart:markerEnd]))
	}

	// Put a margin before the marker indent
	margin := marginWithLineText(maxMargin, loc.Line)

	return MsgDetail{
		Path:   loc.File,
		Line:   loc.Line,
		Column: loc.Column,

		SourceBefore: margin + lineText[:markerStart],
		SourceMarked: lineText[markerStart:markerEnd],
		SourceAfter:  lineText[markerEnd:],

		Indent:     indent,
		Marker:     marker,
		Suggestion: loc.Suggestion,

		ContentAfter: afterFirstLine,
	}
}

// Estimate the number of columns this string will take when printed
func estimateWidthInTerminal(text string) int {
	// For now just assume each code point is one column. This is wrong but is
	// less wrong than assuming each code unit is one column.
	width := 0
	for text != "" {
		c, size := utf8.DecodeRuneInString(text)
		text = text[size:]

		// Ignore the Zero Width No-Break Space character (UTF-8 BOM)
		if c != 0xFEFF {
			width++
		}
	}
	return width
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

func (log Log) Add(kind MsgKind, tracker *LineColumnTracker, r Range, text string) {
	log.AddMsg(Msg{
		Kind: kind,
		Data: tracker.MsgData(r, text),
	})
}

func (log Log) AddWithNotes(kind MsgKind, tracker *LineColumnTracker, r Range, text string, notes []MsgData) {
	log.AddMsg(Msg{
		Kind:  kind,
		Data:  tracker.MsgData(r, text),
		Notes: notes,
	})
}
