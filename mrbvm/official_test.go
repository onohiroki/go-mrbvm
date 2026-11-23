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
	runManifest(t, filepath.Join("..", "testdata", "official.txt"), filepath.Join(root, "t"), string(assertSource))
}

// TestOfficialGems runs the mrbgems tests listed in the gem manifest.
func TestOfficialGems(t *testing.T) {
	root := filepath.Join("..", "mruby", "test")
	assertSource, err := os.ReadFile(filepath.Join(root, "assert.rb"))
	if err != nil {
		t.Skip("mruby reference checkout is not available")
	}
	runManifest(t, filepath.Join("..", "testdata", "official-gems.txt"), filepath.Join("..", "mruby", "mrbgems"), string(assertSource))
}

func runManifest(t *testing.T, manifestPath, root, assertSource string) {
	t.Helper()
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("manifest is not available")
		}
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
	prelude := testPrelude(assertSource)
	ctx := context.Background()
	for _, name := range files {
		name := name
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			preludeBytecode, err := compileTestSource(ctx, prelude, filepath.Join(root, "assert.rb"))
			if err != nil {
				t.Fatalf("compile assert.rb: %v", err)
			}
			bytecode, err := compileTestSource(ctx, string(source)+"\nreport\n", filepath.Join(root, name))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			var out bytes.Buffer
			state := mrbvm.New(mrbvm.Options{Stdout: &out})
			if _, err := state.Run(ctx, preludeBytecode); err != nil {
				t.Fatalf("run assert.rb: %v\n%s", err, out.String())
			}
			if _, err := state.Run(ctx, bytecode); err != nil {
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

func testPrelude(assertSource string) string {
	var program strings.Builder
	program.WriteString("RUBY_ENGINE = \"mruby\"\n")
	program.WriteString("def t_print(*args); print(*args); end\n")
	program.WriteString("def instance_exec_from_c(*args, &block); instance_exec(*args, &block); end\n")
	program.WriteString("def need_backtracking_stack; true; end\n")
	program.WriteString("module AryShared\n")
	program.WriteString("  def self.heap_empty?(ary); ary.empty?; end\n")
	program.WriteString("  def self.copy_ptr_null?(ary); false; end\n")
	program.WriteString("end\n")
	program.WriteString("module Mrbtest\n")
	program.WriteString("  FLOAT_TOLERANCE = 1e-10\n")
	program.WriteString("  def self.nofree_cstr?; true; end\n")
	program.WriteString("end\n")
	program.WriteString("class TestNotImplement\n")
	program.WriteString("  __define_notimplement__ :gone\n")
	program.WriteString("  NAMELESS_RAISED = true\n")
	program.WriteString("  NAMELESS_RESULT = NotImplementedError.new(\"function is unimplemented on this machine\")\n")
	program.WriteString("end\n")
	program.WriteString(assertSource)
	program.WriteString("\n")
	return program.String()
}

func compileTestSource(ctx context.Context, source, fileName string) ([]byte, error) {
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
