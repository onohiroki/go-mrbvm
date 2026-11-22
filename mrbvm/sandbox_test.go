package mrbvm_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func runWithOptions(t *testing.T, source string, opts mrbvm.Options) (string, error) {
	t.Helper()
	data := compileSource(t, "sandbox_case.rb", source)
	var out bytes.Buffer
	opts.Stdout = &out
	state := mrbvm.New(opts)
	_, err := state.Run(context.Background(), data)
	return out.String(), err
}

func TestInstructionLimit(t *testing.T) {
	_, err := runWithOptions(t, "i = 0\nwhile true; i += 1; end\n", mrbvm.Options{MaxInstructions: 1000})
	if err == nil || !strings.Contains(err.Error(), "instruction limit") {
		t.Fatalf("err = %v, want instruction limit", err)
	}
}

func TestTimeLimit(t *testing.T) {
	_, err := runWithOptions(t, "while true; end\n", mrbvm.Options{MaxInstructions: 1 << 30, MaxTime: 50 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "time limit") {
		t.Fatalf("err = %v, want time limit", err)
	}
}

func TestDepthLimit(t *testing.T) {
	source := `
def recurse(n)
  recurse(n + 1)
end
begin
  recurse(0)
rescue SystemStackError
  p :deep
end
`
	out, err := runWithOptions(t, source, mrbvm.Options{MaxDepth: 50, MaxInstructions: 1 << 30})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if out != ":deep\n" {
		t.Fatalf("out = %q, want :deep", out)
	}
}

func TestOutputLimit(t *testing.T) {
	out, err := runWithOptions(t, `puts "abcdef"`+"\n", mrbvm.Options{MaxOutputBytes: 4})
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("err = %v, want output limit", err)
	}
	if out != "abcd" {
		t.Fatalf("out = %q, want abcd", out)
	}
}

func TestCollectionLimits(t *testing.T) {
	cases := []struct {
		name   string
		source string
		opts   mrbvm.Options
		want   string
	}{
		{"array", "a = []\n10.times { a << 1 }\n", mrbvm.Options{MaxArraySize: 5}, "array size limit"},
		{"hash", "h = {}\n10.times { |i| h[i] = i }\n", mrbvm.Options{MaxHashSize: 5}, "hash size limit"},
		{"string", "s = \"\"\n10.times { s << \"x\" }\n", mrbvm.Options{MaxStringSize: 5}, "string size limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runWithOptions(t, tc.source, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestCapabilityPolicy(t *testing.T) {
	policy := sandbox.New()
	state := mrbvm.New(mrbvm.Options{Policy: policy})
	if state.Allowed(sandbox.FileRead) {
		t.Fatal("FileRead should be denied by default")
	}
	policy.Permit(sandbox.FileRead)
	if !state.Allowed(sandbox.FileRead) {
		t.Fatal("FileRead should be allowed after Permit")
	}
	policy.Deny(sandbox.FileRead)
	if state.Allowed(sandbox.FileRead) {
		t.Fatal("FileRead should be denied after Deny")
	}
}
