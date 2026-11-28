package bundle_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/bundle"
	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func compile(t *testing.T, filePath, source string) []byte {
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

func archive(t *testing.T, entry string, loadPaths []string, entries []bundle.Entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := bundle.Write(&buf, entry, loadPaths, entries); err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func TestArchiveRoundTrip(t *testing.T) {
	data := archive(t, "main.mrb", []string{"lib"}, []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", "p 1\n")},
		{Path: "lib/foo.mrb", Data: compile(t, "lib/foo.rb", "def foo; 1; end\n")},
	})
	b, err := bundle.Open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := b.Entry(); got != "main.mrb" {
		t.Fatalf("entry = %q", got)
	}
	manifest := b.Manifest()
	if manifest.Format != bundle.FormatVersion {
		t.Fatalf("format = %d", manifest.Format)
	}
	if len(manifest.LoadPaths) != 1 || manifest.LoadPaths[0] != "lib" {
		t.Fatalf("load paths = %#v", manifest.LoadPaths)
	}
	if len(manifest.Files) != 2 || manifest.Files[0].Path != "lib/foo.mrb" {
		t.Fatalf("files = %#v", manifest.Files)
	}
	if manifest.Files[0].Kind != "mrb" {
		t.Fatalf("kind = %q", manifest.Files[0].Kind)
	}
	if got := strings.Join(b.Files(), ","); got != "lib/foo.mrb,main.mrb" {
		t.Fatalf("files = %q", got)
	}
	read, err := b.ReadFile("lib/foo.mrb")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.HasPrefix(read, []byte("RITE")) {
		t.Fatalf("member is not bytecode")
	}
	if _, err := b.ReadFile("../escape"); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	fsys := b.SourceFS()
	info, err := fsys.Stat("main.mrb")
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatalf("unexpected file info: %#v", info)
	}
}

func TestArchiveIsDeterministic(t *testing.T) {
	entries := []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", "p 1\n")},
		{Path: "lib/foo.mrb", Data: compile(t, "lib/foo.rb", "def foo; 1; end\n")},
	}
	first := archive(t, "main.mrb", []string{"lib"}, entries)
	reverse := []bundle.Entry{entries[1], entries[0]}
	second := archive(t, "main.mrb", []string{"lib"}, reverse)
	if !bytes.Equal(first, second) {
		t.Fatal("archives differ for the same inputs")
	}
}

func TestWriteRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		entry   string
		entries []bundle.Entry
	}{
		{"empty", "main.mrb", nil},
		{"traversal", "main.mrb", []bundle.Entry{{Path: "../main.mrb"}}},
		{"absolute", "main.mrb", []bundle.Entry{{Path: "/main.mrb"}}},
		{"missing entry", "main.mrb", []bundle.Entry{{Path: "other.mrb"}}},
		{"duplicate", "main.mrb", []bundle.Entry{{Path: "main.mrb"}, {Path: "main.mrb"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := bundle.Write(&buf, tc.entry, nil, tc.entries); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestOpenRejectsNonArchive(t *testing.T) {
	if _, err := bundle.Open([]byte("not a bundle")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunBundle(t *testing.T) {
	data := archive(t, "main.mrb", []string{"lib"}, []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", `
require "helper"
require_relative "lib/helper"
p [helper_value, ARGV, $0]
`)},
		{Path: "lib/helper.mrb", Data: compile(t, "lib/helper.rb", "def helper_value; \"ok\"; end\n")},
	})
	var stdout, stderr bytes.Buffer
	code := bundle.Run(context.Background(), data, []string{"app", "a", "b"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	want := "[\"ok\", [\"a\", \"b\"], \"main.mrb\"]\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSetupRegistersHostFunctions(t *testing.T) {
	data := archive(t, "main.mrb", nil, []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", `p [host_echo(ARGV[0]), $0]`)},
	})
	var stdout, stderr bytes.Buffer
	code := bundle.Run(context.Background(), data, []string{"app", "hello"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{
		Setup: func(state *mrbvm.State) error {
			state.DefineFunction("host_echo", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
				return mrbvm.StringValue("host:" + args[0].String()), nil
			})
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if want := "[\"host:hello\", \"main.mrb\"]\n"; stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunSetupError(t *testing.T) {
	data := archive(t, "main.mrb", nil, []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", `puts "unreachable"`)},
	})
	var stdout, stderr bytes.Buffer
	code := bundle.Run(context.Background(), data, []string{"app"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{
		Setup: func(*mrbvm.State) error { return errors.New("boom") },
	})
	if code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "bundle: setup: boom") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunDefaultPolicyDeniesHostFiles(t *testing.T) {
	data := archive(t, "main.mrb", nil, []bundle.Entry{
		{Path: "main.mrb", Data: compile(t, "main.rb", `File.read("/etc/passwd")`)},
	})
	var stdout, stderr bytes.Buffer
	code := bundle.Run(context.Background(), data, []string{"app"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{})
	if code != 1 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "capability denied: file-read") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if bundle.DefaultPolicy().Allowed(sandbox.FileRead) {
		t.Fatal("default policy allows file reads")
	}
}

func TestRunSourceEntryNeedsCompiler(t *testing.T) {
	data := archive(t, "main.rb", nil, []bundle.Entry{
		{Path: "main.rb", Data: []byte(`p "source"`)},
	})
	var stdout, stderr bytes.Buffer
	code := bundle.Run(context.Background(), data, []string{"app"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{})
	if code != 1 || !strings.Contains(stderr.String(), "no compiler") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = bundle.Run(context.Background(), data, []string{"app"}, strings.NewReader(""), &stdout, &stderr, bundle.Options{
		Compiler: func(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
			return mrbc.Compile(ctx, source, mrbc.Options{FilePath: opts.FilePath})
		},
	})
	if code != 0 || stdout.String() != "\"source\"\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}
