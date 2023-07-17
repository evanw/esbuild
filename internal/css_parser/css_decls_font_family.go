package css_parser

import (
	"strings"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
)

// These keywords usually require special handling when parsing.

// Declaring a property to have these values explicitly specifies a particular
// defaulting behavior instead of setting the property to that identifier value.
// As specified in CSS Values and Units Level 3, all CSS properties can accept
// these values.
//
// For example, "font-family: 'inherit'" sets the font family to the font named
// "inherit" while "font-family: inherit" sets the font family to the inherited
// value.
//
// Note that other CSS specifications can define additional CSS-wide keywords,
// which we should copy here whenever new ones are created so we can quote those
// identifiers to avoid collisions with any newly-created CSS-wide keywords.
var cssWideAndReservedKeywords = map[string]bool{
	// CSS Values and Units Level 3: https://drafts.csswg.org/css-values-3/#common-keywords
	"initial": true, // CSS-wide keyword
	"inherit": true, // CSS-wide keyword
	"unset":   true, // CSS-wide keyword
	"default": true, // CSS reserved keyword

	// CSS Cascading and Inheritance Level 5: https://drafts.csswg.org/css-cascade-5/#defaulting-keywords
	"revert":       true, // Cascade-dependent keyword
	"revert-layer": true, // Cascade-dependent keyword
}

// Font family names that happen to be the same as a keyword value must be
// quoted to prevent confusion with the keywords with the same names. UAs must
// not consider these keywords as matching the <family-name> type.
// Specification: https://drafts.csswg.org/css-fonts/#generic-font-families
var genericFamilyNames = map[string]bool{
	"serif":         true,
	"sans-serif":    true,
	"cursive":       true,
	"fantasy":       true,
	"monospace":     true,
	"system-ui":     true,
	"emoji":         true,
	"math":          true,
	"fangsong":      true,
	"ui-serif":      true,
	"ui-sans-serif": true,
	"ui-monospace":  true,
	"ui-rounded":    true,
}

// Specification: https://drafts.csswg.org/css-fonts/#font-family-prop
func (p *parser) mangleFontFamily(tokens []css_ast.Token) ([]css_ast.Token, bool) {
	result, rest, ok := p.mangleFamilyNameOrGenericName(nil, tokens)
	if !ok {
		return nil, false
	}

	for len(rest) > 0 && rest[0].Kind == css_lexer.TComma {
		result, rest, ok = p.mangleFamilyNameOrGenericName(append(result, rest[0]), rest[1:])
		if !ok {
			return nil, false
		}
	}

	if len(rest) > 0 {
		return nil, false
	}

	return result, true
}

func (p *parser) mangleFamilyNameOrGenericName(result []css_ast.Token, tokens []css_ast.Token) ([]css_ast.Token, []css_ast.Token, bool) {
	if len(tokens) > 0 {
		t := tokens[0]

		// Handle <generic-family>
		if t.Kind == css_lexer.TIdent && genericFamilyNames[t.Text] {
			return append(result, t), tokens[1:], true
		}

		// Handle <family-name>
		if t.Kind == css_lexer.TString {
			// "If a sequence of identifiers is given as a <family-name>, the computed
			// value is the name converted to a string by joining all the identifiers
			// in the sequence by single spaces."
			//
			// More information: https://mathiasbynens.be/notes/unquoted-font-family
			names := strings.Split(t.Text, " ")
			for _, name := range names {
				if !isValidCustomIdent(name, genericFamilyNames) {
					return append(result, t), tokens[1:], true
				}
			}
			for i, name := range names {
				var whitespace css_ast.WhitespaceFlags
				if i != 0 || !p.options.minifyWhitespace {
					whitespace = css_ast.WhitespaceBefore
				}
				result = append(result, css_ast.Token{
					Loc:        t.Loc,
					Kind:       css_lexer.TIdent,
					Text:       name,
					Whitespace: whitespace,
				})
			}
			return result, tokens[1:], true
		}

		// "Font family names other than generic families must either be given
		// quoted as <string>s, or unquoted as a sequence of one or more
		// <custom-ident>."
		if t.Kind == css_lexer.TIdent {
			for {
				if !isValidCustomIdent(t.Text, genericFamilyNames) {
					return nil, nil, false
				}
				result = append(result, t)
				tokens = tokens[1:]
				if len(tokens) == 0 || tokens[0].Kind != css_lexer.TIdent {
					break
				}
				t = tokens[0]
			}
			return result, tokens, true
		}
	}

	// Anything other than the cases listed above causes us to bail
	return nil, nil, false
}

// Specification: https://drafts.csswg.org/css-values-4/#custom-idents
func isValidCustomIdent(text string, predefinedKeywords map[string]bool) bool {
	loweredText := strings.ToLower(text)

	if predefinedKeywords[loweredText] {
		return false
	}
	if cssWideAndReservedKeywords[loweredText] {
		return false
	}
	if loweredText == "" {
		return false
	}

	// validate if it contains characters which needs to be escaped
	if !css_lexer.WouldStartIdentifierWithoutEscapes(text) {
		return false
	}
	for _, c := range text {
		if !css_lexer.IsNameContinue(c) {
			return false
		}
	}

	return true
}
