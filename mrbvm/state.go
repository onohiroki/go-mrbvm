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
	ObjectClass  *Class
	IntegerClass *Class
	FloatClass   *Class
	StringClass  *Class
	SymbolClass  *Class
	ArrayClass   *Class
	HashClass    *Class
	RangeClass   *Class
	ProcClass    *Class

	TopSelf Value

	globals   map[string]Value
	constants map[string]Value
	stdout    io.Writer
	stderr    io.Writer
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
		globals:   make(map[string]Value),
		constants: make(map[string]Value),
		stdout:    stdout,
		stderr:    stderr,
	}
	s.ObjectClass = newClass("Object", nil)
	s.IntegerClass = newClass("Integer", s.ObjectClass)
	s.FloatClass = newClass("Float", s.ObjectClass)
	s.StringClass = newClass("String", s.ObjectClass)
	s.SymbolClass = newClass("Symbol", s.ObjectClass)
	s.ArrayClass = newClass("Array", s.ObjectClass)
	s.HashClass = newClass("Hash", s.ObjectClass)
	s.RangeClass = newClass("Range", s.ObjectClass)
	s.ProcClass = newClass("Proc", s.ObjectClass)

	s.TopSelf = ObjectValue(&Object{class: s.ObjectClass})
	s.ObjectClass.define("puts", s.kernelPuts)
	s.ObjectClass.define("p", s.kernelP)

	s.ArrayClass.define("[]", s.arrayIndexMethod)
	s.ArrayClass.define("length", s.arrayLengthMethod)
	s.ArrayClass.define("to_a", func(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
		return self, nil
	})
	s.HashClass.define("[]", s.hashIndexMethod)
	s.HashClass.define("length", s.hashLengthMethod)
	s.HashClass.define("deconstruct_keys", s.hashDeconstructKeysMethod)
	s.HashClass.define("__pat_values", s.hashPatValuesMethod)
	s.HashClass.define("__except", s.hashExceptMethod)
	s.RangeClass.define("to_a", s.rangeToAMethod)
	s.RangeClass.define("===", s.rangeCaseEqMethod)
	s.StringClass.define("length", s.stringLengthMethod)
	s.ArrayClass.define("each", s.arrayEachMethod)
	s.ProcClass.define("call", s.procCallMethod)
	return s
}

func (s *State) index(ctx context.Context, recv, index Value) (Value, error) {
	if recv.kind == KindObject && recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if index.Kind() != KindInt {
				return Nil, fmt.Errorf("mrbvm: array index must be an integer")
			}
			pos := int(index.Int())
			if pos < 0 {
				pos += len(data.values)
			}
			if pos < 0 || pos >= len(data.values) {
				return Nil, nil
			}
			return data.values[pos], nil
		case *hashData:
			for _, entry := range data.entries {
				if valuesEqual(entry.key, index) {
					return entry.value, nil
				}
			}
			return Nil, nil
		case *stringData:
			if index.Kind() != KindInt {
				return Nil, fmt.Errorf("mrbvm: string index must be an integer")
			}
			runes := []rune(data.value)
			pos := int(index.Int())
			if pos < 0 {
				pos += len(runes)
			}
			if pos < 0 || pos >= len(runes) {
				return Nil, nil
			}
			return StringValue(string(runes[pos])), nil
		}
	}
	return Nil, fmt.Errorf("mrbvm: %s is not indexable", recv.Inspect())
}

func (s *State) arrayIndexMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: Array#[] expects one argument")
	}
	return s.index(ctx, self, args[0])
}

func (s *State) hashIndexMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: Hash#[] expects one argument")
	}
	return s.index(ctx, self, args[0])
}

func (s *State) arrayLengthMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	return IntValue(int64(len(data.values))), nil
}

func (s *State) hashLengthMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	return IntValue(int64(len(data.entries))), nil
}

func (s *State) stringLengthMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	return IntValue(int64(len([]rune(self.String())))), nil
}

func (s *State) hashEntries(self Value) ([]hashEntry, bool) {
	if self.Kind() != KindObject || self.obj == nil {
		return nil, false
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return nil, false
	}
	return data.entries, true
}

func (s *State) hashDeconstructKeysMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	entries, ok := s.hashEntries(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects one argument")
	}
	if args[0].IsNil() {
		return HashValue(append([]hashEntry(nil), entries...)), nil
	}
	if args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects an array or nil")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects an array or nil")
	}
	result := make([]hashEntry, 0, len(keys.values))
	for _, key := range keys.values {
		for _, entry := range entries {
			if valuesEqual(entry.key, key) {
				result = append(result, entry)
				break
			}
		}
	}
	return HashValue(result), nil
}

func (s *State) hashPatValuesMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	entries, ok := s.hashEntries(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 || args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: __pat_values expects an array")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: __pat_values expects an array")
	}
	values := make([]Value, 0, len(keys.values))
	for _, key := range keys.values {
		found := false
		for _, entry := range entries {
			if valuesEqual(entry.key, key) {
				values = append(values, entry.value)
				found = true
				break
			}
		}
		if !found {
			return False, nil
		}
	}
	return ArrayValue(values), nil
}

func (s *State) hashExceptMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	entries, ok := s.hashEntries(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 || args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: __except expects an array")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: __except expects an array")
	}
	var result []hashEntry
	for _, entry := range entries {
		excluded := false
		for _, key := range keys.values {
			if valuesEqual(entry.key, key) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, entry)
		}
	}
	return HashValue(result), nil
}

func (s *State) rangeCaseEqMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if len(args) != 1 || self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: Range#=== expects one argument")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if low, ok := binaryValue(args[0], ">=", data.low); ok && low.IsTruthy() {
		op := "<="
		if data.exclusive {
			op = "<"
		}
		if high, ok := binaryValue(args[0], op, data.high); ok {
			return high, nil
		}
	}
	return False, nil
}

func (s *State) rangeToAMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	low, high := data.low, data.high
	if low.Kind() != KindInt || high.Kind() != KindInt {
		return Nil, fmt.Errorf("mrbvm: only integer ranges are supported")
	}
	var values []Value
	for i := low.Int(); i <= high.Int(); i++ {
		if data.exclusive && i == high.Int() {
			break
		}
		values = append(values, IntValue(i))
	}
	return ArrayValue(values), nil
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
	return s.executeFrame(ctx, irep, s.TopSelf, s.ObjectClass, nil, Nil, nil)
}

func (s *State) setIndex(ctx context.Context, recv, index, value Value) (Value, error) {
	if recv.kind == KindObject && recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if index.Kind() != KindInt {
				return Nil, fmt.Errorf("mrbvm: array index must be an integer")
			}
			pos := int(index.Int())
			if pos < 0 {
				pos += len(data.values)
			}
			if pos < 0 {
				return Nil, fmt.Errorf("mrbvm: index %d too small for array", pos)
			}
			for len(data.values) <= pos {
				data.values = append(data.values, Nil)
			}
			data.values[pos] = value
			return value, nil
		case *hashData:
			for i, entry := range data.entries {
				if valuesEqual(entry.key, index) {
					data.entries[i].value = value
					return value, nil
				}
			}
			data.entries = append(data.entries, hashEntry{key: index, value: value})
			return value, nil
		}
	}
	return Nil, fmt.Errorf("mrbvm: %s does not support index assignment", recv.Inspect())
}

func (s *State) kernelP(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	for _, arg := range args {
		fmt.Fprintln(s.stdout, arg.Inspect())
	}
	switch len(args) {
	case 0:
		return Nil, nil
	case 1:
		return args[0], nil
	default:
		return ArrayValue(args), nil
	}
}

func (s *State) arrayEachMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	for _, value := range data.values {
		_, err := s.callBlock(ctx, block, []Value{value})
		if err != nil {
			if br, ok := err.(*breakError); ok {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) procCallMethod(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	return s.callBlock(ctx, self, args)
}

func (s *State) callBlock(ctx context.Context, block Value, args []Value) (Value, error) {
	if block.Kind() != KindObject || block.obj == nil {
		return Nil, fmt.Errorf("mrbvm: no block given")
	}
	data, ok := block.obj.data.(*blockData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: value is not a block")
	}
	return s.executeFrame(ctx, data.irep, data.self, data.targetClass, args, Nil, data.env)
}

func (s *State) kernelPuts(ctx context.Context, self Value, args []Value, block Value) (Value, error) {
	for _, arg := range args {
		fmt.Fprintln(s.stdout, arg.ToS())
	}
	return Nil, nil
}
