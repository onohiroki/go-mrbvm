package mrbvm_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func runWithBothOutputs(t *testing.T, source string, opts mrbvm.Options) (string, string, error) {
	t.Helper()
	data := compileSource(t, "limit_case.rb", source)
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	state := mrbvm.New(opts)
	defer state.Close()
	_, err := state.Run(context.Background(), data)
	return stdout.String(), stderr.String(), err
}

func TestArraySizeLimitBypasses(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"initialize", "Array.new(100)\n"},
		{"initialize block", "Array.new(100) { |i| i }\n"},
		{"fill", "a = [1]\na.fill(0, 0, 100)\n"},
		{"insert", "a = [1]\na.insert(0, 1, 2, 3, 4, 5, 6)\n"},
		{"index assign", "a = []\na[100] = 1\n"},
		{"repeat", "[1, 2] * 100\n"},
		{"plus", "[1, 2, 3, 4] + [5, 6, 7, 8]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runWithBothOutputs(t, tc.source, mrbvm.Options{MaxArraySize: 5})
			if err == nil || !strings.Contains(err.Error(), "array size limit") {
				t.Fatalf("err = %v, want array size limit", err)
			}
		})
	}
}

func TestStringSizeLimitBypasses(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"ljust", `"x".ljust(100)` + "\n"},
		{"rjust", `"x".rjust(100)` + "\n"},
		{"center", `"x".center(100)` + "\n"},
		{"repeat", `"x" * 100` + "\n"},
		{"format", `"%100s" % "x"` + "\n"},
		{"join", `(["x"] * 100).join` + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runWithBothOutputs(t, tc.source, mrbvm.Options{MaxStringSize: 8})
			if err == nil || !strings.Contains(err.Error(), "string size limit") {
				t.Fatalf("err = %v, want string size limit", err)
			}
		})
	}
}

func TestHashSizeLimitBypass(t *testing.T) {
	source := `Hash[(1..100).map { |i| [i, i] }]` + "\n"
	_, _, err := runWithBothOutputs(t, source, mrbvm.Options{MaxHashSize: 8})
	if err == nil || !strings.Contains(err.Error(), "hash size limit") {
		t.Fatalf("err = %v, want hash size limit", err)
	}
}

func TestOutputLimitCoversStderr(t *testing.T) {
	stdout, stderr, err := runWithBothOutputs(t, `STDERR.puts("abcdef")`+"\n", mrbvm.Options{MaxOutputBytes: 4})
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("err = %v, want output limit", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if len(stderr) != 4 {
		t.Fatalf("stderr = %q, want 4 bytes", stderr)
	}
}

func TestOutputLimitWithExecute(t *testing.T) {
	irep, err := compileEvalForTest(context.Background(), []byte(`puts "abcdef"`+"\n"), mrbvm.CompileOptions{FilePath: "limit.rb"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &stdout, Stderr: &stderr, MaxOutputBytes: 4})
	defer state.Close()
	_, err = state.Execute(context.Background(), irep)
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("err = %v, want output limit", err)
	}
	if stdout.String() != "abcd" {
		t.Fatalf("stdout = %q, want abcd", stdout.String())
	}
}

func TestLimitsResetPerExecute(t *testing.T) {
	long, err := compileEvalForTest(context.Background(), []byte("while true; end\n"), mrbvm.CompileOptions{FilePath: "loop.rb"})
	if err != nil {
		t.Fatal(err)
	}
	short, err := compileEvalForTest(context.Background(), []byte("p 1\n"), mrbvm.CompileOptions{FilePath: "short.rb"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &stdout, MaxInstructions: 1000})
	defer state.Close()
	if _, err := state.Execute(context.Background(), long); err == nil || !strings.Contains(err.Error(), "instruction limit") {
		t.Fatalf("first err = %v, want instruction limit", err)
	}
	if _, err := state.Execute(context.Background(), short); err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "1") {
		t.Fatalf("stdout = %q, want 1", stdout.String())
	}
}
