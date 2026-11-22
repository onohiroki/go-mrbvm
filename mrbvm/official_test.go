package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// TestOfficialSubset runs the mruby 4.0.0 core tests that go-mrbvm currently
// passes. The mruby checkout is a development reference and may be absent, in
// which case the test is skipped.
func TestOfficialSubset(t *testing.T) {
	root := filepath.Join("..", "mruby", "test")
	assertSource, err := os.ReadFile(filepath.Join(root, "assert.rb"))
	if err != nil {
		t.Skip("mruby reference checkout is not available")
	}
	manifest, err := os.ReadFile(filepath.Join("..", "testdata", "official.txt"))
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0)
	for _, line := range strings.Split(string(manifest), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}
	ctx := context.Background()
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, "t", name))
			if err != nil {
				t.Fatal(err)
			}
			var program strings.Builder
			program.WriteString("RUBY_ENGINE = \"mruby\"\n")
			program.WriteString("def t_print(*args); print(*args); end\n")
			program.WriteString("module AryShared\n")
			program.WriteString("  def self.heap_empty?(ary); ary.empty?; end\n")
			program.WriteString("  def self.copy_ptr_null?(ary); false; end\n")
			program.WriteString("end\n")
			program.Write(assertSource)
			program.WriteString("\n")
			program.Write(source)
			program.WriteString("\nreport\n")

			temp, err := os.CreateTemp(t.TempDir(), "mrbtest-*.rb")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := temp.WriteString(program.String()); err != nil {
				t.Fatal(err)
			}
			if err := temp.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := mrbc.CompileFile(ctx, temp.Name(), mrbc.Options{DebugInfo: true})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			var out bytes.Buffer
			state := mrbvm.New(mrbvm.Options{Stdout: &out})
			if _, err := state.Run(ctx, data); err != nil {
				t.Fatalf("run: %v\n%s", err, out.String())
			}
			got := out.String()
			if !strings.Contains(got, "  Total:") {
				t.Fatalf("test did not reach report:\n%s", got)
			}
			if !strings.Contains(got, "     KO: 0") || !strings.Contains(got, "  Crash: 0") {
				t.Fatalf("official test failures:\n%s", got)
			}
		})
	}
}
