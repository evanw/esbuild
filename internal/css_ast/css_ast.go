package css_ast

import (
	"strconv"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/helpers"
	"github.com/evanw/esbuild/internal/logger"
)

// CSS syntax comes in two layers: a minimal syntax that generally accepts
// anything that looks vaguely like CSS, and a large set of built-in rules
// (the things browsers actually interpret). That way CSS parsers can read
// unknown rules and skip over them without having to stop due to errors.
//
// This AST format is mostly just the minimal syntax. It parses unknown rules
// into a tree with enough information that it can write them back out again.
// There are some additional layers of syntax including selectors and @-rules
// which allow for better pretty-printing and minification.
//
// Most of the AST just references ranges of the original file by keeping the
// original "Token" values around from the lexer. This is a memory-efficient
// representation that helps provide good parsing and printing performance.

type AST struct {
	ImportRecords        []ast.ImportRecord
	Rules                []Rule
	SourceMapComment     logger.Span
	ApproximateLineCount int32
}

// We create a lot of tokens, so make sure this layout is memory-efficient.
// The layout here isn't optimal because it biases for convenience (e.g.
// "string" could be shorter) but at least the ordering of fields was
// deliberately chosen to minimize size.
type Token struct {
	// Contains the child tokens for component values that are simple blocks.
	// These are either "(", "{", "[", or function tokens. The closing token is
	// implicit and is not stored.
	Children *[]Token // 8 bytes

	// This is the raw contents of the token most of the time. However, it
	// contains the decoded string contents for "TString" tokens.
	Text string // 16 bytes

	// URL tokens have an associated import record at the top-level of the AST.
	// This index points to that import record.
	ImportRecordIndex uint32 // 4 bytes

	// The division between the number and the unit for "TDimension" tokens.
	UnitOffset uint16 // 2 bytes

	// This will never be "TWhitespace" because whitespace isn't stored as a
	// token directly. Instead it is stored in "HasWhitespaceAfter" on the
	// previous token. This is to make it easier to pattern-match against
	// tokens when handling CSS rules, since whitespace almost always doesn't
	// matter. That way you can pattern match against e.g. "rgb(r, g, b)" and
	// not have to handle all possible combinations of embedded whitespace
	// tokens.
	//
	// There is one exception to this: when in verbatim whitespace mode and
	// the token list is non-empty and is only whitespace tokens. In that case
	// a single whitespace token is emitted. This is because otherwise there
	// would be no tokens to attach the whitespace before/after flags to.
	Kind css_lexer.T // 1 byte

	// These flags indicate the presence of a "TWhitespace" token before or after
	// this token. There should be whitespace printed between two tokens if either
	// token indicates that there should be whitespace. Note that whitespace may
	// be altered by processing in certain situations (e.g. minification).
	Whitespace WhitespaceFlags // 1 byte
}

type WhitespaceFlags uint8

const (
	WhitespaceBefore WhitespaceFlags = 1 << iota
	WhitespaceAfter
)

// This is necessary when comparing tokens between two different files
type CrossFileEqualityCheck struct {
	ImportRecordsA []ast.ImportRecord
	ImportRecordsB []ast.ImportRecord
}

func (a Token) Equal(b Token, check *CrossFileEqualityCheck) bool {
	if a.Kind == b.Kind && a.Text == b.Text && a.Whitespace == b.Whitespace {
		// URLs should be compared based on the text of the associated import record
		// (which is what will actually be printed) instead of the original text
		if a.Kind == css_lexer.TURL {
			if check == nil {
				// If both tokens are in the same file, just compare the index
				if a.ImportRecordIndex != b.ImportRecordIndex {
					return false
				}
			} else {
				// If the tokens come from separate files, compare the import records
				// themselves instead of comparing the indices. This can happen when
				// the linker runs a "DuplicateRuleRemover" during bundling. This
				// doesn't compare the source indices because at this point during
				// linking, paths inside the bundle (e.g. due to the "copy" loader)
				// should have already been converted into text (e.g. the "unique key"
				// string).
				if check.ImportRecordsA[a.ImportRecordIndex].Path.Text !=
					check.ImportRecordsB[b.ImportRecordIndex].Path.Text {
					return false
				}
			}
		}

		if a.Children == nil && b.Children == nil {
			return true
		}

		if a.Children != nil && b.Children != nil && TokensEqual(*a.Children, *b.Children, check) {
			return true
		}
	}

	return false
}

func TokensEqual(a []Token, b []Token, check *CrossFileEqualityCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i, ai := range a {
		if !ai.Equal(b[i], check) {
			return false
		}
	}
	return true
}

func HashTokens(hash uint32, tokens []Token) uint32 {
	hash = helpers.HashCombine(hash, uint32(len(tokens)))

	for _, t := range tokens {
		hash = helpers.HashCombine(hash, uint32(t.Kind))
		if t.Kind != css_lexer.TURL {
			hash = helpers.HashCombineString(hash, t.Text)
		}
		if t.Children != nil {
			hash = HashTokens(hash, *t.Children)
		}
	}

	return hash
}

func (a Token) EqualIgnoringWhitespace(b Token) bool {
	if a.Kind == b.Kind && a.Text == b.Text && a.ImportRecordIndex == b.ImportRecordIndex {
		if a.Children == nil && b.Children == nil {
			return true
		}

		if a.Children != nil && b.Children != nil && TokensEqualIgnoringWhitespace(*a.Children, *b.Children) {
			return true
		}
	}

	return false
}

func TokensEqualIgnoringWhitespace(a []Token, b []Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i, c := range a {
		if !c.EqualIgnoringWhitespace(b[i]) {
			return false
		}
	}
	return true
}

func TokensAreCommaSeparated(tokens []Token) bool {
	if n := len(tokens); (n & 1) != 0 {
		for i := 1; i < n; i += 2 {
			if tokens[i].Kind != css_lexer.TComma {
				return false
			}
		}
		return true
	}
	return false
}

func (t Token) FractionForPercentage() (float64, bool) {
	if t.Kind == css_lexer.TPercentage {
		if f, err := strconv.ParseFloat(t.PercentageValue(), 64); err == nil {
			if f < 0 {
				return 0, true
			}
			if f > 100 {
				return 1, true
			}
			return f / 100.0, true
		}
	}
	return 0, false
}

// https://drafts.csswg.org/css-values-3/#lengths
// For zero lengths the unit identifier is optional
// (i.e. can be syntactically represented as the <number> 0).
func (t *Token) TurnLengthIntoNumberIfZero() bool {
	if t.Kind == css_lexer.TDimension && t.DimensionValue() == "0" {
		t.Kind = css_lexer.TNumber
		t.Text = "0"
		return true
	}
	return false
}

func (t *Token) TurnLengthOrPercentageIntoNumberIfZero() bool {
	if t.Kind == css_lexer.TPercentage && t.PercentageValue() == "0" {
		t.Kind = css_lexer.TNumber
		t.Text = "0"
		return true
	}
	return t.TurnLengthIntoNumberIfZero()
}

func (t Token) PercentageValue() string {
	return t.Text[:len(t.Text)-1]
}

func (t Token) DimensionValue() string {
	return t.Text[:t.UnitOffset]
}

func (t Token) DimensionUnit() string {
	return t.Text[t.UnitOffset:]
}

func (t Token) DimensionUnitIsSafeLength() bool {
	switch t.DimensionUnit() {
	// These units can be reasonably expected to be supported everywhere.
	// Information used: https://developer.mozilla.org/en-US/docs/Web/CSS/length
	case "cm", "em", "in", "mm", "pc", "pt", "px":
		return true
	}
	return false
}

func (t Token) IsZero() bool {
	return t.Kind == css_lexer.TNumber && t.Text == "0"
}

func (t Token) IsOne() bool {
	return t.Kind == css_lexer.TNumber && t.Text == "1"
}

func (t Token) IsAngle() bool {
	if t.Kind == css_lexer.TDimension {
		unit := t.DimensionUnit()
		return unit == "deg" || unit == "grad" || unit == "rad" || unit == "turn"
	}
	return false
}

func CloneTokensWithImportRecords(
	tokensIn []Token, importRecordsIn []ast.ImportRecord,
	tokensOut []Token, importRecordsOut []ast.ImportRecord,
) ([]Token, []ast.ImportRecord) {
	for _, t := range tokensIn {
		// If this is a URL token, also clone the import record
		if t.Kind == css_lexer.TURL {
			importRecordIndex := uint32(len(importRecordsOut))
			importRecordsOut = append(importRecordsOut, importRecordsIn[t.ImportRecordIndex])
			t.ImportRecordIndex = importRecordIndex
		}

		// Also search for URL tokens in this token's children
		if t.Children != nil {
			var children []Token
			children, importRecordsOut = CloneTokensWithImportRecords(*t.Children, importRecordsIn, children, importRecordsOut)
			t.Children = &children
		}

		tokensOut = append(tokensOut, t)
	}

	return tokensOut, importRecordsOut
}

type Rule struct {
	Data R
	Loc  logger.Loc
}

type R interface {
	Equal(rule R, check *CrossFileEqualityCheck) bool
	Hash() (uint32, bool)
}

func RulesEqual(a []Rule, b []Rule, check *CrossFileEqualityCheck) bool {
	if len(a) != len(b) {
		return false
	}
	for i, ai := range a {
		if !ai.Data.Equal(b[i].Data, check) {
			return false
		}
	}
	return true
}

func HashRules(hash uint32, rules []Rule) uint32 {
	hash = helpers.HashCombine(hash, uint32(len(rules)))
	for _, child := range rules {
		if childHash, ok := child.Data.Hash(); ok {
			hash = helpers.HashCombine(hash, childHash)
		} else {
			hash = helpers.HashCombine(hash, 0)
		}
	}
	return hash
}

type RAtCharset struct {
	Encoding string
}

func (a *RAtCharset) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RAtCharset)
	return ok && a.Encoding == b.Encoding
}

func (r *RAtCharset) Hash() (uint32, bool) {
	hash := uint32(1)
	hash = helpers.HashCombineString(hash, r.Encoding)
	return hash, true
}

type RAtImport struct {
	ImportConditions  []Token
	ImportRecordIndex uint32
}

func (*RAtImport) Equal(rule R, check *CrossFileEqualityCheck) bool {
	return false
}

func (r *RAtImport) Hash() (uint32, bool) {
	return 0, false
}

type RAtKeyframes struct {
	AtToken string
	Name    string
	Blocks  []KeyframeBlock
}

type KeyframeBlock struct {
	Selectors []string
	Rules     []Rule
}

func (a *RAtKeyframes) Equal(rule R, check *CrossFileEqualityCheck) bool {
	if b, ok := rule.(*RAtKeyframes); ok && a.AtToken == b.AtToken && a.Name == b.Name && len(a.Blocks) == len(b.Blocks) {
		for i, ai := range a.Blocks {
			bi := b.Blocks[i]
			if len(ai.Selectors) != len(bi.Selectors) {
				return false
			}
			for j, aj := range ai.Selectors {
				if aj != bi.Selectors[j] {
					return false
				}
			}
			if !RulesEqual(ai.Rules, bi.Rules, check) {
				return false
			}
		}
		return true
	}
	return false
}

func (r *RAtKeyframes) Hash() (uint32, bool) {
	hash := uint32(2)
	hash = helpers.HashCombineString(hash, r.AtToken)
	hash = helpers.HashCombineString(hash, r.Name)
	hash = helpers.HashCombine(hash, uint32(len(r.Blocks)))
	for _, block := range r.Blocks {
		hash = helpers.HashCombine(hash, uint32(len(block.Selectors)))
		for _, sel := range block.Selectors {
			hash = helpers.HashCombineString(hash, sel)
		}
		hash = HashRules(hash, block.Rules)
	}
	return hash, true
}

type RKnownAt struct {
	AtToken string
	Prelude []Token
	Rules   []Rule
}

func (a *RKnownAt) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RKnownAt)
	return ok && a.AtToken == b.AtToken && TokensEqual(a.Prelude, b.Prelude, check) && RulesEqual(a.Rules, b.Rules, check)
}

func (r *RKnownAt) Hash() (uint32, bool) {
	hash := uint32(3)
	hash = helpers.HashCombineString(hash, r.AtToken)
	hash = HashTokens(hash, r.Prelude)
	hash = HashRules(hash, r.Rules)
	return hash, true
}

type RUnknownAt struct {
	AtToken string
	Prelude []Token
	Block   []Token
}

func (a *RUnknownAt) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RUnknownAt)
	return ok && a.AtToken == b.AtToken && TokensEqual(a.Prelude, b.Prelude, check) && TokensEqual(a.Block, b.Block, check)
}

func (r *RUnknownAt) Hash() (uint32, bool) {
	hash := uint32(4)
	hash = helpers.HashCombineString(hash, r.AtToken)
	hash = HashTokens(hash, r.Prelude)
	hash = HashTokens(hash, r.Block)
	return hash, true
}

type RSelector struct {
	Selectors []ComplexSelector
	Rules     []Rule
}

func (a *RSelector) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RSelector)
	if ok && len(a.Selectors) == len(b.Selectors) {
		for i, ai := range a.Selectors {
			if !ai.Equal(b.Selectors[i], check) {
				return false
			}
		}
		return RulesEqual(a.Rules, b.Rules, check)
	}

	return false
}

func (r *RSelector) Hash() (uint32, bool) {
	hash := uint32(5)
	hash = helpers.HashCombine(hash, uint32(len(r.Selectors)))
	for _, complex := range r.Selectors {
		hash = helpers.HashCombine(hash, uint32(len(complex.Selectors)))
		for _, sel := range complex.Selectors {
			if sel.TypeSelector != nil {
				hash = helpers.HashCombineString(hash, sel.TypeSelector.Name.Text)
			} else {
				hash = helpers.HashCombine(hash, 0)
			}
			hash = helpers.HashCombine(hash, uint32(len(sel.SubclassSelectors)))
			for _, sub := range sel.SubclassSelectors {
				hash = helpers.HashCombine(hash, sub.Hash())
			}
			hash = helpers.HashCombine(hash, uint32(sel.Combinator))
		}
	}
	hash = HashRules(hash, r.Rules)
	return hash, true
}

type RQualified struct {
	Prelude []Token
	Rules   []Rule
}

func (a *RQualified) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RQualified)
	return ok && TokensEqual(a.Prelude, b.Prelude, check) && RulesEqual(a.Rules, b.Rules, check)
}

func (r *RQualified) Hash() (uint32, bool) {
	hash := uint32(6)
	hash = HashTokens(hash, r.Prelude)
	hash = HashRules(hash, r.Rules)
	return hash, true
}

type RDeclaration struct {
	KeyText   string
	Value     []Token
	KeyRange  logger.Range
	Key       D // Compare using this instead of "Key" for speed
	Important bool
}

func (a *RDeclaration) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RDeclaration)
	return ok && a.KeyText == b.KeyText && TokensEqual(a.Value, b.Value, check) && a.Important == b.Important
}

func (r *RDeclaration) Hash() (uint32, bool) {
	var hash uint32
	if r.Key == DUnknown {
		if r.Important {
			hash = uint32(7)
		} else {
			hash = uint32(8)
		}
		hash = helpers.HashCombineString(hash, r.KeyText)
	} else {
		if r.Important {
			hash = uint32(9)
		} else {
			hash = uint32(10)
		}
		hash = helpers.HashCombine(hash, uint32(r.Key))
	}
	hash = HashTokens(hash, r.Value)
	return hash, true
}

type RBadDeclaration struct {
	Tokens []Token
}

func (a *RBadDeclaration) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RBadDeclaration)
	return ok && TokensEqual(a.Tokens, b.Tokens, check)
}

func (r *RBadDeclaration) Hash() (uint32, bool) {
	hash := uint32(11)
	hash = HashTokens(hash, r.Tokens)
	return hash, true
}

type RComment struct {
	Text string
}

func (a *RComment) Equal(rule R, check *CrossFileEqualityCheck) bool {
	b, ok := rule.(*RComment)
	return ok && a.Text == b.Text
}

func (r *RComment) Hash() (uint32, bool) {
	hash := uint32(12)
	hash = helpers.HashCombineString(hash, r.Text)
	return hash, true
}

type RAtLayer struct {
	Names [][]string
	Rules []Rule
}

func (a *RAtLayer) Equal(rule R, check *CrossFileEqualityCheck) bool {
	if b, ok := rule.(*RAtLayer); ok && len(a.Names) == len(b.Names) && len(a.Rules) == len(b.Rules) {
		for i, ai := range a.Names {
			bi := b.Names[i]
			if len(ai) != len(bi) {
				return false
			}
			for j, aj := range ai {
				if aj != bi[j] {
					return false
				}
			}
		}
		if !RulesEqual(a.Rules, b.Rules, check) {
			return false
		}
	}
	return false
}

func (r *RAtLayer) Hash() (uint32, bool) {
	hash := uint32(13)
	hash = helpers.HashCombine(hash, uint32(len(r.Names)))
	for _, parts := range r.Names {
		hash = helpers.HashCombine(hash, uint32(len(parts)))
		for _, part := range parts {
			hash = helpers.HashCombineString(hash, part)
		}
	}
	hash = HashRules(hash, r.Rules)
	return hash, true
}

type ComplexSelector struct {
	Selectors []CompoundSelector
}

func (s ComplexSelector) AppendToTokensWithoutLeadingCombinator(tokens []Token) []Token {
	for i, sel := range s.Selectors {
		if n := len(tokens); i > 0 && n > 0 {
			tokens[n-1].Whitespace |= WhitespaceAfter
		}
		if i == 0 {
			sel.Combinator = 0
		}
		tokens = sel.AppendToTokens(tokens)
	}
	return tokens
}

func (sel ComplexSelector) IsRelative() bool {
	if sel.Selectors[0].Combinator == 0 {
		for _, inner := range sel.Selectors {
			if inner.HasNestingSelector {
				return false
			}
			for _, ss := range inner.SubclassSelectors {
				if class, ok := ss.(*SSPseudoClass); ok && tokensContainAmpersandRecursive(class.Args) {
					return false
				}
			}
		}
	}
	return true
}

func tokensContainAmpersandRecursive(tokens []Token) bool {
	for _, t := range tokens {
		if t.Kind == css_lexer.TDelimAmpersand {
			return true
		}
		if children := t.Children; children != nil && tokensContainAmpersandRecursive(*children) {
			return true
		}
	}
	return false
}

func (sel ComplexSelector) UsesPseudoElement() bool {
	for _, sel := range sel.Selectors {
		for _, sub := range sel.SubclassSelectors {
			if class, ok := sub.(*SSPseudoClass); ok {
				if class.IsElement {
					return true
				}

				// https://www.w3.org/TR/selectors-4/#single-colon-pseudos
				// The four Level 2 pseudo-elements (::before, ::after, ::first-line,
				// and ::first-letter) may, for legacy reasons, be represented using
				// the <pseudo-class-selector> grammar, with only a single ":"
				// character at their start.
				switch class.Name {
				case "before", "after", "first-line", "first-letter":
					return true
				}
			}
		}
	}
	return false
}

func (a ComplexSelector) Equal(b ComplexSelector, check *CrossFileEqualityCheck) bool {
	if len(a.Selectors) != len(b.Selectors) {
		return false
	}

	for i, ai := range a.Selectors {
		bi := b.Selectors[i]
		if ai.HasNestingSelector != bi.HasNestingSelector || ai.Combinator != bi.Combinator {
			return false
		}

		if ats, bts := ai.TypeSelector, bi.TypeSelector; (ats == nil) != (bts == nil) {
			return false
		} else if ats != nil && bts != nil && !ats.Equal(*bts) {
			return false
		}

		if len(ai.SubclassSelectors) != len(bi.SubclassSelectors) {
			return false
		}
		for j, aj := range ai.SubclassSelectors {
			if !aj.Equal(bi.SubclassSelectors[j], check) {
				return false
			}
		}
	}

	return true
}

type CompoundSelector struct {
	TypeSelector       *NamespacedName
	SubclassSelectors  []SS
	Combinator         uint8 // Optional, may be 0
	HasNestingSelector bool  // "&"
}

func (sel CompoundSelector) IsSingleAmpersand() bool {
	return sel.HasNestingSelector && sel.Combinator == 0 && sel.TypeSelector == nil && len(sel.SubclassSelectors) == 0
}

func (sel CompoundSelector) AppendToTokens(tokens []Token) []Token {
	if sel.Combinator != 0 {
		switch sel.Combinator {
		case '>':
			tokens = append(tokens, Token{Kind: css_lexer.TDelimGreaterThan, Text: ">", Whitespace: WhitespaceAfter})
		case '+':
			tokens = append(tokens, Token{Kind: css_lexer.TDelimPlus, Text: "+", Whitespace: WhitespaceAfter})
		case '~':
			tokens = append(tokens, Token{Kind: css_lexer.TDelimTilde, Text: "~", Whitespace: WhitespaceAfter})
		default:
			panic("Internal error")
		}
	}

	if sel.TypeSelector != nil {
		tokens = sel.TypeSelector.AppendToTokens(tokens)
	}

	// Put this after the type selector in case it's substituted for ":is()"
	// ".foo { > &a, > &b {} }" => ".foo > :is(b.foo, c.foo) {}" (we don't want to get ".foo > :is(.foob, .fooc) {}" instead)
	if sel.HasNestingSelector {
		tokens = append(tokens, Token{Kind: css_lexer.TDelimAmpersand, Text: "&"})
	}

	for _, ss := range sel.SubclassSelectors {
		tokens = ss.AppendToTokens(tokens)
	}

	return tokens
}

type NameToken struct {
	Text string
	Kind css_lexer.T
}

type NamespacedName struct {
	// If present, this is an identifier or "*" and is followed by a "|" character
	NamespacePrefix *NameToken

	// This is an identifier or "*"
	Name NameToken
}

func (n NamespacedName) AppendToTokens(tokens []Token) []Token {
	if n.NamespacePrefix != nil {
		tokens = append(tokens,
			Token{Kind: n.NamespacePrefix.Kind, Text: n.NamespacePrefix.Text},
			Token{Kind: css_lexer.TDelimBar, Text: "|"},
		)
	}
	return append(tokens, Token{Kind: n.Name.Kind, Text: n.Name.Text})
}

func (a NamespacedName) Equal(b NamespacedName) bool {
	return a.Name == b.Name && (a.NamespacePrefix == nil) == (b.NamespacePrefix == nil) &&
		(a.NamespacePrefix == nil || b.NamespacePrefix == nil || *a.NamespacePrefix == *b.NamespacePrefix)
}

type SS interface {
	Equal(ss SS, check *CrossFileEqualityCheck) bool
	Hash() uint32
	AppendToTokens(tokens []Token) []Token
}

type SSHash struct {
	Name string
}

func (a *SSHash) Equal(ss SS, check *CrossFileEqualityCheck) bool {
	b, ok := ss.(*SSHash)
	return ok && a.Name == b.Name
}

func (ss *SSHash) Hash() uint32 {
	hash := uint32(1)
	hash = helpers.HashCombineString(hash, ss.Name)
	return hash
}

func (ss *SSHash) AppendToTokens(tokens []Token) []Token {
	return append(tokens, Token{Kind: css_lexer.THash, Text: ss.Name})
}

type SSClass struct {
	Name string
}

func (a *SSClass) Equal(ss SS, check *CrossFileEqualityCheck) bool {
	b, ok := ss.(*SSClass)
	return ok && a.Name == b.Name
}

func (ss *SSClass) Hash() uint32 {
	hash := uint32(2)
	hash = helpers.HashCombineString(hash, ss.Name)
	return hash
}

func (ss *SSClass) AppendToTokens(tokens []Token) []Token {
	return append(tokens,
		Token{Kind: css_lexer.TDelimDot, Text: "."},
		Token{Kind: css_lexer.TIdent, Text: ss.Name},
	)
}

type SSAttribute struct {
	MatcherOp       string // Either "" or one of: "=" "~=" "|=" "^=" "$=" "*="
	MatcherValue    string
	NamespacedName  NamespacedName
	MatcherModifier byte // Either 0 or one of: 'i' 'I' 's' 'S'
}

func (a *SSAttribute) Equal(ss SS, check *CrossFileEqualityCheck) bool {
	b, ok := ss.(*SSAttribute)
	return ok && a.NamespacedName.Equal(b.NamespacedName) && a.MatcherOp == b.MatcherOp &&
		a.MatcherValue == b.MatcherValue && a.MatcherModifier == b.MatcherModifier
}

func (ss *SSAttribute) Hash() uint32 {
	hash := uint32(3)
	hash = helpers.HashCombineString(hash, ss.NamespacedName.Name.Text)
	hash = helpers.HashCombineString(hash, ss.MatcherOp)
	hash = helpers.HashCombineString(hash, ss.MatcherValue)
	return hash
}

func (ss *SSAttribute) AppendToTokens(tokens []Token) []Token {
	var children []Token
	children = ss.NamespacedName.AppendToTokens(children)

	if ss.MatcherOp != "" {
		switch ss.MatcherOp {
		case "=":
			children = append(children, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		case "~=":
			children = append(children, Token{Kind: css_lexer.TDelimTilde, Text: "~"}, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		case "|=":
			children = append(children, Token{Kind: css_lexer.TDelimBar, Text: "|"}, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		case "^=":
			children = append(children, Token{Kind: css_lexer.TDelimCaret, Text: "^"}, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		case "$=":
			children = append(children, Token{Kind: css_lexer.TDelimDollar, Text: "$"}, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		case "*=":
			children = append(children, Token{Kind: css_lexer.TDelimAsterisk, Text: "*"}, Token{Kind: css_lexer.TDelimEquals, Text: "="})
		default:
			panic("Internal error")
		}
		printAsIdent := false

		// Print the value as an identifier if it's possible
		if css_lexer.WouldStartIdentifierWithoutEscapes(ss.MatcherValue) {
			printAsIdent = true
			for _, c := range ss.MatcherValue {
				if !css_lexer.IsNameContinue(c) {
					printAsIdent = false
					break
				}
			}
		}

		if printAsIdent {
			children = append(children, Token{Kind: css_lexer.TIdent, Text: ss.MatcherValue})
		} else {
			children = append(children, Token{Kind: css_lexer.TString, Text: ss.MatcherValue})
		}
	}

	if ss.MatcherModifier != 0 {
		children = append(children, Token{Kind: css_lexer.TIdent, Text: string(rune(ss.MatcherModifier)), Whitespace: WhitespaceBefore})
	}

	tokens = append(tokens, Token{Kind: css_lexer.TOpenBracket, Text: "[", Children: &children})
	return tokens
}

type SSPseudoClass struct {
	Name      string
	Args      []Token
	IsElement bool // If true, this is prefixed by "::" instead of ":"
}

func (a *SSPseudoClass) Equal(ss SS, check *CrossFileEqualityCheck) bool {
	b, ok := ss.(*SSPseudoClass)
	return ok && a.Name == b.Name && TokensEqual(a.Args, b.Args, check) && a.IsElement == b.IsElement
}

func (ss *SSPseudoClass) Hash() uint32 {
	hash := uint32(4)
	hash = helpers.HashCombineString(hash, ss.Name)
	hash = HashTokens(hash, ss.Args)
	return hash
}

func (ss *SSPseudoClass) AppendToTokens(tokens []Token) []Token {
	if ss.IsElement {
		tokens = append(tokens, Token{Kind: css_lexer.TColon, Text: ":"})
	}

	if ss.Args != nil {
		return append(tokens,
			Token{Kind: css_lexer.TColon, Text: ":"},
			Token{Kind: css_lexer.TFunction, Text: ss.Name, Children: &ss.Args},
		)
	}

	return append(tokens,
		Token{Kind: css_lexer.TColon, Text: ":"},
		Token{Kind: css_lexer.TIdent, Text: ss.Name},
	)
}
