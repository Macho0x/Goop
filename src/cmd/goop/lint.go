package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goop.dev/compiler/internal/checkpipeline"
	"goop.dev/compiler/internal/config"
	"goop.dev/compiler/internal/desugar"
	"goop.dev/compiler/internal/parser"
	"goop.dev/compiler/internal/report"
	"goop.dev/compiler/internal/typecheck"
)

// runLint type-checks and runs safety diagnostics on a .goop file or all
// .goop files under a directory. Prints each diagnostic, then a summary.
// Exit code is non-zero when any errors are reported. Diagnostics whose
// goop.toml [check] severity is "error" count as errors (including checks
// that default to warn when elevated); remaining warnings do not fail the run.
func runLint(path string) int {
	files, err := collectGoopFiles(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no .goop files found in %s\n", path)
		return 1
	}

	var totalErrs, totalWarns int
	for _, file := range files {
		nErr, nWarn := lintFile(file)
		totalErrs += nErr
		totalWarns += nWarn
	}

	fmt.Printf("%d error(s), %d warning(s)\n", totalErrs, totalWarns)
	if totalErrs > 0 {
		return 1
	}
	return 0
}

func collectGoopFiles(path string) ([]string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		if !strings.HasSuffix(path, ".goop") {
			return nil, fmt.Errorf("%s: not a .goop file", path)
		}
		return []string{path}, nil
	}

	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".goop") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func lintFile(file string) (nErr, nWarn int) {
	cfg := loadProjectConfig(file)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", file, err)
		return 1, 0
	}

	mod, err := parser.Parse(file, src)
	if err != nil {
		fmt.Print(report.Render(err, src))
		return 1, 0
	}
	mod = desugar.DesugarModule(mod)

	lock := loadProjectLock(file)
	tm, _, typeErrors := typecheck.CheckWithTypesForFile(mod, file, cfg, lock)
	for _, e := range typeErrors {
		fmt.Print(report.Render(e, src))
	}
	nErr += len(typeErrors)

	checkpipeline.RegisterADTsFromModule(mod)
	linearTypes := checkpipeline.BuildLinearTypes(mod)
	r := checkpipeline.Run(mod, tm, linearTypes, cfg)

	emitErrs := func(errs []error) {
		for _, e := range errs {
			fmt.Print(report.Render(e, src))
		}
		nErr += len(errs)
	}
	emitWarns := func(warns []error) {
		for _, w := range warns {
			fmt.Print(report.Render(w, src))
		}
		nWarn += len(warns)
	}

	emitErrs(r.LinearErrors)
	emitWarns(r.LinearWarnings)
	emitErrs(r.ChannelRaceErrors)
	emitWarns(r.ChannelRaceWarns)
	emitErrs(r.DeadlockErrors)
	emitWarns(r.DeadlockWarns)
	emitErrs(r.NilchanErrors)
	emitErrs(r.RefineErrors)
	emitWarns(r.RefineWarnings)
	emitErrs(r.ExhaustErrors)
	emitWarns(r.ExhaustWarns)

	return nErr, nWarn
}
