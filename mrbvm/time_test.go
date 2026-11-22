package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestTime(t *testing.T) {
	source := `
t = Time.at(0)
p t.to_i
p t.to_f
p t.year
p t.month
p t.day
p t.strftime("%Y-%m-%d")
p t.class
p (t + 60).to_i
p (t + 60) - t
p Time.at(0) <=> Time.at(1)
p Time.now.class
`
	data := compileSource(t, "time_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "0\n0.0\n1970\n1\n1\n\"1970-01-01\"\nTime\n60\n60.0\n-1\nTime\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
