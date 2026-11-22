package mrbvm_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func TestFileCapabilities(t *testing.T) {
	source := `
begin
  File.read("/etc/hosts")
rescue => e
  p e.class
end
`
	data := compileSource(t, "file_denied.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if got := out.String(); got != "SecurityError\n" {
		t.Fatalf("out = %q, want SecurityError", got)
	}
}

func TestFileReadWrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.txt")
	output := filepath.Join(dir, "output.txt")
	if err := os.WriteFile(input, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
p File.read(%q)
p File.write(%q, "abc")
p File.read(%q)
f = File.open(%q, "a")
f.write("!")
f.close
p f.closed?
p File.read(%q)
File.open(%q, "r") { |file| p file.read(2) }
p File.exist?(%q)
`, input, output, output, output, output, output, output)
	data := compileSource(t, "file_allowed.rb", source)
	var out bytes.Buffer
	policy := sandbox.New().Permit(sandbox.FileRead, sandbox.FileWrite)
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Policy: policy})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "\"abc\"\n3\n\"abc\"\ntrue\n\"abc!\"\n\"ab\"\ntrue\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestIOStreams(t *testing.T) {
	source := `
STDOUT.write("hi")
STDOUT.puts("!")
p STDIN.gets
p STDIN.eof?
`
	data := compileSource(t, "io_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdin: strings.NewReader("line\n"), Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "hi!\n\"line\\n\"\ntrue\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestFilePathHelpersAndSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
p File.basename(%q)
p File.dirname(%q)
p File.extname(%q)
p File.size(%q)
begin
  File.open(%q, "bad")
rescue => e
  p e.class
end
p File.delete(%q)
p File.exist?(%q)
`, path, path, path, path, path, path, path)
	data := compileSource(t, "file_paths.rb", source)
	var out bytes.Buffer
	policy := sandbox.New().Permit(sandbox.FileRead, sandbox.FileWrite)
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Policy: policy})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := fmt.Sprintf("\"sample.txt\"\n%q\n\".txt\"\n5\nArgumentError\n0\nfalse\n", dir)
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
