package lexer

import (
	"esbuild/logging"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

func assertEqual(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func assertEqualStrings(t *testing.T, a string, b string) {
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
	log, join := logging.NewDeferLog()
	lexer := NewLexer(log, logging.Source{
		Index:        0,
		AbsolutePath: "<stdin>",
		PrettyPath:   "<stdin>",
		Contents:     contents,
	})
	join()
	return lexer.Token
}

func expectLexerError(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		func() {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		text := ""
		for _, msg := range msgs {
			text += msg.String(logging.StderrOptions{}, logging.TerminalInfo{})
		}
		assertEqual(t, text, expected)
	})
}

func expectHashbang(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		assertEqual(t, len(msgs), 0)
		assertEqual(t, lexer.Token, THashbang)
		assertEqual(t, lexer.Identifier, expected)
	})
}

func TestHashbang(t *testing.T) {
	expectHashbang(t, "#!/usr/bin/env node", "#!/usr/bin/env node")
	expectHashbang(t, "#!/usr/bin/env node\n", "#!/usr/bin/env node")
	expectHashbang(t, "#!/usr/bin/env node\nlet x", "#!/usr/bin/env node")
	expectLexerError(t, " #!/usr/bin/env node", "<stdin>: error: Syntax error \"#\"\n")
}

func expectIdentifier(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		assertEqual(t, len(msgs), 0)
		assertEqual(t, lexer.Token, TIdentifier)
		assertEqual(t, lexer.Identifier, expected)
	})
}

func TestIdentifier(t *testing.T) {
	expectIdentifier(t, "_", "_")
	expectIdentifier(t, "$", "$")
	expectIdentifier(t, "test", "test")
	expectIdentifier(t, "t\\u0065st", "test")
	expectIdentifier(t, "t\\u{65}st", "test")

	expectLexerError(t, "t\\u.", "<stdin>: error: Syntax error \".\"\n")
	expectLexerError(t, "t\\u0.", "<stdin>: error: Syntax error \".\"\n")
	expectLexerError(t, "t\\u00.", "<stdin>: error: Syntax error \".\"\n")
	expectLexerError(t, "t\\u006.", "<stdin>: error: Syntax error \".\"\n")
	expectLexerError(t, "t\\u{.", "<stdin>: error: Syntax error \".\"\n")
	expectLexerError(t, "t\\u{0.", "<stdin>: error: Syntax error \".\"\n")

	expectIdentifier(t, "a\u200C", "a\u200C")
	expectIdentifier(t, "a\u200D", "a\u200D")
}

func expectNumber(t *testing.T, contents string, expected float64) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		assertEqual(t, len(msgs), 0)
		assertEqual(t, lexer.Token, TNumericLiteral)
		assertEqual(t, lexer.Number, expected)
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
	expectLexerError(t, "0b", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0B", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0b012", "<stdin>: error: Syntax error \"2\"\n")
	expectLexerError(t, "0b018", "<stdin>: error: Syntax error \"8\"\n")
	expectLexerError(t, "0b01a", "<stdin>: error: Syntax error \"a\"\n")
	expectLexerError(t, "0b01A", "<stdin>: error: Syntax error \"A\"\n")

	expectNumber(t, "0o12345", 5349.0)
	expectNumber(t, "0O12345", 5349.0)
	expectNumber(t, "0o1234567654321", 89755965649.0)
	expectNumber(t, "0O1234567654321", 89755965649.0)
	expectLexerError(t, "0o", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0O", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0o018", "<stdin>: error: Syntax error \"8\"\n")
	expectLexerError(t, "0o01a", "<stdin>: error: Syntax error \"a\"\n")
	expectLexerError(t, "0o01A", "<stdin>: error: Syntax error \"A\"\n")

	expectNumber(t, "0x12345678", float64(0x12345678))
	expectNumber(t, "0xFEDCBA987", float64(0xFEDCBA987))
	expectNumber(t, "0x000012345678", float64(0x12345678))
	expectNumber(t, "0x123456781234", float64(0x123456781234))
	expectLexerError(t, "0x", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0X", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "0xGFEDCBA", "<stdin>: error: Syntax error \"G\"\n")
	expectLexerError(t, "0xABCDEFG", "<stdin>: error: Syntax error \"G\"\n")

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

	expectLexerError(t, "1e", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, ".1e", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.e", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.1e", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1e+", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, ".1e+", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.e+", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.1e+", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1e-", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, ".1e-", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.e-", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1.1e-", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "1e+-1", "<stdin>: error: Syntax error \"-\"\n")
	expectLexerError(t, "1e-+1", "<stdin>: error: Syntax error \"+\"\n")

	expectLexerError(t, "1z", "<stdin>: error: Syntax error \"z\"\n")
	expectLexerError(t, "1.z", "<stdin>: error: Syntax error \"z\"\n")
	expectLexerError(t, "1.0f", "<stdin>: error: Syntax error \"f\"\n")
	expectLexerError(t, "0b1z", "<stdin>: error: Syntax error \"z\"\n")
	expectLexerError(t, "0o1z", "<stdin>: error: Syntax error \"z\"\n")
	expectLexerError(t, "0x1z", "<stdin>: error: Syntax error \"z\"\n")
	expectLexerError(t, "1e1z", "<stdin>: error: Syntax error \"z\"\n")

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

	expectLexerError(t, "1__2", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, ".1__2", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "1e2__3", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0b1__0", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0B1__0", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0o1__2", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0O1__2", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0x1__2", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0X1__2", "<stdin>: error: Syntax error \"_\"\n")

	expectLexerError(t, "1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "1._", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, ".1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "1e_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "1e1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "1_e1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, ".1_e1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0b_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0B_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0o_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0O_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0x_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0X_1", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0b1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0B1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0o1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0O1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0x1_", "<stdin>: error: Syntax error \"_\"\n")
	expectLexerError(t, "0X1_", "<stdin>: error: Syntax error \"_\"\n")
}

func expectBigInteger(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		assertEqual(t, len(msgs), 0)
		assertEqual(t, lexer.Token, TBigIntegerLiteral)
		assertEqual(t, lexer.Identifier, expected)
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

	expectLexerError(t, "1e2n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, "1.0n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, ".1n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, "000n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, "0123n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, "089n", "<stdin>: error: Syntax error \"n\"\n")
	expectLexerError(t, "0_1n", "<stdin>: error: Syntax error \"n\"\n")
}

func expectString(t *testing.T, contents string, expected string) {
	t.Run(contents, func(t *testing.T) {
		log, join := logging.NewDeferLog()
		lexer := func() Lexer {
			defer func() {
				r := recover()
				if _, isLexerPanic := r.(LexerPanic); r != nil && !isLexerPanic {
					panic(r)
				}
			}()
			return NewLexer(log, logging.Source{
				Index:        0,
				AbsolutePath: "<stdin>",
				PrettyPath:   "<stdin>",
				Contents:     contents,
			})
		}()
		msgs := join()
		assertEqual(t, len(msgs), 0)
		assertEqual(t, lexer.Token, TStringLiteral)
		assertEqualStrings(t, UTF16ToString(lexer.StringLiteral), expected)
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
	expectLexerError(t, "'\\u{110000}'", "<stdin>: error: Unicode escape sequence is out of range\n")
	expectLexerError(t, "'\\u{FFFFFFFF}'", "<stdin>: error: Unicode escape sequence is out of range\n")

	// Line continuation
	expectLexerError(t, "'\n'", "<stdin>: error: Unterminated string literal\n")
	expectLexerError(t, "'\r'", "<stdin>: error: Unterminated string literal\n")
	expectLexerError(t, "\"\n\"", "<stdin>: error: Unterminated string literal\n")
	expectLexerError(t, "\"\r\"", "<stdin>: error: Unterminated string literal\n")

	expectString(t, "'\u2028'", "\u2028")
	expectString(t, "'\u2029'", "\u2029")
	expectString(t, "\"\u2028\"", "\u2028")
	expectString(t, "\"\u2029\"", "\u2029")

	expectString(t, "'1\\\r2'", "12")
	expectString(t, "'1\\\n2'", "12")
	expectString(t, "'1\\\r\n2'", "12")
	expectString(t, "'1\\\u20282'", "12")
	expectString(t, "'1\\\u20292'", "12")
	expectLexerError(t, "'1\\\n\r2'", "<stdin>: error: Unterminated string literal\n")

	expectLexerError(t, "\"'", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "'\"", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "'\\", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "'\\'", "<stdin>: error: Unexpected end of file\n")

	expectLexerError(t, "'\\x", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "'\\x'", "<stdin>: error: Syntax error \"'\"\n")
	expectLexerError(t, "'\\xG'", "<stdin>: error: Syntax error \"G\"\n")
	expectLexerError(t, "'\\xF'", "<stdin>: error: Syntax error \"'\"\n")
	expectLexerError(t, "'\\xFG'", "<stdin>: error: Syntax error \"G\"\n")

	expectLexerError(t, "'\\u", "<stdin>: error: Unexpected end of file\n")
	expectLexerError(t, "'\\u'", "<stdin>: error: Syntax error \"'\"\n")
	expectLexerError(t, "'\\u0'", "<stdin>: error: Syntax error \"'\"\n")
	expectLexerError(t, "'\\u00'", "<stdin>: error: Syntax error \"'\"\n")
	expectLexerError(t, "'\\u000'", "<stdin>: error: Syntax error \"'\"\n")
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

		// Strict mode reserved words
		{"implements", TImplements},
		{"interface", TInterface},
		{"let", TLet},
		{"package", TPackage},
		{"private", TPrivate},
		{"protected", TProtected},
		{"public", TPublic},
		{"static", TStatic},
		{"yield", TYield},
	}

	for _, it := range expected {
		contents := it.contents
		token := it.token
		t.Run(contents, func(t *testing.T) {
			assertEqual(t, lexToken(t, contents), token)
		})
	}
}
