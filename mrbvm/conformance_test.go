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

// TestConformance runs the tested subset of the shared behaviour corpus.
// The list grows as the VM implements more of mruby.
func TestConformance(t *testing.T) {
	cases := []string{
		"advanced_classes",
		"alias_undef",
		"assignment_targets",
		"basic_hello",
		"blocks",
		"case_and_logic",
		"classes",
		"collections",
		"control_flow",
		"destructuring",
		"ensure_control",
		"exceptions",
		"for_blocks",
		"for_loops",
		"keywords",
		"lambda",
		"lambda_super",
		"methods",
		"modern_syntax",
		"multiple_assignment",
		"operator_assignment",
		"parameters",
		"pattern_matching",
		"patterns_extended",
		"retry_redo",
		"runtime_edges",
		"scalars",
		"semantic_edges",
		"splat",
	}

	root := filepath.Join("..", "testdata", "conformance")
	ctx := context.Background()
	for _, name := range cases {
		dir := filepath.Join(root, name)
		mainPath := filepath.Join(dir, "main.rb")
		want, err := os.ReadFile(filepath.Join(dir, "stdout.txt"))
		if err != nil {
			t.Fatalf("%s: read stdout.txt: %v", name, err)
		}

		t.Run(name+"/go-mrbc", func(t *testing.T) {
			data, err := mrbc.CompileFile(ctx, mainPath, mrbc.Options{})
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
			binary := filepath.Join(t.TempDir(), "case.mrb")
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
