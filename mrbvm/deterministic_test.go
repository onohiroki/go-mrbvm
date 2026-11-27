package mrbvm_test

import (
	"testing"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

const deterministicSource = `
a = Object.new
b = Object.new
p [Time.now.to_i, a.object_id, b.object_id, "s".object_id]
`

func TestDeterministicMode(t *testing.T) {
	opts := mrbvm.Options{Deterministic: true}
	out1, err := runWithOptions(t, deterministicSource, opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	out2, err := runWithOptions(t, deterministicSource, opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if out1 != out2 {
		t.Fatalf("deterministic output differs:\n first: %s\nsecond: %s", out1, out2)
	}
	if out1 == "" {
		t.Fatal("empty output")
	}
}

func TestDeterministicClock(t *testing.T) {
	fixed := time.Unix(123, 0).UTC()
	out, err := runWithOptions(t, "p Time.now.to_i\n", mrbvm.Options{Clock: func() time.Time { return fixed }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "123\n" {
		t.Fatalf("output = %q, want 123", out)
	}
}
