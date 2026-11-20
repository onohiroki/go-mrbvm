package mrbvm

import "context"

type stringData struct {
	value string
}

// Object is the base representation of a heap-allocated Ruby object.
type Object struct {
	class *Class
	data  any
}

// Class is a minimal mruby class or module.
type Class struct {
	name    string
	super   *Class
	methods map[string]NativeFunc
}

// NativeFunc is a Go function exposed as a Ruby method.
//
// The host API will grow a more ergonomic wrapper later; this is the minimal
// internal calling convention used by the VM.
type NativeFunc func(ctx context.Context, self Value, args []Value) (Value, error)

func newClass(name string, super *Class) *Class {
	return &Class{
		name:    name,
		super:   super,
		methods: make(map[string]NativeFunc),
	}
}

func (c *Class) lookup(name string) (NativeFunc, bool) {
	for current := c; current != nil; current = current.super {
		if fn, ok := current.methods[name]; ok {
			return fn, true
		}
	}
	return nil, false
}

func (c *Class) define(name string, fn NativeFunc) {
	c.methods[name] = fn
}

func (s *State) classOf(v Value) *Class {
	if v.kind == KindObject && v.obj != nil && v.obj.class != nil {
		return v.obj.class
	}
	return s.ObjectClass
}
