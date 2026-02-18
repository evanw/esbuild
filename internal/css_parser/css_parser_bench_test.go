package css_parser

import (
	"testing"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/config"
	"github.com/evanw/esbuild/internal/css_printer"
	"github.com/evanw/esbuild/internal/logger"
	"github.com/evanw/esbuild/internal/test"
)

const benchCSS = `
:root {
  --primary-color: #3498db;
  --secondary-color: #2ecc71;
  --font-stack: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  --spacing-unit: 8px;
  --border-radius: 4px;
  --transition-speed: 200ms;
}

*,
*::before,
*::after {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: var(--font-stack);
  line-height: 1.6;
  color: #333;
  background-color: #fafafa;
}

.container {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: calc(var(--spacing-unit) * 3);
  max-width: 1200px;
  margin: 0 auto;
  padding: calc(var(--spacing-unit) * 2);
}

.card {
  display: flex;
  flex-direction: column;
  border-radius: var(--border-radius);
  background: white;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  transition: transform var(--transition-speed) ease, box-shadow var(--transition-speed) ease;
  overflow: hidden;

  &:hover {
    transform: translateY(-4px);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
  }

  & .card-header {
    padding: calc(var(--spacing-unit) * 2);
    background: linear-gradient(135deg, var(--primary-color), var(--secondary-color));
    color: white;
    font-size: 1.25rem;
    font-weight: 600;
  }

  & .card-body {
    flex: 1;
    padding: calc(var(--spacing-unit) * 2);
  }
}

.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: calc(var(--spacing-unit) * 1.5) calc(var(--spacing-unit) * 3);
  border: none;
  border-radius: var(--border-radius);
  font-size: 1rem;
  cursor: pointer;
  transition: background-color var(--transition-speed) ease;
}

.btn-primary {
  background-color: var(--primary-color);
  color: white;
}

.btn-primary:hover {
  background-color: #2980b9;
}

@media (max-width: 768px) {
  .container {
    grid-template-columns: 1fr;
    padding: var(--spacing-unit);
  }

  .card .card-header {
    font-size: 1.1rem;
  }
}

@media (prefers-color-scheme: dark) {
  :root {
    --primary-color: #5dade2;
    --secondary-color: #58d68d;
  }

  body {
    color: #e0e0e0;
    background-color: #1a1a2e;
  }

  .card {
    background: #16213e;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
  }
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.animate-in {
  animation: fadeIn 0.4s ease-out forwards;
}

input[type="text"],
input[type="email"],
textarea {
  width: 100%;
  padding: var(--spacing-unit);
  border: 1px solid #ddd;
  border-radius: var(--border-radius);
  font-family: inherit;
  font-size: 1rem;
  transition: border-color var(--transition-speed) ease;
}

input[type="text"]:focus,
input[type="email"]:focus,
textarea:focus {
  outline: none;
  border-color: var(--primary-color);
  box-shadow: 0 0 0 3px rgba(52, 152, 219, 0.2);
}
`

func BenchmarkParseCSS(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, test.SourceForTest(benchCSS), OptionsFromConfig(config.LoaderCSS, &config.Options{}))
	}
}

func BenchmarkParseCSSMinify(b *testing.B) {
	options := config.Options{
		MinifyWhitespace: true,
		MinifySyntax:     true,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
		Parse(log, test.SourceForTest(benchCSS), OptionsFromConfig(config.LoaderCSS, &options))
	}
}

func BenchmarkPrintCSS(b *testing.B) {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	tree := Parse(log, test.SourceForTest(benchCSS), OptionsFromConfig(config.LoaderCSS, &config.Options{}))
	symbols := ast.NewSymbolMap(1)
	symbols.SymbolsForSource[0] = tree.Symbols
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		css_printer.Print(tree, symbols, css_printer.Options{})
	}
}
