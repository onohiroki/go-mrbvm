package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// TestLoadMrblib verifies the opt-in mrblib subset. It exercises Enumerable
// and Comparable on user-defined classes plus NameError's Ruby-level
// initialize.
func TestLoadMrblib(t *testing.T) {
	source := `
class Bag
  include Enumerable
  def initialize(*values)
    @values = values
  end
  def each(&block)
    @values.each(&block)
    self
  end
end

b = Bag.new(3, 1, 2)
p b.map { |x| x * 2 }
p b.select { |x| x > 1 }
p b.inject(:+)
p b.sort

class V
  include Comparable
  attr_reader :n
  def initialize(n); @n = n; end
  def <=>(other); @n <=> other.n; end
end
p V.new(1) < V.new(2)
p NameError.new("x").message
`
	temp, err := os.CreateTemp(t.TempDir(), "mrblib-*.rb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temp.WriteString(source); err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	data, err := mrbc.CompileFile(ctx, temp.Name(), mrbc.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, LoadMrblib: true})
	if _, err := state.Run(ctx, data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "[6, 2, 4]\n[3, 2]\n6\n[1, 2, 3]\ntrue\n\"x\"\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
