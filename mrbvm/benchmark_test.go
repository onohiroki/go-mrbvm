package mrbvm_test

import (
	"context"
	"io"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func compileBench(b *testing.B, source string) *mrbvm.Irep {
	b.Helper()
	irep, err := mrbc.Compile(context.Background(), []byte(source), mrbc.Options{FilePath: "bench.rb"})
	if err != nil {
		b.Fatal(err)
	}
	return irep
}

func runBench(b *testing.B, source string) {
	irep := compileBench(b, source)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := mrbvm.New(mrbvm.Options{Stdout: io.Discard})
		if _, err := state.Execute(ctx, irep); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArithmeticLoop(b *testing.B) {
	runBench(b, `
i = 0
sum = 0
while i < 10000
  sum += i
  i += 1
end
`)
}

func BenchmarkMethodCall(b *testing.B) {
	runBench(b, `
def add(a, b)
  a + b
end
i = 0
while i < 5000
  add(i, 1)
  i += 1
end
`)
}

func BenchmarkArrayMap(b *testing.B) {
	runBench(b, `
a = (1..100).to_a
i = 0
while i < 100
  a.map { |x| x * 2 }
  i += 1
end
`)
}

func BenchmarkHashWrite(b *testing.B) {
	runBench(b, `
h = {}
i = 0
while i < 5000
  h[i] = i
  i += 1
end
`)
}

func BenchmarkStringAppend(b *testing.B) {
	runBench(b, `
s = ""
i = 0
while i < 2000
  s << "x"
  i += 1
end
`)
}

func BenchmarkRescue(b *testing.B) {
	runBench(b, `
i = 0
while i < 2000
  begin
    raise "x"
  rescue
  end
  i += 1
end
`)
}
