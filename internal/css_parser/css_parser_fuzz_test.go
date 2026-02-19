//go:build go1.18

package css_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/compat"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

var cssFuzzSeeds = []string{
	`body { color: red }`,
	`@media (max-width: 768px) { .x { margin: 0 } }`,
	`:root { --x: calc(1px + 2em) }`,
	`@keyframes spin { from { transform: rotate(0) } to { transform: rotate(360deg) } }`,
	`.a { & .b { color: red } }`,
	`@scope (.card) to (.content) { :scope { border: 1px solid } }`,
	`@layer base, override; @layer base { .x { color: red } }`,
	`@container (min-width: 400px) { .x { font-size: 1.5em } }`,
	`@property --x { syntax: "<color>"; inherits: false; initial-value: red }`,
	`.a { .b { & .c { color: red } } }`,
	`div { width: calc(100% / 3 - 2px * 2) }`,
	`div { color: color-mix(in srgb, red 50%, blue) }`,
	`div:has(> .a):is(.b, .c) { color: red }`,
	`@supports (display: grid) { .x { display: grid } }`,
	`@font-face { unicode-range: U+0025-00FF, U+4?? }`,
	`div { --my-prop: var(--other, fallback-value) }`,
	`.a { composes: b from "file.css" }`,
	`:global(.foo) :local(.bar) { color: red }`,
}

func FuzzParseCSS(f *testing.F) {
	for _, seed := range cssFuzzSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, OptionsFromConfig(config.LoaderCSS, &config.Options{}))
	})
}

func FuzzParseAndPrintCSS(f *testing.F) {
	for _, seed := range cssFuzzSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		tree := Parse(log, source, OptionsFromConfig(config.LoaderCSS, &config.Options{}))
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		css_printer.Print(tree, symbols, css_printer.Options{})
	})
}

func FuzzParseAndPrintCSSMangle(f *testing.F) {
	for _, seed := range cssFuzzSeeds {
		f.Add([]byte(seed))
	}

	opts := &config.Options{
		MinifySyntax:     true,
		MinifyWhitespace: true,
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		tree := Parse(log, source, OptionsFromConfig(config.LoaderCSS, opts))
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		css_printer.Print(tree, symbols, css_printer.Options{
			MinifyWhitespace: true,
		})
	})
}

func FuzzParseCSSLocal(f *testing.F) {
	for _, seed := range cssFuzzSeeds {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, OptionsFromConfig(config.LoaderLocalCSS, &config.Options{}))
	})
}

func FuzzParseCSSLower(f *testing.F) {
	for _, seed := range cssFuzzSeeds {
		f.Add([]byte(seed))
	}

	opts := &config.Options{
		UnsupportedCSSFeatures: ^compat.CSSFeature(0),
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, OptionsFromConfig(config.LoaderCSS, opts))
	})
}
