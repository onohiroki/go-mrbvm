// Package toolchain locates the external mruby tools used by tests.
package toolchain

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// Result is the result of running an external command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Find locates name on PATH.
func Find(name string) (string, error) {
	return exec.LookPath(name)
}

// Run executes bin with args and captures stdout and stderr. A non-zero exit
// status is returned in Result and is not treated as an error.
func Run(bin string, args ...string) (Result, error) {
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("run %s: %w", bin, err)
}

// RequireVersion checks that bin exists and reports the expected version
// prefix. It is intended for tests that need mruby 4.0.0.
func RequireVersion(bin string, args []string, want string) error {
	result, err := Run(bin, args...)
	if err != nil {
		return err
	}
	combined := result.Stdout + result.Stderr
	if result.ExitCode != 0 {
		return fmt.Errorf("%s %v exited with %d: %s", bin, args, result.ExitCode, combined)
	}
	if !bytes.Contains([]byte(combined), []byte(want)) {
		return fmt.Errorf("%s does not report %q: %s", bin, want, combined)
	}
	return nil
}
