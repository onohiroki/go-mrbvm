package mrbvm_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestEnumeratorExternalIteration(t *testing.T) {
	source := `
e = [1, 2, 3].each
p e.next
p e.peek
p e.next
e.rewind
p e.next
p e.next
p e.next
begin
  e.next
rescue StopIteration
  p :stop
end
`
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	defer state.Close()
	if _, err := state.Run(context.Background(), compileSource(t, "enumerator_next.rb", source)); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	want := "1\n2\n2\n1\n2\n3\n:stop\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestEnumeratorRewindRepeated(t *testing.T) {
	source := `
1000.times do
  e = [1, 2, 3].each
  e.next
  e.rewind
end
p :ok
`
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	defer state.Close()
	if _, err := state.Run(context.Background(), compileSource(t, "enumerator_rewind.rb", source)); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if got := out.String(); !strings.Contains(got, ":ok") {
		t.Fatalf("output = %q, want :ok", got)
	}
}

func TestEnumeratorSuspendedThenClose(t *testing.T) {
	before := goroutineBaseline()
	func() {
		var out bytes.Buffer
		state := mrbvm.New(mrbvm.Options{Stdout: &out})
		source := "e = [1, 2, 3].each\np e.next\n"
		if _, err := state.Run(context.Background(), compileSource(t, "enumerator_close.rb", source)); err != nil {
			t.Fatalf("run: %v", err)
		}
		state.Close()
	}()
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
}
