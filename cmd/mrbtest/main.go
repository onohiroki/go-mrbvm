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
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mrbtest [-v] [-stress] test.rb...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
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
	for _, path := range flag.Args() {
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

func runFile(ctx context.Context, path, assertSource string, verbose, stress bool) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var program strings.Builder
	program.WriteString("RUBY_ENGINE = \"mruby\"\n")
	program.WriteString("def t_print(*args); print(*args); end\n")
	if verbose {
		program.WriteString("$mrbtest_verbose = true\n")
	}
	if stress {
		program.WriteString("$mrbtest_stress = true\n")
	}
	program.WriteString(assertSource)
	program.WriteString("\n")
	program.Write(source)
	program.WriteString("\nreport\n")

	temp, err := os.CreateTemp("", "mrbtest-*.rb")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.WriteString(program.String()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	bytecode, err := mrbc.CompileFile(ctx, tempPath, mrbc.Options{})
	if err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	var output bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &output, Stderr: &output})
	_, runErr := state.Run(ctx, bytecode)
	fmt.Print(output.String())
	if runErr != nil {
		return runErr
	}
	text := output.String()
	if !strings.Contains(text, "  Total:") {
		return fmt.Errorf("test did not reach report")
	}
	if !strings.Contains(text, "     KO: 0") || !strings.Contains(text, "  Crash: 0") {
		return fmt.Errorf("assertion failures")
	}
	return nil
}
