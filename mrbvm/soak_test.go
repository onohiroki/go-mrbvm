//go:build soak

package mrbvm_test

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func soakRun(t *testing.T, source string) {
	t.Helper()
	data := compileSource(t, "soak.rb", source)
	state := mrbvm.New(mrbvm.Options{Stdout: io.Discard})
	if _, err := state.Run(context.Background(), data); err != nil {
		state.Close()
		t.Fatalf("run: %v", err)
	}
	state.Close()
}

func TestSoakLifecycle(t *testing.T) {
	before := goroutineBaseline()
	var beforeMem runtime.MemStats
	runtime.ReadMemStats(&beforeMem)

	for i := 0; i < 10000; i++ {
		state := mrbvm.New(mrbvm.Options{Stdout: io.Discard})
		if _, err := state.Run(context.Background(), []byte("RITE")); err == nil {
			state.Close()
			t.Fatal("invalid bytecode unexpectedly succeeded")
		}
		state.Close()
	}
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > beforeMem.HeapAlloc+32<<20 {
		t.Fatalf("heap grew from %d to %d", beforeMem.HeapAlloc, after.HeapAlloc)
	}
}

func TestSoakFiberEnumerator(t *testing.T) {
	before := goroutineBaseline()
	source := `
100.times do
  e = [1, 2, 3].each
  e.next
  f = Fiber.new { Fiber.yield 1 }
  f.resume
end
`
	for i := 0; i < 100; i++ {
		soakRun(t, source)
	}
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
}

func TestSoakRepeatedExecution(t *testing.T) {
	data := compileSource(t, "soak_exec.rb", "p 1 + 1\n")
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, MaxInstructions: 10000})
	defer state.Close()
	for i := 0; i < 10000; i++ {
		if _, err := state.Run(context.Background(), data); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	if out.Len() == 0 {
		t.Fatal("no output")
	}
}

func TestSoakCloseSuspended(t *testing.T) {
	before := goroutineBaseline()
	source := `
f = Fiber.new { Fiber.yield 1 }
f.resume
e = [1, 2, 3].each
e.next
`
	for i := 0; i < 1000; i++ {
		soakRun(t, source)
	}
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
}

func TestSoakRepeatedClose(t *testing.T) {
	before := goroutineBaseline()
	for i := 0; i < 1000; i++ {
		state := mrbvm.New(mrbvm.Options{Stdout: io.Discard})
		state.Close()
		state.Close()
	}
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
}
