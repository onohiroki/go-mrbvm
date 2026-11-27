package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// randomProgram generates a small deterministic Ruby program in the subset
// supported by both go-mrbc and the reference compiler. The generated source
// only prints deterministic values; time, randomness, IO, and filesystem
// access are deliberately excluded.
func randomProgram(rng *rand.Rand) string {
	var b strings.Builder
	b.WriteString("def add(a, b)\n  a + b\nend\n")
	b.WriteString("def apply(x)\n  yield(x)\nend\n")
	b.WriteString("class Box\n  def initialize(v)\n    @v = v\n  end\n  def value\n    @v\n  end\nend\n")

	n := 3 + rng.Intn(4)
	vars := make([]string, n)
	for i := range vars {
		vars[i] = fmt.Sprintf("v%d", i)
		fmt.Fprintf(&b, "%s = %d\n", vars[i], rng.Intn(200)-100)
	}
	fmt.Fprintf(&b, "p [%s]\n", strings.Join(vars, ", "))

	a := 2 + rng.Intn(4)
	elems := make([]string, a)
	for i := range elems {
		elems[i] = fmt.Sprint(rng.Intn(30) - 10)
	}
	fmt.Fprintf(&b, "ary = [%s]\n", strings.Join(elems, ", "))
	fmt.Fprintf(&b, "p ary\np ary.map { |x| x * %d }\n", 1+rng.Intn(4))
	fmt.Fprintf(&b, "p ary.select { |x| x %% 2 == 0 }\n")
	fmt.Fprintf(&b, "p ary.inject(0) { |s, x| s + x }\n")

	fmt.Fprintf(&b, "h = { a: %d, b: %d, c: %d }\n", rng.Intn(50), rng.Intn(50), rng.Intn(50))
	b.WriteString("p h\np h[:a]\np h.fetch(:missing, -1)\n")

	fmt.Fprintf(&b, "p add(%d, %d)\n", rng.Intn(100), rng.Intn(100))
	fmt.Fprintf(&b, "p apply(%d) { |x| x * 2 }\n", rng.Intn(100))
	fmt.Fprintf(&b, "p Box.new(%d).value\n", rng.Intn(100))

	limit := 3 + rng.Intn(8)
	fmt.Fprintf(&b, "i = 0\nsum = 0\nwhile i < %d\n  sum += i\n  i += 1\nend\np sum\n", limit)
	fmt.Fprintf(&b, "p (1..%d).to_a\n", limit)
	b.WriteString("begin\n  raise \"boom\"\nrescue => e\n  p e.message\nend\n")
	return b.String()
}
