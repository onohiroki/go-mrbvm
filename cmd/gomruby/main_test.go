package main

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

func runGomruby(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"gomruby"}, args...), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func compileTestBytecode(t *testing.T, source string) []byte {
	t.Helper()
	irep, err := mrbc.Compile(context.Background(), []byte(source), mrbc.Options{FilePath: "test.rb"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := mrbc.Dump(irep)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRunSourceFile(t *testing.T) {
	path := writeTestFile(t, "script.rb", "p [1 + 1, $0, $DEBUG]\n")
	code, stdout, stderr := runGomruby(t, "", path)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	want := "[2, \"" + path + "\", false]\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunSourceWithoutRbExtension(t *testing.T) {
	path := writeTestFile(t, "script", "p 42\n")
	code, stdout, stderr := runGomruby(t, "", path)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "42\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "42\n")
	}
}

func TestRunCommandLineOptions(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "", "-e", "p [$0, $DEBUG, ARGV]", "a", "b")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "[\"-e\", false, [\"a\", \"b\"]]\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	code, stdout, stderr = runGomruby(t, "", "-d", "-e", "p $DEBUG")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "true\n" {
		t.Fatalf("stdout = %q, want true", stdout)
	}

	code, stdout, stderr = runGomruby(t, "", "-e", "x = 1", "-e", "p x")
	if code != 0 {
		t.Fatalf("multiple -e exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "1\n" {
		t.Fatalf("multiple -e stdout = %q, want 1", stdout)
	}
}

func TestRunStdin(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "p [$0, ARGV]\n")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "[\"-\", []]\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	code, stdout, stderr = runGomruby(t, "p 5\n", "-")
	if code != 0 {
		t.Fatalf("-: exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "5\n" {
		t.Fatalf("-: stdout = %q, want 5", stdout)
	}
}

func TestRunBytecode(t *testing.T) {
	data := compileTestBytecode(t, "p 7\n")
	path := filepath.Join(t.TempDir(), "program.mrb")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runGomruby(t, "", path)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "7\n" {
		t.Fatalf("stdout = %q, want 7", stdout)
	}

	forced := filepath.Join(t.TempDir(), "program.bin")
	if err := os.WriteFile(forced, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runGomruby(t, "", "-b", forced)
	if code != 0 {
		t.Fatalf("-b exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "7\n" {
		t.Fatalf("-b stdout = %q, want 7", stdout)
	}
}

func TestCheckSyntax(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "", "-c", "-e", "p 1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "Syntax OK\n" {
		t.Fatalf("stdout = %q, want Syntax OK", stdout)
	}

	code, stdout, stderr = runGomruby(t, "", "-c", "-e", "p 1; 1.")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "end-of-input") {
		t.Fatalf("stderr = %q, want a syntax diagnostic", stderr)
	}

	path := filepath.Join(t.TempDir(), "program.mrb")
	if err := os.WriteFile(path, compileTestBytecode(t, "p 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runGomruby(t, "", "-c", path)
	if code != 0 {
		t.Fatalf("bytecode exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "Syntax OK\n" {
		t.Fatalf("bytecode stdout = %q, want Syntax OK", stdout)
	}
}

func TestPreloadLibraries(t *testing.T) {
	sourceLib := writeTestFile(t, "lib.rb", "def libfn; 40; end\n")
	code, stdout, stderr := runGomruby(t, "", "-r", sourceLib, "-e", "p libfn + 2")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "42\n" {
		t.Fatalf("stdout = %q, want 42", stdout)
	}

	bytecodeLib := filepath.Join(t.TempDir(), "lib.mrb")
	if err := os.WriteFile(bytecodeLib, compileTestBytecode(t, "def libfn2; 41; end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runGomruby(t, "", "-r", bytecodeLib, "-e", "p libfn2 + 1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "42\n" {
		t.Fatalf("stdout = %q, want 42", stdout)
	}
}

func TestLibraryErrors(t *testing.T) {
	code, _, stderr := runGomruby(t, "", "-r", filepath.Join(t.TempDir(), "missing.rb"), "-e", "p 1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot open library file") {
		t.Fatalf("stderr = %q", stderr)
	}

	code, _, stderr = runGomruby(t, "", "-r", t.TempDir(), "-e", "p 1")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot read library file") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestVersionOptions(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	code, stdout, stderr = runGomruby(t, "", "-v")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "gomruby "+mrbvm.Version) {
		t.Fatalf("stdout = %q, want version", stdout)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout = %q, want one line", stdout)
	}

	code, stdout, stderr = runGomruby(t, "", "-v", "-e", "p 1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "gomruby "+mrbvm.Version) || !strings.HasSuffix(stdout, "1\n") {
		t.Fatalf("stdout = %q, want version and program output", stdout)
	}

	code, stdout, stderr = runGomruby(t, "", "--version")
	if code != 0 || !strings.HasPrefix(stdout, "gomruby "+mrbvm.Version) {
		t.Fatalf("--version: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runGomruby(t, "", "--copyright")
	if code != 0 || !strings.Contains(stdout, "Copyright") {
		t.Fatalf("--copyright: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestVerbose(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "", "--verbose", "-e", "p 1")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "irep[0]") {
		t.Fatalf("stdout = %q, want disassembly", stdout)
	}
	if !strings.HasSuffix(stdout, "1\n") {
		t.Fatalf("stdout = %q, want program output", stdout)
	}
}

func TestProgramArguments(t *testing.T) {
	path := writeTestFile(t, "argv.rb", "p ARGV\n")
	code, stdout, stderr := runGomruby(t, "", path, "a", "b")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "[\"a\", \"b\"]\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestDoubleDash(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "", "-e", "p ARGV", "--", "-x")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "[\"-x\"]\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "short", args: []string{"-z"}, want: "invalid option -z"},
		{name: "long", args: []string{"--unknown"}, want: "invalid option --unknown"},
		{name: "missing code", args: []string{"-e"}, want: "No code specified for -e"},
		{name: "missing library", args: []string{"-r"}, want: "No library specified for -r"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runGomruby(t, "", test.args...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1", code)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, test.want) {
				t.Fatalf("stderr = %q, want %q", stderr, test.want)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	code, stdout, stderr := runGomruby(t, "", "-h")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: gomruby") || !strings.Contains(stdout, "-e 'command'") {
		t.Fatalf("stdout = %q, want usage", stdout)
	}
}

func TestMissingProgramFile(t *testing.T) {
	code, _, stderr := runGomruby(t, "", filepath.Join(t.TempDir(), "missing.rb"))
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot open program file") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDirectoryProgramFile(t *testing.T) {
	code, _, stderr := runGomruby(t, "", t.TempDir())
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot read program file") {
		t.Fatalf("stderr = %q", stderr)
	}
}
