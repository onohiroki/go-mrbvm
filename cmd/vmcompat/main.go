// Command vmcompat compares go-mrbvm against the reference mruby runtime.
//
// For each Ruby file it runs the reference mruby interpreter, compiles the
// file with go-mrbc and the official mrbc, and compares stdout and exit
// status from both bytecode streams with the reference result.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onohiroki/go-mrbc/mrbc"
	localtoolchain "github.com/onohiroki/go-mrbvm/internal/toolchain"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

func main() {
	verbose := flag.Bool("v", false, "print every comparison")
	strict := flag.Bool("strict", false, "fail when mrbc or mruby is missing")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vmcompat [-v] [-strict] file.rb...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	mrubyPath, mrubyErr := findMRuby()
	mrbcPath, mrbcErr := findMRBC()
	if *strict {
		if mrubyErr != nil {
			fmt.Fprintln(os.Stderr, mrubyErr)
			os.Exit(2)
		}
		if mrbcErr != nil {
			fmt.Fprintln(os.Stderr, mrbcErr)
			os.Exit(2)
		}
	}
	if mrubyErr != nil {
		fmt.Fprintf(os.Stderr, "vmcompat: reference mruby not found: %v\n", mrubyErr)
	}
	if mrbcErr != nil {
		fmt.Fprintf(os.Stderr, "vmcompat: official mrbc not found: %v\n", mrbcErr)
	}

	failures := 0
	for _, path := range flag.Args() {
		if err := compareFile(ctx, path, mrubyPath, mrbcPath, *verbose); err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", path, err)
		} else if *verbose {
			fmt.Printf("ok   %s\n", path)
		}
	}
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "%d file(s) failed\n", failures)
		os.Exit(1)
	}
}

func findMRuby() (string, error) {
	if path, err := localtoolchain.Find("mruby"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("mruby not found on PATH")
}

func findMRBC() (string, error) {
	if path, err := localtoolchain.Find("mrbc"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("mrbc not found on PATH")
}

func compareFile(ctx context.Context, path, mrubyPath, mrbcPath string, verbose bool) error {
	var want result
	if mrubyPath != "" {
		run, err := localtoolchain.Run(mrubyPath, path)
		if err != nil {
			return err
		}
		want = result{stdout: run.Stdout, stderr: run.Stderr, exitCode: run.ExitCode}
	}

	goData, err := mrbc.CompileFile(ctx, path, mrbc.Options{})
	if err != nil {
		return fmt.Errorf("go-mrbc compile: %w", err)
	}
	goResult := runVM(ctx, goData)
	if mrubyPath != "" {
		if err := sameResult("go-mrbc", goResult, want); err != nil {
			return err
		}
	}
	if verbose {
		fmt.Printf("     go-mrbc: %d bytes, exit %d\n", len(goData), goResult.exitCode)
	}

	if mrbcPath == "" {
		return nil
	}
	temp, err := os.CreateTemp("", "vmcompat-*.mrb")
	if err != nil {
		return err
	}
	binary := temp.Name()
	temp.Close()
	defer os.Remove(binary)

	compile, err := localtoolchain.Run(mrbcPath, "-o", binary, path)
	if err != nil {
		return err
	}
	if compile.ExitCode != 0 {
		return fmt.Errorf("mrbc failed: %s", compile.Stderr)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	mrbcResult := runVM(ctx, data)
	if mrubyPath != "" {
		if err := sameResult("mrbc", mrbcResult, want); err != nil {
			return err
		}
	}
	if verbose {
		fmt.Printf("     mrbc:    %d bytes, exit %d\n", len(data), mrbcResult.exitCode)
	}
	return nil
}

func runVM(ctx context.Context, data []byte) result {
	var stdout, stderr bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &stdout, Stderr: &stderr})
	_, err := state.Run(ctx, data)
	exitCode := 0
	if err != nil {
		fmt.Fprintln(&stderr, err)
		exitCode = 1
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCode}
}

func sameResult(label string, got, want result) error {
	if got.stdout != want.stdout {
		return fmt.Errorf("%s stdout mismatch\n--- got ---\n%s--- want ---\n%s", label, got.stdout, want.stdout)
	}
	if got.exitCode != want.exitCode {
		return fmt.Errorf("%s exit status %d, want %d (stderr: %s)", label, got.exitCode, want.exitCode, got.stderr)
	}
	return nil
}

func init() {
	// Make usage output show the binary name consistently.
	if len(os.Args) > 0 {
		os.Args[0] = filepath.Base(os.Args[0])
	}
}
