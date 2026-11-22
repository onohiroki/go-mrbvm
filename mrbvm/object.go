package mrbvm

import (
	"context"
	"math/big"
)

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
	entries      []hashEntry
	defaultValue Value
}

type rangeData struct {
	low       Value
	high      Value
	exclusive bool
}

// Object is the base representation of a heap-allocated Ruby object.
type Object struct {
	class     *Class
	singleton *Class
	data      any
	ivars     map[string]Value
	frozen    bool
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
	name           string
	super          *Class
	methods        map[string]*method
	classMethods   map[string]*method
	constants      map[string]Value
	classVars      map[string]Value
	object         *Object
	singletonOf    *Class
	singleton      *Class
	included       []*Class
	privateMethods map[string]bool
	state          *State
}

type method struct {
	owner  *Class
	name   string
	native NativeFunc
	irep   *Irep
}

// NativeFunc is a Go function exposed as a Ruby method.
//
// The host API will grow a more ergonomic wrapper later; this is the minimal
// internal calling convention used by the VM.
type NativeFunc func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error)

type environment struct {
	parent      *environment
	regs        []Value
	block       Value
	methodFrame bool
}

type blockData struct {
	irep        *Irep
	env         *environment
	self        Value
	targetClass *Class
	strict      bool
	mid         string
	owner       *Class
}

// exceptionData is the payload of a Ruby exception object.
type exceptionData struct {
	message     string
	backtrace   []string
	cause       Value
	name        Value
	receiver    Value
	receiverSet bool
	args        Value
}

type bigIntData struct {
	value *big.Int
}

// unwindKind identifies a non-local control-flow signal.
type unwindKind int

const (
	unwindException unwindKind = iota
	unwindReturn
	unwindBreak
	unwindJump
)

// unwindSignal travels through Go error returns while a rescue or ensure
// handler catches it. Carrying the signal as a value lets OP_EXCEPT and
// OP_RAISEIF reproduce mruby's mrb->exc based unwinding.
type unwindSignal struct {
	kind       unwindKind
	value      Value
	target     *environment
	jumpTarget int
}

func (e *unwindSignal) Error() string {
	switch e.kind {
	case unwindException:
		text := "unhandled exception: " + e.value.Inspect() + " (" + exceptionMessage(e.value) + ")"
		if e.value.obj != nil {
			if data, ok := e.value.obj.data.(*exceptionData); ok {
				for _, line := range data.backtrace {
					text += "\n\tfrom " + line
				}
			}
		}
		return text
	case unwindReturn:
		return "return"
	case unwindBreak:
		return "break"
	default:
		return "jump"
	}
}

// signalData wraps a control-flow signal so it can live in a register while
// an ensure body runs.
type signalData struct {
	signal *unwindSignal
}

type enumeratorData struct {
	collect func(ctx context.Context, yield func([]Value) error) error
	items   [][]Value
	loaded  bool
	index   int
}

// nativeBlockData lets Go code act as a Ruby block. Enumerable uses it to
// pass a collector block to an arbitrary object's each method.
type nativeBlockData struct {
	fn func(ctx context.Context, args []Value) (Value, error)
}

func newException(class *Class, message string) Value {
	return ObjectValue(&Object{class: class, data: &exceptionData{message: message, cause: Nil}})
}

func exceptionMessage(v Value) string {
	if v.Kind() == KindObject && v.obj != nil {
		if data, ok := v.obj.data.(*exceptionData); ok {
			return data.message
		}
	}
	return v.ToS()
}

func signalValue(sig *unwindSignal) Value {
	return ObjectValue(&Object{data: &signalData{signal: sig}})
}

func asSignal(v Value) (*unwindSignal, bool) {
	if v.Kind() == KindObject && v.obj != nil {
		if data, ok := v.obj.data.(*signalData); ok {
			return data.signal, true
		}
	}
	return nil, false
}

func newClass(name string, super *Class) *Class {
	return &Class{
		name:           name,
		super:          super,
		methods:        make(map[string]*method),
		classMethods:   make(map[string]*method),
		constants:      make(map[string]Value),
		classVars:      make(map[string]Value),
		privateMethods: make(map[string]bool),
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
	c.methods[name] = &method{owner: c, name: name, native: fn}
}

func (c *Class) defineIrep(name string, irep *Irep) {
	if c.singletonOf != nil {
		c.singletonOf.classMethods[name] = &method{owner: c.singletonOf, name: name, irep: irep}
		return
	}
	c.methods[name] = &method{owner: c, name: name, irep: irep}
}

func (c *Class) defineClassIrep(name string, irep *Irep) {
	c.classMethods[name] = &method{owner: c, name: name, irep: irep}
}

func (c *Class) lookupClassMethod(name string) (*method, bool) {
	for current := c; current != nil; current = current.super {
		if m, ok := current.classMethods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

func (s *State) ensureClassObject(c *Class) *Object {
	if c.state == nil {
		c.state = s
	}
	if c.object == nil {
		c.object = &Object{class: s.ClassClass, data: c}
	}
	return c.object
}

func (s *State) classValue(c *Class) Value {
	return ObjectValue(s.ensureClassObject(c))
}

func (s *State) asClass(v Value) (*Class, bool) {
	if v.Kind() == KindObject && v.obj != nil {
		c, ok := v.obj.data.(*Class)
		return c, ok
	}
	return nil, false
}

func (s *State) classOf(v Value) *Class {
	if v.obj != nil && v.obj.singleton != nil {
		return v.obj.singleton
	}
	if v.kind == KindObject && v.obj != nil {
		if v.obj.class != nil {
			return v.obj.class
		}
		switch v.obj.data.(type) {
		case *arrayData:
			return s.ArrayClass
		case *hashData:
			return s.HashClass
		case *rangeData:
			return s.RangeClass
		case *Class:
			return s.ClassClass
		case *blockData:
			return s.ProcClass
		case *enumeratorData:
			return s.EnumeratorClass
		case *timeData:
			return s.TimeClass
		case *fileData:
			return s.FileClass
		}
	}
	switch v.kind {
	case KindNil:
		return s.NilClass
	case KindTrue:
		return s.TrueClass
	case KindFalse:
		return s.FalseClass
	case KindInt, KindBigInt:
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
