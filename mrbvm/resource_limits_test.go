package mrbvm_test

import (
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestFiberLimit(t *testing.T) {
	source := `
f1 = Fiber.new { 1 }
f2 = Fiber.new { 2 }
Fiber.new { 3 }
`
	_, err := runWithOptions(t, source, mrbvm.Options{MaxFibers: 2})
	if err == nil || !strings.Contains(err.Error(), "fiber limit") {
		t.Fatalf("err = %v, want fiber limit", err)
	}
}

func TestFiberLimitReleasedOnTermination(t *testing.T) {
	source := `
3.times do
  f = Fiber.new { 1 }
  p f.resume
end
`
	out, err := runWithOptions(t, source, mrbvm.Options{MaxFibers: 1})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got := strings.Count(out, "1\n"); got != 3 {
		t.Fatalf("output = %q, want three results", out)
	}
}

func TestEnumeratorLimit(t *testing.T) {
	source := `
e1 = [1].each
e1.next
e2 = [1].each
e2.next
e3 = [1].each
e3.next
`
	_, err := runWithOptions(t, source, mrbvm.Options{MaxEnumerators: 2})
	if err == nil || !strings.Contains(err.Error(), "enumerator limit") {
		t.Fatalf("err = %v, want enumerator limit", err)
	}
}

func TestEnumeratorLimitReleasedOnRewind(t *testing.T) {
	source := `
3.times do
  e = [1].each
  e.next
  e.rewind
end
p :ok
`
	out, err := runWithOptions(t, source, mrbvm.Options{MaxEnumerators: 1})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, ":ok") {
		t.Fatalf("output = %q, want :ok", out)
	}
}

func TestResourceLimitsDisabledByDefault(t *testing.T) {
	source := `
10.times do
  Fiber.new { 1 }
  e = [1].each
  e.next
end
p :ok
`
	out, err := runWithOptions(t, source, mrbvm.Options{})
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, ":ok") {
		t.Fatalf("output = %q, want :ok", out)
	}
}
