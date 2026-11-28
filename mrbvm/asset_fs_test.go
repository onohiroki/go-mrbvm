package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func runAssetCase(t *testing.T, source string, policy *sandbox.Policy) (string, error) {
	t.Helper()
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{
		Stdout:   &out,
		Compiler: compileEvalForTest,
		Policy:   policy,
		AssetFS: mrbvm.NewSourceFS(fstest.MapFS{
			"data/msg.txt": {Data: []byte("asset")},
			"same.txt":     {Data: []byte("from-asset")},
		}),
	})
	irep, err := compileEvalForTest(context.Background(), []byte(source), mrbvm.CompileOptions{FilePath: "asset_case.rb"})
	if err != nil {
		return "", err
	}
	_, err = state.Execute(context.Background(), irep)
	return out.String(), err
}

func TestAssetFSFileRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "same.txt"), []byte("from-host"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host.txt"), []byte("host-only"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	source := `
p [File.read("data/msg.txt"), File.exist?("data/msg.txt"), File.size("data/msg.txt")]
p [File.read("same.txt"), File.read("host.txt")]
`
	out, err := runAssetCase(t, source, sandbox.New().Permit(sandbox.FileRead))
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	want := "[\"asset\", true, 5]\n[\"from-asset\", \"host-only\"]\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestAssetFSNeedsNoFileCapability(t *testing.T) {
	source := `
p [File.read("data/msg.txt"), File.exist?("data/msg.txt")]
begin
  File.read("host-only.txt")
rescue SecurityError => e
  puts e.class
end
`
	out, err := runAssetCase(t, source, sandbox.New())
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	want := "[\"asset\", true]\nSecurityError\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
