package cache

import (
	"sync"

	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_parser"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_parser"
	"github.com/evanw/esbuild/internal/logger"
)

// This cache intends to avoid unnecessarily re-parsing files in subsequent
// builds. For a given path, parsing can be avoided if the contents of the file
// and the options for the parser are the same as last time. Even if the
// contents of the file are the same, the options for the parser may have
// changed if they depend on some other file ("package.json" for example).
//
// This cache checks if the file contents have changed even though we have
// the ability to detect if a file has changed on the file system by reading
// its metadata. First of all, if the file contents are cached then they should
// be the same pointer, which makes the comparison trivial. Also we want to
// cache the AST for plugins in the common case that the plugin output stays
// the same.

////////////////////////////////////////////////////////////////////////////////
// CSS

type CSSCache struct {
	entries map[logger.Path]*cssCacheEntry
	mutex   sync.Mutex
}

type cssCacheEntry struct {
	source  logger.Source
	msgs    []logger.Msg
	ast     css_ast.AST
	options css_parser.Options
}

func (c *CSSCache) Parse(log logger.Log, source logger.Source, options css_parser.Options) css_ast.AST {
	// Check the cache
	entry := func() *cssCacheEntry {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		return c.entries[source.KeyPath]
	}()

	// Cache hit
	if entry != nil && entry.source == source && entry.options.Equal(&options) {
		for _, msg := range entry.msgs {
			log.AddMsg(msg)
		}
		return entry.ast
	}

	// Cache miss
	tempLog := logger.NewDeferLog(logger.DeferLogAll, log.Overrides)
	ast := css_parser.Parse(tempLog, source, options)
	msgs := tempLog.Done()
	for _, msg := range msgs {
		log.AddMsg(msg)
	}

	// Create the cache entry
	entry = &cssCacheEntry{
		source:  source,
		options: options,
		ast:     ast,
		msgs:    msgs,
	}

	// Save for next time
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.entries[source.KeyPath] = entry
	return ast
}

////////////////////////////////////////////////////////////////////////////////
// JSON

type JSONCache struct {
	entries map[logger.Path]*jsonCacheEntry
	mutex   sync.Mutex
}

type jsonCacheEntry struct {
	expr    js_ast.Expr
	msgs    []logger.Msg
	source  logger.Source
	options js_parser.JSONOptions
	ok      bool
}

func (c *JSONCache) Parse(log logger.Log, source logger.Source, options js_parser.JSONOptions) (js_ast.Expr, bool) {
	// Check the cache
	entry := func() *jsonCacheEntry {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		return c.entries[source.KeyPath]
	}()

	// Cache hit
	if entry != nil && entry.source == source && entry.options == options {
		for _, msg := range entry.msgs {
			log.AddMsg(msg)
		}
		return entry.expr, entry.ok
	}

	// Cache miss
	tempLog := logger.NewDeferLog(logger.DeferLogAll, log.Overrides)
	expr, ok := js_parser.ParseJSON(tempLog, source, options)
	msgs := tempLog.Done()
	for _, msg := range msgs {
		log.AddMsg(msg)
	}

	// Create the cache entry
	entry = &jsonCacheEntry{
		source:  source,
		options: options,
		expr:    expr,
		ok:      ok,
		msgs:    msgs,
	}

	// Save for next time
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.entries[source.KeyPath] = entry
	return expr, ok
}

////////////////////////////////////////////////////////////////////////////////
// JS

type JSCache struct {
	entries map[logger.Path]*jsCacheEntry
	mutex   sync.Mutex
}

type jsCacheEntry struct {
	source  logger.Source
	msgs    []logger.Msg
	options js_parser.Options
	ast     js_ast.AST
	ok      bool
}

func (c *JSCache) Parse(log logger.Log, source logger.Source, options js_parser.Options) (js_ast.AST, bool) {
	// Check the cache
	entry := func() *jsCacheEntry {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		return c.entries[source.KeyPath]
	}()

	// Cache hit
	if entry != nil && entry.source == source && entry.options.Equal(&options) {
		for _, msg := range entry.msgs {
			log.AddMsg(msg)
		}
		return entry.ast, entry.ok
	}

	// Cache miss
	tempLog := logger.NewDeferLog(logger.DeferLogAll, log.Overrides)
	ast, ok := js_parser.Parse(tempLog, source, options)
	msgs := tempLog.Done()
	for _, msg := range msgs {
		log.AddMsg(msg)
	}

	// Create the cache entry
	entry = &jsCacheEntry{
		source:  source,
		options: options,
		ast:     ast,
		ok:      ok,
		msgs:    msgs,
	}

	// Save for next time
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.entries[source.KeyPath] = entry
	return ast, ok
}
