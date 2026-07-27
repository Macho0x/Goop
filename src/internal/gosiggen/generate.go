package gosiggen

import (
	"bytes"
	"context"
	"fmt"
	"go/token"
	gotypes "go/types"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

// Options controls package loading and emission.
type Options struct {
	// LoadDir is passed to packages.Load (module root). Empty = process cwd.
	LoadDir string
	// Timeout for packages.Load. Zero = 30s default.
	Timeout time.Duration
}

// Skip records an exported object that could not be emitted.
type Skip struct {
	Name   string
	Kind   string // "func", "method", "type", "const", "var"
	Reason string
}

// Result is a generated .gosig for one Go package.
type Result struct {
	ImportPath string
	PkgName    string
	Content    string
	Skipped    []Skip
	Warnings   []string
	TODOH6     []string // names with (T, error) product form (coerced at check/codegen)
}

// Generate loads importPath via go/packages and emits a .gosig stub.
func Generate(importPath string, opts Options) (*Result, error) {
	pkg, err := loadPackage(importPath, opts)
	if err != nil {
		return nil, err
	}
	return emit(pkg)
}

func loadPackage(importPath string, opts Options) (*packages.Package, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedModule,
		Context: ctx,
		Dir:     opts.LoadDir,
		Fset:    token.NewFileSet(),
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("loading package %q: %w", importPath, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for import path %q", importPath)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package %q has errors: %v", importPath, pkg.Errors[0])
	}
	if pkg.Types == nil || pkg.Types.Scope() == nil {
		return nil, fmt.Errorf("package %q has no type information", importPath)
	}
	return pkg, nil
}

func emit(pkg *packages.Package) (*Result, error) {
	importPath := pkg.Types.Path()
	if importPath == "" {
		importPath = pkg.PkgPath
	}
	res := &Result{
		ImportPath: importPath,
		PkgName:    pkg.Types.Name(),
	}

	scope := pkg.Types.Scope()
	names := scope.Names()
	sort.Strings(names)

	var typeDecls []string
	var valDecls []string
	seenTypes := map[string]bool{}

	for _, name := range names {
		obj := scope.Lookup(name)
		if obj == nil || !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *gotypes.TypeName:
			typeDecls = append(typeDecls, "type "+name)
			seenTypes[name] = true
			// Methods on this named type (value + pointer receivers).
			if named, ok := o.Type().(*gotypes.Named); ok {
				emitMethods(named, importPath, name, &valDecls, res)
			}

		case *gotypes.Func:
			if o.Type() == nil {
				continue
			}
			sig, ok := o.Type().(*gotypes.Signature)
			if !ok || sig.Recv() != nil {
				continue // methods collected via named types
			}
			mr := MapType(sig, importPath)
			if !mr.OK {
				res.Skipped = append(res.Skipped, Skip{Name: name, Kind: "func", Reason: mr.Reason})
				continue
			}
			line := fmt.Sprintf("val %s : %s", name, mr.Goop)
			if mr.TODOH6 {
				line += "  (* (T, error): typecheck coerces to result *)"
				res.TODOH6 = append(res.TODOH6, name)
				if mr.Reason != "" {
					res.Warnings = append(res.Warnings, name+": "+mr.Reason)
				}
			}
			valDecls = append(valDecls, line)

		case *gotypes.Const:
			mr := MapType(o.Type(), importPath)
			if !mr.OK {
				res.Skipped = append(res.Skipped, Skip{Name: name, Kind: "const", Reason: mr.Reason})
				continue
			}
			valDecls = append(valDecls, fmt.Sprintf("val %s : %s", name, mr.Goop))

		case *gotypes.Var:
			mr := MapType(o.Type(), importPath)
			if !mr.OK {
				res.Skipped = append(res.Skipped, Skip{Name: name, Kind: "var", Reason: mr.Reason})
				continue
			}
			valDecls = append(valDecls, fmt.Sprintf("val %s : %s", name, mr.Goop))
		}
	}

	res.Content = formatGosig(res, typeDecls, valDecls)
	return res, nil
}

func emitMethods(named *gotypes.Named, importPath, typeName string, valDecls *[]string, res *Result) {
	// Collect both value and pointer method sets; dedupe by name preferring
	// the declaration's actual receiver.
	seen := map[string]bool{}
	add := func(mset *gotypes.MethodSet) {
		for i := 0; i < mset.Len(); i++ {
			sel := mset.At(i)
			fn, ok := sel.Obj().(*gotypes.Func)
			if !ok || !fn.Exported() {
				continue
			}
			name := fn.Name()
			if seen[name] {
				continue
			}
			seen[name] = true

			sig, ok := fn.Type().(*gotypes.Signature)
			if !ok {
				continue
			}
			recv := sig.Recv()
			recvMR := ReceiverTypeString(recv, importPath)
			if !recvMR.OK {
				res.Skipped = append(res.Skipped, Skip{
					Name: typeName + "." + name, Kind: "method", Reason: "recv: " + recvMR.Reason,
				})
				continue
			}
			// Method type after stripping receiver.
			withoutRecv := gotypes.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
			bodyMR := MapType(withoutRecv, importPath)
			if !bodyMR.OK {
				res.Skipped = append(res.Skipped, Skip{
					Name: typeName + "." + name, Kind: "method", Reason: bodyMR.Reason,
				})
				continue
			}
			recvName := "x"
			if recv != nil && recv.Name() != "" && recv.Name() != "_" {
				recvName = strings.ToLower(recv.Name()[:1])
			} else if typeName != "" {
				recvName = strings.ToLower(typeName[:1])
			}
			line := fmt.Sprintf("val (%s : %s).%s : %s", recvName, recvMR.Goop, name, bodyMR.Goop)
			if bodyMR.TODOH6 {
				line += "  (* (T, error): typecheck coerces to result *)"
				res.TODOH6 = append(res.TODOH6, typeName+"."+name)
			}
			*valDecls = append(*valDecls, line)
		}
	}
	add(gotypes.NewMethodSet(named))
	add(gotypes.NewMethodSet(gotypes.NewPointer(named)))
}

func formatGosig(res *Result, typeDecls, valDecls []string) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "(* Generated by goop gen-sig — DO NOT EDIT.\n")
	fmt.Fprintf(&b, " * Package: %s (%s)\n", res.PkgName, res.ImportPath)
	fmt.Fprintf(&b, " *\n")
	fmt.Fprintf(&b, " * Mapping notes (H5 foundation):\n")
	fmt.Fprintf(&b, " *   - any / interface{} → obj\n")
	fmt.Fprintf(&b, " *   - []T → T go_slice (best-effort); []byte → bytes\n")
	fmt.Fprintf(&b, " *   - chan T → T chan\n")
	fmt.Fprintf(&b, " *   - maps skipped (no Goop map FFI yet)\n")
	fmt.Fprintf(&b, " *   - (T, error) emitted as `T * error`; typecheck+codegen\n")
	fmt.Fprintf(&b, " *     coerce call sites to `('ok, error) result` (H6).\n")
	fmt.Fprintf(&b, " *     Use `import go raw \"…\"` to keep the raw tuple.\n")
	fmt.Fprintf(&b, " *)\n\n")

	if len(typeDecls) > 0 {
		b.WriteString("(* Types *)\n")
		for _, d := range typeDecls {
			b.WriteString(d)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	if len(valDecls) > 0 {
		b.WriteString("(* Vals (funcs, methods, consts, vars) *)\n")
		for _, d := range valDecls {
			b.WriteString(d)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	if len(res.Skipped) > 0 {
		b.WriteString("(* Skipped exports:\n")
		for _, s := range res.Skipped {
			fmt.Fprintf(&b, " *   [%s] %s — %s\n", s.Kind, s.Name, s.Reason)
		}
		b.WriteString(" *)\n")
	}

	return b.String()
}

// GenerateAndWrite generates a sig and writes it under the cache (or outDir).
// If outDir is non-empty it is used instead of the default cache path.
func GenerateAndWrite(importPath string, opts Options, goopHome, outDir string) (path string, res *Result, err error) {
	res, err = Generate(importPath, opts)
	if err != nil {
		return "", nil, err
	}
	if outDir != "" {
		path = filepath.Join(outDir, SigFileName(importPath))
	} else {
		path = CachePath(goopHome, importPath)
	}
	if err := WriteSig(path, res.Content); err != nil {
		return path, res, err
	}
	return path, res, nil
}
