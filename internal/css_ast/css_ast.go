package css_ast

import (
	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_lexer"
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
	AtToken css_lexer.Token
	Name    string
	Blocks  []KeyframeBlock
}

type KeyframeBlock struct {
	Selectors []css_lexer.Token
	Rules     []R
}

type RKnownAt struct {
	Name    css_lexer.Token
	Prelude []css_lexer.Token
	Rules   []R
}

type RUnknownAt struct {
	Name    css_lexer.Token
	Prelude []css_lexer.Token
	Block   []css_lexer.Token
}

type RSelector struct {
	Selectors []ComplexSelector
	Rules     []R
}

type RQualified struct {
	Prelude []css_lexer.Token
	Rules   []R
}

type RDeclaration struct {
	Key       css_lexer.Token
	Value     []css_lexer.Token
	Important bool
}

type RBadDeclaration struct {
	Tokens []css_lexer.Token
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
	Args []css_lexer.Token
}

func (*SSHash) isSubclassSelector()        {}
func (*SSClass) isSubclassSelector()       {}
func (*SSAttribute) isSubclassSelector()   {}
func (*SSPseudoClass) isSubclassSelector() {}
