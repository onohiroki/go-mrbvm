package mrbvm

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Options controls VM construction.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
}

// State is one mruby VM instance. A State is owned by a single goroutine.
type State struct {
	ObjectClass *Class
	TopSelf     Value

	globals map[string]Value
	stdout  io.Writer
	stderr  io.Writer
}

// New creates a VM state with the minimal core needed to execute VM-0
// bytecode.
func New(opts Options) *State {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	s := &State{
		globals: make(map[string]Value),
		stdout:  stdout,
		stderr:  stderr,
	}
	s.ObjectClass = newClass("Object", nil)
	s.TopSelf = ObjectValue(&Object{class: s.ObjectClass})
	s.ObjectClass.define("puts", s.kernelPuts)
	return s
}

// LoadBytecode parses a RITE 04.00 binary.
func (s *State) LoadBytecode(data []byte) (*Irep, error) {
	return DecodeBytecode(data)
}

// Run loads and executes a RITE 04.00 binary.
func (s *State) Run(ctx context.Context, data []byte) (Value, error) {
	irep, err := s.LoadBytecode(data)
	if err != nil {
		return Nil, err
	}
	return s.Execute(ctx, irep)
}

// Execute runs a top-level irep.
func (s *State) Execute(ctx context.Context, irep *Irep) (Value, error) {
	if irep == nil {
		return Nil, fmt.Errorf("mrbvm: nil irep")
	}
	return s.executeIrep(ctx, irep, s.TopSelf)
}

func (s *State) kernelPuts(ctx context.Context, self Value, args []Value) (Value, error) {
	for _, arg := range args {
		if arg.Kind() == KindString {
			fmt.Fprintln(s.stdout, arg.String())
		} else {
			fmt.Fprintln(s.stdout, arg.Inspect())
		}
	}
	return Nil, nil
}
