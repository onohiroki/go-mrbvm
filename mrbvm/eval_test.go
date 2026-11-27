package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func compileEvalForTest(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
	return mrbc.Compile(ctx, source, mrbc.Options{
		FilePath:     opts.FilePath,
		LocalNames:   opts.LocalNames,
		LineOffset:   opts.LineOffset,
		Eval:         opts.Eval,
		EvalInMethod: opts.EvalInMethod,
	})
}

func TestEvalAndBinding(t *testing.T) {
	source := `
p eval('1 * 10')
a = 10
p eval('a')
p eval('a = 20')
p a
p eval("[__FILE__, __LINE__]", nil, "test.rb", 10)

x = 1
b = binding
p b.eval('x + 1')
b.eval('y = 5')
p b.local_variable_get(:y)
p b.local_variable_defined?(:y)

def evalled_product(n)
  eval('n * 2')
end
p evalled_product(3)

obj = Object.new
obj.instance_eval('@value = 7')
p obj.instance_eval('@value')

klass = Class.new
klass.class_eval('def hi; :hi; end')
p klass.new.hi

p eval('def evalled_global_method; :defined; end')
p evalled_global_method
`
	data := compileSource(t, "eval_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Compiler: compileEvalForTest, Policy: sandbox.New().Permit(sandbox.Eval)})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "10\n10\n20\n20\n[\"test.rb\", 10]\n2\n5\ntrue\n6\n7\n:hi\n:evalled_global_method\n:defined\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
