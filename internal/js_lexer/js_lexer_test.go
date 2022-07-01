package js_lexer

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

func assertEqualStrings(t *testing.T, a string, b string) {
	t.Helper()
	pretty := func(text string) string {
		builder := strings.Builder{}
		builder.WriteRune('"')
		i := 0
		for i < len(text) {
			c, width := utf8.DecodeRuneInString(text[i:])
			builder.WriteString(fmt.Sprintf("\\u{%X}", c))
			i += width
		}
		builder.WriteRune('"')
		return builder.String()
	}
	if a != b {
		t.Fatalf("%s != %s", pretty(a), pretty(b))
	}
}

func lexToken(t *testing.T, contents string) T {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	lexer := NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
	return lexer.Token
}

func expectLexerError(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		func() {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func TestComment(t *testing.T) {
	expectLexerError(t, "/*", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectLexerError(t, "/*/", "<stdin>: ERROR: Expected \"*/\" to terminate multi-line comment\n<stdin>: NOTE: The multi-line comment starts here:\n")
	expectLexerError(t, "/**/", "")
	expectLexerError(t, "//", "")
}

func expectHashbang(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		msgs := log.Done()
		test.AssertEqual(t, len(msgs), 0)
		test.AssertEqual(t, lexer.Token, THashbang)
		test.AssertEqual(t, lexer.Identifier.String, expected)
	})
}

func TestHashbang(t *testing.T) {
	expectHashbang(t, "#!/usr/bin/env node", "#!/usr/bin/env node")
	expectHashbang(t, "#!/usr/bin/env node\n", "#!/usr/bin/env node")
	expectHashbang(t, "#!/usr/bin/env node\nlet x", "#!/usr/bin/env node")
	expectLexerError(t, " #!/usr/bin/env node", "<stdin>: ERROR: Syntax error \"!\"\n")
}

func expectIdentifier(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		msgs := log.Done()
		test.AssertEqual(t, len(msgs), 0)
		test.AssertEqual(t, lexer.Token, TIdentifier)
		test.AssertEqual(t, lexer.Identifier.String, expected)
	})
}

func TestIdentifier(t *testing.T) {
	expectIdentifier(t, "_", "_")
	expectIdentifier(t, "$", "$")
	expectIdentifier(t, "test", "test")
	expectIdentifier(t, "t\\u0065st", "test")
	expectIdentifier(t, "t\\u{65}st", "test")

	expectLexerError(t, "t\\u.", "<stdin>: ERROR: Syntax error \".\"\n")
	expectLexerError(t, "t\\u0.", "<stdin>: ERROR: Syntax error \".\"\n")
	expectLexerError(t, "t\\u00.", "<stdin>: ERROR: Syntax error \".\"\n")
	expectLexerError(t, "t\\u006.", "<stdin>: ERROR: Syntax error \".\"\n")
	expectLexerError(t, "t\\u{.", "<stdin>: ERROR: Syntax error \".\"\n")
	expectLexerError(t, "t\\u{0.", "<stdin>: ERROR: Syntax error \".\"\n")

	expectIdentifier(t, "a\u200C", "a\u200C")
	expectIdentifier(t, "a\u200D", "a\u200D")
}

func expectNumber(t *testing.T, contents string, expected float64) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		msgs := log.Done()
		test.AssertEqual(t, len(msgs), 0)
		test.AssertEqual(t, lexer.Token, TNumericLiteral)
		test.AssertEqual(t, lexer.Number, expected)
	})
}

func TestNumericLiteral(t *testing.T) {
	expectNumber(t, "0", 0.0)
	expectNumber(t, "000", 0.0)
	expectNumber(t, "010", 8.0)
	expectNumber(t, "123", 123.0)
	expectNumber(t, "987", 987.0)
	expectNumber(t, "0000", 0.0)
	expectNumber(t, "0123", 83.0)
	expectNumber(t, "0123.4567", 83.0)
	expectNumber(t, "0987", 987.0)
	expectNumber(t, "0987.6543", 987.6543)
	expectNumber(t, "01289", 1289.0)
	expectNumber(t, "01289.345", 1289.0)
	expectNumber(t, "999999999", 999999999.0)
	expectNumber(t, "9999999999", 9999999999.0)
	expectNumber(t, "99999999999", 99999999999.0)
	expectNumber(t, "123456789123456789", 123456789123456780.0)
	expectNumber(t, "123456789123456789"+strings.Repeat("0", 128), 1.2345678912345679e+145)

	expectNumber(t, "0b00101", 5.0)
	expectNumber(t, "0B00101", 5.0)
	expectNumber(t, "0b1011101011101011101011101011101011101", 100352251741.0)
	expectNumber(t, "0B1011101011101011101011101011101011101", 100352251741.0)
	expectLexerError(t, "0b", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0B", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0b012", "<stdin>: ERROR: Syntax error \"2\"\n")
	expectLexerError(t, "0b018", "<stdin>: ERROR: Syntax error \"8\"\n")
	expectLexerError(t, "0b01a", "<stdin>: ERROR: Syntax error \"a\"\n")
	expectLexerError(t, "0b01A", "<stdin>: ERROR: Syntax error \"A\"\n")

	expectNumber(t, "0o12345", 5349.0)
	expectNumber(t, "0O12345", 5349.0)
	expectNumber(t, "0o1234567654321", 89755965649.0)
	expectNumber(t, "0O1234567654321", 89755965649.0)
	expectLexerError(t, "0o", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0O", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0o018", "<stdin>: ERROR: Syntax error \"8\"\n")
	expectLexerError(t, "0o01a", "<stdin>: ERROR: Syntax error \"a\"\n")
	expectLexerError(t, "0o01A", "<stdin>: ERROR: Syntax error \"A\"\n")

	expectNumber(t, "0x12345678", float64(0x12345678))
	expectNumber(t, "0xFEDCBA987", float64(0xFEDCBA987))
	expectNumber(t, "0x000012345678", float64(0x12345678))
	expectNumber(t, "0x123456781234", float64(0x123456781234))
	expectLexerError(t, "0x", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0X", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "0xGFEDCBA", "<stdin>: ERROR: Syntax error \"G\"\n")
	expectLexerError(t, "0xABCDEFG", "<stdin>: ERROR: Syntax error \"G\"\n")

	expectNumber(t, "123.", 123.0)
	expectNumber(t, ".0123", 0.0123)
	expectNumber(t, "0.0123", 0.0123)
	expectNumber(t, "2.2250738585072014e-308", 2.2250738585072014e-308)
	expectNumber(t, "1.7976931348623157e+308", 1.7976931348623157e+308)

	// Underflow
	expectNumber(t, "4.9406564584124654417656879286822e-324", 5e-324)
	expectNumber(t, "5e-324", 5e-324)
	expectNumber(t, "1e-325", 0.0)

	// Overflow
	expectNumber(t, "1.797693134862315708145274237317e+308", 1.7976931348623157e+308)
	expectNumber(t, "1.797693134862315808e+308", math.Inf(1))
	expectNumber(t, "1e+309", math.Inf(1))

	// int32
	expectNumber(t, "0x7fff_ffff", 2147483647.0)
	expectNumber(t, "0x8000_0000", 2147483648.0)
	expectNumber(t, "0x8000_0001", 2147483649.0)

	// uint32
	expectNumber(t, "0xffff_ffff", 4294967295.0)
	expectNumber(t, "0x1_0000_0000", 4294967296.0)
	expectNumber(t, "0x1_0000_0001", 4294967297.0)

	// int64
	expectNumber(t, "0x7fff_ffff_ffff_fdff", 9223372036854774784)
	expectNumber(t, "0x8000_0000_0000_0000", 9.223372036854776e+18)
	expectNumber(t, "0x8000_0000_0000_3000", 9.223372036854788e+18)

	// uint64
	expectNumber(t, "0xffff_ffff_ffff_fbff", 1.844674407370955e+19)
	expectNumber(t, "0x1_0000_0000_0000_0000", 1.8446744073709552e+19)
	expectNumber(t, "0x1_0000_0000_0000_1000", 1.8446744073709556e+19)

	expectNumber(t, "1.", 1.0)
	expectNumber(t, ".1", 0.1)
	expectNumber(t, "1.1", 1.1)
	expectNumber(t, "1e1", 10.0)
	expectNumber(t, "1e+1", 10.0)
	expectNumber(t, "1e-1", 0.1)
	expectNumber(t, ".1e1", 1.0)
	expectNumber(t, ".1e+1", 1.0)
	expectNumber(t, ".1e-1", 0.01)
	expectNumber(t, "1.e1", 10.0)
	expectNumber(t, "1.e+1", 10.0)
	expectNumber(t, "1.e-1", 0.1)
	expectNumber(t, "1.1e1", 11.0)
	expectNumber(t, "1.1e+1", 11.0)
	expectNumber(t, "1.1e-1", 0.11)

	expectLexerError(t, "1e", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, ".1e", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.e", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.1e", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1e+", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, ".1e+", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.e+", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.1e+", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1e-", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, ".1e-", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.e-", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1.1e-", "<stdin>: ERROR: Unexpected end of file\n")
	expectLexerError(t, "1e+-1", "<stdin>: ERROR: Syntax error \"-\"\n")
	expectLexerError(t, "1e-+1", "<stdin>: ERROR: Syntax error \"+\"\n")

	expectLexerError(t, "1z", "<stdin>: ERROR: Syntax error \"z\"\n")
	expectLexerError(t, "1.z", "<stdin>: ERROR: Syntax error \"z\"\n")
	expectLexerError(t, "1.0f", "<stdin>: ERROR: Syntax error \"f\"\n")
	expectLexerError(t, "0b1z", "<stdin>: ERROR: Syntax error \"z\"\n")
	expectLexerError(t, "0o1z", "<stdin>: ERROR: Syntax error \"z\"\n")
	expectLexerError(t, "0x1z", "<stdin>: ERROR: Syntax error \"z\"\n")
	expectLexerError(t, "1e1z", "<stdin>: ERROR: Syntax error \"z\"\n")

	expectNumber(t, "1_2_3", 123)
	expectNumber(t, ".1_2", 0.12)
	expectNumber(t, "1_2.3_4", 12.34)
	expectNumber(t, "1e2_3", 1e23)
	expectNumber(t, "1_2e3_4", 12e34)
	expectNumber(t, "1_2.3_4e5_6", 12.34e56)
	expectNumber(t, "0b1_0", 2)
	expectNumber(t, "0B1_0", 2)
	expectNumber(t, "0o1_2", 10)
	expectNumber(t, "0O1_2", 10)
	expectNumber(t, "0x1_2", 0x12)
	expectNumber(t, "0X1_2", 0x12)
	expectNumber(t, "08.0_1", 8.01)
	expectNumber(t, "09.0_1", 9.01)

	expectLexerError(t, "0_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0_7", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0_8", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0_9", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "00_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "01_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "07_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "08_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "09_0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "08_0.1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "09_0.1", "<stdin>: ERROR: Syntax error \"_\"\n")

	expectLexerError(t, "1__2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, ".1__2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1e2__3", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0b1__0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0B1__0", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0o1__2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0O1__2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0x1__2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0X1__2", "<stdin>: ERROR: Syntax error \"_\"\n")

	expectLexerError(t, "1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1._", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1_.", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, ".1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1e_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1e1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1_e1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, ".1_e1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1._2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "1_.2", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0b_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0B_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0o_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0O_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0x_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0X_1", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0b1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0B1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0o1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0O1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0x1_", "<stdin>: ERROR: Syntax error \"_\"\n")
	expectLexerError(t, "0X1_", "<stdin>: ERROR: Syntax error \"_\"\n")
}

func expectBigInteger(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		msgs := log.Done()
		test.AssertEqual(t, len(msgs), 0)
		test.AssertEqual(t, lexer.Token, TBigIntegerLiteral)
		test.AssertEqual(t, lexer.Identifier.String, expected)
	})
}

func TestBigIntegerLiteral(t *testing.T) {
	expectBigInteger(t, "0n", "0")
	expectBigInteger(t, "123n", "123")
	expectBigInteger(t, "9007199254740993n", "9007199254740993") // This can't fit in a float64

	expectBigInteger(t, "0b00101n", "0b00101")
	expectBigInteger(t, "0B00101n", "0B00101")
	expectBigInteger(t, "0b1011101011101011101011101011101011101n", "0b1011101011101011101011101011101011101")
	expectBigInteger(t, "0B1011101011101011101011101011101011101n", "0B1011101011101011101011101011101011101")

	expectBigInteger(t, "0o12345n", "0o12345")
	expectBigInteger(t, "0O12345n", "0O12345")
	expectBigInteger(t, "0o1234567654321n", "0o1234567654321")
	expectBigInteger(t, "0O1234567654321n", "0O1234567654321")

	expectBigInteger(t, "0x12345678n", "0x12345678")
	expectBigInteger(t, "0xFEDCBA987n", "0xFEDCBA987")
	expectBigInteger(t, "0x000012345678n", "0x000012345678")
	expectBigInteger(t, "0x123456781234n", "0x123456781234")

	expectBigInteger(t, "1_2_3n", "123")
	expectBigInteger(t, "0b1_0_1n", "0b101")
	expectBigInteger(t, "0o1_2_3n", "0o123")
	expectBigInteger(t, "0x1_2_3n", "0x123")

	expectLexerError(t, "1e2n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, "1.0n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, ".1n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, "000n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, "0123n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, "089n", "<stdin>: ERROR: Syntax error \"n\"\n")
	expectLexerError(t, "0_1n", "<stdin>: ERROR: Syntax error \"_\"\n")
}

func expectString(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
		}()
		text := lexer.StringLiteral()
		msgs := log.Done()
		test.AssertEqual(t, len(msgs), 0)
		test.AssertEqual(t, lexer.Token, TStringLiteral)
		assertEqualStrings(t, helpers.UTF16ToString(text), expected)
	})
}

func expectLexerErrorString(t *testing.T, contents string, expected string) {
	t.Helper()
	t.Run(contents, func(t *testing.T) {
		t.Helper()
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		func() {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			lexer := NewLexer(log, test.SourceForTest(contents), config.TSOptions{})
			lexer.StringLiteral()
		}()
		msgs := log.Done()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logger.OutputOptions{}, logger.TerminalInfo{})
		}
		test.AssertEqual(t, text, expected)
	})
}

func TestStringLiteral(t *testing.T) {
	expectString(t, "''", "")
	expectString(t, "'123'", "123")

	expectString(t, "'\"'", "\"")
	expectString(t, "'\\''", "'")
	expectString(t, "'\\\"'", "\"")
	expectString(t, "'\\\\'", "\\")
	expectString(t, "'\\a'", "a")
	expectString(t, "'\\b'", "\b")
	expectString(t, "'\\f'", "\f")
	expectString(t, "'\\n'", "\n")
	expectString(t, "'\\r'", "\r")
	expectString(t, "'\\t'", "\t")
	expectString(t, "'\\v'", "\v")

	expectString(t, "'\\0'", "\000")
	expectString(t, "'\\1'", "\001")
	expectString(t, "'\\2'", "\002")
	expectString(t, "'\\3'", "\003")
	expectString(t, "'\\4'", "\004")
	expectString(t, "'\\5'", "\005")
	expectString(t, "'\\6'", "\006")
	expectString(t, "'\\7'", "\007")

	expectString(t, "'\\000'", "\000")
	expectString(t, "'\\001'", "\001")
	expectString(t, "'\\002'", "\002")
	expectString(t, "'\\003'", "\003")
	expectString(t, "'\\004'", "\004")
	expectString(t, "'\\005'", "\005")
	expectString(t, "'\\006'", "\006")
	expectString(t, "'\\007'", "\007")

	expectString(t, "'\\000'", "\000")
	expectString(t, "'\\100'", "\100")
	expectString(t, "'\\200'", "\u0080")
	expectString(t, "'\\300'", "\u00C0")
	expectString(t, "'\\377'", "\u00FF")
	expectString(t, "'\\378'", "\0378")
	expectString(t, "'\\400'", "\0400")
	expectString(t, "'\\500'", "\0500")
	expectString(t, "'\\600'", "\0600")
	expectString(t, "'\\700'", "\0700")

	expectString(t, "'\\x00'", "\x00")
	expectString(t, "'\\X11'", "X11")
	expectString(t, "'\\x71'", "\x71")
	expectString(t, "'\\x7f'", "\x7f")
	expectString(t, "'\\x7F'", "\x7F")

	expectString(t, "'\\u0000'", "\u0000")
	expectString(t, "'\\ucafe\\uCAFE\\u7FFF'", "\ucafe\uCAFE\u7FFF")
	expectString(t, "'\\uD800'", "\xED\xA0\x80")
	expectString(t, "'\\uDC00'", "\xED\xB0\x80")
	expectString(t, "'\\U0000'", "U0000")

	expectString(t, "'\\u{100000}'", "\U00100000")
	expectString(t, "'\\u{10FFFF}'", "\U0010FFFF")
	expectLexerErrorString(t, "'\\u{110000}'", "<stdin>: ERROR: Unicode escape sequence is out of range\n")
	expectLexerErrorString(t, "'\\u{FFFFFFFF}'", "<stdin>: ERROR: Unicode escape sequence is out of range\n")

	// Line continuation
	expectLexerErrorString(t, "'\n'", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\r'", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "\"\n\"", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "\"\r\"", "<stdin>: ERROR: Unterminated string literal\n")

	expectString(t, "'\u2028'", "\u2028")
	expectString(t, "'\u2029'", "\u2029")
	expectString(t, "\"\u2028\"", "\u2028")
	expectString(t, "\"\u2029\"", "\u2029")

	expectString(t, "'1\\\r2'", "12")
	expectString(t, "'1\\\n2'", "12")
	expectString(t, "'1\\\r\n2'", "12")
	expectString(t, "'1\\\u20282'", "12")
	expectString(t, "'1\\\u20292'", "12")
	expectLexerErrorString(t, "'1\\\n\r2'", "<stdin>: ERROR: Unterminated string literal\n")

	expectLexerErrorString(t, "\"'", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\"", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\\", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\\'", "<stdin>: ERROR: Unterminated string literal\n")

	expectLexerErrorString(t, "'\\x", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\\x'", "<stdin>: ERROR: Syntax error \"'\"\n")
	expectLexerErrorString(t, "'\\xG'", "<stdin>: ERROR: Syntax error \"G\"\n")
	expectLexerErrorString(t, "'\\xF'", "<stdin>: ERROR: Syntax error \"'\"\n")
	expectLexerErrorString(t, "'\\xFG'", "<stdin>: ERROR: Syntax error \"G\"\n")

	expectLexerErrorString(t, "'\\u", "<stdin>: ERROR: Unterminated string literal\n")
	expectLexerErrorString(t, "'\\u'", "<stdin>: ERROR: Syntax error \"'\"\n")
	expectLexerErrorString(t, "'\\u0'", "<stdin>: ERROR: Syntax error \"'\"\n")
	expectLexerErrorString(t, "'\\u00'", "<stdin>: ERROR: Syntax error \"'\"\n")
	expectLexerErrorString(t, "'\\u000'", "<stdin>: ERROR: Syntax error \"'\"\n")
}

func TestTokens(t *testing.T) {
	expected := []struct {
		contents string
		token    T
	}{
		{"", TEndOfFile},
		{"\x00", TSyntaxError},

		// "#!/usr/bin/env node"
		{"#!", THashbang},

		// Punctuation
		{"(", TOpenParen},
		{")", TCloseParen},
		{"[", TOpenBracket},
		{"]", TCloseBracket},
		{"{", TOpenBrace},
		{"}", TCloseBrace},

		// Reserved words
		{"break", TBreak},
		{"case", TCase},
		{"catch", TCatch},
		{"class", TClass},
		{"const", TConst},
		{"continue", TContinue},
		{"debugger", TDebugger},
		{"default", TDefault},
		{"delete", TDelete},
		{"do", TDo},
		{"else", TElse},
		{"enum", TEnum},
		{"export", TExport},
		{"extends", TExtends},
		{"false", TFalse},
		{"finally", TFinally},
		{"for", TFor},
		{"function", TFunction},
		{"if", TIf},
		{"import", TImport},
		{"in", TIn},
		{"instanceof", TInstanceof},
		{"new", TNew},
		{"null", TNull},
		{"return", TReturn},
		{"super", TSuper},
		{"switch", TSwitch},
		{"this", TThis},
		{"throw", TThrow},
		{"true", TTrue},
		{"try", TTry},
		{"typeof", TTypeof},
		{"var", TVar},
		{"void", TVoid},
		{"while", TWhile},
		{"with", TWith},
	}

	for _, it := range expected {
		contents := it.contents
		token := it.token
		t.Run(contents, func(t *testing.T) {
			test.AssertEqual(t, lexToken(t, contents), token)
		})
	}
}
