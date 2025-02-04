package css_parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

// This is mostly a normal CSS parser with one exception: the addition of
// support for parsing https://drafts.csswg.org/css-nesting-1/.

type parser struct {
	log               logger.Log
	source            logger.Source
	tokens            []css_lexer.Token
	allComments       []logger.Range
	legalComments     []css_lexer.Comment
	stack             []css_lexer.T
	importRecords     []ast.ImportRecord
	symbols           []ast.Symbol
	composes          map[ast.Ref]*css_ast.Composes
	localSymbols      []ast.LocRef
	localScope        map[string]ast.LocRef
	globalScope       map[string]ast.LocRef
	nestingWarnings   map[logger.Loc]struct{}
	tracker           logger.LineColumnTracker
	enclosingAtMedia  [][]css_ast.Token
	layersPreImport   [][]string
	layersPostImport  [][]string
	enclosingLayer    []string
	anonLayerCount    int
	index             int
	legalCommentIndex int
	inSelectorSubtree int
	prevError         logger.Loc
	options           Options
	nestingIsPresent  bool
	makeLocalSymbols  bool
	hasSeenAtImport   bool
}

type Options struct {
	cssPrefixData map[css_ast.D]compat.CSSPrefix

	// This is an embedded struct. Always access these directly instead of off
	// the name "optionsThatSupportStructuralEquality". This is only grouped like
	// this to make the equality comparison easier and safer (and hopefully faster).
	optionsThatSupportStructuralEquality
}

type symbolMode uint8

const (
	symbolModeDisabled symbolMode = iota
	symbolModeGlobal
	symbolModeLocal
)

type optionsThatSupportStructuralEquality struct {
	originalTargetEnv      string
	unsupportedCSSFeatures compat.CSSFeature
	minifySyntax           bool
	minifyWhitespace       bool
	minifyIdentifiers      bool
	symbolMode             symbolMode
}

func OptionsFromConfig(loader config.Loader, options *config.Options) Options {
	var symbolMode symbolMode
	switch loader {
	case config.LoaderGlobalCSS:
		symbolMode = symbolModeGlobal
	case config.LoaderLocalCSS:
		symbolMode = symbolModeLocal
	}

	return Options{
		cssPrefixData: options.CSSPrefixData,

		optionsThatSupportStructuralEquality: optionsThatSupportStructuralEquality{
			minifySyntax:           options.MinifySyntax,
			minifyWhitespace:       options.MinifyWhitespace,
			minifyIdentifiers:      options.MinifyIdentifiers,
			unsupportedCSSFeatures: options.UnsupportedCSSFeatures,
			originalTargetEnv:      options.OriginalTargetEnv,
			symbolMode:             symbolMode,
		},
	}
}

func (a *Options) Equal(b *Options) bool {
	// Compare "optionsThatSupportStructuralEquality"
	if a.optionsThatSupportStructuralEquality != b.optionsThatSupportStructuralEquality {
		return false
	}

	// Compare "cssPrefixData"
	if len(a.cssPrefixData) != len(b.cssPrefixData) {
		return false
	}
	for k, va := range a.cssPrefixData {
		vb, ok := b.cssPrefixData[k]
		if !ok || va != vb {
			return false
		}
	}
	for k := range b.cssPrefixData {
		if _, ok := b.cssPrefixData[k]; !ok {
			return false
		}
	}

	return true
}

func Parse(log logger.Log, source logger.Source, options Options) css_ast.AST {
	result := css_lexer.Tokenize(log, source, css_lexer.Options{
		RecordAllComments: options.minifyIdentifiers,
	})
	p := parser{
		log:              log,
		source:           source,
		tracker:          logger.MakeLineColumnTracker(&source),
		options:          options,
		tokens:           result.Tokens,
		allComments:      result.AllComments,
		legalComments:    result.LegalComments,
		prevError:        logger.Loc{Start: -1},
		composes:         make(map[ast.Ref]*css_ast.Composes),
		localScope:       make(map[string]ast.LocRef),
		globalScope:      make(map[string]ast.LocRef),
		makeLocalSymbols: options.symbolMode == symbolModeLocal,
	}
	rules := p.parseListOfRules(ruleContext{
		isTopLevel:     true,
		parseSelectors: true,
	})
	p.expect(css_lexer.TEndOfFile)
	return css_ast.AST{
		Rules:                rules,
		CharFreq:             p.computeCharacterFrequency(),
		Symbols:              p.symbols,
		ImportRecords:        p.importRecords,
		ApproximateLineCount: result.ApproximateLineCount,
		SourceMapComment:     result.SourceMapComment,
		LocalSymbols:         p.localSymbols,
		LocalScope:           p.localScope,
		GlobalScope:          p.globalScope,
		Composes:             p.composes,
		LayersPreImport:      p.layersPreImport,
		LayersPostImport:     p.layersPostImport,
	}
}

// Compute a character frequency histogram for everything that's not a bound
// symbol. This is used to modify how minified names are generated for slightly
// better gzip compression. Even though it's a very small win, we still do it
// because it's simple to do and very cheap to compute.
func (p *parser) computeCharacterFrequency() *ast.CharFreq {
	if !p.options.minifyIdentifiers {
		return nil
	}

	// Add everything in the file to the histogram
	charFreq := &ast.CharFreq{}
	charFreq.Scan(p.source.Contents, 1)

	// Subtract out all comments
	for _, commentRange := range p.allComments {
		charFreq.Scan(p.source.TextForRange(commentRange), -1)
	}

	// Subtract out all import paths
	for _, record := range p.importRecords {
		if !record.SourceIndex.IsValid() {
			charFreq.Scan(record.Path.Text, -1)
		}
	}

	// Subtract out all symbols that will be minified
	for _, symbol := range p.symbols {
		if symbol.Kind == ast.SymbolLocalCSS {
			charFreq.Scan(symbol.OriginalName, -int32(symbol.UseCountEstimate))
		}
	}

	return charFreq
}

func (p *parser) advance() {
	if p.index < len(p.tokens) {
		p.index++
	}
}

func (p *parser) at(index int) css_lexer.Token {
	if index < len(p.tokens) {
		return p.tokens[index]
	}
	return css_lexer.Token{
		Kind:  css_lexer.TEndOfFile,
		Range: logger.Range{Loc: logger.Loc{Start: int32(len(p.source.Contents))}},
	}
}

func (p *parser) current() css_lexer.Token {
	return p.at(p.index)
}

func (p *parser) next() css_lexer.Token {
	return p.at(p.index + 1)
}

func (p *parser) raw() string {
	t := p.current()
	return p.source.Contents[t.Range.Loc.Start:t.Range.End()]
}

func (p *parser) decoded() string {
	return p.current().DecodedText(p.source.Contents)
}

func (p *parser) peek(kind css_lexer.T) bool {
	return kind == p.current().Kind
}

func (p *parser) eat(kind css_lexer.T) bool {
	if p.peek(kind) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(kind css_lexer.T) bool {
	return p.expectWithMatchingLoc(kind, logger.Loc{Start: -1})
}

func (p *parser) expectWithMatchingLoc(kind css_lexer.T, matchingLoc logger.Loc) bool {
	if p.eat(kind) {
		return true
	}
	t := p.current()
	if (t.Flags & css_lexer.DidWarnAboutSingleLineComment) != 0 {
		return false
	}

	var text string
	var suggestion string
	var notes []logger.MsgData

	expected := kind.String()
	if strings.HasPrefix(expected, "\"") && strings.HasSuffix(expected, "\"") {
		suggestion = expected[1 : len(expected)-1]
	}

	if (kind == css_lexer.TSemicolon || kind == css_lexer.TColon) && p.index > 0 && p.at(p.index-1).Kind == css_lexer.TWhitespace {
		// Have a nice error message for forgetting a trailing semicolon or colon
		text = fmt.Sprintf("Expected %s", expected)
		t = p.at(p.index - 1)
	} else if (kind == css_lexer.TCloseBrace || kind == css_lexer.TCloseBracket || kind == css_lexer.TCloseParen) &&
		matchingLoc.Start != -1 && int(matchingLoc.Start)+1 <= len(p.source.Contents) {
		// Have a nice error message for forgetting a closing brace/bracket/parenthesis
		c := p.source.Contents[matchingLoc.Start : matchingLoc.Start+1]
		text = fmt.Sprintf("Expected %s to go with %q", expected, c)
		notes = append(notes, p.tracker.MsgData(logger.Range{Loc: matchingLoc, Len: 1}, fmt.Sprintf("The unbalanced %q is here:", c)))
	} else {
		switch t.Kind {
		case css_lexer.TEndOfFile, css_lexer.TWhitespace:
			text = fmt.Sprintf("Expected %s but found %s", expected, t.Kind.String())
			t.Range.Len = 0
		case css_lexer.TBadURL, css_lexer.TUnterminatedString:
			text = fmt.Sprintf("Expected %s but found %s", expected, t.Kind.String())
		default:
			text = fmt.Sprintf("Expected %s but found %q", expected, p.raw())
		}
	}

	if t.Range.Loc.Start > p.prevError.Start {
		data := p.tracker.MsgData(t.Range, text)
		data.Location.Suggestion = suggestion
		p.log.AddMsgID(logger.MsgID_CSS_CSSSyntaxError, logger.Msg{Kind: logger.Warning, Data: data, Notes: notes})
		p.prevError = t.Range.Loc
	}
	return false
}

func (p *parser) unexpected() {
	if t := p.current(); t.Range.Loc.Start > p.prevError.Start && (t.Flags&css_lexer.DidWarnAboutSingleLineComment) == 0 {
		var text string
		switch t.Kind {
		case css_lexer.TEndOfFile, css_lexer.TWhitespace:
			text = fmt.Sprintf("Unexpected %s", t.Kind.String())
			t.Range.Len = 0
		case css_lexer.TBadURL, css_lexer.TUnterminatedString:
			text = fmt.Sprintf("Unexpected %s", t.Kind.String())
		default:
			text = fmt.Sprintf("Unexpected %q", p.raw())
		}
		p.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, t.Range, text)
		p.prevError = t.Range.Loc
	}
}

func (p *parser) symbolForName(loc logger.Loc, name string) ast.LocRef {
	var kind ast.SymbolKind
	var scope map[string]ast.LocRef

	if p.makeLocalSymbols {
		kind = ast.SymbolLocalCSS
		scope = p.localScope
	} else {
		kind = ast.SymbolGlobalCSS
		scope = p.globalScope
	}

	entry, ok := scope[name]
	if !ok {
		entry = ast.LocRef{
			Loc: loc,
			Ref: ast.Ref{
				SourceIndex: p.source.Index,
				InnerIndex:  uint32(len(p.symbols)),
			},
		}
		p.symbols = append(p.symbols, ast.Symbol{
			Kind:         kind,
			OriginalName: name,
			Link:         ast.InvalidRef,
		})
		scope[name] = entry
		if kind == ast.SymbolLocalCSS {
			p.localSymbols = append(p.localSymbols, entry)
		}
	}

	p.symbols[entry.Ref.InnerIndex].UseCountEstimate++
	return entry
}

func (p *parser) recordAtLayerRule(layers [][]string) {
	if p.anonLayerCount > 0 {
		return
	}

	for _, layer := range layers {
		if len(p.enclosingLayer) > 0 {
			clone := make([]string, 0, len(p.enclosingLayer)+len(layer))
			layer = append(append(clone, p.enclosingLayer...), layer...)
		}
		p.layersPostImport = append(p.layersPostImport, layer)
	}
}

type ruleContext struct {
	isTopLevel     bool
	parseSelectors bool
}

func (p *parser) parseListOfRules(context ruleContext) []css_ast.Rule {
	atRuleContext := atRuleContext{}
	if context.isTopLevel {
		atRuleContext.charsetValidity = atRuleValid
		atRuleContext.importValidity = atRuleValid
		atRuleContext.isTopLevel = true
	}
	rules := []css_ast.Rule{}
	didFindAtImport := false

loop:
	for {
		if context.isTopLevel {
			p.nestingIsPresent = false
		}

		// If there are any legal comments immediately before the current token,
		// turn them all into comment rules and append them to the current rule list
		for p.legalCommentIndex < len(p.legalComments) {
			comment := p.legalComments[p.legalCommentIndex]
			if comment.TokenIndexAfter > uint32(p.index) {
				break
			}
			if comment.TokenIndexAfter == uint32(p.index) {
				rules = append(rules, css_ast.Rule{Loc: comment.Loc, Data: &css_ast.RComment{Text: comment.Text}})
			}
			p.legalCommentIndex++
		}

		switch p.current().Kind {
		case css_lexer.TEndOfFile:
			break loop

		case css_lexer.TCloseBrace:
			if !context.isTopLevel {
				break loop
			}

		case css_lexer.TWhitespace:
			p.advance()
			continue

		case css_lexer.TAtKeyword:
			rule := p.parseAtRule(atRuleContext)

			// Disallow "@charset" and "@import" after other rules
			if context.isTopLevel {
				switch r := rule.Data.(type) {
				case *css_ast.RAtCharset:
					// This doesn't invalidate anything because it always comes first

				case *css_ast.RAtImport:
					didFindAtImport = true
					if atRuleContext.charsetValidity == atRuleValid {
						atRuleContext.afterLoc = rule.Loc
						atRuleContext.charsetValidity = atRuleInvalidAfter
					}

				case *css_ast.RAtLayer:
					if atRuleContext.charsetValidity == atRuleValid {
						atRuleContext.afterLoc = rule.Loc
						atRuleContext.charsetValidity = atRuleInvalidAfter
					}

					// From the specification: "Note: No @layer rules are allowed between
					// @import and @namespace rules. Any @layer rule that comes after an
					// @import or @namespace rule will cause any subsequent @import or
					// @namespace rules to be ignored."
					if atRuleContext.importValidity == atRuleValid && (r.Rules != nil || didFindAtImport) {
						atRuleContext.afterLoc = rule.Loc
						atRuleContext.charsetValidity = atRuleInvalidAfter
						atRuleContext.importValidity = atRuleInvalidAfter
					}

				default:
					if atRuleContext.importValidity == atRuleValid {
						atRuleContext.afterLoc = rule.Loc
						atRuleContext.charsetValidity = atRuleInvalidAfter
						atRuleContext.importValidity = atRuleInvalidAfter
					}
				}
			}

			// Lower CSS nesting if it's not supported (but only at the top level)
			if p.nestingIsPresent && p.options.unsupportedCSSFeatures.Has(compat.Nesting) && context.isTopLevel {
				rules = p.lowerNestingInRule(rule, rules)
			} else {
				rules = append(rules, rule)
			}
			continue

		case css_lexer.TCDO, css_lexer.TCDC:
			if context.isTopLevel {
				p.advance()
				continue
			}
		}

		if atRuleContext.importValidity == atRuleValid {
			atRuleContext.afterLoc = p.current().Range.Loc
			atRuleContext.charsetValidity = atRuleInvalidAfter
			atRuleContext.importValidity = atRuleInvalidAfter
		}

		// Note: CSS recently changed to parse and discard declarations
		// here instead of treating them as the start of a qualified rule.
		// See also: https://github.com/w3c/csswg-drafts/issues/8834
		if !context.isTopLevel {
			if scan, index := p.scanForEndOfRule(); scan == endOfRuleSemicolon {
				tokens := p.convertTokens(p.tokens[p.index:index])
				rules = append(rules, css_ast.Rule{Loc: p.current().Range.Loc, Data: &css_ast.RBadDeclaration{Tokens: tokens}})
				p.index = index + 1
				continue
			}
		}

		var rule css_ast.Rule
		if context.parseSelectors {
			rule = p.parseSelectorRule(context.isTopLevel, parseSelectorOpts{})
		} else {
			rule = p.parseQualifiedRule(parseQualifiedRuleOpts{isTopLevel: context.isTopLevel})
		}

		// Lower CSS nesting if it's not supported (but only at the top level)
		if p.nestingIsPresent && p.options.unsupportedCSSFeatures.Has(compat.Nesting) && context.isTopLevel {
			rules = p.lowerNestingInRule(rule, rules)
		} else {
			rules = append(rules, rule)
		}
	}

	if p.options.minifySyntax {
		rules = p.mangleRules(rules, context.isTopLevel)
	}
	return rules
}

type listOfDeclarationsOpts struct {
	composesContext      *composesContext
	canInlineNoOpNesting bool
}

func (p *parser) parseListOfDeclarations(opts listOfDeclarationsOpts) (list []css_ast.Rule) {
	list = []css_ast.Rule{}
	foundNesting := false

	for {
		switch p.current().Kind {
		case css_lexer.TWhitespace, css_lexer.TSemicolon:
			p.advance()

		case css_lexer.TEndOfFile, css_lexer.TCloseBrace:
			list = p.processDeclarations(list, opts.composesContext)
			if p.options.minifySyntax {
				list = p.mangleRules(list, false /* isTopLevel */)

				// Pull out all unnecessarily-nested declarations and stick them at the end
				if opts.canInlineNoOpNesting {
					// "a { & { x: y } }" => "a { x: y }"
					// "a { & { b: c } d: e }" => "a { d: e; b: c }"
					if foundNesting {
						var inlineDecls []css_ast.Rule
						n := 0
						for _, rule := range list {
							if rule, ok := rule.Data.(*css_ast.RSelector); ok && len(rule.Selectors) == 1 {
								if sel := rule.Selectors[0]; len(sel.Selectors) == 1 && sel.Selectors[0].IsSingleAmpersand() {
									inlineDecls = append(inlineDecls, rule.Rules...)
									continue
								}
							}
							list[n] = rule
							n++
						}
						list = append(list[:n], inlineDecls...)
					}
				} else {
					// "a, b::before { & { x: y } }" => "a, b::before { & { x: y } }"
				}
			}
			return

		case css_lexer.TAtKeyword:
			if p.inSelectorSubtree > 0 {
				p.nestingIsPresent = true
			}
			list = append(list, p.parseAtRule(atRuleContext{
				isDeclarationList:    true,
				canInlineNoOpNesting: opts.canInlineNoOpNesting,
			}))

		// Reference: https://drafts.csswg.org/css-nesting-1/
		default:
			if scan, _ := p.scanForEndOfRule(); scan == endOfRuleOpenBrace {
				p.nestingIsPresent = true
				foundNesting = true
				rule := p.parseSelectorRule(false, parseSelectorOpts{
					isDeclarationContext: true,
					composesContext:      opts.composesContext,
				})

				// If this rule was a single ":global" or ":local", inline it here. This
				// is handled differently than a bare "&" with normal CSS nesting because
				// that would be inlined at the end of the parent rule's body instead,
				// which is probably unexpected (e.g. it would trip people up when trying
				// to write rules in a specific order).
				if sel, ok := rule.Data.(*css_ast.RSelector); ok && len(sel.Selectors) == 1 {
					if first := sel.Selectors[0]; len(first.Selectors) == 1 {
						if first := first.Selectors[0]; first.WasEmptyFromLocalOrGlobal && first.IsSingleAmpersand() {
							list = append(list, sel.Rules...)
							continue
						}
					}
				}

				list = append(list, rule)
			} else {
				list = append(list, p.parseDeclaration())
			}
		}
	}
}

func (p *parser) mangleRules(rules []css_ast.Rule, isTopLevel bool) []css_ast.Rule {
	// Remove empty rules
	mangledRules := make([]css_ast.Rule, 0, len(rules))
	var prevNonComment css_ast.R
next:
	for _, rule := range rules {
		nextNonComment := rule.Data

		switch r := rule.Data.(type) {
		case *css_ast.RAtKeyframes:
			// Do not remove empty "@keyframe foo {}" rules. Even empty rules still
			// dispatch JavaScript animation events, so removing them changes
			// behavior: https://bugzilla.mozilla.org/show_bug.cgi?id=1004377.

		case *css_ast.RAtLayer:
			if len(r.Rules) == 0 && len(r.Names) > 0 {
				// Do not remove empty "@layer foo {}" rules. The specification says:
				// "Cascade layers are sorted by the order in which they first are
				// declared, with nested layers grouped within their parent layers
				// before any unlayered rules." So removing empty rules could change
				// the order in which they are first declared, and is therefore invalid.
				//
				// We can turn "@layer foo {}" into "@layer foo;" to be shorter. But
				// don't collapse anonymous "@layer {}" into "@layer;" because that is
				// a syntax error.
				r.Rules = nil
			} else if len(r.Rules) == 1 && len(r.Names) == 1 {
				// Only collapse layers if each layer has exactly one name
				if r2, ok := r.Rules[0].Data.(*css_ast.RAtLayer); ok && len(r2.Names) == 1 {
					// "@layer a { @layer b {} }" => "@layer a.b;"
					// "@layer a { @layer b { c {} } }" => "@layer a.b { c {} }"
					r.Names[0] = append(r.Names[0], r2.Names[0]...)
					r.Rules = r2.Rules
				}
			}

		case *css_ast.RKnownAt:
			if len(r.Rules) == 0 && atKnownRuleCanBeRemovedIfEmpty[r.AtToken] {
				continue
			}

			// Unwrap "@media" rules that duplicate conditions from a parent "@media"
			// rule. This is unlikely to be authored manually but can be automatically
			// generated when using a CSS framework such as Tailwind.
			//
			//   @media (min-width: 1024px) {
			//     .md\:class {
			//       color: red;
			//     }
			//     @media (min-width: 1024px) {
			//       .md\:class {
			//         color: red;
			//       }
			//     }
			//   }
			//
			// This converts that code into the following:
			//
			//   @media (min-width: 1024px) {
			//     .md\:class {
			//       color: red;
			//     }
			//     .md\:class {
			//       color: red;
			//     }
			//   }
			//
			// Which can then be mangled further.
			if strings.EqualFold(r.AtToken, "media") {
				for _, prelude := range p.enclosingAtMedia {
					if css_ast.TokensEqualIgnoringWhitespace(r.Prelude, prelude) {
						mangledRules = append(mangledRules, r.Rules...)
						continue next
					}
				}
			}

		case *css_ast.RSelector:
			if len(r.Rules) == 0 {
				continue
			}

			// Merge adjacent selectors with the same content
			// "a { color: red; } b { color: red; }" => "a, b { color: red; }"
			if prevNonComment != nil {
				if r, ok := rule.Data.(*css_ast.RSelector); ok {
					if prev, ok := prevNonComment.(*css_ast.RSelector); ok && css_ast.RulesEqual(r.Rules, prev.Rules, nil) &&
						isSafeSelectors(r.Selectors) && isSafeSelectors(prev.Selectors) {
					nextSelector:
						for _, sel := range r.Selectors {
							for _, prevSel := range prev.Selectors {
								if sel.Equal(prevSel, nil) {
									// Don't add duplicate selectors more than once
									continue nextSelector
								}
							}
							prev.Selectors = append(prev.Selectors, sel)
						}
						continue
					}
				}
			}

		case *css_ast.RComment:
			nextNonComment = nil
		}

		if nextNonComment != nil {
			prevNonComment = nextNonComment
		}

		mangledRules = append(mangledRules, rule)
	}

	// Mangle non-top-level rules using a back-to-front pass. Top-level rules
	// will be mangled by the linker instead for cross-file rule mangling.
	if !isTopLevel {
		remover := MakeDuplicateRuleMangler(ast.SymbolMap{})
		mangledRules = remover.RemoveDuplicateRulesInPlace(p.source.Index, mangledRules, p.importRecords)
	}

	return mangledRules
}

type ruleEntry struct {
	data        css_ast.R
	callCounter uint32
}

type hashEntry struct {
	rules []ruleEntry
}

type callEntry struct {
	importRecords []ast.ImportRecord
	sourceIndex   uint32
}

type DuplicateRuleRemover struct {
	entries map[uint32]hashEntry
	calls   []callEntry
	check   css_ast.CrossFileEqualityCheck
}

func MakeDuplicateRuleMangler(symbols ast.SymbolMap) DuplicateRuleRemover {
	return DuplicateRuleRemover{
		entries: make(map[uint32]hashEntry),
		check:   css_ast.CrossFileEqualityCheck{Symbols: symbols},
	}
}

func (remover *DuplicateRuleRemover) RemoveDuplicateRulesInPlace(sourceIndex uint32, rules []css_ast.Rule, importRecords []ast.ImportRecord) []css_ast.Rule {
	// The caller may call this function multiple times, each with a different
	// set of import records. Remember each set of import records for equality
	// checks later.
	callCounter := uint32(len(remover.calls))
	remover.calls = append(remover.calls, callEntry{importRecords, sourceIndex})

	// Remove duplicate rules, scanning from the back so we keep the last
	// duplicate. Note that the linker calls this, so we do not want to do
	// anything that modifies the rules themselves. One reason is that ASTs
	// are immutable at the linking stage. Another reason is that merging
	// CSS ASTs from separate files will mess up source maps because a single
	// AST cannot simultaneously represent offsets from multiple files.
	n := len(rules)
	start := n
skipRule:
	for i := n - 1; i >= 0; i-- {
		rule := rules[i]

		// For duplicate rules, omit all but the last copy
		if hash, ok := rule.Data.Hash(); ok {
			entry := remover.entries[hash]
			for _, current := range entry.rules {
				var check *css_ast.CrossFileEqualityCheck

				// If this rule was from another file, then pass along both arrays
				// of import records so that the equality check for "url()" tokens
				// can use them to check for equality.
				if current.callCounter != callCounter {
					// Reuse the same memory allocation
					check = &remover.check
					call := remover.calls[current.callCounter]
					check.ImportRecordsA = importRecords
					check.ImportRecordsB = call.importRecords
					check.SourceIndexA = sourceIndex
					check.SourceIndexB = call.sourceIndex
				}

				if rule.Data.Equal(current.data, check) {
					continue skipRule
				}
			}
			entry.rules = append(entry.rules, ruleEntry{
				data:        rule.Data,
				callCounter: callCounter,
			})
			remover.entries[hash] = entry
		}

		start--
		rules[start] = rule
	}

	return rules[start:]
}

// Reference: https://developer.mozilla.org/en-US/docs/Web/HTML/Element
var nonDeprecatedElementsSupportedByIE7 = map[string]bool{
	"a":          true,
	"abbr":       true,
	"address":    true,
	"area":       true,
	"b":          true,
	"base":       true,
	"blockquote": true,
	"body":       true,
	"br":         true,
	"button":     true,
	"caption":    true,
	"cite":       true,
	"code":       true,
	"col":        true,
	"colgroup":   true,
	"dd":         true,
	"del":        true,
	"dfn":        true,
	"div":        true,
	"dl":         true,
	"dt":         true,
	"em":         true,
	"embed":      true,
	"fieldset":   true,
	"form":       true,
	"h1":         true,
	"h2":         true,
	"h3":         true,
	"h4":         true,
	"h5":         true,
	"h6":         true,
	"head":       true,
	"hr":         true,
	"html":       true,
	"i":          true,
	"iframe":     true,
	"img":        true,
	"input":      true,
	"ins":        true,
	"kbd":        true,
	"label":      true,
	"legend":     true,
	"li":         true,
	"link":       true,
	"map":        true,
	"menu":       true,
	"meta":       true,
	"noscript":   true,
	"object":     true,
	"ol":         true,
	"optgroup":   true,
	"option":     true,
	"p":          true,
	"param":      true,
	"pre":        true,
	"q":          true,
	"ruby":       true,
	"s":          true,
	"samp":       true,
	"script":     true,
	"select":     true,
	"small":      true,
	"span":       true,
	"strong":     true,
	"style":      true,
	"sub":        true,
	"sup":        true,
	"table":      true,
	"tbody":      true,
	"td":         true,
	"textarea":   true,
	"tfoot":      true,
	"th":         true,
	"thead":      true,
	"title":      true,
	"tr":         true,
	"u":          true,
	"ul":         true,
	"var":        true,
}

// This only returns true if all of these selectors are considered "safe" which
// means that they are very likely to work in any browser a user might reasonably
// be using. We do NOT want to merge adjacent qualified rules with the same body
// if any of the selectors are unsafe, since then browsers which don't support
// that particular feature would ignore the entire merged qualified rule:
//
//	Input:
//	  a { color: red }
//	  b { color: red }
//	  input::-moz-placeholder { color: red }
//
//	Valid output:
//	  a, b { color: red }
//	  input::-moz-placeholder { color: red }
//
//	Invalid output:
//	  a, b, input::-moz-placeholder { color: red }
//
// This considers IE 7 and above to be a browser that a user could possibly use.
// Versions of IE less than 6 are not considered.
func isSafeSelectors(complexSelectors []css_ast.ComplexSelector) bool {
	for _, complex := range complexSelectors {
		for _, compound := range complex.Selectors {
			if len(compound.NestingSelectorLocs) > 0 {
				// Bail because this is an extension: https://drafts.csswg.org/css-nesting-1/
				return false
			}

			if compound.Combinator.Byte != 0 {
				// "Before Internet Explorer 10, the combinator only works in standards mode"
				// Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_Selectors
				return false
			}

			if compound.TypeSelector != nil {
				if compound.TypeSelector.NamespacePrefix != nil {
					// Bail if we hit a namespace, which doesn't work in IE before version 9
					// Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/Type_selectors
					return false
				}

				if compound.TypeSelector.Name.Kind == css_lexer.TIdent && !nonDeprecatedElementsSupportedByIE7[compound.TypeSelector.Name.Text] {
					// Bail if this element is either deprecated or not supported in IE 7
					return false
				}
			}

			for _, ss := range compound.SubclassSelectors {
				switch s := ss.Data.(type) {
				case *css_ast.SSAttribute:
					if s.MatcherModifier != 0 {
						// Bail if we hit a case modifier, which doesn't work in IE at all
						// Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/Attribute_selectors
						return false
					}

				case *css_ast.SSPseudoClass:
					// Bail if this pseudo class doesn't match a hard-coded list that's
					// known to work everywhere. For example, ":focus" doesn't work in IE 7.
					// Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/Pseudo-classes
					if s.Args == nil && !s.IsElement {
						switch s.Name {
						case "active", "first-child", "hover", "link", "visited":
							continue
						}
					}
					return false

				case *css_ast.SSPseudoClassWithSelectorList:
					// These definitely don't work in IE 7
					return false
				}
			}
		}
	}
	return true
}

func (p *parser) parseURLOrString() (string, logger.Range, bool) {
	t := p.current()
	switch t.Kind {
	case css_lexer.TString:
		text := p.decoded()
		p.advance()
		return text, t.Range, true

	case css_lexer.TURL:
		text := p.decoded()
		p.advance()
		return text, t.Range, true

	case css_lexer.TFunction:
		if strings.EqualFold(p.decoded(), "url") {
			matchingLoc := logger.Loc{Start: p.current().Range.End() - 1}
			i := p.index + 1

			// Skip over whitespace
			for p.at(i).Kind == css_lexer.TWhitespace {
				i++
			}

			// Consume a string
			if p.at(i).Kind == css_lexer.TString {
				stringIndex := i
				i++

				// Skip over whitespace
				for p.at(i).Kind == css_lexer.TWhitespace {
					i++
				}

				// Consume a closing parenthesis
				if close := p.at(i).Kind; close == css_lexer.TCloseParen || close == css_lexer.TEndOfFile {
					t := p.at(stringIndex)
					text := t.DecodedText(p.source.Contents)
					p.index = i
					p.expectWithMatchingLoc(css_lexer.TCloseParen, matchingLoc)
					return text, t.Range, true
				}
			}
		}
	}

	return "", logger.Range{}, false
}

func (p *parser) expectURLOrString() (url string, r logger.Range, ok bool) {
	url, r, ok = p.parseURLOrString()
	if !ok {
		p.expect(css_lexer.TURL)
	}
	return
}

type atRuleKind uint8

const (
	atRuleUnknown atRuleKind = iota
	atRuleDeclarations
	atRuleInheritContext
	atRuleQualifiedOrEmpty
	atRuleEmpty
)

var specialAtRules = map[string]atRuleKind{
	"media":    atRuleInheritContext,
	"supports": atRuleInheritContext,

	"font-face": atRuleDeclarations,
	"page":      atRuleDeclarations,

	// These go inside "@page": https://www.w3.org/TR/css-page-3/#syntax-page-selector
	"bottom-center":       atRuleDeclarations,
	"bottom-left-corner":  atRuleDeclarations,
	"bottom-left":         atRuleDeclarations,
	"bottom-right-corner": atRuleDeclarations,
	"bottom-right":        atRuleDeclarations,
	"left-bottom":         atRuleDeclarations,
	"left-middle":         atRuleDeclarations,
	"left-top":            atRuleDeclarations,
	"right-bottom":        atRuleDeclarations,
	"right-middle":        atRuleDeclarations,
	"right-top":           atRuleDeclarations,
	"top-center":          atRuleDeclarations,
	"top-left-corner":     atRuleDeclarations,
	"top-left":            atRuleDeclarations,
	"top-right-corner":    atRuleDeclarations,
	"top-right":           atRuleDeclarations,

	// These properties are very deprecated and appear to only be useful for
	// mobile versions of internet explorer (which may no longer exist?), but
	// they are used by the https://ant.design/ design system so we recognize
	// them to avoid the warning.
	//
	//   Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@viewport
	//   Discussion: https://github.com/w3c/csswg-drafts/issues/4766
	//
	"viewport":     atRuleDeclarations,
	"-ms-viewport": atRuleDeclarations,

	// This feature has been removed from the web because it's actively harmful.
	// However, there is one exception where "@-moz-document url-prefix() {" is
	// accepted by Firefox to basically be an "if Firefox" conditional rule.
	//
	//   Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@document
	//   Discussion: https://bugzilla.mozilla.org/show_bug.cgi?id=1035091
	//
	"document":      atRuleInheritContext,
	"-moz-document": atRuleInheritContext,

	// This is a new feature that changes how the CSS rule cascade works. It can
	// end in either a "{}" block or a ";" rule terminator so we need this special
	// case to support both.
	//
	//   Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@layer
	//   Motivation: https://developer.chrome.com/blog/cascade-layers/
	//
	"layer": atRuleQualifiedOrEmpty,

	// Reference: https://drafts.csswg.org/css-cascade-6/#scoped-styles
	"scope": atRuleInheritContext,

	// Reference: https://drafts.csswg.org/css-fonts-4/#font-palette-values
	"font-palette-values": atRuleDeclarations,

	// Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@counter-style
	// Reference: https://drafts.csswg.org/css-counter-styles/#the-counter-style-rule
	"counter-style": atRuleDeclarations,

	// Documentation: https://developer.mozilla.org/en-US/docs/Web/CSS/@font-feature-values
	// Reference: https://drafts.csswg.org/css-fonts/#font-feature-values
	"font-feature-values": atRuleDeclarations,
	"annotation":          atRuleDeclarations,
	"character-variant":   atRuleDeclarations,
	"historical-forms":    atRuleDeclarations,
	"ornaments":           atRuleDeclarations,
	"styleset":            atRuleDeclarations,
	"stylistic":           atRuleDeclarations,
	"swash":               atRuleDeclarations,

	// Container Queries
	// Reference: https://drafts.csswg.org/css-contain-3/#container-rule
	"container": atRuleInheritContext,

	// Defining before-change style: the @starting-style rule
	// Reference: https://drafts.csswg.org/css-transitions-2/#defining-before-change-style-the-starting-style-rule
	"starting-style": atRuleInheritContext,

	// Anchor Positioning
	// Reference: https://drafts.csswg.org/css-anchor-position-1/#at-ruledef-position-try
	"position-try": atRuleDeclarations,
}

var atKnownRuleCanBeRemovedIfEmpty = map[string]bool{
	"media":     true,
	"supports":  true,
	"font-face": true,
	"page":      true,

	// https://www.w3.org/TR/css-page-3/#syntax-page-selector
	"bottom-center":       true,
	"bottom-left-corner":  true,
	"bottom-left":         true,
	"bottom-right-corner": true,
	"bottom-right":        true,
	"left-bottom":         true,
	"left-middle":         true,
	"left-top":            true,
	"right-bottom":        true,
	"right-middle":        true,
	"right-top":           true,
	"top-center":          true,
	"top-left-corner":     true,
	"top-left":            true,
	"top-right-corner":    true,
	"top-right":           true,

	// https://drafts.csswg.org/css-cascade-6/#scoped-styles
	"scope": true,

	// https://drafts.csswg.org/css-fonts-4/#font-palette-values
	"font-palette-values": true,

	// https://drafts.csswg.org/css-contain-3/#container-rule
	"container": true,
}

type atRuleValidity uint8

const (
	atRuleInvalid atRuleValidity = iota
	atRuleValid
	atRuleInvalidAfter
)

type atRuleContext struct {
	afterLoc             logger.Loc
	charsetValidity      atRuleValidity
	importValidity       atRuleValidity
	canInlineNoOpNesting bool
	isDeclarationList    bool
	isTopLevel           bool
}

func (p *parser) parseAtRule(context atRuleContext) css_ast.Rule {
	// Parse the name
	atToken := p.decoded()
	atRange := p.current().Range
	lowerAtToken := strings.ToLower(atToken)
	kind := specialAtRules[lowerAtToken]
	p.advance()

	// Parse the prelude
	preludeStart := p.index
abortRuleParser:
	switch lowerAtToken {
	case "charset":
		switch context.charsetValidity {
		case atRuleInvalid:
			p.log.AddID(logger.MsgID_CSS_InvalidAtCharset, logger.Warning, &p.tracker, atRange, "\"@charset\" must be the first rule in the file")

		case atRuleInvalidAfter:
			p.log.AddIDWithNotes(logger.MsgID_CSS_InvalidAtCharset, logger.Warning, &p.tracker, atRange,
				"\"@charset\" must be the first rule in the file",
				[]logger.MsgData{p.tracker.MsgData(logger.Range{Loc: context.afterLoc},
					"This rule cannot come before a \"@charset\" rule")})

		case atRuleValid:
			kind = atRuleEmpty
			p.expect(css_lexer.TWhitespace)
			if p.peek(css_lexer.TString) {
				encoding := p.decoded()
				if !strings.EqualFold(encoding, "UTF-8") {
					p.log.AddID(logger.MsgID_CSS_UnsupportedAtCharset, logger.Warning, &p.tracker, p.current().Range,
						fmt.Sprintf("\"UTF-8\" will be used instead of unsupported charset %q", encoding))
				}
				p.advance()
				p.expect(css_lexer.TSemicolon)
				return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtCharset{Encoding: encoding}}
			}
			p.expect(css_lexer.TString)
		}

	case "import":
		switch context.importValidity {
		case atRuleInvalid:
			p.log.AddID(logger.MsgID_CSS_InvalidAtImport, logger.Warning, &p.tracker, atRange, "\"@import\" is only valid at the top level")

		case atRuleInvalidAfter:
			p.log.AddIDWithNotes(logger.MsgID_CSS_InvalidAtImport, logger.Warning, &p.tracker, atRange,
				"All \"@import\" rules must come first",
				[]logger.MsgData{p.tracker.MsgData(logger.Range{Loc: context.afterLoc},
					"This rule cannot come before an \"@import\" rule")})

		case atRuleValid:
			kind = atRuleEmpty
			p.eat(css_lexer.TWhitespace)
			if path, r, ok := p.expectURLOrString(); ok {
				var conditions css_ast.ImportConditions
				importConditionsStart := p.index
				for {
					if kind := p.current().Kind; kind == css_lexer.TSemicolon || kind == css_lexer.TOpenBrace ||
						kind == css_lexer.TCloseBrace || kind == css_lexer.TEndOfFile {
						break
					}
					p.parseComponentValue()
				}
				if p.current().Kind == css_lexer.TOpenBrace {
					break // Avoid parsing an invalid "@import" rule
				}
				conditions.Media = p.convertTokens(p.tokens[importConditionsStart:p.index])

				// Insert or remove whitespace before the first token
				var importConditions *css_ast.ImportConditions
				if len(conditions.Media) > 0 {
					importConditions = &conditions

					// Handle "layer()"
					if t := conditions.Media[0]; (t.Kind == css_lexer.TIdent || t.Kind == css_lexer.TFunction) && strings.EqualFold(t.Text, "layer") {
						conditions.Layers = conditions.Media[:1]
						conditions.Media = conditions.Media[1:]
					}

					// Handle "supports()"
					if len(conditions.Media) > 0 {
						if t := conditions.Media[0]; t.Kind == css_lexer.TFunction && strings.EqualFold(t.Text, "supports") {
							conditions.Supports = conditions.Media[:1]
							conditions.Media = conditions.Media[1:]
						}
					}

					// Remove leading and trailing whitespace
					if len(conditions.Layers) > 0 {
						conditions.Layers[0].Whitespace &= ^(css_ast.WhitespaceBefore | css_ast.WhitespaceAfter)
					}
					if len(conditions.Supports) > 0 {
						conditions.Supports[0].Whitespace &= ^(css_ast.WhitespaceBefore | css_ast.WhitespaceAfter)
					}
					if n := len(conditions.Media); n > 0 {
						conditions.Media[0].Whitespace &= ^css_ast.WhitespaceBefore
						conditions.Media[n-1].Whitespace &= ^css_ast.WhitespaceAfter
					}
				}

				p.expect(css_lexer.TSemicolon)
				importRecordIndex := uint32(len(p.importRecords))
				p.importRecords = append(p.importRecords, ast.ImportRecord{
					Kind:  ast.ImportAt,
					Path:  logger.Path{Text: path},
					Range: r,
				})

				// Fill in the pre-import layers once we see the first "@import"
				if !p.hasSeenAtImport {
					p.hasSeenAtImport = true
					p.layersPreImport = p.layersPostImport
					p.layersPostImport = nil
				}

				return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtImport{
					ImportRecordIndex: importRecordIndex,
					ImportConditions:  importConditions,
				}}
			}
		}

	case "keyframes", "-webkit-keyframes", "-moz-keyframes", "-ms-keyframes", "-o-keyframes":
		p.eat(css_lexer.TWhitespace)
		nameLoc := p.current().Range.Loc
		var name string

		if p.peek(css_lexer.TIdent) {
			name = p.decoded()
			if isInvalidAnimationName(name) {
				msg := logger.Msg{
					ID:    logger.MsgID_CSS_CSSSyntaxError,
					Kind:  logger.Warning,
					Data:  p.tracker.MsgData(p.current().Range, fmt.Sprintf("Cannot use %q as a name for \"@keyframes\" without quotes", name)),
					Notes: []logger.MsgData{{Text: fmt.Sprintf("You can put %q in quotes to prevent it from becoming a CSS keyword.", name)}},
				}
				msg.Data.Location.Suggestion = fmt.Sprintf("%q", name)
				p.log.AddMsg(msg)
				break
			}
			p.advance()
		} else if p.peek(css_lexer.TString) {
			// Note: Strings as names is allowed in the CSS specification and works in
			// Firefox and Safari but Chrome has strangely decided to deliberately not
			// support this. We always turn all string names into identifiers to avoid
			// them silently breaking in Chrome.
			name = p.decoded()
			p.advance()
			if !p.makeLocalSymbols && isInvalidAnimationName(name) {
				break
			}
		} else if !p.expect(css_lexer.TIdent) {
			break
		}

		p.eat(css_lexer.TWhitespace)
		blockStart := p.index

		matchingLoc := p.current().Range.Loc
		if p.expect(css_lexer.TOpenBrace) {
			var blocks []css_ast.KeyframeBlock

		badSyntax:
			for {
				switch p.current().Kind {
				case css_lexer.TWhitespace:
					p.advance()
					continue

				case css_lexer.TCloseBrace:
					closeBraceLoc := p.current().Range.Loc
					p.advance()
					return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtKeyframes{
						AtToken:       atToken,
						Name:          p.symbolForName(nameLoc, name),
						Blocks:        blocks,
						CloseBraceLoc: closeBraceLoc,
					}}

				case css_lexer.TEndOfFile:
					break badSyntax

				case css_lexer.TOpenBrace:
					p.expect(css_lexer.TPercentage)
					break badSyntax

				default:
					var selectors []string
					var firstSelectorLoc logger.Loc

				selectors:
					for {
						t := p.current()
						switch t.Kind {
						case css_lexer.TWhitespace:
							p.advance()
							continue

						case css_lexer.TOpenBrace:
							blockMatchingLoc := p.current().Range.Loc
							p.advance()
							rules := p.parseListOfDeclarations(listOfDeclarationsOpts{})
							closeBraceLoc := p.current().Range.Loc
							if !p.expectWithMatchingLoc(css_lexer.TCloseBrace, blockMatchingLoc) {
								closeBraceLoc = logger.Loc{}
							}

							// "@keyframes { from {} to { color: red } }" => "@keyframes { to { color: red } }"
							if !p.options.minifySyntax || len(rules) > 0 {
								blocks = append(blocks, css_ast.KeyframeBlock{
									Selectors:     selectors,
									Rules:         rules,
									Loc:           firstSelectorLoc,
									CloseBraceLoc: closeBraceLoc,
								})
							}
							break selectors

						case css_lexer.TCloseBrace, css_lexer.TEndOfFile:
							p.expect(css_lexer.TOpenBrace)
							break badSyntax

						case css_lexer.TIdent, css_lexer.TPercentage:
							if firstSelectorLoc.Start == 0 {
								firstSelectorLoc = p.current().Range.Loc
							}
							text := p.decoded()
							if t.Kind == css_lexer.TIdent {
								if strings.EqualFold(text, "from") {
									if p.options.minifySyntax {
										text = "0%" // "0%" is equivalent to but shorter than "from"
									}
								} else if !strings.EqualFold(text, "to") {
									p.expect(css_lexer.TPercentage)
								}
							} else if p.options.minifySyntax && text == "100%" {
								text = "to" // "to" is equivalent to but shorter than "100%"
							}
							selectors = append(selectors, text)
							p.advance()

							// Keyframe selectors are comma-separated
							p.eat(css_lexer.TWhitespace)
							if p.eat(css_lexer.TComma) {
								p.eat(css_lexer.TWhitespace)
								if k := p.current().Kind; k != css_lexer.TIdent && k != css_lexer.TPercentage {
									p.expect(css_lexer.TPercentage)
									break badSyntax
								}
							} else if k := p.current().Kind; k != css_lexer.TOpenBrace && k != css_lexer.TCloseBrace && k != css_lexer.TEndOfFile {
								p.expect(css_lexer.TComma)
								break badSyntax
							}

						default:
							p.expect(css_lexer.TPercentage)
							break badSyntax
						}
					}
				}
			}

			// Otherwise, finish parsing the body and return an unknown rule
			for !p.peek(css_lexer.TCloseBrace) && !p.peek(css_lexer.TEndOfFile) {
				p.parseComponentValue()
			}
			p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc)
			prelude := p.convertTokens(p.tokens[preludeStart:blockStart])
			block, _ := p.convertTokensHelper(p.tokens[blockStart:p.index], css_lexer.TEndOfFile, convertTokensOpts{allowImports: true})
			return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude, Block: block}}
		}

	case "layer":
		// Reference: https://developer.mozilla.org/en-US/docs/Web/CSS/@layer

		// Read the layer name list
		var names [][]string
		p.eat(css_lexer.TWhitespace)
		if p.peek(css_lexer.TIdent) {
			for {
				ident, ok := p.expectValidLayerNameIdent()
				if !ok {
					break abortRuleParser
				}
				name := []string{ident}
				for {
					p.eat(css_lexer.TWhitespace)
					if !p.eat(css_lexer.TDelimDot) {
						break
					}
					p.eat(css_lexer.TWhitespace)
					ident, ok := p.expectValidLayerNameIdent()
					if !ok {
						break abortRuleParser
					}
					name = append(name, ident)
				}
				names = append(names, name)
				p.eat(css_lexer.TWhitespace)
				if !p.eat(css_lexer.TComma) {
					break
				}
				p.eat(css_lexer.TWhitespace)
			}
		}

		// Read the optional block
		matchingLoc := p.current().Range.Loc
		if len(names) <= 1 && p.eat(css_lexer.TOpenBrace) {
			p.recordAtLayerRule(names)
			oldEnclosingLayer := p.enclosingLayer
			if len(names) == 1 {
				p.enclosingLayer = append(p.enclosingLayer, names[0]...)
			} else {
				p.anonLayerCount++
			}
			var rules []css_ast.Rule
			if context.isDeclarationList {
				rules = p.parseListOfDeclarations(listOfDeclarationsOpts{
					canInlineNoOpNesting: context.canInlineNoOpNesting,
				})
			} else {
				rules = p.parseListOfRules(ruleContext{
					parseSelectors: true,
				})
			}
			if len(names) != 1 {
				p.anonLayerCount--
			}
			p.enclosingLayer = oldEnclosingLayer
			closeBraceLoc := p.current().Range.Loc
			if !p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
				closeBraceLoc = logger.Loc{}
			}
			return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtLayer{Names: names, Rules: rules, CloseBraceLoc: closeBraceLoc}}
		}

		// Handle lack of a block
		if len(names) >= 1 && p.eat(css_lexer.TSemicolon) {
			p.recordAtLayerRule(names)
			return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtLayer{Names: names}}
		}

		// Otherwise there's some kind of syntax error
		switch p.current().Kind {
		case css_lexer.TEndOfFile:
			p.expect(css_lexer.TSemicolon)
			p.recordAtLayerRule(names)
			return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtLayer{Names: names}}

		case css_lexer.TCloseBrace:
			p.expect(css_lexer.TSemicolon)
			if !context.isTopLevel {
				p.recordAtLayerRule(names)
				return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RAtLayer{Names: names}}
			}

		case css_lexer.TOpenBrace:
			p.expect(css_lexer.TSemicolon)

		default:
			p.unexpected()
		}

	default:
		if kind == atRuleUnknown && lowerAtToken == "namespace" {
			// CSS namespaces are a weird feature that appears to only really be
			// useful for styling XML. And the world has moved on from XHTML to
			// HTML5 so pretty much no one uses CSS namespaces anymore. They are
			// also complicated to support in a bundler because CSS namespaces are
			// file-scoped, which means:
			//
			// * Default namespaces can be different in different files, in which
			//   case some default namespaces would have to be converted to prefixed
			//   namespaces to avoid collisions.
			//
			// * Prefixed namespaces from different files can use the same name, in
			//   which case some prefixed namespaces would need to be renamed to
			//   avoid collisions.
			//
			// Instead of implementing all of that for an extremely obscure feature,
			// CSS namespaces are just explicitly not supported.
			p.log.AddID(logger.MsgID_CSS_UnsupportedAtNamespace, logger.Warning, &p.tracker, atRange, "\"@namespace\" rules are not supported")
		}
	}

	// Parse an unknown prelude
prelude:
	for {
		switch p.current().Kind {
		case css_lexer.TOpenBrace, css_lexer.TEndOfFile:
			break prelude

		case css_lexer.TSemicolon, css_lexer.TCloseBrace:
			prelude := p.convertTokens(p.tokens[preludeStart:p.index])

			switch kind {
			case atRuleQualifiedOrEmpty:
				// Parse a known at rule below
				break prelude

			case atRuleEmpty, atRuleUnknown:
				// Parse an unknown at rule
				p.expect(css_lexer.TSemicolon)
				return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude}}

			default:
				// Report an error for rules that should have blocks
				p.expect(css_lexer.TOpenBrace)
				p.eat(css_lexer.TSemicolon)
				return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude}}
			}

		default:
			p.parseComponentValue()
		}
	}
	prelude := p.convertTokens(p.tokens[preludeStart:p.index])
	blockStart := p.index

	switch kind {
	case atRuleEmpty:
		// Report an error for rules that shouldn't have blocks
		p.expect(css_lexer.TSemicolon)
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)
		block := p.convertTokens(p.tokens[blockStart:p.index])
		return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude, Block: block}}

	case atRuleDeclarations:
		// Parse known rules whose blocks always consist of declarations
		matchingLoc := p.current().Range.Loc
		p.expect(css_lexer.TOpenBrace)
		rules := p.parseListOfDeclarations(listOfDeclarationsOpts{})
		closeBraceLoc := p.current().Range.Loc
		if !p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
			closeBraceLoc = logger.Loc{}
		}

		// Handle local names for "@counter-style"
		if len(prelude) == 1 && lowerAtToken == "counter-style" {
			if t := &prelude[0]; t.Kind == css_lexer.TIdent {
				t.Kind = css_lexer.TSymbol
				t.PayloadIndex = p.symbolForName(t.Loc, t.Text).Ref.InnerIndex
			}
		}

		return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude, Rules: rules, CloseBraceLoc: closeBraceLoc}}

	case atRuleInheritContext:
		// Parse known rules whose blocks consist of whatever the current context is
		matchingLoc := p.current().Range.Loc
		p.expect(css_lexer.TOpenBrace)
		var rules []css_ast.Rule

		// Push the "@media" conditions
		isAtMedia := lowerAtToken == "media"
		if isAtMedia {
			p.enclosingAtMedia = append(p.enclosingAtMedia, prelude)
		}

		// Parse the block for this rule
		if context.isDeclarationList {
			rules = p.parseListOfDeclarations(listOfDeclarationsOpts{
				canInlineNoOpNesting: context.canInlineNoOpNesting,
			})
		} else {
			rules = p.parseListOfRules(ruleContext{
				parseSelectors: true,
			})
		}

		// Pop the "@media" conditions
		if isAtMedia {
			p.enclosingAtMedia = p.enclosingAtMedia[:len(p.enclosingAtMedia)-1]
		}

		closeBraceLoc := p.current().Range.Loc
		if !p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
			closeBraceLoc = logger.Loc{}
		}

		// Handle local names for "@container"
		if len(prelude) >= 1 && lowerAtToken == "container" {
			if t := &prelude[0]; t.Kind == css_lexer.TIdent && strings.ToLower(t.Text) != "not" {
				t.Kind = css_lexer.TSymbol
				t.PayloadIndex = p.symbolForName(t.Loc, t.Text).Ref.InnerIndex
			}
		}

		return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude, Rules: rules, CloseBraceLoc: closeBraceLoc}}

	case atRuleQualifiedOrEmpty:
		matchingLoc := p.current().Range.Loc
		if p.eat(css_lexer.TOpenBrace) {
			rules := p.parseListOfRules(ruleContext{
				parseSelectors: true,
			})
			closeBraceLoc := p.current().Range.Loc
			if !p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
				closeBraceLoc = logger.Loc{}
			}
			return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude, Rules: rules, CloseBraceLoc: closeBraceLoc}}
		}
		p.expect(css_lexer.TSemicolon)
		return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RKnownAt{AtToken: atToken, Prelude: prelude}}

	default:
		// Otherwise, parse an unknown rule
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)
		block, _ := p.convertTokensHelper(p.tokens[blockStart:p.index], css_lexer.TEndOfFile, convertTokensOpts{allowImports: true})
		return css_ast.Rule{Loc: atRange.Loc, Data: &css_ast.RUnknownAt{AtToken: atToken, Prelude: prelude, Block: block}}
	}
}

func (p *parser) expectValidLayerNameIdent() (string, bool) {
	r := p.current().Range
	text := p.decoded()
	if !p.expect(css_lexer.TIdent) {
		return "", false
	}
	switch text {
	case "initial", "inherit", "unset":
		p.log.AddID(logger.MsgID_CSS_InvalidAtLayer, logger.Warning, &p.tracker, r, fmt.Sprintf("%q cannot be used as a layer name", text))
		p.prevError = r.Loc
		return "", false
	}
	return text, true
}

func (p *parser) convertTokens(tokens []css_lexer.Token) []css_ast.Token {
	result, _ := p.convertTokensHelper(tokens, css_lexer.TEndOfFile, convertTokensOpts{})
	return result
}

type convertTokensOpts struct {
	allowImports         bool
	verbatimWhitespace   bool
	isInsideCalcFunction bool
}

func (p *parser) convertTokensHelper(tokens []css_lexer.Token, close css_lexer.T, opts convertTokensOpts) ([]css_ast.Token, []css_lexer.Token) {
	result := []css_ast.Token{}
	var nextWhitespace css_ast.WhitespaceFlags

	// Enable verbatim whitespace mode when the first two non-whitespace tokens
	// are a CSS variable name followed by a colon. This is because it could be
	// a form of CSS variable usage, and removing whitespace could potentially
	// break this usage. For example, the following CSS is ignored by Chrome if
	// the whitespace isn't preserved:
	//
	//   @supports (--foo: ) {
	//     html { background: green; }
	//   }
	//
	// Strangely whitespace removal doesn't cause the declaration to be ignored
	// in Firefox or Safari, so there's definitely a browser bug somewhere.
	if !opts.verbatimWhitespace {
		for i, t := range tokens {
			if t.Kind == css_lexer.TWhitespace {
				continue
			}
			if t.Kind == css_lexer.TIdent && strings.HasPrefix(t.DecodedText(p.source.Contents), "--") {
				for _, t := range tokens[i+1:] {
					if t.Kind == css_lexer.TWhitespace {
						continue
					}
					if t.Kind == css_lexer.TColon {
						opts.verbatimWhitespace = true
					}
					break
				}
			}
			break
		}
	}

loop:
	for len(tokens) > 0 {
		t := tokens[0]
		tokens = tokens[1:]
		if t.Kind == close {
			break loop
		}
		token := css_ast.Token{
			Loc:        t.Range.Loc,
			Kind:       t.Kind,
			Text:       t.DecodedText(p.source.Contents),
			Whitespace: nextWhitespace,
		}
		nextWhitespace = 0

		// Warn about invalid "+" and "-" operators that break the containing "calc()"
		if opts.isInsideCalcFunction && t.Kind.IsNumeric() && len(result) > 0 && result[len(result)-1].Kind.IsNumeric() &&
			(strings.HasPrefix(token.Text, "+") || strings.HasPrefix(token.Text, "-")) {
			// "calc(1+2)" and "calc(1-2)" are invalid
			p.log.AddID(logger.MsgID_CSS_InvalidCalc, logger.Warning, &p.tracker, logger.Range{Loc: t.Range.Loc, Len: 1},
				fmt.Sprintf("The %q operator only works if there is whitespace on both sides", token.Text[:1]))
		}

		switch t.Kind {
		case css_lexer.TWhitespace:
			if last := len(result) - 1; last >= 0 {
				result[last].Whitespace |= css_ast.WhitespaceAfter
			}
			nextWhitespace = css_ast.WhitespaceBefore
			continue

		case css_lexer.TDelimPlus, css_lexer.TDelimMinus:
			// Warn about invalid "+" and "-" operators that break the containing "calc()"
			if opts.isInsideCalcFunction && len(tokens) > 0 {
				if len(result) == 0 || result[len(result)-1].Kind == css_lexer.TComma {
					// "calc(-(1 + 2))" is invalid
					p.log.AddID(logger.MsgID_CSS_InvalidCalc, logger.Warning, &p.tracker, t.Range,
						fmt.Sprintf("%q can only be used as an infix operator, not a prefix operator", token.Text))
				} else if token.Whitespace != css_ast.WhitespaceBefore || tokens[0].Kind != css_lexer.TWhitespace {
					// "calc(1- 2)" and "calc(1 -(2))" are invalid
					p.log.AddID(logger.MsgID_CSS_InvalidCalc, logger.Warning, &p.tracker, t.Range,
						fmt.Sprintf("The %q operator only works if there is whitespace on both sides", token.Text))
				}
			}

		case css_lexer.TNumber:
			if p.options.minifySyntax {
				if text, ok := mangleNumber(token.Text); ok {
					token.Text = text
				}
			}

		case css_lexer.TPercentage:
			if p.options.minifySyntax {
				if text, ok := mangleNumber(token.PercentageValue()); ok {
					token.Text = text + "%"
				}
			}

		case css_lexer.TDimension:
			token.UnitOffset = t.UnitOffset

			if p.options.minifySyntax {
				if text, ok := mangleNumber(token.DimensionValue()); ok {
					token.Text = text + token.DimensionUnit()
					token.UnitOffset = uint16(len(text))
				}

				if value, unit, ok := mangleDimension(token.DimensionValue(), token.DimensionUnit()); ok {
					token.Text = value + unit
					token.UnitOffset = uint16(len(value))
				}
			}

		case css_lexer.TURL:
			token.PayloadIndex = uint32(len(p.importRecords))
			var flags ast.ImportRecordFlags
			if !opts.allowImports {
				flags |= ast.IsUnused
			}
			p.importRecords = append(p.importRecords, ast.ImportRecord{
				Kind:  ast.ImportURL,
				Path:  logger.Path{Text: token.Text},
				Range: t.Range,
				Flags: flags,
			})
			token.Text = ""

		case css_lexer.TFunction:
			var nested []css_ast.Token
			original := tokens
			nestedOpts := opts
			if strings.EqualFold(token.Text, "var") {
				// CSS variables require verbatim whitespace for correctness
				nestedOpts.verbatimWhitespace = true
			}
			if strings.EqualFold(token.Text, "calc") {
				nestedOpts.isInsideCalcFunction = true
			}
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseParen, nestedOpts)
			token.Children = &nested

			// Apply "calc" simplification rules when minifying
			if p.options.minifySyntax && strings.EqualFold(token.Text, "calc") {
				token = p.tryToReduceCalcExpression(token)
			}

			// Treat a URL function call with a string just like a URL token
			if strings.EqualFold(token.Text, "url") && len(nested) == 1 && nested[0].Kind == css_lexer.TString {
				token.Kind = css_lexer.TURL
				token.Text = ""
				token.Children = nil
				token.PayloadIndex = uint32(len(p.importRecords))
				var flags ast.ImportRecordFlags
				if !opts.allowImports {
					flags |= ast.IsUnused
				}
				p.importRecords = append(p.importRecords, ast.ImportRecord{
					Kind:  ast.ImportURL,
					Path:  logger.Path{Text: nested[0].Text},
					Range: original[0].Range,
					Flags: flags,
				})
			}

		case css_lexer.TOpenParen:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseParen, opts)
			token.Children = &nested

		case css_lexer.TOpenBrace:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseBrace, opts)

			// Pretty-printing: insert leading and trailing whitespace when not minifying
			if !opts.verbatimWhitespace && !p.options.minifyWhitespace && len(nested) > 0 {
				nested[0].Whitespace |= css_ast.WhitespaceBefore
				nested[len(nested)-1].Whitespace |= css_ast.WhitespaceAfter
			}

			token.Children = &nested

		case css_lexer.TOpenBracket:
			var nested []css_ast.Token
			nested, tokens = p.convertTokensHelper(tokens, css_lexer.TCloseBracket, opts)
			token.Children = &nested
		}

		result = append(result, token)
	}

	if !opts.verbatimWhitespace {
		for i := range result {
			token := &result[i]

			// Always remove leading and trailing whitespace
			if i == 0 {
				token.Whitespace &= ^css_ast.WhitespaceBefore
			}
			if i+1 == len(result) {
				token.Whitespace &= ^css_ast.WhitespaceAfter
			}

			switch token.Kind {
			case css_lexer.TComma:
				// Assume that whitespace can always be removed before a comma
				token.Whitespace &= ^css_ast.WhitespaceBefore
				if i > 0 {
					result[i-1].Whitespace &= ^css_ast.WhitespaceAfter
				}

				// Assume whitespace can always be added after a comma
				if p.options.minifyWhitespace {
					token.Whitespace &= ^css_ast.WhitespaceAfter
					if i+1 < len(result) {
						result[i+1].Whitespace &= ^css_ast.WhitespaceBefore
					}
				} else {
					token.Whitespace |= css_ast.WhitespaceAfter
					if i+1 < len(result) {
						result[i+1].Whitespace |= css_ast.WhitespaceBefore
					}
				}
			}
		}
	}

	// Insert an explicit whitespace token if we're in verbatim mode and all
	// tokens were whitespace. In this case there is no token to attach the
	// whitespace before/after flags so this is the only way to represent this.
	// This is the only case where this function generates an explicit whitespace
	// token. It represents whitespace as flags in all other cases.
	if opts.verbatimWhitespace && len(result) == 0 && nextWhitespace == css_ast.WhitespaceBefore {
		result = append(result, css_ast.Token{
			Kind: css_lexer.TWhitespace,
		})
	}

	return result, tokens
}

func shiftDot(text string, dotOffset int) (string, bool) {
	// This doesn't handle numbers with exponents
	if strings.ContainsAny(text, "eE") {
		return "", false
	}

	// Handle a leading sign
	sign := ""
	if len(text) > 0 && (text[0] == '-' || text[0] == '+') {
		sign = text[:1]
		text = text[1:]
	}

	// Remove the dot
	dot := strings.IndexByte(text, '.')
	if dot == -1 {
		dot = len(text)
	} else {
		text = text[:dot] + text[dot+1:]
	}

	// Move the dot
	dot += dotOffset

	// Remove any leading zeros before the dot
	for len(text) > 0 && dot > 0 && text[0] == '0' {
		text = text[1:]
		dot--
	}

	// Remove any trailing zeros after the dot
	for len(text) > 0 && len(text) > dot && text[len(text)-1] == '0' {
		text = text[:len(text)-1]
	}

	// Does this number have no fractional component?
	if dot >= len(text) {
		trailingZeros := strings.Repeat("0", dot-len(text))
		return fmt.Sprintf("%s%s%s", sign, text, trailingZeros), true
	}

	// Potentially add leading zeros
	if dot < 0 {
		text = strings.Repeat("0", -dot) + text
		dot = 0
	}

	// Insert the dot again
	return fmt.Sprintf("%s%s.%s", sign, text[:dot], text[dot:]), true
}

func mangleDimension(value string, unit string) (string, string, bool) {
	const msLen = 2
	const sLen = 1

	// Mangle times: https://developer.mozilla.org/en-US/docs/Web/CSS/time
	if strings.EqualFold(unit, "ms") {
		if shifted, ok := shiftDot(value, -3); ok && len(shifted)+sLen < len(value)+msLen {
			// Convert "ms" to "s" if shorter
			return shifted, "s", true
		}
	}
	if strings.EqualFold(unit, "s") {
		if shifted, ok := shiftDot(value, 3); ok && len(shifted)+msLen < len(value)+sLen {
			// Convert "s" to "ms" if shorter
			return shifted, "ms", true
		}
	}

	return "", "", false
}

func mangleNumber(t string) (string, bool) {
	original := t

	if dot := strings.IndexByte(t, '.'); dot != -1 {
		// Remove trailing zeros
		for len(t) > 0 && t[len(t)-1] == '0' {
			t = t[:len(t)-1]
		}

		// Remove the decimal point if it's unnecessary
		if dot+1 == len(t) {
			t = t[:dot]
			if t == "" || t == "+" || t == "-" {
				t += "0"
			}
		} else {
			// Remove a leading zero
			if len(t) >= 3 && t[0] == '0' && t[1] == '.' && t[2] >= '0' && t[2] <= '9' {
				t = t[1:]
			} else if len(t) >= 4 && (t[0] == '+' || t[0] == '-') && t[1] == '0' && t[2] == '.' && t[3] >= '0' && t[3] <= '9' {
				t = t[0:1] + t[2:]
			}
		}
	}

	return t, t != original
}

func (p *parser) parseSelectorRule(isTopLevel bool, opts parseSelectorOpts) css_ast.Rule {
	// Save and restore the local symbol state in case there are any bare
	// ":global" or ":local" annotations. The effect of these should be scoped
	// to within the selector rule.
	local := p.makeLocalSymbols
	preludeStart := p.index

	// Try parsing the prelude as a selector list
	if list, ok := p.parseSelectorList(opts); ok {
		canInlineNoOpNesting := true
		for _, sel := range list {
			// We cannot transform the CSS "a, b::before { & { color: red } }" into
			// "a, b::before { color: red }" because it's basically equivalent to
			// ":is(a, b::before) { color: red }" which only applies to "a", not to
			// "b::before" because pseudo-elements are not valid within :is():
			// https://www.w3.org/TR/selectors-4/#matches-pseudo. This restriction
			// may be relaxed in the future, but this restriction hash shipped so
			// we're stuck with it: https://github.com/w3c/csswg-drafts/issues/7433.
			if sel.UsesPseudoElement() {
				canInlineNoOpNesting = false
				break
			}
		}
		selector := css_ast.RSelector{Selectors: list}
		matchingLoc := p.current().Range.Loc
		if p.expect(css_lexer.TOpenBrace) {
			p.inSelectorSubtree++
			declOpts := listOfDeclarationsOpts{
				canInlineNoOpNesting: canInlineNoOpNesting,
			}

			// Prepare for "composes" declarations
			if opts.composesContext != nil && len(list) == 1 && len(list[0].Selectors) == 1 && list[0].Selectors[0].IsSingleAmpersand() {
				// Support code like this:
				//
				//   .foo {
				//     :local { composes: bar }
				//     :global { composes: baz }
				//   }
				//
				declOpts.composesContext = opts.composesContext
			} else {
				composesContext := composesContext{parentRange: list[0].Selectors[0].Range()}
				if opts.composesContext != nil {
					composesContext.problemRange = opts.composesContext.parentRange
				}
				for _, sel := range list {
					first := sel.Selectors[0]
					if first.Combinator.Byte != 0 {
						composesContext.problemRange = logger.Range{Loc: first.Combinator.Loc, Len: 1}
					} else if first.TypeSelector != nil {
						composesContext.problemRange = first.TypeSelector.Range()
					} else if len(first.NestingSelectorLocs) > 0 {
						composesContext.problemRange = logger.Range{Loc: first.NestingSelectorLocs[0], Len: 1}
					} else {
						for i, ss := range first.SubclassSelectors {
							class, ok := ss.Data.(*css_ast.SSClass)
							if i > 0 || !ok {
								composesContext.problemRange = ss.Range
							} else {
								composesContext.parentRefs = append(composesContext.parentRefs, class.Name.Ref)
							}
						}
					}
					if composesContext.problemRange.Len > 0 {
						break
					}
					if len(sel.Selectors) > 1 {
						composesContext.problemRange = sel.Selectors[1].Range()
						break
					}
				}
				declOpts.composesContext = &composesContext
			}

			selector.Rules = p.parseListOfDeclarations(declOpts)
			p.inSelectorSubtree--
			closeBraceLoc := p.current().Range.Loc
			if p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
				selector.CloseBraceLoc = closeBraceLoc
			}
			p.makeLocalSymbols = local
			return css_ast.Rule{Loc: p.tokens[preludeStart].Range.Loc, Data: &selector}
		}
	}

	p.makeLocalSymbols = local
	p.index = preludeStart

	// Otherwise, parse a generic qualified rule
	return p.parseQualifiedRule(parseQualifiedRuleOpts{
		isAlreadyInvalid:     true,
		isTopLevel:           isTopLevel,
		isDeclarationContext: opts.isDeclarationContext,
	})
}

type parseQualifiedRuleOpts struct {
	isAlreadyInvalid     bool
	isTopLevel           bool
	isDeclarationContext bool
}

func (p *parser) parseQualifiedRule(opts parseQualifiedRuleOpts) css_ast.Rule {
	preludeStart := p.index
	preludeLoc := p.current().Range.Loc

loop:
	for {
		switch p.current().Kind {
		case css_lexer.TOpenBrace, css_lexer.TEndOfFile:
			break loop

		case css_lexer.TCloseBrace:
			if !opts.isTopLevel {
				break loop
			}

		case css_lexer.TSemicolon:
			if opts.isDeclarationContext {
				return css_ast.Rule{Loc: preludeLoc, Data: &css_ast.RBadDeclaration{
					Tokens: p.convertTokens(p.tokens[preludeStart:p.index]),
				}}
			}
		}

		p.parseComponentValue()
	}

	qualified := css_ast.RQualified{
		Prelude: p.convertTokens(p.tokens[preludeStart:p.index]),
	}

	matchingLoc := p.current().Range.Loc
	if p.eat(css_lexer.TOpenBrace) {
		qualified.Rules = p.parseListOfDeclarations(listOfDeclarationsOpts{})
		closeBraceLoc := p.current().Range.Loc
		if p.expectWithMatchingLoc(css_lexer.TCloseBrace, matchingLoc) {
			qualified.CloseBraceLoc = closeBraceLoc
		}
	} else if !opts.isAlreadyInvalid {
		p.expect(css_lexer.TOpenBrace)
	}

	return css_ast.Rule{Loc: preludeLoc, Data: &qualified}
}

type endOfRuleScan uint8

const (
	endOfRuleUnknown endOfRuleScan = iota
	endOfRuleSemicolon
	endOfRuleOpenBrace
)

// Note: This was a late change to the CSS nesting syntax.
// See also: https://github.com/w3c/csswg-drafts/issues/7961
func (p *parser) scanForEndOfRule() (endOfRuleScan, int) {
	var initialStack [4]css_lexer.T
	stack := initialStack[:0]

	for i, t := range p.tokens[p.index:] {
		switch t.Kind {
		case css_lexer.TSemicolon:
			if len(stack) == 0 {
				return endOfRuleSemicolon, p.index + i
			}

		case css_lexer.TFunction, css_lexer.TOpenParen:
			stack = append(stack, css_lexer.TCloseParen)

		case css_lexer.TOpenBracket:
			stack = append(stack, css_lexer.TCloseBracket)

		case css_lexer.TOpenBrace:
			if len(stack) == 0 {
				return endOfRuleOpenBrace, p.index + i
			}
			stack = append(stack, css_lexer.TCloseBrace)

		case css_lexer.TCloseParen, css_lexer.TCloseBracket:
			if n := len(stack); n > 0 && t.Kind == stack[n-1] {
				stack = stack[:n-1]
			}

		case css_lexer.TCloseBrace:
			if n := len(stack); n > 0 && t.Kind == stack[n-1] {
				stack = stack[:n-1]
			} else {
				return endOfRuleUnknown, -1
			}
		}
	}

	return endOfRuleUnknown, -1
}

func (p *parser) parseDeclaration() css_ast.Rule {
	// Parse the key
	keyStart := p.index
	keyRange := p.tokens[keyStart].Range
	keyIsIdent := p.expect(css_lexer.TIdent)
	ok := false
	if keyIsIdent {
		p.eat(css_lexer.TWhitespace)
		ok = p.eat(css_lexer.TColon)
	}

	// Parse the value
	valueStart := p.index
stop:
	for {
		switch p.current().Kind {
		case css_lexer.TEndOfFile, css_lexer.TSemicolon, css_lexer.TCloseBrace:
			break stop

		default:
			p.parseComponentValue()
		}
	}

	// Stop now if this is not a valid declaration
	if !ok {
		if keyIsIdent {
			if end := keyRange.End(); end > p.prevError.Start {
				p.prevError.Start = end
				data := p.tracker.MsgData(logger.Range{Loc: logger.Loc{Start: end}}, "Expected \":\"")
				data.Location.Suggestion = ":"
				p.log.AddMsgID(logger.MsgID_CSS_CSSSyntaxError, logger.Msg{
					Kind: logger.Warning,
					Data: data,
				})
			}
		}

		return css_ast.Rule{Loc: keyRange.Loc, Data: &css_ast.RBadDeclaration{
			Tokens: p.convertTokens(p.tokens[keyStart:p.index]),
		}}
	}

	keyToken := p.tokens[keyStart]
	keyText := keyToken.DecodedText(p.source.Contents)
	value := p.tokens[valueStart:p.index]
	verbatimWhitespace := strings.HasPrefix(keyText, "--")

	// Remove trailing "!important"
	important := false
	i := len(value) - 1
	if i >= 0 && value[i].Kind == css_lexer.TWhitespace {
		i--
	}
	if i >= 0 && value[i].Kind == css_lexer.TIdent && strings.EqualFold(value[i].DecodedText(p.source.Contents), "important") {
		i--
		if i >= 0 && value[i].Kind == css_lexer.TWhitespace {
			i--
		}
		if i >= 0 && value[i].Kind == css_lexer.TDelimExclamation {
			value = value[:i]
			important = true
		}
	}

	result, _ := p.convertTokensHelper(value, css_lexer.TEndOfFile, convertTokensOpts{
		allowImports: true,

		// CSS variables require verbatim whitespace for correctness
		verbatimWhitespace: verbatimWhitespace,
	})

	// Insert or remove whitespace before the first token
	if !verbatimWhitespace && len(result) > 0 {
		if p.options.minifyWhitespace {
			result[0].Whitespace &= ^css_ast.WhitespaceBefore
		} else {
			result[0].Whitespace |= css_ast.WhitespaceBefore
		}
	}

	lowerKeyText := strings.ToLower(keyText)
	key := css_ast.KnownDeclarations[lowerKeyText]

	// Attempt to point out trivial typos
	if key == css_ast.DUnknown {
		if corrected, ok := css_ast.MaybeCorrectDeclarationTypo(lowerKeyText); ok {
			data := p.tracker.MsgData(keyToken.Range, fmt.Sprintf("%q is not a known CSS property", keyText))
			data.Location.Suggestion = corrected
			p.log.AddMsgID(logger.MsgID_CSS_UnsupportedCSSProperty, logger.Msg{Kind: logger.Warning, Data: data,
				Notes: []logger.MsgData{{Text: fmt.Sprintf("Did you mean %q instead?", corrected)}}})
		}
	}

	return css_ast.Rule{Loc: keyRange.Loc, Data: &css_ast.RDeclaration{
		Key:       key,
		KeyText:   keyText,
		KeyRange:  keyToken.Range,
		Value:     result,
		Important: important,
	}}
}

func (p *parser) parseComponentValue() {
	switch p.current().Kind {
	case css_lexer.TFunction:
		p.parseBlock(css_lexer.TFunction, css_lexer.TCloseParen)

	case css_lexer.TOpenParen:
		p.parseBlock(css_lexer.TOpenParen, css_lexer.TCloseParen)

	case css_lexer.TOpenBrace:
		p.parseBlock(css_lexer.TOpenBrace, css_lexer.TCloseBrace)

	case css_lexer.TOpenBracket:
		p.parseBlock(css_lexer.TOpenBracket, css_lexer.TCloseBracket)

	case css_lexer.TEndOfFile:
		p.unexpected()

	default:
		p.advance()
	}
}

func (p *parser) parseBlock(open css_lexer.T, close css_lexer.T) {
	current := p.current()
	matchingStart := current.Range.End() - 1
	if p.expect(open) {
		for !p.eat(close) {
			if p.peek(css_lexer.TEndOfFile) {
				p.expectWithMatchingLoc(close, logger.Loc{Start: matchingStart})
				return
			}

			p.parseComponentValue()
		}
	}
}
