package gosiggen

import (
	"fmt"
	"os"
	"strings"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/parser"
)

// LoadImportBindings resolves a .gosig for importPath and parses its type/val
// declarations into ExternType / ExternVal slices suitable for typecheck.
//
// Resolution: project goop-sigs/ override → $GOOP_HOME cache.
// If generateOnMiss is true and the path is curated (or alwaysGenerate),
// GenerateAndWrite fills the cache on miss.
func LoadImportBindings(projectRoot, goopHome, importPath string, generateOnMiss bool) (types []ast.ExternType, vals []ast.ExternVal, fromOverride bool, err error) {
	path, fromOv, exists := ResolveSigPath(projectRoot, goopHome, importPath)
	if !exists {
		if generateOnMiss && (IsCurated(importPath) || alwaysGenerate(importPath)) {
			_, _, genErr := GenerateAndWrite(importPath, Options{LoadDir: projectRoot}, goopHome, "")
			if genErr != nil {
				return nil, nil, false, fmt.Errorf("auto-generate .gosig for %q: %w", importPath, genErr)
			}
			path, fromOv, exists = ResolveSigPath(projectRoot, goopHome, importPath)
		}
	}
	if !exists {
		return nil, nil, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fromOv, err
	}
	types, vals, err = ParseSigContent(importPath, string(data))
	return types, vals, fromOv, err
}

func alwaysGenerate(importPath string) bool {
	// Keep generate-on-miss limited to curated packages for v1 of this train.
	return false
}

// ParseSigContent parses a .gosig file body into ExternType / ExternVal lists
// by wrapping it in a synthetic `import go "…" { … }` module.
func ParseSigContent(importPath, content string) ([]ast.ExternType, []ast.ExternVal, error) {
	// Strip (* … *) comments so the import-block parser stays simple.
	stripped := stripGosigComments(content)
	src := fmt.Sprintf("module __gosig\nimport go %q {\n%s\n}\n", importPath, stripped)
	mod, err := parser.Parse("__gosig.gosig", []byte(src))
	if err != nil {
		return nil, nil, fmt.Errorf("parse .gosig for %q: %w", importPath, err)
	}
	if len(mod.Imports) == 0 {
		return nil, nil, nil
	}
	spec := mod.Imports[0]
	return rewriteObjTypes(spec.Types), rewriteObjVals(spec.Vals), nil
}

func stripGosigComments(s string) string {
	var b strings.Builder
	for {
		start := strings.Index(s, "(*")
		if start < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:start])
		end := strings.Index(s[start+2:], "*)")
		if end < 0 {
			break
		}
		s = s[start+2+end+2:]
	}
	return b.String()
}

func rewriteObjTypes(types []ast.ExternType) []ast.ExternType {
	return types
}

func rewriteObjVals(vals []ast.ExternVal) []ast.ExternVal {
	out := make([]ast.ExternVal, len(vals))
	for i, v := range vals {
		v.Type = rewriteObjInType(v.Type)
		v.RecvType = rewriteObjInType(v.RecvType)
		out[i] = v
	}
	return out
}

// rewriteObjInType rewrites TIdent "obj" → "any" (generator emits obj).
func rewriteObjInType(t ast.Type) ast.Type {
	if t == nil {
		return nil
	}
	switch t := t.(type) {
	case *ast.TIdent:
		if t.Name == "obj" {
			return &ast.TIdent{Name: "any"}
		}
		return t
	case *ast.TFun:
		return &ast.TFun{From: rewriteObjInType(t.From), To: rewriteObjInType(t.To), Effects: t.Effects}
	case *ast.TApp:
		return &ast.TApp{Func: rewriteObjInType(t.Func), Arg: rewriteObjInType(t.Arg)}
	case *ast.TTuple:
		elems := make([]ast.Type, len(t.Elems))
		for i, e := range t.Elems {
			elems[i] = rewriteObjInType(e)
		}
		return &ast.TTuple{Elems: elems}
	case *ast.TPtr:
		return &ast.TPtr{Elem: rewriteObjInType(t.Elem)}
	case *ast.TGoSlice:
		return &ast.TGoSlice{Elem: rewriteObjInType(t.Elem)}
	case *ast.TVariadic:
		return &ast.TVariadic{Elem: rewriteObjInType(t.Elem)}
	case *ast.TChan:
		return &ast.TChan{Elem: rewriteObjInType(t.Elem)}
	case *ast.TMap:
		return &ast.TMap{Key: rewriteObjInType(t.Key), Val: rewriteObjInType(t.Val)}
	case *ast.TRecord:
		fields := make([]ast.FieldType, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = ast.FieldType{Name: f.Name, Type: rewriteObjInType(f.Type)}
		}
		return &ast.TRecord{Fields: fields, Open: t.Open}
	default:
		return t
	}
}
