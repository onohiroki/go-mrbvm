package mrbvm

import (
	"context"
	"testing"

	"github.com/onohiroki/go-mrbc/bytecode"
)

func TestProgramRejectsInvalidLocals(t *testing.T) {
	state := New(Options{})
	defer state.Close()
	_, err := state.program(&Irep{
		Nlocals: 2,
		Nregs:   1,
		Code:    []byte{byte(bytecode.OpSTOP)},
	})
	if err == nil {
		t.Fatal("expected nlocals > nregs to be rejected")
	}
}

func TestProgramRejectsInvalidJump(t *testing.T) {
	state := New(Options{})
	defer state.Close()
	_, err := state.program(&Irep{
		Nregs: 1,
		Code:  []byte{byte(bytecode.OpJMP), 0xff, 0xff},
	})
	if err == nil {
		t.Fatal("expected a jump into the middle of an instruction to be rejected")
	}
}

func TestRunRejectsMalformedData(t *testing.T) {
	state := New(Options{})
	defer state.Close()
	cases := [][]byte{
		nil,
		{},
		[]byte("RITE"),
		[]byte("RITE0400"),
		[]byte("not bytecode"),
	}
	for _, data := range cases {
		if _, err := state.Run(context.Background(), data); err == nil {
			t.Fatalf("Run(%q) unexpectedly succeeded", data)
		}
	}
}
