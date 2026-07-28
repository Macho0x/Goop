// Package checkpipeline runs post-typecheck safety analyses in a fixed order.
package checkpipeline

import (
	"goop.dev/compiler/internal/ast"
	"goop.dev/compiler/internal/channelrace"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/deadlock"
	"goop.dev/compiler/internal/discardedresult"
	"goop.dev/compiler/internal/exhaustive"
	"goop.dev/compiler/internal/linear"
	"goop.dev/compiler/internal/moneyfloat"
	"goop.dev/compiler/internal/nilchan"
	"goop.dev/compiler/internal/refine"
	"goop.dev/compiler/internal/typeinfo"
	"goop.dev/compiler/internal/unused"
	"goop.dev/compiler/internal/visibilitywarns"
)

// Result holds outcomes from all safety passes.
type Result struct {
	LinearErrors      []error
	LinearWarnings    []error
	ChannelRaceErrors []error
	ChannelRaceWarns  []error
	DeadlockErrors    []error
	DeadlockWarns     []error
	ResultErrors      []error
	ResultWarns       []error
	UnusedErrors      []error
	UnusedWarns       []error
	VisErrors         []error
	VisWarns          []error
	MoneyErrors       []error
	MoneyWarns        []error
	NilchanErrors     []error
	RefineProven      refine.ProvenSites
	RefineFuncProven  map[string]bool
	RefineWarnings    []error
	RefineErrors      []error
	ExhaustErrors     []error
	ExhaustWarns      []error
}

// Run executes linear, channelrace, deadlock, nil-channel, refinement, and exhaustiveness checks.
func Run(mod *ast.Module, tm typeinfo.TypeMap, linearTypes map[string]bool, cfg *config.Config) Result {
	var r Result
	r.LinearErrors, r.LinearWarnings = linear.CheckWithConfig(mod, linearTypes, cfg)
	r.ChannelRaceErrors, r.ChannelRaceWarns = channelrace.CheckWithConfig(mod, cfg)
	r.DeadlockErrors, r.DeadlockWarns = deadlock.CheckWithConfig(mod, cfg)
	r.ResultErrors, r.ResultWarns = discardedresult.CheckWithConfig(mod, tm, cfg)
	r.UnusedErrors, r.UnusedWarns = unused.CheckWithConfig(mod, tm, cfg)
	r.VisErrors, r.VisWarns = visibilitywarns.CheckWithConfig(mod, cfg)
	r.MoneyErrors, r.MoneyWarns = moneyfloat.CheckWithConfig(mod, cfg)
	r.NilchanErrors = nilchan.Check(mod)
	r.RefineProven, r.RefineFuncProven, r.RefineWarnings, r.RefineErrors = refine.CheckRefinements(mod, tm, cfg)
	exErrs, exWarns := exhaustive.CheckWithConfig(mod, cfg)
	r.ExhaustErrors = exErrs
	r.ExhaustWarns = exWarns
	return r
}

// BuildLinearTypes extracts linear type names from a module.
func BuildLinearTypes(mod *ast.Module) map[string]bool {
	lt := make(map[string]bool)
	for _, d := range mod.Decls {
		td, ok := d.(*ast.TypeDecl)
		if !ok {
			continue
		}
		if td.Quantity == 1 {
			lt[td.Name] = true
		}
	}
	lt["owned_chan"] = true
	return lt
}

// RegisterADTsFromModule populates the exhaustive ADT registry from type decls.
func RegisterADTsFromModule(mod *ast.Module) {
	exhaustive.ADTRegistry = map[string][]string{
		"result": {"Ok", "Error"},
		"option": {"None", "Some"},
	}
	exhaustive.OpenADTRegistry = make(map[string]bool)
	for _, d := range mod.Decls {
		td, ok := d.(*ast.TypeDecl)
		if !ok {
			continue
		}
		var ctors []string
		switch k := td.Kind.(type) {
		case *ast.ADTTypeKind:
			for _, c := range k.Cases {
				ctors = append(ctors, c.Name)
			}
		case *ast.GADTTypeKind:
			for _, c := range k.Cases {
				ctors = append(ctors, c.Name)
			}
		case *ast.ExtensibleTypeKind:
			exhaustive.RegisterADT(td.Name, nil)
			exhaustive.OpenADTRegistry[td.Name] = true
			continue
		default:
			continue
		}
		exhaustive.RegisterADT(td.Name, ctors)
	}
	for _, d := range mod.Decls {
		if ext, ok := d.(*ast.ExtensibleVariantDecl); ok {
			exhaustive.ADTRegistry[ext.TypeName] = append(exhaustive.ADTRegistry[ext.TypeName], ctorNames(ext.Cases)...)
		}
	}
}

func ctorNames(cases []ast.ADTCase) []string {
	names := make([]string, len(cases))
	for i, c := range cases {
		names[i] = c.Name
	}
	return names
}
