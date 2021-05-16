package bundler

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/graph"
	"github.com/evanw/esbuild/internal/js_ast"
	"github.com/evanw/esbuild/internal/js_printer"
)

// Set this to true and then load the resulting metafile in "graph-debugger.html"
// to debug graph information.
//
// This is deliberately not exposed in the final binary. It is *very* internal
// and only exists to help debug esbuild itself. Make sure this is always set
// back to false before committing.
const debugVerboseMetafile = false

func (c *linkerContext) generateExtraDataForFileJS(sourceIndex uint32) string {
	if !debugVerboseMetafile {
		return ""
	}

	file := &c.graph.Files[sourceIndex]
	repr := file.InputFile.Repr.(*graph.JSRepr)
	sb := strings.Builder{}

	quoteSym := func(ref js_ast.Ref) string {
		name := fmt.Sprintf("%d:%d [%s]", ref.SourceIndex, ref.InnerIndex, c.graph.Symbols.Get(ref).OriginalName)
		return string(js_printer.QuoteForJSON(name, c.options.ASCIIOnly))
	}

	sb.WriteString(`,"parts":[`)
	for partIndex, part := range repr.AST.Parts {
		if partIndex > 0 {
			sb.WriteByte(',')
		}
		var isFirst bool
		code := ""

		sb.WriteString(fmt.Sprintf(`{"isLive":%v`, part.IsLive))
		sb.WriteString(fmt.Sprintf(`,"canBeRemovedIfUnused":%v`, part.CanBeRemovedIfUnused))

		if partIndex == int(js_ast.NSExportPartIndex) {
			sb.WriteString(`,"nsExportPartIndex":true`)
		} else if ast.MakeIndex32(uint32(partIndex)) == repr.Meta.WrapperPartIndex {
			sb.WriteString(`,"wrapperPartIndex":true`)
		} else if len(part.Stmts) > 0 {
			start := part.Stmts[0].Loc.Start
			end := len(file.InputFile.Source.Contents)
			if partIndex+1 < len(repr.AST.Parts) {
				if nextStmts := repr.AST.Parts[partIndex+1].Stmts; len(nextStmts) > 0 {
					if nextStart := nextStmts[0].Loc.Start; nextStart >= start {
						end = int(nextStart)
					}
				}
			}
			code = file.InputFile.Source.Contents[start:end]
		}

		// importRecords
		sb.WriteString(`,"importRecords":[`)
		isFirst = true
		for _, importRecordIndex := range part.ImportRecordIndices {
			record := repr.AST.ImportRecords[importRecordIndex]
			if !record.SourceIndex.IsValid() {
				continue
			}
			if isFirst {
				isFirst = false
			} else {
				sb.WriteByte(',')
			}
			path := c.graph.Files[record.SourceIndex.GetIndex()].InputFile.Source.PrettyPath
			sb.WriteString(fmt.Sprintf(`{"source":%s}`, js_printer.QuoteForJSON(path, c.options.ASCIIOnly)))
		}
		sb.WriteByte(']')

		// declaredSymbols
		sb.WriteString(`,"declaredSymbols":[`)
		isFirst = true
		for _, declSym := range part.DeclaredSymbols {
			if !declSym.IsTopLevel {
				continue
			}
			if isFirst {
				isFirst = false
			} else {
				sb.WriteByte(',')
			}
			sb.WriteString(fmt.Sprintf(`{"name":%s}`, quoteSym(declSym.Ref)))
		}
		sb.WriteByte(']')

		// symbolUses
		sb.WriteString(`,"symbolUses":[`)
		isFirst = true
		for ref, uses := range part.SymbolUses {
			if isFirst {
				isFirst = false
			} else {
				sb.WriteByte(',')
			}
			sb.WriteString(fmt.Sprintf(`{"name":%s,"countEstimate":%d}`, quoteSym(ref), uses.CountEstimate))
		}
		sb.WriteByte(']')

		// dependencies
		sb.WriteString(`,"dependencies":[`)
		for i, dep := range part.Dependencies {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(fmt.Sprintf(`{"source":%s,"partIndex":%d}`,
				js_printer.QuoteForJSON(c.graph.Files[dep.SourceIndex].InputFile.Source.PrettyPath, c.options.ASCIIOnly),
				dep.PartIndex,
			))
		}
		sb.WriteByte(']')

		// code
		sb.WriteString(`,"code":`)
		sb.Write(js_printer.QuoteForJSON(strings.TrimRight(code, "\n"), c.options.ASCIIOnly))

		sb.WriteByte('}')
	}
	sb.WriteString(`]`)

	return sb.String()
}
