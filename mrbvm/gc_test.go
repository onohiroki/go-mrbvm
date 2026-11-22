package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestGCShim(t *testing.T) {
	source := `
p GC.enable?
GC.disable
p GC.enable?
GC.enable
p GC.enable?
p GC.count.is_a?(Integer)
stats = GC.stat
p stats[:count].is_a?(Integer)
p stats[:heap_alloc] >= 0
GC.start
p :ok
`
	data := compileSource(t, "gc_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "true\nfalse\ntrue\ntrue\ntrue\ntrue\n:ok\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
