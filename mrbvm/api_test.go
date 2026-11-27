package mrbvm_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestHostAPI(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})

	state.DefineFunction("go_greet", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
		if len(args) != 1 {
			return mrbvm.Nil, s.Raise(s.ArgumentErrorClass, "go_greet expects one argument")
		}
		return mrbvm.StringValue("hi " + args[0].String()), nil
	})
	state.DefineFunction("go_fail", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
		return mrbvm.Nil, s.Raise(s.ArgumentErrorClass, "nope")
	})
	state.DefineConst(nil, "ANSWER", mrbvm.IntValue(42))
	greeter := state.DefineClass("Greeter", state.ObjectClass, func(class *mrbvm.Class) {
		class.Define("greet", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
			return mrbvm.StringValue("hello"), nil
		})
		class.DefineClassMethod("kind", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
			return mrbvm.StringValue("host"), nil
		})
		class.SetConst("VALUE", mrbvm.IntValue(7))
	})
	if greeter.Name() != "Greeter" || greeter.Super() != state.ObjectClass {
		t.Fatal("class metadata mismatch")
	}

	source := `
def ruby_add(a, b)
  a + b
end

p go_greet("bob")
p Greeter.new.greet
p Greeter.kind
p Greeter::VALUE
p ANSWER
p ruby_add(2, 3)
begin
  go_fail
rescue => e
  p e.message
end
`
	temp, err := os.CreateTemp(t.TempDir(), "api-*.rb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temp.WriteString(source); err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := mrbc.CompileFile(ctx, temp.Name(), mrbc.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Run(ctx, data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "\"hi bob\"\n\"hello\"\n\"host\"\n7\n42\n5\n\"nope\"\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}

	result, err := state.Send(ctx, state.TopSelf, "ruby_add", mrbvm.IntValue(4), mrbvm.IntValue(5))
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := mrbvm.AsInt(result); !ok || value != 9 {
		t.Fatalf("Send result = %s, want 9", result.Inspect())
	}
}

func TestSetGlobal(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	state.SetGlobal("$DEBUG", mrbvm.BoolValue(true))
	state.SetGlobal("$0", mrbvm.StringValue("script.rb"))

	irep, err := mrbc.Compile(ctx, []byte("p [$DEBUG, $0]\n"), mrbc.Options{FilePath: "test.rb"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.Execute(ctx, irep); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := out.String(), "[true, \"script.rb\"]\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
