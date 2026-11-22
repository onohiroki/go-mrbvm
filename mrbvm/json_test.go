package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestJSON(t *testing.T) {
	source := `
v = JSON.parse('{"a":1,"b":[true,null,"x"],"c":3.5}')
p v["a"]
p v["b"]
p v["c"]
p JSON.generate({"a" => 1, "b" => [true, nil, "x"]})
begin
  JSON.parse("{")
rescue => e
  p e.class
end
`
	data := compileSource(t, "json_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "1\n[true, nil, \"x\"]\n3.5\n\"{\\\"a\\\":1,\\\"b\\\":[true,null,\\\"x\\\"]}\"\nArgumentError\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
