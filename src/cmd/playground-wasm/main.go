//go:build js && wasm

// Command playground-wasm exposes Goop check/compile to the browser via WASM.
//
// Build (from src/):
//
//	GOOS=js GOARCH=wasm go build -o ../playground/goop.wasm ./cmd/playground-wasm
package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"syscall/js"

	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/checkpipeline"
	"goop.dev/compiler/internal/codegen"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/effects"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/token"
	"goop.dev/compiler/internal/typecheck"
	"goop.dev/compiler/internal/typeinfo"
)

const virtualFile = "playground.goop"

type diagnostic struct {
	Severity string `json:"severity"` // "error" | "warning"
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

type result struct {
	OK          bool         `json:"ok"`
	Diagnostics []diagnostic `json:"diagnostics"`
	Go          string       `json:"go,omitempty"`
	Error       string       `json:"error,omitempty"`
}

func main() {
	js.Global().Set("goopCheck", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return mustJSON(result{OK: false, Error: "check(src) requires a source string"})
		}
		return mustJSON(runCheck(args[0].String()))
	}))
	js.Global().Set("goopCompile", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return mustJSON(result{OK: false, Error: "compile(src) requires a source string"})
		}
		return mustJSON(runCompile(args[0].String()))
	}))
	// Keep the Go runtime alive for JS callbacks.
	select {}
}

func mustJSON(r result) string {
	if r.Diagnostics == nil {
		r.Diagnostics = []diagnostic{}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"diagnostics":[],"error":"json marshal failed"}`
	}
	return string(b)
}

func runCheck(src string) result {
	pipe, diags, ok := analyze(src)
	if !ok {
		return result{OK: false, Diagnostics: diags}
	}
	safetyDiags, fatal := safetyDiagnostics(pipe.safety)
	diags = append(diags, safetyDiags...)
	if fatal {
		return result{OK: false, Diagnostics: diags}
	}
	return result{OK: true, Diagnostics: diags}
}

func runCompile(src string) result {
	pipe, diags, ok := analyze(src)
	if !ok {
		return result{OK: false, Diagnostics: diags}
	}
	safetyDiags, fatal := safetyDiagnostics(pipe.safety)
	diags = append(diags, safetyDiags...)
	if fatal {
		return result{OK: false, Diagnostics: diags}
	}

	cfg := config.DefaultConfig()
	mod := effects.TransformCPS(pipe.mod)
	gen := codegen.NewGenerator(virtualFile, cfg)
	gen.SetTypeMap(pipe.tm, pipe.vtm)
	gen.SetProvenSites(pipe.safety.RefineProven)
	gen.SetRefinementMeta(pipe.safety.RefineFuncProven)
	goSrc, err := gen.Generate(mod)
	if err != nil {
		diags = append(diags, diagFromErr(err, "error"))
		return result{OK: false, Diagnostics: diags}
	}
	return result{OK: true, Diagnostics: diags, Go: goSrc}
}

type pipeline struct {
	mod    *ast.Module
	tm     typeinfo.TypeMap
	vtm    typeinfo.VarTypeMap
	safety checkpipeline.Result
}

// analyze parses, desugars, type-checks, and runs safety passes on in-memory source.
func analyze(src string) (pipeline, []diagnostic, bool) {
	mod, err := parser.Parse(virtualFile, []byte(src))
	if err != nil {
		return pipeline{}, []diagnostic{diagFromErr(err, "error")}, false
	}
	mod = desugar.DesugarModule(mod)
	tm, vtm, typeErrs := typecheck.CheckWithTypes(mod)
	var diags []diagnostic
	for _, e := range typeErrs {
		diags = append(diags, diagFromErr(e, "error"))
	}
	if len(typeErrs) > 0 {
		return pipeline{mod: mod, tm: tm, vtm: vtm}, diags, false
	}

	cfg := config.DefaultConfig()
	checkpipeline.RegisterADTsFromModule(mod)
	safety := checkpipeline.Run(mod, tm, checkpipeline.BuildLinearTypes(mod), cfg)
	return pipeline{mod: mod, tm: tm, vtm: vtm, safety: safety}, diags, true
}

func safetyDiagnostics(r checkpipeline.Result) ([]diagnostic, bool) {
	var diags []diagnostic
	add := func(errs []error, sev string) {
		for _, e := range errs {
			diags = append(diags, diagFromErr(e, sev))
		}
	}
	add(r.LinearErrors, "error")
	add(r.ChannelRaceErrors, "error")
	add(r.DeadlockErrors, "error")
	add(r.ResultErrors, "error")
	add(r.UnusedErrors, "error")
	add(r.VisErrors, "error")
	add(r.NilchanErrors, "error")
	add(r.RefineErrors, "error")
	add(r.ExhaustErrors, "error")
	add(r.LinearWarnings, "warning")
	add(r.ChannelRaceWarns, "warning")
	add(r.DeadlockWarns, "warning")
	add(r.ResultWarns, "warning")
	add(r.UnusedWarns, "warning")
	add(r.VisWarns, "warning")
	add(r.RefineWarnings, "warning")
	add(r.ExhaustWarns, "warning")

	fatal := len(r.LinearErrors) > 0 ||
		len(r.ChannelRaceErrors) > 0 ||
		len(r.DeadlockErrors) > 0 ||
		len(r.ResultErrors) > 0 ||
		len(r.UnusedErrors) > 0 ||
		len(r.VisErrors) > 0 ||
		len(r.NilchanErrors) > 0 ||
		len(r.RefineErrors) > 0 ||
		len(r.ExhaustErrors) > 0
	return diags, fatal
}

func diagFromErr(err error, severity string) diagnostic {
	d := diagnostic{Severity: severity, Message: err.Error()}
	if te, ok := err.(*typecheck.TypeError); ok {
		d.Message = te.Msg
		d.File = te.Loc.File
		d.Line = te.Loc.Line
		d.Column = te.Loc.Column
		return d
	}
	if pe, ok := err.(*parser.ParseError); ok {
		d.Message = pe.Msg
		d.File = pe.Loc.File
		d.Line = pe.Loc.Line
		d.Column = pe.Loc.Column
		return d
	}
	if locater, ok := err.(interface{ GetLoc() token.SourceLoc }); ok {
		loc := locater.GetLoc()
		d.File = loc.File
		d.Line = loc.Line
		d.Column = loc.Column
		return d
	}
	// "file:line:col: msg"
	parts := strings.SplitN(err.Error(), ":", 4)
	if len(parts) >= 4 {
		d.File = strings.TrimSpace(parts[0])
		d.Line, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		d.Column, _ = strconv.Atoi(strings.TrimSpace(parts[2]))
		d.Message = strings.TrimSpace(parts[3])
	}
	return d
}
