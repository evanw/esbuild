package css_parser

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/internal/ast"
	"github.com/evanw/esbuild/internal/css_ast"
	"github.com/evanw/esbuild/internal/css_lexer"
	"github.com/evanw/esbuild/internal/logger"
)

type composesContext struct {
	parentRefs   []ast.Ref
	parentRange  logger.Range
	problemRange logger.Range
}

func (p *parser) handleComposesPragma(context composesContext, tokens []css_ast.Token) {
	type nameWithLoc struct {
		loc  logger.Loc
		text string
	}
	var names []nameWithLoc
	fromGlobal := false

	for i, t := range tokens {
		if t.Kind == css_lexer.TIdent {
			// Check for a "from" clause at the end
			if strings.EqualFold(t.Text, "from") && i+2 == len(tokens) {
				last := tokens[i+1]

				// A string or a URL is an external file
				if last.Kind == css_lexer.TString || last.Kind == css_lexer.TURL {
					var importRecordIndex uint32
					if last.Kind == css_lexer.TString {
						importRecordIndex = uint32(len(p.importRecords))
						p.importRecords = append(p.importRecords, ast.ImportRecord{
							Kind:  ast.ImportComposesFrom,
							Path:  logger.Path{Text: last.Text},
							Range: p.source.RangeOfString(last.Loc),
						})
					} else {
						importRecordIndex = last.PayloadIndex
						p.importRecords[importRecordIndex].Kind = ast.ImportComposesFrom
					}
					for _, parentRef := range context.parentRefs {
						composes := p.composes[parentRef]
						for _, name := range names {
							composes.ImportedNames = append(composes.ImportedNames, css_ast.ImportedComposesName{
								ImportRecordIndex: importRecordIndex,
								Alias:             name.text,
								AliasLoc:          name.loc,
							})
						}
					}
					return
				}

				// An identifier must be "global"
				if last.Kind == css_lexer.TIdent {
					if strings.EqualFold(last.Text, "global") {
						fromGlobal = true
						break
					}

					p.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, css_lexer.RangeOfIdentifier(p.source, last.Loc),
						fmt.Sprintf("\"composes\" declaration uses invalid location %q", last.Text))
					p.prevError = t.Loc
					return
				}
			}

			names = append(names, nameWithLoc{t.Loc, t.Text})
			continue
		}

		// Any unexpected tokens are a syntax error
		var text string
		switch t.Kind {
		case css_lexer.TURL, css_lexer.TBadURL, css_lexer.TString, css_lexer.TUnterminatedString:
			text = fmt.Sprintf("Unexpected %s", t.Kind.String())
		default:
			text = fmt.Sprintf("Unexpected %q", t.Text)
		}
		p.log.AddID(logger.MsgID_CSS_CSSSyntaxError, logger.Warning, &p.tracker, logger.Range{Loc: t.Loc}, text)
		p.prevError = t.Loc
		return
	}

	// If we get here, all of these names are not references to another file
	old := p.makeLocalSymbols
	if fromGlobal {
		p.makeLocalSymbols = false
	}
	for _, parentRef := range context.parentRefs {
		composes := p.composes[parentRef]
		for _, name := range names {
			composes.Names = append(composes.Names, p.symbolForName(name.loc, name.text))
		}
	}
	p.makeLocalSymbols = old
}
