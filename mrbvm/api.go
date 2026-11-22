package mrbvm

import (
	"context"
	"sort"
	"strings"
)

// Func is the host-facing native method signature. It omits the internal
// keyword dictionary; use Send when keywords are needed.
type Func func(ctx context.Context, s *State, self Value, args []Value, block Value) (Value, error)

// DefineFunction defines a Kernel-level function on Object. Ruby code can call
// it as a normal method; a Go error is converted to an unhandled VM error.
func (s *State) DefineFunction(name string, fn Func) {
	s.ObjectClass.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		// Host functions may retain the argument slice, so hand them an
		// independent copy.
		return fn(ctx, s, self, append([]Value(nil), args...), block)
	})
}

// DefineClass creates a class with the given superclass, registers it as a
// top-level constant, and runs configure. The configure callback may be nil.
func (s *State) DefineClass(name string, super *Class, configure func(*Class)) *Class {
	if super == nil {
		super = s.ObjectClass
	}
	class := newClass(name, super)
	s.ensureClassObject(class)
	value := s.classValue(class)
	s.constants[name] = value
	s.ObjectClass.constants[name] = value
	if configure != nil {
		configure(class)
	}
	return class
}

// DefineModule creates a module (a class without a superclass) and registers
// it as a top-level constant.
func (s *State) DefineModule(name string, configure func(*Class)) *Class {
	module := newClass(name, nil)
	s.ensureClassObject(module)
	value := s.classValue(module)
	s.constants[name] = value
	s.ObjectClass.constants[name] = value
	if configure != nil {
		configure(module)
	}
	return module
}

// DefineConst defines a constant in a class or module. A nil scope means the
// top-level Object scope.
func (s *State) DefineConst(scope *Class, name string, value Value) {
	if scope == nil {
		scope = s.ObjectClass
	}
	scope.constants[name] = value
	if scope == s.ObjectClass {
		s.constants[name] = value
	}
}

// Send calls a Ruby method from Go.
func (s *State) Send(ctx context.Context, receiver Value, name string, args ...Value) (Value, error) {
	return s.call(ctx, receiver, name, args, Nil, Nil)
}

// Raise returns an error that the VM treats as a Ruby exception. Host
// functions can return it directly.
func (s *State) Raise(class *Class, message string) error {
	if class == nil {
		class = s.RuntimeErrorClass
	}
	return &unwindSignal{kind: unwindException, value: newException(class, message)}
}

// Define defines an instance method on a class or module.
func (c *Class) Define(name string, fn Func) {
	c.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		state := stateFromClass(c)
		return fn(ctx, state, self, args, block)
	})
}

// DefineClassMethod defines a class-level method.
func (c *Class) DefineClassMethod(name string, fn Func) {
	c.classMethods[name] = &method{owner: c, name: name, native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return fn(ctx, stateFromClass(c), self, args, block)
	}}
}

// SetConst defines a constant directly on the class.
func (c *Class) SetConst(name string, value Value) {
	c.constants[name] = value
}

// Name returns the class name.
func (c *Class) Name() string { return c.name }

// Super returns the superclass, or nil for a module/root class.
func (c *Class) Super() *Class { return c.super }

// Native methods need the owning State. Classes created through
// State.DefineClass or State.DefineModule record it when ensureClassObject
// initializes the class object.
func stateFromClass(c *Class) *State {
	if c != nil && c.state != nil {
		return c.state
	}
	return nil
}

// AsInt converts an Integer value.
func AsInt(v Value) (int64, bool) {
	if v.Kind() != KindInt {
		return 0, false
	}
	return v.Int(), true
}

// AsFloat converts a Float value.
func AsFloat(v Value) (float64, bool) {
	if v.Kind() != KindFloat {
		return 0, false
	}
	return v.Float(), true
}

// AsString converts a String value.
func AsString(v Value) (string, bool) {
	if v.Kind() != KindString {
		return "", false
	}
	return v.String(), true
}

// AsBool converts true/false. It reports ok for nil as false.
func AsBool(v Value) (bool, bool) {
	switch v.Kind() {
	case KindTrue:
		return true, true
	case KindFalse, KindNil:
		return false, true
	default:
		return false, false
	}
}

// Complete returns constant and method names matching prefix. It is a small
// completion source for REPL front ends.
func (s *State) Complete(prefix string) []string {
	names := make(map[string]bool)
	for name := range s.constants {
		if strings.HasPrefix(name, prefix) {
			names[name] = true
		}
	}
	for name := range s.ObjectClass.methods {
		if strings.HasPrefix(name, prefix) {
			names[name] = true
		}
	}
	for name := range s.ObjectClass.classMethods {
		if strings.HasPrefix(name, prefix) {
			names[name] = true
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
