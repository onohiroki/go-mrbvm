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

func runGomirb(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"gomirb"}, args...), strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeGomirbFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func compileGomirbBytecode(t *testing.T, source string) []byte {
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

func TestRunREPLNonInteractive(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	input := strings.NewReader("x = 1\nx + 2\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, replConfig{sourceName: "(mirb)"}); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if got := out.String(); got != "=> 3\n" {
		t.Fatalf("out = %q, want %q", got, "=> 3\n")
	}
}

func TestRunREPLInteractivePersistsState(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	input := strings.NewReader("$x = 4\n$x * 2\nexit\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, replConfig{interactive: true}); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "=> 8\n") {
		t.Fatalf("out = %q, missing result", out.String())
	}
}

func TestIncompleteSource(t *testing.T) {
	if !incompleteSource("unexpected end-of-input") {
		t.Fatal("expected incomplete source")
	}
	if incompleteSource("syntax error") {
		t.Fatal("plain syntax error should not continue")
	}
}

func TestHistoryAndCompletion(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	historyPath := t.TempDir() + "/history"
	input := strings.NewReader("1 + 1\nhistory\ncomplete File\nexit\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, replConfig{
		historyPath: historyPath,
		interactive: true,
	}); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "1: 1 + 1") {
		t.Fatalf("out = %q, missing history", out.String())
	}
	if !strings.Contains(out.String(), "File") {
		t.Fatalf("out = %q, missing completion", out.String())
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "1 + 1") {
		t.Fatalf("history file = %q", data)
	}
}

func TestRunSetsDebugAndArgv(t *testing.T) {
	path := writeGomirbFile(t, "script.rb", "p [$DEBUG, ARGV]\n")
	code, stdout, stderr := runGomirb(t, "", "-d", path, "a", "b")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "[true, [\"a\", \"b\"]]\n=> [true, [\"a\", \"b\"]]\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunVersionContinues(t *testing.T) {
	code, stdout, stderr := runGomirb(t, "1 + 1\n", "-v")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "gomirb "+mrbvm.Version) {
		t.Fatalf("stdout = %q, want version", stdout)
	}
	if !strings.Contains(stdout, "=> 2\n") {
		t.Fatalf("stdout = %q, want REPL result", stdout)
	}
}

func TestRunVersionExitOptions(t *testing.T) {
	code, stdout, stderr := runGomirb(t, "p 1\n", "--version")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "gomirb "+mrbvm.Version) || strings.Contains(stdout, "=>") {
		t.Fatalf("stdout = %q, want only version", stdout)
	}

	code, stdout, stderr = runGomirb(t, "", "--copyright")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Copyright") {
		t.Fatalf("stdout = %q, want copyright", stdout)
	}
}

func TestRunQuiet(t *testing.T) {
	code, stdout, stderr := runGomirb(t, "1 + 1\n", "-q")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestRunPreload(t *testing.T) {
	lib := writeGomirbFile(t, "lib.rb", "def libfn; 40; end\n")
	code, stdout, stderr := runGomirb(t, "libfn + 2\n", "-r", lib)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "=> 42\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunPreloadBytecode(t *testing.T) {
	bytecodeLib := filepath.Join(t.TempDir(), "lib.mrb")
	if err := os.WriteFile(bytecodeLib, compileGomirbBytecode(t, "def bytecode_libfn; 40; end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runGomirb(t, "bytecode_libfn + 2\n", "-r", bytecodeLib)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "=> 42\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}

	// The RITE header selects the bytecode loader even without .mrb.
	magicLib := filepath.Join(t.TempDir(), "lib.bin")
	if err := os.WriteFile(magicLib, compileGomirbBytecode(t, "def magic_libfn; 41; end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = runGomirb(t, "magic_libfn + 1\n", "-r", magicLib)
	if code != 0 {
		t.Fatalf("magic exit = %d, stderr = %q", code, stderr)
	}
	if want := "=> 42\n"; stdout != want {
		t.Fatalf("magic stdout = %q, want %q", stdout, want)
	}
}

func TestRunPreloadMultipleLibraries(t *testing.T) {
	sourceLib := writeGomirbFile(t, "one.rb", "def one; 1; end\n")
	bytecodeLib := filepath.Join(t.TempDir(), "two.mrb")
	if err := os.WriteFile(bytecodeLib, compileGomirbBytecode(t, "def two; 2; end\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runGomirb(t, "one + two\n", "-r", sourceLib, "-r", bytecodeLib)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if want := "=> 3\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunLibraryErrors(t *testing.T) {
	code, _, stderr := runGomirb(t, "", "-r", filepath.Join(t.TempDir(), "missing.rb"))
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot open library file") {
		t.Fatalf("stderr = %q", stderr)
	}

	code, _, stderr = runGomirb(t, "", "-r", t.TempDir())
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot read library file") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunVerbose(t *testing.T) {
	code, stdout, stderr := runGomirb(t, "1 + 1\n", "--verbose")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "irep[0]") {
		t.Fatalf("stdout = %q, want disassembly", stdout)
	}
	if !strings.Contains(stdout, "=> 2\n") {
		t.Fatalf("stdout = %q, want result", stdout)
	}
}

func TestRunParseErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "short", args: []string{"-z"}, want: "invalid option -z"},
		{name: "long", args: []string{"--unknown"}, want: "invalid option --unknown"},
		{name: "missing library", args: []string{"-r"}, want: "No library specified for -r"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := runGomirb(t, "", test.args...)
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

func TestRunHelp(t *testing.T) {
	code, stdout, stderr := runGomirb(t, "", "-h")
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Usage: gomirb") || !strings.Contains(stdout, "-d") {
		t.Fatalf("stdout = %q, want usage", stdout)
	}
}

func TestRunMissingProgramFile(t *testing.T) {
	code, _, stderr := runGomirb(t, "", filepath.Join(t.TempDir(), "missing.rb"))
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot open program file") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunDirectoryProgramFile(t *testing.T) {
	code, _, stderr := runGomirb(t, "", t.TempDir())
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Cannot read program file") {
		t.Fatalf("stderr = %q", stderr)
	}
}
