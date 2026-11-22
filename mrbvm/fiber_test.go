package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestFiber(t *testing.T) {
	source := `
f = Fiber.new do |x|
  a = Fiber.yield(x + 1)
  b = Fiber.yield(a * 2)
  a + b
end

p f.resume(3)
p f.alive?
p f.resume(5)
p f.resume(7)
p f.alive?
p f.dead?
begin
  f.resume
rescue => e
  p e.class
end

def helper(x)
  Fiber.yield(x + 10)
  x + 20
end
g = Fiber.new { |x| helper(x) }
p g.resume(1)
p g.resume(nil)
`
	data := compileSource(t, "fiber_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "4\ntrue\n10\n12\nfalse\ntrue\nFiberError\n11\n21\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
