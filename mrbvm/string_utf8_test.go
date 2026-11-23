package mrbvm_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// TestUTF8CharacterPositions pins the chosen UTF-8 policy: character
// positions drive length/index, while bytesize and byte APIs stay byte based.
// Regexp begin/end/offset and sprintf width follow the same character unit.
func TestUTF8CharacterPositions(t *testing.T) {
	source := `p "あいう".length
p "あいう".bytesize
p "あいう".index("う")
p "あいう" =~ /う/
m = /う/.match("あいう")
p [m.begin(0), m.end(0), m.offset(0)]
p /あ/.match("あいあ", 2).begin(0)
p ("%5s" % "あ").length
p ("%.2s" % "あいう")
p ("%5s" % "あ")
p "\xED\xA0\x80".length
p "\xED\xA0\x80"[0]
`
	want := `3
9
2
2
[2, 3, [2, 3]]
2
5
"あい"
"    あ"
3
"\xED"
`
	ctx := context.Background()
	irep, err := mrbc.Compile(ctx, []byte(source), mrbc.Options{FilePath: "utf8_positions.rb"})
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
