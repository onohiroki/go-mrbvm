package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	localtoolchain "github.com/onohiroki/go-mrbvm/internal/toolchain"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestHelloFromGoMrbc(t *testing.T) {
	ctx := context.Background()
	irep, err := mrbc.Compile(ctx, []byte("puts \"hello\"\n"), mrbc.Options{FilePath: "hello.rb"})
	if err != nil {
		t.Fatalf("compile with go-mrbc: %v", err)
	}

	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Execute(ctx, irep); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "hello\n" {
		t.Fatalf("stdout = %q, want %q", got, "hello\n")
	}
}

func TestHelloFromMrbc(t *testing.T) {
	mrbcPath, err := localtoolchain.Find("mrbc")
	if err != nil {
		t.Skip("mrbc not found in PATH")
	}
	if err := localtoolchain.RequireVersion(mrbcPath, []string{"--version"}, "4.0"); err != nil {
		t.Skipf("mrbc 4.0.0 is required: %v", err)
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "hello.rb")
	binary := filepath.Join(dir, "hello.mrb")
	if err := os.WriteFile(source, []byte("puts \"hello\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := localtoolchain.Run(mrbcPath, "-o", binary, source)
	if err != nil {
		t.Fatalf("run mrbc: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("mrbc failed: %s", result.Stderr)
	}

	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "hello\n" {
		t.Fatalf("stdout = %q, want %q", got, "hello\n")
	}
	if !strings.Contains(string(data), "RITE") {
		t.Fatal("reference output is not a RITE binary")
	}
}
