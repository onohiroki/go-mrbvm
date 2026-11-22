package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestRegexp(t *testing.T) {
	source := `
r = Regexp.new("a(b+)c")
p r.source
p r =~ "xxabbbcyy"
p r.match?("abc")
p r === "abc"
m = r.match("xxabbbcyy")
p m[0]
p m[1]
p m.to_a
p m.pre_match
p m.post_match
p "abc" =~ /b(c)/
p "abc".sub(/b/, "X")
p "a1b2".gsub(/\d/, "#")
p "a1b2".gsub(/(\d)/, '<\1>')
p "a1b2c3".scan(/(\d)/)
p Regexp.escape("a.b")
p Regexp.new("abc", "i").match?("ABC")
`
	data := compileSource(t, "regexp_case.rb", source)
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), data); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "\"a(b+)c\"\n2\ntrue\ntrue\n\"abbbc\"\n\"bbb\"\n[\"abbbc\", \"bbb\"]\n\"xx\"\n\"yy\"\n1\n\"aXc\"\n\"a#b#\"\n\"a<1>b<2>\"\n[\"1\", \"2\", \"3\"]\n\"a\\\\.b\"\ntrue\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch:\n got %q\nwant %q", got, want)
	}
}
