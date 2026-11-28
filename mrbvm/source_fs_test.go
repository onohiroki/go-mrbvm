package mrbvm_test

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func compileBundled(t *testing.T, filePath, source string) []byte {
	t.Helper()
	irep, err := mrbc.Compile(context.Background(), []byte(source), mrbc.Options{FilePath: filePath, DebugInfo: true})
	if err != nil {
		t.Fatalf("compile %s: %v", filePath, err)
	}
	data, err := mrbc.DumpWithOptions(irep, bytecode.DumpOptions{DebugInfo: true})
	if err != nil {
		t.Fatalf("dump %s: %v", filePath, err)
	}
	return data
}

func TestSourceFSRequire(t *testing.T) {
	fsys := fstest.MapFS{
		"lib/foo.rb": {
			Data: []byte("$foo_loads = ($foo_loads || 0) + 1\ndef foo_value; \"foo\"; end\n"),
		},
		"lib/bar.mrb": {
			Data: compileBundled(t, "lib/bar.rb", "def bar_value; \"bar\"; end\n"),
		},
		"lib/nested/c.mrb": {
			Data: compileBundled(t, "lib/nested/c.rb", `
require_relative "d"
def c_value
  "c+" + d_value
end
`),
		},
		"lib/nested/d.rb": {
			Data: []byte("def d_value; \"d\"; end\n"),
		},
		"extra.rb": {
			Data: []byte("$extra = \"loaded\"\n"),
		},
	}
	source := `
require "foo"
require "lib/bar"
require "lib/bar"
require "lib/nested/c"
load "extra.rb"
p [foo_value, bar_value, c_value, $foo_loads, $extra]
begin
  require "/etc/passwd"
rescue LoadError
  puts "absolute denied"
end
begin
  require "../../outside"
rescue LoadError
  puts "escape denied"
end
`
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{
		Stdout:   &out,
		Compiler: compileEvalForTest,
		Policy:   sandbox.AllowAll(),
		SourceFS: mrbvm.NewSourceFS(fsys),
		LoadPaths: []string{
			"lib",
		},
	})
	irep, err := compileEvalForTest(context.Background(), []byte(source), mrbvm.CompileOptions{FilePath: "main.rb"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Execute(context.Background(), irep); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	want := "[\"foo\", \"bar\", \"c+d\", 1, \"loaded\"]\nabsolute denied\nescape denied\n"
	if out.String() != want {
		t.Fatalf("output:\n got %q\nwant %q", out.String(), want)
	}
}

func TestSourceFSRequiresCompilerForSource(t *testing.T) {
	fsys := fstest.MapFS{
		"lib/foo.rb": {Data: []byte("def foo; 1; end\n")},
	}
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{
		Stdout:   &out,
		Policy:   sandbox.AllowAll(),
		SourceFS: mrbvm.NewSourceFS(fsys),
	})
	irep, err := compileEvalForTest(context.Background(), []byte("require \"lib/foo\"\n"), mrbvm.CompileOptions{FilePath: "main.rb"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Execute(context.Background(), irep); err == nil {
		t.Fatal("expected an error loading source without a compiler")
	}
}
