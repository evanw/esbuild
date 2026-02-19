//go:build go1.18

package js_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/js_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/renamer"
	"github.com/evanw/esbuild/internal/test"
)

var jsFuzzSeeds = []string{
	`var x = 1;`,
	`export default function() {}`,
	`import { foo } from 'bar'`,
	`class Foo { #x = 1 }`,
	`async function* gen() { yield await 1 }`,
	`const x = a?.b ?? c`,
	"const x = `hello ${world} ${`nested ${tmpl}`}`",
	`const {a: {b}, ...c} = x`,
	`@dec class Foo { @dec method() {} }`,
	`using x = resource()`,
	`await using x = resource()`,
	`const re = /(?<=x)(?<!y)[^]*/gimsuvy`,
	`const x = '\u0041\u{42}\x43'`,
	`#!/usr/bin/env node
var x = 1`,
	`with (obj) { x }`,
	`outer: for (;;) { inner: for (;;) { break outer; continue inner; } }`,
	`const x = {[Symbol.iterator]: 1, get y() {}, set y(v) {}}`,
	`console.log(import.meta.url)`,
	`const x = import('foo')`,
	`for await (const x of y) {}`,
	`const x = 0n + 123_456n`,
}

func FuzzParseJS(f *testing.F) {
	for _, seed := range jsFuzzSeeds {
		f.Add([]byte(seed))
	}

	options := config.Options{
		OmitRuntimeForTests: true,
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, opts)
	})
}

func FuzzParseAndPrintJS(f *testing.F) {
	for _, seed := range jsFuzzSeeds {
		f.Add([]byte(seed))
	}

	options := config.Options{
		OmitRuntimeForTests: true,
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		tree, _ := Parse(log, source, opts)
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js_printer.Print(tree, symbols, r, js_printer.Options{})
	})
}

func FuzzParseAndPrintJSMangle(f *testing.F) {
	for _, seed := range jsFuzzSeeds {
		f.Add([]byte(seed))
	}

	options := config.Options{
		OmitRuntimeForTests: true,
		MinifySyntax:        true,
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		tree, _ := Parse(log, source, opts)
		symbols := ast.NewSymbolMap(1)
		symbols.SymbolsForSource[0] = tree.Symbols
		r := renamer.NewNoOpRenamer(symbols)
		js_printer.Print(tree, symbols, r, js_printer.Options{
			MinifySyntax:     true,
			MinifyWhitespace: true,
		})
	})
}

func FuzzParseTS(f *testing.F) {
	for _, seed := range jsFuzzSeeds {
		f.Add([]byte(seed))
	}
	f.Add([]byte(`const x: number = 1; interface Foo { bar: string }`))
	f.Add([]byte(`function foo<T extends string>(x: T): T { return x }`))
	f.Add([]byte(`type X = {[K in keyof T]: T[K]}`))
	f.Add([]byte(`const x = <T,>(a: T) => a`))
	f.Add([]byte(`declare module 'foo' { export const x: number }`))
	f.Add([]byte(`enum Foo { A, B = "b" }`))
	f.Add([]byte(`const x = value satisfies Type`))
	f.Add([]byte(`export type { Foo } from 'bar'`))
	f.Add([]byte(`abstract class Foo { abstract method(): void }`))

	options := config.Options{
		OmitRuntimeForTests: true,
		TS: config.TSOptions{
			Parse: true,
		},
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, opts)
	})
}

func FuzzParseJSX(f *testing.F) {
	for _, seed := range jsFuzzSeeds {
		f.Add([]byte(seed))
	}
	f.Add([]byte(`const x = <div className="foo">hello</div>`))
	f.Add([]byte(`const x = <><div/><span/></>`))
	f.Add([]byte(`const x = <div {...props} key={k} />`))
	f.Add([]byte(`const x = <ns:tag ns:attr="val" />`))
	f.Add([]byte(`const x = <div>{...children}</div>`))

	options := config.Options{
		OmitRuntimeForTests: true,
		JSX: config.JSXOptions{
			Parse: true,
		},
	}
	opts := OptionsFromConfig(&options)

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		Parse(log, source, opts)
	})
}

func FuzzParseJSON(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"a": 1, "b": [2, 3], "c": null}`))
	f.Add([]byte(`[1, "two", true, false, null, 1.5e10]`))
	f.Add([]byte(`"hello\nworld\u0041"`))
	f.Add([]byte(`{"nested": {"deeply": {"value": [1, [2, [3]]]}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		source := test.SourceForTest(string(data))
		ParseJSON(log, source, JSONOptions{})
	})
}
