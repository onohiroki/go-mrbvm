package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
)

func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeApp(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "main.rb"), `
require "helper"
puts "hello " + ARGV.join(",")
puts helper_value
puts $0
`)
	writeFile(t, filepath.Join(dir, "lib", "helper.rb"), `
def helper_value
  "helper"
end
`)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// builtExecutable returns the path the build command writes. On Windows,
// build appends .exe when the requested output has no extension.
func builtExecutable(path string) string {
	if runtime.GOOS == "windows" {
		return path + ".exe"
	}
	return path
}

func TestPackAndRun(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir)
	output := filepath.Join(dir, "app.mrbb")
	code, _, stderr := runTool(t, "pack", "--root", dir, "--entry", "main.rb", "-I", "lib", "-o", output, filepath.Join(dir, "main.rb"), filepath.Join(dir, "lib"))
	if code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}

	code, stdout, stderr := runTool(t, "run", output, "a", "b")
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	want := "hello a,b\nhelper\nmain.mrb\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestPackDataAssets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.rb"), `puts File.read("data/msg.txt")`+"\n")
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "data", "msg.txt"), "asset\n")

	output := filepath.Join(dir, "app.mrbb")
	code, _, stderr := runTool(t, "pack", "--root", dir, "--entry", "main.rb", "--include", filepath.Join(dir, "data"),
		"--quiet", "-o", output, filepath.Join(dir, "main.rb"))
	if code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}

	code, stdout, stderr := runTool(t, "run", output)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "asset\n\n" {
		t.Fatalf("stdout = %q", stdout)
	}

	code, stdout, stderr = runTool(t, "list", output)
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "data/msg.txt") {
		t.Fatalf("list = %q", stdout)
	}
}

func TestPackRejectsCodeAndDataCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.rb")
	writeFile(t, path, "puts 1\n")
	code, _, stderr := runTool(t, "pack", "--root", dir, "-o", filepath.Join(dir, "app.mrbb"),
		"--include", path, path)
	if code == 0 || !strings.Contains(stderr, "both code and data") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestPackIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir)
	first := filepath.Join(dir, "first.mrbb")
	second := filepath.Join(dir, "second.mrbb")
	args := func(output string) []string {
		return []string{"pack", "--root", dir, "--entry", "main.rb", "-I", "lib", "--quiet", "-o", output,
			filepath.Join(dir, "main.rb"), filepath.Join(dir, "lib")}
	}
	if code, _, stderr := runTool(t, args(first)...); code != 0 {
		t.Fatalf("first pack exit = %d, stderr = %q", code, stderr)
	}
	if code, _, stderr := runTool(t, args(second)...); code != 0 {
		t.Fatalf("second pack exit = %d, stderr = %q", code, stderr)
	}
	firstData, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("pack output is not deterministic")
	}
}

func TestPackRejectsFilesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "main.rb")
	writeFile(t, outside, "puts 1\n")
	code, _, stderr := runTool(t, "pack", "--root", root, "-o", filepath.Join(root, "app.mrbb"), outside)
	if code == 0 {
		t.Fatal("expected pack to fail")
	}
	if !strings.Contains(stderr, "outside --root") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestPackFindsMainEntry(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir)
	writeFile(t, filepath.Join(dir, "README.md"), "not Ruby\n")
	writeFile(t, filepath.Join(dir, "old.mrbb"), "not an archive\n")
	output := filepath.Join(dir, "app.mrbb")
	if code, _, stderr := runTool(t, "pack", "--root", dir, "-o", output, filepath.Join(dir, "lib"), filepath.Join(dir, "main.rb")); code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}
	code, stdout, stderr := runTool(t, "list", output)
	if code != 0 {
		t.Fatalf("list exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "* mrb") || !strings.Contains(stdout, "main.mrb") {
		t.Fatalf("list = %q", stdout)
	}
}

func TestInfoAndVersion(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir)
	output := filepath.Join(dir, "app.mrbb")
	if code, _, stderr := runTool(t, "pack", "--root", dir, "--entry", "main.rb", "--quiet", "-o", output, dir); code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}
	code, stdout, stderr := runTool(t, "info", output)
	if code != 0 {
		t.Fatalf("info exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, `"entry": "main.mrb"`) || !strings.Contains(stdout, `"format": 1`) {
		t.Fatalf("info = %q", stdout)
	}

	code, stdout, stderr = runTool(t, "version")
	if code != 0 || !strings.Contains(stdout, "gomrbpack") {
		t.Fatalf("version exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	writeApp(t, dir)
	output := filepath.Join(dir, "app.mrbb")
	if code, _, stderr := runTool(t, "pack", "--root", dir, "--entry", "main.rb", "-o", output, dir); code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}
	extractDir := filepath.Join(dir, "out")
	if code, _, stderr := runTool(t, "extract", "-o", extractDir, output); code != 0 {
		t.Fatalf("extract exit = %d, stderr = %q", code, stderr)
	}
	for _, name := range []string{"main.mrb", "lib/helper.mrb"} {
		info, err := os.Stat(filepath.Join(extractDir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.IsDir() {
			t.Fatalf("%s is a directory", name)
		}
	}
}

func TestPackBytecodeInput(t *testing.T) {
	dir := t.TempDir()
	irep, err := mrbc.Compile(context.Background(), []byte("puts \"from bytecode\"\n"), mrbc.Options{FilePath: "main.rb", DebugInfo: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := mrbc.DumpWithOptions(irep, bytecode.DumpOptions{DebugInfo: true})
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "main.mrb")
	if err := os.WriteFile(input, data, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "app.mrbb")
	if code, _, stderr := runTool(t, "pack", "--root", dir, "--entry", "main.mrb", "-o", output, input); code != 0 {
		t.Fatalf("pack exit = %d, stderr = %q", code, stderr)
	}
	code, stdout, stderr := runTool(t, "run", output)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "from bytecode\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestPackRejectsUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeFile(t, path, "hello\n")
	code, _, stderr := runTool(t, "pack", "--root", dir, "-o", filepath.Join(dir, "app.mrbb"), path)
	if code == 0 || !strings.Contains(stderr, "unsupported file type") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
}

func TestBuildWithCompiler(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "main.rb")
	writeFile(t, source, `puts eval("6 * 7")`+"\n")
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	withCompiler := filepath.Join(dir, "with-compiler")
	code, stdout, stderr := runTool(t, "build", "--root", dir, "--entry", "main.rb", "--replace", moduleRoot,
		"--with-compiler", "--quiet", "-o", withCompiler, source)
	if code != 0 {
		t.Fatalf("build exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if out, err := exec.Command(builtExecutable(withCompiler)).CombinedOutput(); err != nil || string(out) != "42\n" {
		t.Fatalf("with-compiler app: %v, output = %q", err, out)
	}

	withoutCompiler := filepath.Join(dir, "without-compiler")
	code, stdout, stderr = runTool(t, "build", "--root", dir, "--entry", "main.rb", "--replace", moduleRoot,
		"--quiet", "-o", withoutCompiler, source)
	if code != 0 {
		t.Fatalf("build exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	cmd := exec.Command(builtExecutable(withoutCompiler))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err == nil {
		t.Fatalf("eval unexpectedly succeeded: %q", out.String())
	}
	if !strings.Contains(out.String(), "capability denied: eval") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go build in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go command not available")
	}
	dir := t.TempDir()
	writeApp(t, dir)
	output := filepath.Join(dir, "app")
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTool(t, "build", "--root", dir, "--entry", "main.rb", "-I", "lib",
		"--replace", moduleRoot, "--quiet", "-o", output, dir)
	if code != 0 {
		t.Fatalf("build exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	cmd := exec.Command(builtExecutable(output), "x", "y")
	var runOut bytes.Buffer
	cmd.Stdout = &runOut
	cmd.Stderr = &runOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("run built app: %v\n%s", err, runOut.String())
	}
	want := "hello x,y\nhelper\nmain.mrb\n"
	if runOut.String() != want {
		t.Fatalf("output = %q, want %q", runOut.String(), want)
	}
}
