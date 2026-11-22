package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func compileSource(t *testing.T, name, source string) []byte {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/" + name
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := mrbc.CompileFile(context.Background(), path, mrbc.Options{DebugInfo: true})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return data
}

func TestExceptionBacktraceAndCause(t *testing.T) {
	ctx := context.Background()
	source := `
def inner
  raise "boom"
end

begin
  inner
rescue => e
  p e.message
  p e.backtrace.empty?
  p e.backtrace.first.include?("exception_case.rb")
  p $!.equal?(e)
end

begin
  begin
    raise "a"
  rescue
    raise "b"
  end
rescue => e
  p e.cause.message
end
`
	data := compileSource(t, "exception_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(ctx, data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "\"boom\"\nfalse\ntrue\ntrue\n\"a\"\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestUncaughtExceptionIncludesBacktrace(t *testing.T) {
	source := `
def explode
  raise "oops"
end
explode
`
	data := compileSource(t, "uncaught_case.rb", source)
	state := mrbvm.New(mrbvm.Options{})
	_, err := state.Run(context.Background(), data)
	if err == nil {
		t.Fatal("expected an error")
	}
	text := err.Error()
	if !strings.Contains(text, "oops") {
		t.Fatalf("error does not include message: %s", text)
	}
	if !strings.Contains(text, "uncaught_case.rb:") {
		t.Fatalf("error does not include file:line backtrace: %s", text)
	}
}

func TestBacktraceLineNumbers(t *testing.T) {
	ctx := context.Background()
	source := `def inner
  raise "boom"
end

def outer
  inner
end

begin
  outer
rescue => e
  puts e.backtrace[0].split(":")[-1]
  puts e.backtrace[1].split(":")[-1]
  puts e.backtrace[2].split(":")[-1]
end

def divide
  value = 10 / 0
  value
end

begin
  divide
rescue => e
  puts e.backtrace[0].split(":")[-1]
end
`
	data := compileSource(t, "backtrace_lines.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(ctx, data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "2\n6\n10\n18\n"
	if got := out.String(); got != want {
		t.Fatalf("line numbers mismatch:\n got %q\nwant %q", got, want)
	}
}
