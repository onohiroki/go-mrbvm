package mrbvm_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/conformance"
	"github.com/onohiroki/go-mrbc/mrbc"
	localtoolchain "github.com/onohiroki/go-mrbvm/internal/toolchain"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// TestConformance runs every case in the corpus shared with go-mrbc.
func TestConformance(t *testing.T) {
	cases, err := conformance.Cases()
	if err != nil {
		t.Fatalf("list conformance cases: %v", err)
	}

	ctx := context.Background()
	for _, name := range cases {
		name := name
		t.Run(name+"/go-mrbc", func(t *testing.T) {
			source, err := fs.ReadFile(conformance.FS, name+"/main.rb")
			if err != nil {
				t.Fatalf("read main.rb: %v", err)
			}
			want, err := fs.ReadFile(conformance.FS, name+"/stdout.txt")
			if err != nil {
				t.Fatalf("read stdout.txt: %v", err)
			}
			dir := t.TempDir()
			mainPath := filepath.Join(dir, "main.rb")
			if err := os.WriteFile(mainPath, source, 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := mrbc.CompileFile(ctx, mainPath, mrbc.Options{FilePath: name + "/main.rb"})
			if err != nil {
				t.Fatalf("compile with go-mrbc: %v", err)
			}
			var out bytes.Buffer
			state := mrbvm.New(mrbvm.Options{Stdout: &out})
			if _, err := state.Run(ctx, data); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := out.String(); got != string(want) {
				t.Fatalf("stdout mismatch:\n got %q\nwant %q", got, string(want))
			}
		})

		t.Run(name+"/mrbc", func(t *testing.T) {
			mrbcPath, err := localtoolchain.Find("mrbc")
			if err != nil {
				t.Skip("mrbc not found in PATH")
			}
			if err := localtoolchain.RequireVersion(mrbcPath, []string{"--version"}, "4.0"); err != nil {
				t.Skipf("mrbc 4.0.0 is required: %v", err)
			}
			source, err := fs.ReadFile(conformance.FS, name+"/main.rb")
			if err != nil {
				t.Fatalf("read main.rb: %v", err)
			}
			want, err := fs.ReadFile(conformance.FS, name+"/stdout.txt")
			if err != nil {
				t.Fatalf("read stdout.txt: %v", err)
			}
			dir := t.TempDir()
			mainPath := filepath.Join(dir, "main.rb")
			if err := os.WriteFile(mainPath, source, 0o644); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "case.mrb")
			result, err := localtoolchain.Run(mrbcPath, "-o", binary, mainPath)
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
			if _, err := state.Run(ctx, data); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := out.String(); got != string(want) {
				t.Fatalf("stdout mismatch:\n got %q\nwant %q", got, string(want))
			}
			if !strings.Contains(string(data), "RITE") {
				t.Fatal("reference output is not RITE")
			}
		})
	}
}
