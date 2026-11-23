// Command mrbtest runs mruby's official test files with assert.rb on go-mrbvm.
//
// Each file is concatenated with mruby/test/assert.rb and compiled with
// go-mrbc. The command prints the assertion log and a summary, then exits
// non-zero if any test crashed or reported a failure.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func main() {
	verbose := flag.Bool("v", false, "show every assertion label")
	stress := flag.Bool("stress", false, "run stress tests")
	assertPath := flag.String("assert", filepath.Join("mruby", "test", "assert.rb"), "path to assert.rb")
	manifest := flag.String("manifest", "", "file listing test names under mruby/test/t")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mrbtest [-v] [-stress] [-manifest file] test.rb...")
		flag.PrintDefaults()
	}
	flag.Parse()
	files := append([]string(nil), flag.Args()...)
	if *manifest != "" {
		listed, err := readManifest(*manifest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mrbtest: %v\n", err)
			os.Exit(2)
		}
		for _, name := range listed {
			if strings.Contains(name, "/") {
				files = append(files, filepath.Join("mruby", "mrbgems", name))
			} else {
				files = append(files, filepath.Join("mruby", "test", "t", name))
			}
		}
	}
	if len(files) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	assertSource, err := os.ReadFile(*assertPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mrbtest: read %s: %v\n", *assertPath, err)
		os.Exit(2)
	}

	ctx := context.Background()
	failures := 0
	for _, path := range files {
		if err := runFile(ctx, path, string(assertSource), *verbose, *stress); err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", path, err)
		}
	}
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "%d file(s) failed\n", failures)
		os.Exit(1)
	}
}

func readManifest(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}
	return files, nil
}

func compileSource(ctx context.Context, source, fileName string) ([]byte, error) {
	temp, err := os.CreateTemp("", "mrbtest-*.rb")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.WriteString(source); err != nil {
		temp.Close()
		return nil, err
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	return mrbc.CompileFile(ctx, tempPath, mrbc.Options{DebugInfo: true, FilePath: fileName})
}

func runFile(ctx context.Context, path, assertSource string, verbose, stress bool) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var prelude strings.Builder
	prelude.WriteString("RUBY_ENGINE = \"mruby\"\n")
	prelude.WriteString("def t_print(*args); print(*args); end\n")
	prelude.WriteString("def instance_exec_from_c(*args, &block); instance_exec(*args, &block); end\n")
	prelude.WriteString("def need_backtracking_stack; true; end\n")
	prelude.WriteString("module AryShared\n")
	prelude.WriteString("  def self.heap_empty?(ary); ary.empty?; end\n")
	prelude.WriteString("  def self.copy_ptr_null?(ary); false; end\n")
	prelude.WriteString("end\n")
	// The official runner links the mruby-test C gem, which provides these
	// test-support constants. Per-file runs need the same names to exist.
	prelude.WriteString("module Mrbtest\n")
	prelude.WriteString("  FLOAT_TOLERANCE = 1e-10\n")
	prelude.WriteString("  def self.nofree_cstr?; true; end\n")
	prelude.WriteString("end\n")
	prelude.WriteString("class TestNotImplement\n")
	prelude.WriteString("  __define_notimplement__ :gone\n")
	prelude.WriteString("  NAMELESS_RAISED = true\n")
	prelude.WriteString("  NAMELESS_RESULT = NotImplementedError.new(\"function is unimplemented on this machine\")\n")
	prelude.WriteString("end\n")
	prelude.WriteString(assertSource)
	prelude.WriteString("\n")

	var program strings.Builder
	if verbose {
		program.WriteString("$mrbtest_verbose = true\n")
	}
	if stress {
		program.WriteString("$mrbtest_stress = true\n")
	}
	program.Write(source)
	program.WriteString("\nreport\n")

	preludeBytecode, err := compileSource(ctx, prelude.String(), path)
	if err != nil {
		return fmt.Errorf("compile assert.rb: %w", err)
	}
	bytecode, err := compileSource(ctx, program.String(), path)
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	var output bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &output, Stderr: &output})
	if _, err := state.Run(ctx, preludeBytecode); err != nil {
		return err
	}
	if _, err := state.Run(ctx, bytecode); err != nil {
		fmt.Print(output.String())
		return err
	}
	fmt.Print(output.String())
	text := output.String()
	if !strings.Contains(text, "  Total:") {
		return fmt.Errorf("test did not reach report")
	}
	if !strings.Contains(text, "     KO: 0") || !strings.Contains(text, "  Crash: 0") {
		return fmt.Errorf("assertion failures")
	}
	return nil
}
