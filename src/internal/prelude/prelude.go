// Package prelude defines the built-in bindings available to all Goop programs
// without explicit `open` statements.
//
// Each binding has:
//   - A user‑visible name (e.g. "print_line")
//   - A Goop type (e.g. "string -> unit")
//   - A Go lowering that describes how to emit the corresponding Go code
//
// Prelude bindings are bound into the type‑checker environment before any
// user declarations, but they are shadowable — a user can define their own
// `print_line` and the prelude version is hidden.
package prelude

import (
	"goop.dev/compiler/internal/types"
)

// Binding describes one prelude entry.
type Binding struct {
	Name     string        // user‑visible name, e.g. "print_line"
	Scheme   *types.Scheme // type scheme
	Lowering Lowering      // how to emit Go code for a call
	Effects  *[]string     // nil = unknown; non-nil lists effect tags for the outermost function
}

// Lowering describes how a call to a prelude function is lowered to Go.
type Lowering struct {
	// Func is the Go function name (e.g. "fmt.Println").  If empty, the
	// lowering uses a custom Emit function.
	Func string
	// Pkg is the Go import path needed (e.g. "fmt", "strconv").
	Pkg string
	// Operator, if set, means the call is lowered to a binary operator.
	// For example, string_concat uses "+".
	Operator string
	// Wrap controls how arguments are passed:
	//   ""     — standard call: Func(arg1, arg2, ...)
	//   "fmt.Sprintf" — use Sprintf format string (first arg is format)
	Wrap string
	// Custom, if set, uses a built-in lowering template:
	//   "assert"       — if !arg { panic("assertion failed") }
	//   "assert_equal" — if arg1 != arg2 { panic("assert_equal failed") }
	Custom string
}

// Prelude collects all built-in bindings.
type Prelude struct {
	Bindings []Binding
	// ImportMap maps Go import paths to aliases needed by the prelude.
	ImportMap map[string]string // Go import path → alias (e.g. "fmt" → "fmt")
}

// Default returns the standard prelude.
func Default() *Prelude {
	// Fresh type variables for polymorphic bindings
	a := types.Fresh("'a")
	b := types.Fresh("'b")

	p := &Prelude{
		ImportMap: make(map[string]string),
	}

	ioEff := []string{"io"}
	asyncEff := []string{"async"}
	panicEff := []string{"panic"}
	pureEff := []string{}

	// print_line : string -> unit
	p.addWithEffects("print_line",
		types.Mono(&types.TFun{From: types.String, To: types.Unit}),
		Lowering{Func: "fmt.Println", Pkg: "fmt"},
		&ioEff,
	)

	// print : string -> unit
	p.addWithEffects("print",
		types.Mono(&types.TFun{From: types.String, To: types.Unit}),
		Lowering{Func: "fmt.Print", Pkg: "fmt"},
		&ioEff,
	)

	// int_to_string : int -> string
	p.addWithEffects("int_to_string",
		types.Mono(&types.TFun{From: types.Int, To: types.String}),
		Lowering{Func: "strconv.Itoa", Pkg: "strconv"},
		&pureEff,
	)

	// float_to_string : float -> string
	p.addWithEffects("float_to_string",
		types.Mono(&types.TFun{From: types.Float, To: types.String}),
		Lowering{Func: "fmt.Sprintf", Pkg: "fmt", Wrap: "fmt.Sprintf"},
		&pureEff,
	)

	// string_concat : string -> string -> string
	p.addWithEffects("string_concat",
		types.Mono(&types.TFun{
			From: types.String,
			To:   &types.TFun{From: types.String, To: types.String},
		}),
		Lowering{Operator: "+"},
		&pureEff,
	)

	// list_length : 'a list -> int
	listA := types.ListType(a)
	p.addWithEffects("list_length",
		types.Mono(&types.TFun{From: listA, To: types.Int}),
		Lowering{Func: "len", Pkg: ""},
		&pureEff,
	)

	// list_append : 'a list -> 'a list -> 'a list
	p.addWithEffects("list_append",
		types.Mono(&types.TFun{
			From: listA,
			To:   &types.TFun{From: listA, To: listA},
		}),
		Lowering{Func: "append", Pkg: ""},
		&pureEff,
	)

	// Go-slice FFI helpers. Goop lists already lower to Go slices, so the two
	// conversion helpers are identity functions at runtime.
	goSliceA := &types.TGoSlice{Elem: a}
	p.addWithEffects("any_of",
		types.Generalize(&types.TFun{From: a, To: &types.Prim{Name: "any"}}, nil),
		Lowering{Func: "any_of"},
		&pureEff,
	)
	p.addWithEffects("go_slice_len",
		types.Mono(&types.TFun{From: goSliceA, To: types.Int}),
		Lowering{Func: "go_slice_len"},
		&pureEff,
	)
	p.addWithEffects("go_slice_append",
		types.Mono(&types.TFun{From: goSliceA, To: &types.TFun{From: a, To: goSliceA}}),
		Lowering{Func: "go_slice_append"},
		&pureEff,
	)
	p.addWithEffects("go_slice_get",
		types.Mono(&types.TFun{From: goSliceA, To: &types.TFun{From: types.Int, To: a}}),
		Lowering{Func: "go_slice_get"},
		&pureEff,
	)
	p.addWithEffects("go_slice_of_list",
		types.Mono(&types.TFun{From: listA, To: goSliceA}),
		Lowering{Func: "go_slice_of_list"},
		&pureEff,
	)
	p.addWithEffects("list_of_go_slice",
		types.Mono(&types.TFun{From: goSliceA, To: listA}),
		Lowering{Func: "list_of_go_slice"},
		&pureEff,
	)

	arrayA := types.ArrayType(a)
	p.addWithEffects("Array.make",
		types.Mono(&types.TFun{
			From: types.Int,
			To:   &types.TFun{From: a, To: arrayA},
		}),
		Lowering{Custom: "array_make"},
		&pureEff,
	)
	p.addWithEffects("Array.length",
		types.Mono(&types.TFun{From: arrayA, To: types.Int}),
		Lowering{Func: "len", Pkg: ""},
		&pureEff,
	)

	// String.length / String.sub — Go byte semantics (len / s[i:i+n])
	p.addWithEffects("String.length",
		types.Mono(&types.TFun{From: types.String, To: types.Int}),
		Lowering{Func: "len", Pkg: ""},
		&pureEff,
	)
	p.addWithEffects("String.sub",
		types.Mono(&types.TFun{
			From: types.String,
			To: &types.TFun{
				From: types.Int,
				To:   &types.TFun{From: types.Int, To: types.String},
			},
		}),
		Lowering{Custom: "string_sub"},
		&pureEff,
	)

	// failwith : string -> 'a
	p.addWithEffects("failwith",
		types.Mono(&types.TFun{From: types.String, To: b}),
		Lowering{Func: "panic", Pkg: ""},
		&panicEff,
	)

	// Lazy.force : 'a lazy -> 'a
	lazyA := types.LazyType(a)
	p.addWithEffects("Lazy.force",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: lazyA, To: a},
		},
		Lowering{Custom: "lazy_force"},
		&pureEff,
	)
	p.addWithEffects("Lazy.from_val",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: a, To: lazyA},
		},
		Lowering{Custom: "lazy_from_val"},
		&pureEff,
	)

	// ref : 'a -> 'a ref
	p.addWithEffects("ref",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: a, To: types.RefType(a)},
		},
		Lowering{Custom: "ref_make"},
		&pureEff,
	)

	p.addWithEffects("Console.print_line",
		types.Mono(&types.TFun{From: types.String, To: types.Unit}),
		Lowering{Func: "fmt.Println", Pkg: "fmt"},
		&ioEff,
	)

	p.addWithEffects("assert",
		types.Mono(&types.TFun{From: types.Bool, To: types.Unit}),
		Lowering{Custom: "assert"},
		&panicEff,
	)

	p.addWithEffects("assert_equal",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{
				From: a,
				To:   &types.TFun{From: a, To: types.Unit},
			},
		},
		Lowering{Custom: "assert_equal"},
		&panicEff,
	)

	p.addWithEffects("Chan.make",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: types.Unit, To: &types.TChan{Elem: a}},
		},
		Lowering{Custom: "chan_make"},
		&pureEff,
	)
	p.addWithEffects("Chan.send",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{
				From: &types.TChan{Elem: a},
				To:   &types.TFun{From: a, To: types.Unit},
			},
		},
		Lowering{Custom: "chan_send"},
		&asyncEff,
	)
	p.addWithEffects("Chan.recv",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: &types.TChan{Elem: a}, To: a},
		},
		Lowering{Custom: "chan_recv"},
		&asyncEff,
	)
	p.addWithEffects("Chan.close",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: &types.TChan{Elem: a}, To: types.Unit},
		},
		Lowering{Custom: "chan_close"},
		&asyncEff,
	)

	p.addWithEffects("OwnedChan.make",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{From: types.Unit, To: &types.TAdt{Name: "owned_chan", Params: []types.Type{a}}},
		},
		Lowering{Custom: "owned_chan_make"},
		&pureEff,
	)

	p.addWithEffects("OwnedChan.send",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{
				From: &types.TAdt{Name: "owned_chan", Params: []types.Type{a}},
				To:   &types.TFun{From: a, To: types.Unit},
			},
		},
		Lowering{Custom: "owned_chan_send"},
		&asyncEff,
	)

	p.addWithEffects("OwnedChan.recv",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{
				From: &types.TAdt{Name: "owned_chan", Params: []types.Type{a}},
				To:   a,
			},
		},
		Lowering{Custom: "owned_chan_recv"},
		&asyncEff,
	)

	p.addWithEffects("OwnedChan.close",
		&types.Scheme{
			Vars: []*types.TVar{a},
			Type: &types.TFun{
				From: &types.TAdt{Name: "owned_chan", Params: []types.Type{a}},
				To:   types.Unit,
			},
		},
		Lowering{Custom: "owned_chan_close"},
		&asyncEff,
	)

	// Map.make : unit -> map['a] 'b
	p.addWithEffects("Map.make",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{From: types.Unit, To: &types.TMap{Key: a, Val: b}},
		},
		Lowering{Custom: "map_make"},
		&pureEff,
	)
	// Map.get : map['a] 'b -> 'a -> 'b option
	p.addWithEffects("Map.get",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{
				From: &types.TMap{Key: a, Val: b},
				To:   &types.TFun{From: a, To: types.OptionType(b)},
			},
		},
		Lowering{Custom: "map_get"},
		&pureEff,
	)
	// Map.add : map['a] 'b -> 'a -> 'b -> unit  (mutates)
	p.addWithEffects("Map.add",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{
				From: &types.TMap{Key: a, Val: b},
				To: &types.TFun{
					From: a,
					To:   &types.TFun{From: b, To: types.Unit},
				},
			},
		},
		Lowering{Custom: "map_add"},
		&pureEff,
	)
	// Map.remove : map['a] 'b -> 'a -> unit
	p.addWithEffects("Map.remove",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{
				From: &types.TMap{Key: a, Val: b},
				To:   &types.TFun{From: a, To: types.Unit},
			},
		},
		Lowering{Custom: "map_remove"},
		&pureEff,
	)
	// Map.mem : map['a] 'b -> 'a -> bool
	p.addWithEffects("Map.mem",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{
				From: &types.TMap{Key: a, Val: b},
				To:   &types.TFun{From: a, To: types.Bool},
			},
		},
		Lowering{Custom: "map_mem"},
		&pureEff,
	)
	// Map.size : map['a] 'b -> int
	p.addWithEffects("Map.size",
		&types.Scheme{
			Vars: []*types.TVar{a, b},
			Type: &types.TFun{From: &types.TMap{Key: a, Val: b}, To: types.Int},
		},
		Lowering{Custom: "map_size"},
		&pureEff,
	)

	p.addWithEffects("http_get_string",
		types.Mono(&types.TFun{From: types.String, To: types.String}),
		Lowering{Custom: "http_get_string"},
		&ioEff,
	)

	p.addWithEffects("json_extract_floats",
		types.Mono(&types.TFun{
			From: types.String,
			To:   &types.TFun{From: types.Int, To: types.ListType(types.Float)},
		}),
		Lowering{Custom: "json_extract_floats"},
		&pureEff,
	)

	p.addWithEffects("json_extract_strings",
		types.Mono(&types.TFun{
			From: types.String,
			To:   &types.TFun{From: types.Int, To: types.ListType(types.String)},
		}),
		Lowering{Custom: "json_extract_strings"},
		&pureEff,
	)

	_ = a
	_ = b
	return p
}

func (p *Prelude) add(name string, scheme *types.Scheme, lower Lowering) {
	p.addWithEffects(name, scheme, lower, nil)
}

func (p *Prelude) addWithEffects(name string, scheme *types.Scheme, lower Lowering, effects *[]string) {
	p.Bindings = append(p.Bindings, Binding{
		Name:     name,
		Scheme:   scheme,
		Lowering: lower,
		Effects:  effects,
	})
	if lower.Pkg != "" {
		p.ImportMap[lower.Pkg] = lower.Pkg
	}
}

// Lookup finds a prelude binding by name, or returns nil.
func (p *Prelude) Lookup(name string) *Binding {
	for i := range p.Bindings {
		if p.Bindings[i].Name == name {
			return &p.Bindings[i]
		}
	}
	return nil
}
