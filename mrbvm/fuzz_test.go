package mrbvm_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func FuzzRun(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("RITE"))
	f.Add([]byte("RITE0400"))
	if irep, err := mrbc.Compile(context.Background(), []byte("p 1\n"), mrbc.Options{FilePath: "seed.rb"}); err == nil {
		if data, err := mrbc.Dump(irep); err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		state := mrbvm.New(mrbvm.Options{
			Stdout:          io.Discard,
			Stderr:          io.Discard,
			MaxInstructions: 100000,
			MaxDepth:        50,
			MaxTime:         100 * time.Millisecond,
			MaxOutputBytes:  1024,
			Policy:          sandbox.New(),
		})
		defer state.Close()
		_, _ = state.Run(context.Background(), data)
	})
}

func FuzzDecodeBytecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("RITE"))
	f.Add([]byte("RITE0400"))
	if irep, err := mrbc.Compile(context.Background(), []byte("p 1\n"), mrbc.Options{FilePath: "seed.rb"}); err == nil {
		if data, err := mrbc.Dump(irep); err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = mrbvm.DecodeBytecode(data)
	})
}
