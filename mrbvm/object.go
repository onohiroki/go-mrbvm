package mrbvm

import "context"

type stringData struct {
	value string
}

type arrayData struct {
	values []Value
}

type hashEntry struct {
	key   Value
	value Value
}

type hashData struct {
	entries []hashEntry
}

type rangeData struct {
	low       Value
	high      Value
	exclusive bool
}

// Object is the base representation of a heap-allocated Ruby object.
type Object struct {
	class *Class
	data  any
	ivars map[string]Value
}

func (o *Object) ivar(name string) Value {
	if o.ivars == nil {
		return Nil
	}
	return o.ivars[name]
}

func (o *Object) setIvar(name string, value Value) {
	if o.ivars == nil {
		o.ivars = make(map[string]Value)
	}
	o.ivars[name] = value
}

// Class is a minimal mruby class or module.
type Class struct {
	name    string
	super   *Class
	methods map[string]*method
}

type method struct {
	owner  *Class
	native NativeFunc
	irep   *Irep
}

// NativeFunc is a Go function exposed as a Ruby method.
//
// The host API will grow a more ergonomic wrapper later; this is the minimal
// internal calling convention used by the VM.
type NativeFunc func(ctx context.Context, self Value, args []Value, block Value) (Value, error)

type environment struct {
	parent *environment
	regs   []Value
	block  Value
}

type blockData struct {
	irep        *Irep
	env         *environment
	self        Value
	targetClass *Class
}

type breakError struct {
	value Value
}

func (e *breakError) Error() string { return "break" }

func newClass(name string, super *Class) *Class {
	return &Class{
		name:    name,
		super:   super,
		methods: make(map[string]*method),
	}
}

func (c *Class) lookup(name string) (*method, bool) {
	for current := c; current != nil; current = current.super {
		if m, ok := current.methods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

func (c *Class) define(name string, fn NativeFunc) {
	c.methods[name] = &method{owner: c, native: fn}
}

func (c *Class) defineIrep(name string, irep *Irep) {
	c.methods[name] = &method{owner: c, irep: irep}
}

func (s *State) classOf(v Value) *Class {
	if v.kind == KindObject && v.obj != nil {
		switch v.obj.data.(type) {
		case *arrayData:
			return s.ArrayClass
		case *hashData:
			return s.HashClass
		case *rangeData:
			return s.RangeClass
		case *blockData:
			return s.ProcClass
		}
		if v.obj.class != nil {
			return v.obj.class
		}
	}
	switch v.kind {
	case KindInt:
		return s.IntegerClass
	case KindFloat:
		return s.FloatClass
	case KindString:
		return s.StringClass
	case KindSymbol:
		return s.SymbolClass
	default:
		return s.ObjectClass
	}
}
