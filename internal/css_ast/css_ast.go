package css_ast

import (
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type AST struct {
	Rules []R
}

// This interface is never called. Its purpose is to encode a variant type in
// Go's type system.
type R interface {
	isRule()
}

type RAtImport struct {
	PathText  string
	PathRange logger.Range
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

func (*RAtImport) isRule()       {}
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
