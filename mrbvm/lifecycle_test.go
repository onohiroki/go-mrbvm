package mrbvm_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func goroutineBaseline() int {
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	return runtime.NumGoroutine()
}

func waitForGoroutines(before int) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if runtime.NumGoroutine() <= before+2 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("goroutines did not settle: before=%d after=%d", before, runtime.NumGoroutine())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloseIdempotent(t *testing.T) {
	state := mrbvm.New(mrbvm.Options{})
	state.Close()
	state.Close()
}

func TestRunAfterClose(t *testing.T) {
	data := compileSource(t, "closed.rb", "p 1\n")
	state := mrbvm.New(mrbvm.Options{})
	state.Close()
	_, err := state.Run(context.Background(), data)
	if !errors.Is(err, mrbvm.ErrStateClosed) {
		t.Fatalf("err = %v, want ErrStateClosed", err)
	}
}

func TestCloseStopsScheduler(t *testing.T) {
	before := goroutineBaseline()
	for i := 0; i < 100; i++ {
		state := mrbvm.New(mrbvm.Options{})
		state.Close()
	}
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
}

func TestCloseStopsSuspendedFiber(t *testing.T) {
	before := goroutineBaseline()
	source := `
f = Fiber.new { Fiber.yield 1 }
p f.resume
`
	var out bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out})
	if _, err := state.Run(context.Background(), compileSource(t, "fiber_close.rb", source)); err != nil {
		t.Fatalf("run: %v", err)
	}
	state.Close()
	if err := waitForGoroutines(before); err != nil {
		t.Fatal(err)
	}
}
