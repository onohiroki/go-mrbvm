package mrbvm_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func TestCapabilityDenyAll(t *testing.T) {
	source := `
begin
  eval("1")
rescue SecurityError
  p :eval_denied
end
begin
  instance_eval("1")
rescue SecurityError
  p :instance_eval_denied
end
begin
  Object.class_eval("1")
rescue SecurityError
  p :class_eval_denied
end
begin
  binding.eval("1")
rescue SecurityError
  p :binding_eval_denied
end
begin
  load("no-such-file.rb")
rescue SecurityError
  p :load_denied
end
begin
  require("no-such-file")
rescue SecurityError
  p :require_denied
end
`
	out, err := runWithOptions(t, source, mrbvm.Options{
		Compiler: compileEvalForTest,
		Policy:   sandbox.New(),
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	want := ":eval_denied\n:instance_eval_denied\n:class_eval_denied\n:binding_eval_denied\n:load_denied\n:require_denied\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCapabilityPermitEval(t *testing.T) {
	out, err := runWithOptions(t, `
p eval("1 + 1")
begin
  load("no-such-file.rb")
rescue SecurityError
  p :load_denied
end
`, mrbvm.Options{
		Compiler: compileEvalForTest,
		Policy:   sandbox.New().Permit(sandbox.Eval),
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "2\n:load_denied\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCapabilityPermitLoad(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "cap_lib.rb")
	if err := os.WriteFile(lib, []byte("CAP_LIB_VALUE = 41\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
load(%q)
p CAP_LIB_VALUE
begin
  eval("1")
rescue SecurityError
  p :eval_denied
end
`, lib)
	out, err := runWithOptions(t, source, mrbvm.Options{
		Compiler: compileEvalForTest,
		Policy:   sandbox.New().Permit(sandbox.Load),
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "41\n:eval_denied\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCapabilityPermitRequire(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "cap_required.rb")
	if err := os.WriteFile(lib, []byte("CAP_REQUIRED_VALUE = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
require(%q)
p CAP_REQUIRED_VALUE
begin
  load("no-such-file.rb")
rescue SecurityError
  p :load_denied
end
`, lib)
	out, err := runWithOptions(t, source, mrbvm.Options{
		Compiler: compileEvalForTest,
		Policy:   sandbox.New().Permit(sandbox.Require),
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "42\n:load_denied\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCapabilityDoesNotReadDeniedFile(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "should_not_load.rb")
	if err := os.WriteFile(lib, []byte("SHOULD_NOT_LOAD = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`
begin
  load(%q)
rescue SecurityError
  p :denied
end
p defined?(SHOULD_NOT_LOAD)
`, lib)
	out, err := runWithOptions(t, source, mrbvm.Options{
		Compiler: compileEvalForTest,
		Policy:   sandbox.New(),
	})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if strings.Contains(out, "true") {
		t.Fatalf("denied file was evaluated: %q", out)
	}
	if !strings.Contains(out, ":denied") {
		t.Fatalf("output = %q, want :denied", out)
	}
}
