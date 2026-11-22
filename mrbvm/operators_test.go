package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// TestOperatorRedefinition pins the CRuby and mruby-development-tree
// behavior: replacing Integer#+ and friends must reach the opcode fast paths,
// including += / -=, identity equality, and Array#index. mruby 4.0.0's
// OP_ADD family ignores the redefinition; the development tree fixed that.
func TestOperatorRedefinition(t *testing.T) {
	source := `class Integer
  alias_method :__add_before, :+
  alias_method :__sub_before, :-
  alias_method :__eq_before, :==
  def +(o); [:add, self, o]; end
  def -(o); [:sub, self, o]; end
  def ==(o); [:eq, self, o]; end
end
a = 7
b = 2
p a + b
p a + 1
x = a
x += 1
p x
p a - b
p a - 1
y = a
y -= 1
p y
p a == b
p a == a
p [a].index(b)
p [a].index(a)
class Float
  alias_method :__mul_before, :*
  def *(o); [:mul, self, o]; end
end
p 1.5 * 2.0
p 7 * 2
nan = 0.0 / 0.0
p nan == nan
p "x" == "x"
`
	want := `[:add, 7, 2]
[:add, 7, 1]
[:add, 7, 1]
[:sub, 7, 2]
[:sub, 7, 1]
[:sub, 7, 1]
[:eq, 7, 2]
[:eq, 7, 7]
0
0
[:mul, 1.5, 2.0]
14
false
true
`
	ctx := context.Background()
	irep, err := mrbc.Compile(ctx, []byte(source), mrbc.Options{FilePath: "operator_redefinition.rb"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Execute(ctx, irep); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != want {
		t.Fatalf("stdout mismatch:\n got %q\nwant %q", got, want)
	}
}
