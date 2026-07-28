// Package config reads the project-level goop.toml configuration file and
// resolves Goop module names to Go import paths.
//
// Configuration format (goop.toml):
//
//	module_root = "github.com/example/project"
//
//	[mappings]
//	"Std.IO"   = "goop.dev/std/io"
//	"Std.List" = "goop.dev/std/list"
//
// When no config file exists, built-in defaults are used:
//
//	Std.IO   → goop.dev/std/io   (package io)
//	Std.List → goop.dev/std/list (package list)
//
// Project modules without an explicit mapping are resolved by combining
// ModuleRoot with the lowercased, slash-separated module segments.
//
//	Example: "Trading.OrderBook" with root "github.com/user/p"
//	         → "github.com/user/p/trading/orderbook", package "orderbook"
package config

import (
	"fmt"
	"os"
	"strings"
)

// Severity is warn | error | off for optional checks.
type Severity string

const (
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
	SeverityOff   Severity = "off"
)

// CheckConfig controls optional compile-time safety passes.
type CheckConfig struct {
	ExhaustRedundant   Severity // EXHAUST001/002 (default warn)
	ExhaustMissing     Severity // EXHAUST003 (default error)
	EffectInference    bool     // infer effect rows from function bodies (default true)
	Concurrent         Severity // LINEAR006/007/008 (default error)
	RefinementUnproven Severity // REFINE002 (default warn)
	Deadlock           Severity // DEADLOCK001 (default warn)
	DiscardedResult    Severity // RESULT001 (default warn)
	DiscardedOption    Severity // OPTION001 (default warn)
	Unused             Severity // UNUSED001/002 (default warn)
	PrivateInPublic    Severity // VIS002 (default warn)
	SMT                bool     // use Z3 for refinement VCs when available (default false)
}

// Config holds the project-wide compiler configuration.
type Config struct {
	ModuleRoot   string            // e.g. "github.com/example/project"
	Mappings     map[string]string // Goop logical path → Go import path
	Dependencies map[string]string // canonical path → version pin
	Check        CheckConfig
}

// DefaultConfig returns a working config with built-in mappings.
func DefaultConfig() *Config {
	return &Config{
		ModuleRoot: "",
		Check: CheckConfig{
			ExhaustRedundant:   SeverityWarn,
			ExhaustMissing:     SeverityError,
			EffectInference:    true,
			Concurrent:         SeverityError,
			RefinementUnproven: SeverityWarn,
			Deadlock:           SeverityWarn,
			DiscardedResult:    SeverityWarn,
			DiscardedOption:    SeverityWarn,
			Unused:             SeverityWarn,
			PrivateInPublic:    SeverityWarn,
		},
		Dependencies: make(map[string]string),
		Mappings: map[string]string{
			"std.io":      "github.com/Macho0x/Goop/std/io",
			"std.list":    "github.com/Macho0x/Goop/std/list",
			"std.array":   "github.com/Macho0x/Goop/std/array",
			"std.option":  "github.com/Macho0x/Goop/std/option",
			"std.result":  "github.com/Macho0x/Goop/std/result",
			"std.decimal": "github.com/Macho0x/Goop/std/decimal",
			"std.string":  "github.com/Macho0x/Goop/std/string",
			"std.chan":    "github.com/Macho0x/Goop/std/channel",
			"std.ref":     "github.com/Macho0x/Goop/std/ref",
		},
	}
}

// LoadConfig reads a goop.toml file from the given path.
// If the file does not exist, DefaultConfig is returned.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	return parseConfig(string(data))
}

// ResolveImport resolves a Goop module name (e.g. "Std.IO", "Trading.OrderBook")
// to a Go import path and package name.
func (c *Config) ResolveImport(c0ModuleName string) (goImportPath, goPackageName string) {
	// 1. Check explicit mappings
	if path, ok := c.Mappings[c0ModuleName]; ok {
		return path, packageNameFromPath(path)
	}

	// 2. Built-in defaults
	switch c0ModuleName {
	case "std.io", "Std.IO":
		return "github.com/Macho0x/Goop/std/io", "io"
	case "std.list", "Std.List":
		return "github.com/Macho0x/Goop/std/list", "list"
	case "std.array", "Std.Array":
		return "github.com/Macho0x/Goop/std/array", "array"
	case "std.option", "Std.Option":
		return "github.com/Macho0x/Goop/std/option", "option"
	case "std.result", "Std.Result":
		return "github.com/Macho0x/Goop/std/result", "result"
	case "std.decimal", "Std.Decimal":
		return "github.com/Macho0x/Goop/std/decimal", "decimal"
	case "std.string", "Std.String":
		return "github.com/Macho0x/Goop/std/string", "string"
	case "std.chan", "Std.Chan":
		return "github.com/Macho0x/Goop/std/channel", "channel"
	case "std.ref", "Std.Ref":
		return "github.com/Macho0x/Goop/std/ref", "ref"
	}

	// 3. Project module: combine ModuleRoot with lowercased path segments
	segments := strings.Split(c0ModuleName, ".")
	for i, s := range segments {
		segments[i] = strings.ToLower(s)
	}
	pkg := segments[len(segments)-1]

	if c.ModuleRoot != "" {
		return c.ModuleRoot + "/" + strings.Join(segments, "/"), pkg
	}

	// 4. Fallback: treat the module name as a Go import path
	return strings.ToLower(c0ModuleName), pkg
}

// packageNameFromPath extracts the last segment of a Go import path.
func packageNameFromPath(path string) string {
	segments := strings.Split(path, "/")
	return segments[len(segments)-1]
}

// ---------------------------------------------------------------------------
// Minimal TOML parser (handles only the subset needed for goop.toml)
// ---------------------------------------------------------------------------

func parseConfig(data string) (*Config, error) {
	c := DefaultConfig()
	c.Mappings = make(map[string]string) // start fresh, copy defaults if needed

	lines := strings.Split(data, "\n")
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}

		// Key = value
		if idx := strings.IndexByte(line, '='); idx >= 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			// Strip surrounding quotes from both key and value
			key = strings.Trim(key, `"`)
			key = strings.Trim(key, `'`)
			val = strings.Trim(val, `"`)
			val = strings.Trim(val, `'`)

			switch section {
			case "check":
				switch key {
				case "exhaust_redundant":
					c.Check.ExhaustRedundant = Severity(val)
				case "exhaust_missing":
					c.Check.ExhaustMissing = Severity(val)
				case "effect_inference":
					c.Check.EffectInference = val == "true" || val == "1"
				case "concurrent":
					c.Check.Concurrent = Severity(val)
				case "refinement_unproven":
					c.Check.RefinementUnproven = Severity(val)
				case "deadlock":
					c.Check.Deadlock = Severity(val)
				case "discarded_result":
					c.Check.DiscardedResult = Severity(val)
				case "discarded_option":
					c.Check.DiscardedOption = Severity(val)
				case "unused":
					c.Check.Unused = Severity(val)
				case "private_in_public":
					c.Check.PrivateInPublic = Severity(val)
				case "smt":
					c.Check.SMT = val == "true" || val == "1"
				}
			case "mappings":
				c.Mappings[key] = val
			case "dependencies":
				if c.Dependencies == nil {
					c.Dependencies = make(map[string]string)
				}
				c.Dependencies[key] = val
			default:
				if key == "module_root" {
					c.ModuleRoot = val
				}
			}
		}
	}

	if c.Dependencies == nil {
		c.Dependencies = make(map[string]string)
	}

	// Merge built-in defaults for any mappings not explicitly overridden
	defaults := DefaultConfig()
	for k, v := range defaults.Mappings {
		if _, ok := c.Mappings[k]; !ok {
			c.Mappings[k] = v
		}
	}

	return c, nil
}
