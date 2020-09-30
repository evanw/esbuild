package css_ast

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_lexer"
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
	ImportRecords []ast.ImportRecord
	Rules         []R
}

// We create a lot of tokens, so make sure this layout is memory-efficient.
// The layout here isn't optimal because it biases for convenience (e.g.
// "string" could be shorter) but at least the ordering of fields was
// deliberately chosen to minimize size.
type Token struct {
	// This is the raw contents of the token most of the time. However, it
	// contains the decoded string contents for "TString" tokens.
	Text string // 16 bytes

	// Contains the child tokens for component values that are simple blocks.
	// These are either "(", "{", "[", or function tokens. The closing token is
	// implicit and is not stored.
	Children *[]Token // 8 bytes

	// URL tokens have an associated import record at the top-level of the AST.
	// This index points to that import record.
	ImportRecordIndex uint32 // 4 bytes

	// The division between the number and the unit for "TDimension" tokens.
	UnitOffset uint16 // 2 bytes

	// This will never be "TWhitespace" because whitespace isn't stored as a
	// token directly. Instead it is stored in "HasWhitespaceAfter" on the
	// previous token.
	Kind css_lexer.T // 1 byte

	// This is generally true if there was a "TWhitespace" token before this
	// token. This isn't strictly true in some cases because sometimes this flag
	// is changed to make the generated code look better (e.g. around commas).
	HasWhitespaceAfter bool // 1 byte
}

func (t Token) DimensionValue() string {
	return t.Text[:t.UnitOffset]
}

func (t Token) DimensionUnit() string {
	return t.Text[t.UnitOffset:]
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type R interface {
	isRule()
}

type RAtCharset struct {
	Encoding string
}

type RAtNamespace struct {
	Prefix string
	Path   string
}

type RAtImport struct {
	ImportRecordIndex uint32
}

type RAtKeyframes struct {
	AtToken string
	Name    string
	Blocks  []KeyframeBlock
}

type KeyframeBlock struct {
	Selectors []string
	Rules     []R
}

type RKnownAt struct {
	AtToken string
	Prelude []Token
	Rules   []R
}

type RUnknownAt struct {
	AtToken string
	Prelude []Token
	Block   []Token
}

type RSelector struct {
	Selectors []ComplexSelector
	Rules     []R
}

type RQualified struct {
	Prelude []Token
	Rules   []R
}

type RDeclaration struct {
	KeyText   string
	Value     []Token
	KeyRange  logger.Range
	Key       D // Compare using this instead of "Key" for speed
	Important bool
}

type RBadDeclaration struct {
	Tokens []Token
}

func (*RAtCharset) isRule()      {}
func (*RAtNamespace) isRule()    {}
func (*RAtImport) isRule()       {}
func (*RAtKeyframes) isRule()    {}
func (*RKnownAt) isRule()        {}
func (*RUnknownAt) isRule()      {}
func (*RSelector) isRule()       {}
func (*RQualified) isRule()      {}
func (*RDeclaration) isRule()    {}
func (*RBadDeclaration) isRule() {}

type ComplexSelector struct {
	Selectors []CompoundSelector
}

type CompoundSelector struct {
	HasNestPrefix        bool   // "&"
	Combinator           string // Optional, may be ""
	TypeSelector         *NamespacedName
	SubclassSelectors    []SS
	PseudoClassSelectors []SSPseudoClass // If present, these follow a ":" character
}

type NamespacedName struct {
	// If present, this is an identifier or "*" or "" and is followed by a "|" character
	NamespacePrefix *string

	// This is an identifier or "*" or "&"
	Name string
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type SS interface {
	isSubclassSelector()
}

type SSHash struct {
	Name string
}

type SSClass struct {
	Name string
}

type SSAttribute struct {
	NamespacedName  NamespacedName
	MatcherOp       string
	MatcherValue    string
	MatcherModifier byte
}

type SSPseudoClass struct {
	Name string
	Args []Token
}

func (*SSHash) isSubclassSelector()        {}
func (*SSClass) isSubclassSelector()       {}
func (*SSAttribute) isSubclassSelector()   {}
func (*SSPseudoClass) isSubclassSelector() {}
