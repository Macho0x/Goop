package fmt

import (
	"fmt"

	"goop.dev/compiler/internal/ast"
)

func formatDecl(p *Printer, d ast.TopDecl) {
	switch d := d.(type) {
	case *ast.LetDecl:
		formatLetDecl(p, d)
	case *ast.TypeDecl:
		formatTypeDecl(p, d)
	case *ast.ExternDecl:
		formatExternDecl(p, d)
	case *ast.LangEmbedDecl:
		formatLangEmbedDecl(p, d)
	case *ast.ExceptionDecl:
		p.Write("exception " + d.Name)
		if d.Arg != nil {
			p.Write(" of " + formatType(d.Arg))
		}
		p.Newline()
	case *ast.EffectDecl:
		p.Write("effect " + d.Name + " : " + formatType(d.From) + " -> " + formatType(d.To))
		p.Newline()
	case *ast.NestedModuleDecl:
		p.Write("module " + d.Name + " = struct ... end")
		p.Newline()
	case *ast.ModuleTypeDecl:
		p.Write("module type " + d.Name + " = sig ... end")
		p.Newline()
	case *ast.OpenModuleDecl:
		p.Write("open " + d.Path)
		p.Newline()
	case *ast.IncludeDecl:
		p.Write("include " + d.Path)
		p.Newline()
	case *ast.ClassDecl:
		p.Write("class " + d.Name + " = object ... end")
		p.Newline()
	}
}

func formatLetDecl(p *Printer, d *ast.LetDecl) {
	rec := ""
	if d.Rec {
		rec = "rec "
	}
	mut := ""
	if d.Mutable {
		mut = "mutable "
	}
	priv := ""
	if d.Private {
		priv = "private "
	}

	for i, b := range d.Bindings {
		if i > 0 {
			p.WriteIndent()
			p.Write("and")
			p.Newline()
		}
		p.WriteIndent()
		if d.ActivePattern {
			p.Write("let (|" + b.Name + "|_|)")
		} else if b.RecvType != nil {
			p.Write("let " + priv + rec + mut + "(" + b.RecvName + " : " + formatType(b.RecvType) + ")." + b.Name)
		} else {
			p.Write("let " + priv + rec + mut + b.Name)
		}

		for _, param := range b.Params {
			if len(b.Params) == 1 && param.Type != nil {
				p.Write(" (" + param.Name + " : " + formatType(param.Type) + ")")
			} else {
				p.Write(" " + param.Name)
				if param.Type != nil {
					p.Write(" : " + formatType(param.Type))
				}
			}
		}
		if len(b.Params) > 1 {
			// multi-param functions use curried form without extra parens in Goop
		}
		if b.RetType != nil {
			p.Write(" : " + formatType(b.RetType))
		}
		if b.RetEffects != nil {
			p.Write(formatEffects(b.RetEffects))
		}
		p.Write(" =")
		p.Newline()
		p.Indent()
		formatExprBlock(p, b.Body)
		p.Dedent()
	}
}

func formatTypeDecl(p *Printer, d *ast.TypeDecl) {
	p.WriteIndent()
	priv := ""
	if d.Private {
		priv = "private "
	}
	p.Write(priv + "type " + d.Name)
	for _, tp := range d.TypeParams {
		p.Write(" " + tp)
	}
	if d.Quantity == 1 {
		p.Write(" : 1")
	}

	switch k := d.Kind.(type) {
	case *ast.OpaqueTypeKind:
		p.Newline()
	case *ast.RecordTypeKind:
		p.Write(" = {")
		p.Newline()
		for i, f := range k.Fields {
			p.WriteIndent()
			mut := ""
			if f.Mutable {
				mut = "mutable "
			}
			p.Write("  " + mut + f.Name + " : " + formatType(f.Type))
			if i < len(k.Fields)-1 {
				p.Write(";")
			}
			p.Newline()
		}
		p.WriteIndent()
		p.Write("}")
		p.Newline()
	case *ast.ADTTypeKind:
		p.Write(" =")
		p.Newline()
		for _, c := range k.Cases {
			p.WriteIndent()
			p.Write("| " + c.Name)
			if c.Arg != nil {
				p.Write(" of " + formatType(c.Arg))
			}
			p.Newline()
		}
	case *ast.GADTTypeKind:
		p.Write(" =")
		p.Newline()
		for _, c := range k.Cases {
			p.WriteIndent()
			p.Write("| " + c.Name + " : ")
			if c.Arg != nil {
				p.Write(formatType(c.Arg) + " -> ")
			}
			p.Write(formatType(c.Result))
			p.Newline()
		}
	case *ast.AliasTypeKind:
		p.Write(" = " + formatType(k.Alias))
		p.Newline()
	case *ast.NewtypeTypeKind:
		p.Write(" = newtype " + formatType(k.Rep))
		p.Newline()
	}
}

func formatExternDecl(p *Printer, d *ast.ExternDecl) {
	p.WriteIndent()
	p.Write(fmt.Sprintf("extern %q %q {", d.Lang, d.Path))
	p.Newline()
	for _, v := range d.Vals {
		p.WriteIndent()
		p.Write("  val " + v.Name + " : " + formatType(v.Type))
		p.Newline()
	}
	for _, block := range d.GoBlocks {
		p.WriteIndent()
		p.Write("  go {")
		p.Newline()
		p.Write(block)
		if len(block) > 0 && block[len(block)-1] != '\n' {
			p.Newline()
		}
		p.WriteIndent()
		p.Write("  }")
		p.Newline()
	}
	p.WriteIndent()
	p.Write("}")
	p.Newline()
}

func formatLangEmbedDecl(p *Printer, d *ast.LangEmbedDecl) {
	p.WriteIndent()
	p.Write("@[" + d.Lang + "] {")
	p.Newline()
	p.Write(d.Body)
	if len(d.Body) > 0 && d.Body[len(d.Body)-1] != '\n' {
		p.Newline()
	}
	p.WriteIndent()
	p.Write("}")
	p.Newline()
	for _, v := range d.Vals {
		p.WriteIndent()
		p.Write("val " + v.Name + " : " + formatType(v.Type))
		p.Newline()
	}
}

func formatImportSpec(p *Printer, spec ast.ImportSpec) {
	p.WriteIndent()
	if spec.Alias != "" && spec.Alias != "." {
		p.Write(spec.Alias + " ")
	}
	switch spec.Kind {
	case ast.ImportGo:
		p.Write("go ")
	case ast.ImportGoop:
		p.Write("goop ")
		if spec.Alias == "." {
			p.Write(". ")
		}
	}
	p.Write(fmt.Sprintf("%q", spec.Path))
	if len(spec.Vals) > 0 {
		p.Write(" {")
		p.Newline()
		for _, v := range spec.Vals {
			p.WriteIndent()
			p.Write("  val " + v.Name + " : " + formatType(v.Type))
			p.Newline()
		}
		p.WriteIndent()
		p.Write("}")
	}
	p.Newline()
}
