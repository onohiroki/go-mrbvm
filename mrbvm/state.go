package mrbvm

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// Options controls VM construction.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
}

// State is one mruby VM instance. A State is owned by a single goroutine.
type State struct {
	ObjectClass     *Class
	ClassClass      *Class
	NumericClass    *Class
	NilClass        *Class
	TrueClass       *Class
	FalseClass      *Class
	IntegerClass    *Class
	FloatClass      *Class
	StringClass     *Class
	SymbolClass     *Class
	ArrayClass      *Class
	HashClass       *Class
	RangeClass      *Class
	ProcClass       *Class
	EnumerableClass *Class
	EnumeratorClass *Class
	ComparableClass *Class

	ExceptionClass              *Class
	StandardErrorClass          *Class
	RuntimeErrorClass           *Class
	ArgumentErrorClass          *Class
	TypeErrorClass              *Class
	NameErrorClass              *Class
	NoMethodErrorClass          *Class
	ZeroDivisionErrorClass      *Class
	LocalJumpErrorClass         *Class
	NoMatchingPatternErrorClass *Class
	IndexErrorClass             *Class
	StopIterationClass          *Class
	KeyErrorClass               *Class
	NotImplementedErrorClass    *Class
	RangeErrorClass             *Class

	TopSelf Value

	globals          map[string]Value
	constants        map[string]Value
	stdout           io.Writer
	stderr           io.Writer
	currentException Value

	arrayIndexBuiltin *method
	arraySetBuiltin   *method
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
	s.ClassClass = newClass("Class", s.ObjectClass)
	s.NumericClass = newClass("Numeric", s.ObjectClass)
	s.NilClass = newClass("NilClass", s.ObjectClass)
	s.TrueClass = newClass("TrueClass", s.ObjectClass)
	s.FalseClass = newClass("FalseClass", s.ObjectClass)
	s.IntegerClass = newClass("Integer", s.NumericClass)
	s.FloatClass = newClass("Float", s.NumericClass)
	s.StringClass = newClass("String", s.ObjectClass)
	s.SymbolClass = newClass("Symbol", s.ObjectClass)
	s.ArrayClass = newClass("Array", s.ObjectClass)
	s.HashClass = newClass("Hash", s.ObjectClass)
	s.RangeClass = newClass("Range", s.ObjectClass)
	s.ProcClass = newClass("Proc", s.ObjectClass)
	s.EnumerableClass = newClass("Enumerable", nil)
	s.EnumeratorClass = newClass("Enumerator", s.ObjectClass)
	s.ComparableClass = newClass("Comparable", nil)
	s.ExceptionClass = newClass("Exception", s.ObjectClass)
	s.StandardErrorClass = newClass("StandardError", s.ExceptionClass)
	s.RuntimeErrorClass = newClass("RuntimeError", s.StandardErrorClass)
	s.ArgumentErrorClass = newClass("ArgumentError", s.StandardErrorClass)
	s.TypeErrorClass = newClass("TypeError", s.StandardErrorClass)
	s.NameErrorClass = newClass("NameError", s.StandardErrorClass)
	s.NoMethodErrorClass = newClass("NoMethodError", s.NameErrorClass)
	s.ZeroDivisionErrorClass = newClass("ZeroDivisionError", s.StandardErrorClass)
	s.LocalJumpErrorClass = newClass("LocalJumpError", s.StandardErrorClass)
	s.NoMatchingPatternErrorClass = newClass("NoMatchingPatternError", s.StandardErrorClass)
	s.IndexErrorClass = newClass("IndexError", s.StandardErrorClass)
	s.StopIterationClass = newClass("StopIteration", s.IndexErrorClass)
	s.KeyErrorClass = newClass("KeyError", s.IndexErrorClass)
	s.NotImplementedErrorClass = newClass("NotImplementedError", s.StandardErrorClass)
	s.RangeErrorClass = newClass("RangeError", s.StandardErrorClass)

	for _, class := range []*Class{
		s.ObjectClass, s.ClassClass, s.NumericClass, s.NilClass,
		s.TrueClass, s.FalseClass, s.IntegerClass, s.FloatClass,
		s.StringClass, s.SymbolClass, s.ArrayClass, s.HashClass,
		s.RangeClass, s.ProcClass, s.EnumerableClass, s.EnumeratorClass,
		s.ComparableClass,
		s.ExceptionClass, s.StandardErrorClass, s.RuntimeErrorClass,
		s.ArgumentErrorClass, s.TypeErrorClass, s.NameErrorClass,
		s.NoMethodErrorClass, s.ZeroDivisionErrorClass, s.LocalJumpErrorClass,
		s.NoMatchingPatternErrorClass,
		s.IndexErrorClass, s.StopIterationClass, s.KeyErrorClass,
		s.NotImplementedErrorClass, s.RangeErrorClass,
	} {
		s.ensureClassObject(class)
	}

	s.ClassClass.define("new", s.classNewMethod)
	s.ClassClass.define("superclass", s.classSuperclassMethod)
	s.ClassClass.define("name", s.classNameMethod)
	s.ClassClass.define("include", s.classIncludeMethod)
	s.ClassClass.define("include?", s.classIncludePredicateMethod)
	s.ClassClass.define("class_eval", s.classEvalMethod)
	s.ClassClass.define("module_eval", s.classEvalMethod)
	s.ClassClass.define("class_exec", s.classEvalMethod)
	s.ClassClass.define("alias_method", s.classAliasMethod)
	s.ClassClass.define("remove_method", s.classRemoveMethod)
	s.ClassClass.define("===", s.classCaseEqMethod)
	s.ClassClass.define("attr_reader", s.classAttrReaderMethod)
	s.ClassClass.define("attr", s.classAttrReaderMethod)
	s.ClassClass.define("attr_writer", s.classAttrWriterMethod)
	s.ClassClass.define("attr_accessor", s.classAttrAccessorMethod)
	s.ClassClass.define("const_defined?", s.classConstDefinedMethod)
	s.ClassClass.define("method_defined?", s.classMethodDefinedMethod)
	s.ObjectClass.define("class", s.objectClassMethod)
	s.ObjectClass.define("initialize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, nil
	})

	for name, class := range map[string]*Class{
		"Object": s.ObjectClass, "Class": s.ClassClass, "Numeric": s.NumericClass,
		"NilClass": s.NilClass, "TrueClass": s.TrueClass, "FalseClass": s.FalseClass,
		"Integer": s.IntegerClass,
		"Float":   s.FloatClass, "String": s.StringClass, "Symbol": s.SymbolClass,
		"Array": s.ArrayClass, "Hash": s.HashClass, "Range": s.RangeClass,
		"Proc": s.ProcClass, "Enumerable": s.EnumerableClass,
		"Enumerator":    s.EnumeratorClass,
		"Comparable":    s.ComparableClass,
		"Exception":     s.ExceptionClass,
		"StandardError": s.StandardErrorClass, "RuntimeError": s.RuntimeErrorClass,
		"ArgumentError": s.ArgumentErrorClass, "TypeError": s.TypeErrorClass,
		"NameError": s.NameErrorClass, "NoMethodError": s.NoMethodErrorClass,
		"ZeroDivisionError":      s.ZeroDivisionErrorClass,
		"LocalJumpError":         s.LocalJumpErrorClass,
		"NoMatchingPatternError": s.NoMatchingPatternErrorClass,
		"IndexError":             s.IndexErrorClass,
		"StopIteration":          s.StopIterationClass,
		"KeyError":               s.KeyErrorClass,
		"NotImplementedError":    s.NotImplementedErrorClass,
		"RangeError":             s.RangeErrorClass,
	} {
		value := s.classValue(class)
		s.constants[name] = value
		s.ObjectClass.constants[name] = value
	}

	s.FloatClass.constants["INFINITY"] = FloatValue(math.Inf(1))
	s.FloatClass.constants["NAN"] = FloatValue(math.NaN())
	s.TopSelf = ObjectValue(&Object{class: s.ObjectClass})
	s.ObjectClass.define("puts", s.kernelPuts)
	s.ObjectClass.define("print", s.kernelPrint)
	s.ObjectClass.define("p", s.kernelP)
	s.ObjectClass.define("raise", s.kernelRaise)
	s.ObjectClass.define("fail", s.kernelRaise)
	s.ExceptionClass.define("message", s.exceptionMessageMethod)
	s.ExceptionClass.define("to_s", s.exceptionMessageMethod)
	s.ExceptionClass.define("inspect", s.exceptionInspectMethod)
	s.ExceptionClass.define("backtrace", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return ArrayValue(nil), nil
	})
	s.ExceptionClass.define("cause", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, nil
	})

	s.ArrayClass.define("[]", s.arrayIndexMethod)
	s.arrayIndexBuiltin = s.ArrayClass.methods["[]"]
	s.ArrayClass.define("+", s.arrayPlusMethod)
	s.ArrayClass.define("*", s.arrayRepeatMethod)
	s.ArrayClass.classMethods["[]"] = &method{owner: s.ArrayClass, name: "[]", native: s.arrayClassBracketMethod}
	s.ArrayClass.define("length", s.arrayLengthMethod)
	s.ArrayClass.define("size", s.arrayLengthMethod)
	s.ArrayClass.define("to_a", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.HashClass.define("[]", s.hashIndexMethod)
	s.HashClass.define("length", s.hashLengthMethod)
	s.HashClass.define("size", s.hashLengthMethod)
	s.HashClass.define("deconstruct_keys", s.hashDeconstructKeysMethod)
	s.HashClass.define("__pat_values", s.hashPatValuesMethod)
	s.HashClass.define("__except", s.hashExceptMethod)
	s.RangeClass.define("to_a", s.rangeToAMethod)
	s.RangeClass.define("===", s.rangeCaseEqMethod)
	s.RangeClass.define("include?", s.rangeCaseEqMethod)
	s.RangeClass.define("each", s.rangeEachMethod)
	s.StringClass.define("length", s.stringLengthMethod)
	s.StringClass.define("upcase", s.stringUpcaseMethod)
	s.StringClass.define("downcase", s.stringDowncaseMethod)
	s.ArrayClass.define("each", s.arrayEachMethod)
	s.ArrayClass.define("deconstruct", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.ArrayClass.define("map", s.arrayMapMethod)
	s.ArrayClass.define("sort", s.arraySortMethod)
	s.ArrayClass.define("sort!", s.arraySortBangMethod)
	s.ArrayClass.define("__case_eqq", s.arrayCaseEqMethod)
	s.ArrayClass.define("__svalue", s.arraySValueMethod)
	s.ArrayClass.define("__svalue_eq", s.arraySValueEqMethod)
	s.ArrayClass.define("<<", s.arrayPushMethod)
	s.ArrayClass.define("push", s.arrayPushMethod)
	s.ArrayClass.define("[]=", s.arraySetMethod)
	s.arraySetBuiltin = s.ArrayClass.methods["[]="]
	s.HashClass.define("[]=", s.hashSetMethod)
	s.ProcClass.define("call", s.procCallMethod)
	s.ObjectClass.define("respond_to?", s.objectRespondToMethod)
	s.ObjectClass.define("dup", s.objectDupMethod)
	s.ObjectClass.define("clone", s.objectDupMethod)
	s.ObjectClass.define("proc", s.kernelProcMethod)
	s.ObjectClass.define("lambda", s.kernelProcMethod)

	s.installEnumerableLibrary()
	s.installEnumeratorLibrary()
	s.installComparableLibrary()
	includeModule(s.NumericClass, s.ComparableClass)
	includeModule(s.StringClass, s.ComparableClass)
	includeModule(s.EnumeratorClass, s.EnumerableClass)
	for _, class := range []*Class{s.ArrayClass, s.HashClass, s.RangeClass} {
		includeModule(class, s.EnumerableClass)
	}
	s.installImmediateLibrary()
	s.installCoreLibrary()
	return s
}

func (s *State) classConstDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	name := args[0].String()
	for current := class; current != nil; current = current.super {
		if _, ok := current.constants[name]; ok {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) classAttrReaderMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: attr_reader receiver is not a class")
	}
	for _, arg := range args {
		name := arg.String()
		ivar := "@" + strings.TrimPrefix(name, "@")
		class.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if self.obj == nil {
				return Nil, nil
			}
			return self.obj.ivar(ivar), nil
		})
	}
	return Nil, nil
}

func (s *State) classAttrWriterMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: attr_writer receiver is not a class")
	}
	for _, arg := range args {
		name := arg.String()
		ivar := "@" + strings.TrimPrefix(name, "@")
		setter := name + "="
		class.define(setter, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if len(args) != 1 {
				return Nil, fmt.Errorf("mrbvm: %s expects one argument", setter)
			}
			if self.obj != nil {
				self.obj.setIvar(ivar, args[0])
			}
			return args[0], nil
		})
	}
	return Nil, nil
}

func (s *State) classAttrAccessorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, err := s.classAttrReaderMethod(ctx, self, args, kw, block); err != nil {
		return Nil, err
	}
	return s.classAttrWriterMethod(ctx, self, args, kw, block)
}

func includeModule(class, module *Class) {
	for name, method := range module.methods {
		if _, exists := class.methods[name]; !exists {
			class.methods[name] = method
		}
	}
	for name, value := range module.constants {
		if _, exists := class.constants[name]; !exists {
			class.constants[name] = value
		}
	}
	found := false
	for _, included := range class.included {
		if included == module {
			found = true
			break
		}
	}
	if !found {
		class.included = append(class.included, module)
	}
}

func (s *State) classMethodDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	_, found := class.lookup(args[0].String())
	return BoolValue(found), nil
}

func (s *State) classIncludePredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	module, ok := s.asClass(args[0])
	if !ok {
		return False, nil
	}
	for current := class; current != nil; current = current.super {
		for _, included := range current.included {
			if included == module {
				return True, nil
			}
		}
	}
	return False, nil
}

func (s *State) arrayClassBracketMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := append([]Value(nil), args...)
	class, ok := s.asClass(self)
	if ok && class != s.ArrayClass {
		return ObjectValue(&Object{class: class, data: &arrayData{values: values}}), nil
	}
	return ArrayValue(values), nil
}

func (s *State) arrayPlusMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Array#+ expects one argument")}
	}
	values := append([]Value(nil), arrayValues(self)...)
	values = append(values, arrayValues(args[0])...)
	return ArrayValue(values), nil
}

func (s *State) arrayRepeatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Array#* expects one argument")}
	}
	values := arrayValues(self)
	if args[0].Kind() == KindInt {
		count := args[0].Int()
		if count < 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative argument")}
		}
		result := make([]Value, 0, len(values)*int(count))
		for i := int64(0); i < count; i++ {
			result = append(result, values...)
		}
		return ArrayValue(result), nil
	}
	joined, err := s.joinArrayValues(values, args[0].ToS(), map[*arrayData]bool{})
	if err != nil {
		return Nil, err
	}
	return StringValue(joined), nil
}

func (s *State) joinArrayValues(values []Value, separator string, seen map[*arrayData]bool) (string, error) {
	var parts []string
	for _, value := range values {
		if value.obj != nil {
			if data, ok := value.obj.data.(*arrayData); ok {
				if seen[data] {
					return "", &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "recursive array join")}
				}
				seen[data] = true
				joined, err := s.joinArrayValues(data.values, separator, seen)
				delete(seen, data)
				if err != nil {
					return "", err
				}
				parts = append(parts, joined)
				continue
			}
		}
		parts = append(parts, value.ToS())
	}
	return strings.Join(parts, separator), nil
}

func (s *State) classNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#new receiver is not a class")
	}
	if class == s.ClassClass {
		super := s.ObjectClass
		if len(args) > 0 {
			if parent, ok := s.asClass(args[0]); ok {
				super = parent
			}
		}
		created := newClass("#<Class>", super)
		s.ensureClassObject(created)
		createdValue := s.classValue(created)
		if !block.IsNil() {
			if _, err := s.callBlockWithSelf(ctx, block, createdValue, nil, Nil); err != nil {
				return Nil, err
			}
		}
		return createdValue, nil
	}

	var instance Value
	switch {
	case s.classIsSubclass(class, s.ArrayClass):
		values := []Value(nil)
		if len(args) == 1 {
			if args[0].Kind() == KindInt {
				count := args[0].Int()
				if count < 0 {
					return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
				}
				values = make([]Value, count)
				if !block.IsNil() {
					for i := range values {
						item, err := s.callBlock(ctx, block, []Value{IntValue(int64(i))}, Nil)
						if err != nil {
							return Nil, err
						}
						values[i] = item
					}
				}
			} else if argValues := arrayValues(args[0]); args[0].obj != nil {
				if _, ok := args[0].obj.data.(*arrayData); ok {
					values = append([]Value(nil), argValues...)
				}
			}
		} else if len(args) >= 2 && args[0].Kind() == KindInt {
			count := args[0].Int()
			if count < 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
			}
			values = make([]Value, count)
			for i := range values {
				values[i] = args[1]
			}
		}
		instance = ObjectValue(&Object{class: class, data: &arrayData{values: values}})
	case s.classIsSubclass(class, s.HashClass):
		instance = ObjectValue(&Object{class: class, data: &hashData{}})
	case s.classIsSubclass(class, s.StringClass):
		value := ""
		if len(args) > 0 {
			value = args[0].ToS()
		}
		instance = ObjectValue(&Object{class: class, data: &stringData{value: value}})
	case s.classIsSubclass(class, s.ExceptionClass) && len(args) > 0:
		instance = newException(class, args[0].ToS())
	default:
		instance = ObjectValue(&Object{class: class})
	}
	if m, ok := class.lookup("initialize"); ok {
		if _, err := s.invoke(ctx, m, instance, args, kw, block); err != nil {
			return Nil, err
		}
	}
	return instance, nil
}

func (s *State) classEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return self, nil
	}
	return s.callBlockWithSelf(ctx, block, self, nil, Nil)
}

func (s *State) classAliasMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "alias_method expects two names")}
	}
	newName := args[0].String()
	oldName := args[1].String()
	m, ok := class.lookup(oldName)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+oldName+"'")}
	}
	class.methods[newName] = m
	return SymbolValue(newName), nil
}

func (s *State) classRemoveMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "remove_method expects one name")}
	}
	name := args[0].String()
	if _, ok := class.methods[name]; !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "method '"+name+"' not defined in "+class.name)}
	}
	delete(class.methods, name)
	return self, nil
}

func (s *State) classIsSubclass(class, super *Class) bool {
	for current := class; current != nil; current = current.super {
		if current == super {
			return true
		}
	}
	return false
}

func (s *State) classSuperclassMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#superclass receiver is not a class")
	}
	if class.super == nil {
		return Nil, nil
	}
	return s.classValue(class.super), nil
}

func (s *State) classNameMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#name receiver is not a class")
	}
	return StringValue(class.name), nil
}

func (s *State) classCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	return BoolValue(s.isA(args[0], class)), nil
}

func (s *State) isA(value Value, class *Class) bool {
	for current := s.classOf(value); current != nil; current = current.super {
		if current == class {
			return true
		}
	}
	return false
}

func (s *State) classIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#include receiver is not a class")
	}
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: include expects one module")
	}
	module, ok := s.asClass(args[0])
	if !ok {
		return Nil, fmt.Errorf("mrbvm: include expects a module")
	}
	for name, m := range module.methods {
		if _, exists := class.methods[name]; !exists {
			class.methods[name] = m
		}
	}
	found := false
	for _, included := range class.included {
		if included == module {
			found = true
			break
		}
	}
	if !found {
		class.included = append(class.included, module)
	}
	for name, value := range module.constants {
		if _, exists := class.constants[name]; !exists {
			class.constants[name] = value
		}
	}
	return self, nil
}

func (s *State) objectClassMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.classValue(s.classOf(self)), nil
}

func (s *State) index(ctx context.Context, recv, index Value) (Value, error) {
	if recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					return sliceArray(data.values, rangeIndex), nil
				}
			}
			pos, err := s.indexInteger(index)
			if err != nil {
				return Nil, err
			}
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
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					return sliceString(data.value, rangeIndex), nil
				}
			}
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

func sliceArray(values []Value, data *rangeData) Value {
	length := len(values)
	low := 0
	high := length
	if !data.low.IsNil() {
		if data.low.Kind() == KindInt {
			low = int(data.low.Int())
			if low < 0 {
				low += length
			}
		}
	}
	if !data.high.IsNil() {
		if data.high.Kind() == KindInt {
			high = int(data.high.Int())
			if high < 0 {
				high += length
			}
			if data.exclusive {
				high--
			}
		}
	}
	if low < 0 {
		low = 0
	}
	if high >= length {
		high = length - 1
	}
	if high < low {
		return ArrayValue(nil)
	}
	result := make([]Value, high-low+1)
	copy(result, values[low:high+1])
	return ArrayValue(result)
}

func (s *State) indexInteger(index Value) (int, error) {
	switch index.Kind() {
	case KindInt:
		return int(index.Int()), nil
	case KindFloat:
		f := index.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || f >= math.MaxInt64 || f <= math.MinInt64 {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "float out of range")}
		}
		return int(f), nil
	default:
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "index must be an integer")}
	}
}

func (s *State) arrayIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	switch len(args) {
	case 0:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments (given 0, expected 1..2)")}
	case 1:
		return s.index(ctx, self, args[0])
	case 2:
		start, err := s.indexInteger(args[0])
		if err != nil {
			return Nil, err
		}
		length, err := s.indexInteger(args[1])
		if err != nil {
			return Nil, err
		}
		return s.arraySliceAt(self, start, length)
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))}
	}
}

func (s *State) arraySliceAt(self Value, start, length int) (Value, error) {
	values := arrayValues(self)
	if length < 0 {
		return Nil, nil
	}
	if start < 0 {
		start += len(values)
	}
	if start < 0 || start > len(values) {
		return Nil, nil
	}
	end := start + length
	if end > len(values) {
		end = len(values)
	}
	if end < start {
		end = start
	}
	return ArrayValue(append([]Value(nil), values[start:end]...)), nil
}

func (s *State) hashIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: Hash#[] expects one argument")
	}
	return s.index(ctx, self, args[0])
}

func (s *State) arrayLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	return IntValue(int64(len(data.values))), nil
}

func (s *State) hashLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	return IntValue(int64(len(data.entries))), nil
}

func (s *State) stringLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return IntValue(int64(len([]rune(self.String())))), nil
}

func hashHasKey(hash Value, name string) bool {
	if hash.Kind() != KindObject || hash.obj == nil {
		return false
	}
	data, ok := hash.obj.data.(*hashData)
	if !ok {
		return false
	}
	key := SymbolValue(name)
	for _, entry := range data.entries {
		if valuesEqual(entry.key, key) {
			return true
		}
	}
	return false
}

func hashDeleteKey(hash Value, name string) (Value, bool) {
	if hash.Kind() != KindObject || hash.obj == nil {
		return Nil, false
	}
	data, ok := hash.obj.data.(*hashData)
	if !ok {
		return Nil, false
	}
	key := SymbolValue(name)
	for i, entry := range data.entries {
		if valuesEqual(entry.key, key) {
			data.entries = append(data.entries[:i], data.entries[i+1:]...)
			return entry.value, true
		}
	}
	return Nil, false
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

func (s *State) hashDeconstructKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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

func (s *State) hashPatValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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

func (s *State) hashExceptMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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

func (s *State) rangeCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: Range#=== expects one argument")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if !data.low.IsNil() {
		if low, ok := binaryValue(args[0], ">=", data.low); !ok || !low.IsTruthy() {
			return False, nil
		}
	}
	if !data.high.IsNil() {
		op := "<="
		if data.exclusive {
			op = "<"
		}
		if high, ok := binaryValue(args[0], op, data.high); ok {
			return high, nil
		}
		return False, nil
	}
	return True, nil
}

func (s *State) stringUpcaseMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(strings.ToUpper(self.String())), nil
}

func (s *State) stringDowncaseMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(strings.ToLower(self.String())), nil
}

func (s *State) rangeToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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
	return s.executeFrame(ctx, irep, s.TopSelf, s.ObjectClass, nil, Nil, Nil, nil, "", nil, false)
}

func (s *State) setIndex(ctx context.Context, recv, index, value Value) (Value, error) {
	if recv.kind == KindObject && recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					start, length := rangeBounds(rangeIndex, len(data.values))
					return s.arrayAssign(data, start, length, value)
				}
			}
			pos, err := s.indexInteger(index)
			if err != nil {
				return Nil, err
			}
			if pos < 0 {
				pos += len(data.values)
			}
			if pos < 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, fmt.Sprintf("index %d too small for array", pos))}
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

func (s *State) kernelP(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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

func (s *State) rangeEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if data.low.Kind() != KindInt || data.high.Kind() != KindInt {
		return Nil, fmt.Errorf("mrbvm: only integer ranges are supported")
	}
	for i := data.low.Int(); i <= data.high.Int(); i++ {
		if data.exclusive && i == data.high.Int() {
			break
		}
		if _, err := s.callBlock(ctx, block, []Value{IntValue(i)}, Nil); err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arrayPushMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data.values = append(data.values, args...)
	return self, nil
}

func (s *State) arraySetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	switch len(args) {
	case 0:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments (given 0, expected 2..3)")}
	case 2:
		if rangeData, ok := rangeDataOf(args[0]); ok {
			start, length := rangeBounds(rangeData, len(data.values))
			return s.arrayAssign(data, start, length, args[1])
		}
		return s.setIndex(ctx, self, args[0], args[1])
	case 3:
		start, err := s.indexInteger(args[0])
		if err != nil {
			return Nil, err
		}
		length, err := s.indexInteger(args[1])
		if err != nil {
			return Nil, err
		}
		return s.arrayAssign(data, start, length, args[2])
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected 2..3)", len(args)))}
	}
}

func arrayDataOf(value Value) (*arrayData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*arrayData)
	return data, ok
}

func rangeBounds(data *rangeData, length int) (int, int) {
	start := 0
	end := length - 1
	if !data.low.IsNil() && data.low.Kind() == KindInt {
		start = int(data.low.Int())
		if start < 0 {
			start += length
		}
	}
	if !data.high.IsNil() && data.high.Kind() == KindInt {
		end = int(data.high.Int())
		if end < 0 {
			end += length
		}
		if data.exclusive {
			end--
		}
	}
	if end >= length {
		end = length - 1
	}
	sliceLength := end - start + 1
	if sliceLength < 0 {
		sliceLength = 0
	}
	return start, sliceLength
}

func (s *State) arrayAssign(data *arrayData, start, length int, value Value) (Value, error) {
	if length < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "negative length")}
	}
	if start < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index too small for array")}
	}
	var replacement []Value
	if replacementData, ok := arrayDataOf(value); ok {
		replacement = append([]Value(nil), replacementData.values...)
	} else {
		replacement = []Value{value}
	}
	if start > len(data.values) {
		if start-len(data.values) > 1<<20 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index too big for array")}
		}
		for len(data.values) < start {
			data.values = append(data.values, Nil)
		}
	}
	end := start + length
	if end > len(data.values) {
		end = len(data.values)
	}
	if end < start {
		end = start
	}
	result := make([]Value, 0, len(data.values)-(end-start)+len(replacement))
	result = append(result, data.values[:start]...)
	result = append(result, replacement...)
	result = append(result, data.values[end:]...)
	data.values = result
	return value, nil
}

func (s *State) hashSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return Nil, fmt.Errorf("mrbvm: Hash#[]= expects two arguments")
	}
	return s.setIndex(ctx, self, args[0], args[1])
}

func (s *State) objectDupMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.Kind() != KindObject || self.obj == nil {
		return self, nil
	}
	switch data := self.obj.data.(type) {
	case *arrayData:
		values := make([]Value, len(data.values))
		copy(values, data.values)
		return ArrayValue(values), nil
	case *hashData:
		entries := make([]hashEntry, len(data.entries))
		copy(entries, data.entries)
		return HashValue(entries), nil
	case *stringData:
		return StringValue(data.value), nil
	case *exceptionData:
		return ObjectValue(&Object{class: self.obj.class, data: &exceptionData{message: data.message}, ivars: self.obj.ivars}), nil
	case *Class:
		return self, nil
	case *blockData:
		copyData := *data
		return ObjectValue(&Object{data: &copyData}), nil
	}
	ivars := make(map[string]Value, len(self.obj.ivars))
	for name, value := range self.obj.ivars {
		ivars[name] = value
	}
	return ObjectValue(&Object{class: self.obj.class, ivars: ivars}), nil
}

func (s *State) objectRespondToMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	name := args[0].String()
	if class, ok := s.asClass(self); ok {
		if _, ok := class.lookupClassMethod(name); ok {
			return True, nil
		}
	}
	if _, ok := s.classOf(self).lookup(name); ok {
		return True, nil
	}
	return False, nil
}

func (s *State) kernelProcMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, fmt.Errorf("mrbvm: tried to create Proc without a block")
	}
	return block, nil
}

func (s *State) kernelRaise(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	var class *Class
	var message string
	switch len(args) {
	case 0:
		if !s.currentException.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: s.currentException}
		}
		class = s.RuntimeErrorClass
		message = "RuntimeError"
	case 1:
		if argClass, ok := s.asClass(args[0]); ok {
			class = argClass
			message = argClass.name
		} else if args[0].Kind() == KindObject && args[0].obj != nil {
			if _, ok := args[0].obj.data.(*exceptionData); ok {
				return Nil, &unwindSignal{kind: unwindException, value: args[0]}
			}
			class = s.RuntimeErrorClass
			message = args[0].ToS()
		} else {
			class = s.RuntimeErrorClass
			message = args[0].ToS()
		}
	default:
		argClass, ok := s.asClass(args[0])
		if !ok {
			return Nil, fmt.Errorf("mrbvm: exception class or instance expected")
		}
		class = argClass
		message = args[1].ToS()
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(class, message)}
}

func (s *State) exceptionMessageMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(exceptionMessage(self)), nil
}

func (s *State) exceptionInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class := s.classOf(self)
	return StringValue("#<" + class.name + ": " + exceptionMessage(self) + ">"), nil
}

func (s *State) arraySortMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	values := make([]Value, len(data.values))
	copy(values, data.values)
	copyValue := ArrayValue(values)
	return s.arraySortBangMethod(ctx, copyValue, args, kw, block)
}

func (s *State) arraySortBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	for i := 1; i < len(data.values); i++ {
		for j := i; j > 0; j-- {
			cmp, err := s.compareValues(ctx, data.values[j-1], data.values[j], block)
			if err != nil {
				return Nil, err
			}
			if cmp <= 0 {
				break
			}
			data.values[j-1], data.values[j] = data.values[j], data.values[j-1]
		}
	}
	return self, nil
}

func (s *State) compareValues(ctx context.Context, left, right Value, block Value) (int, error) {
	if !block.IsNil() {
		result, err := s.callBlock(ctx, block, []Value{left, right}, Nil)
		if err != nil {
			return 0, err
		}
		if result.Kind() == KindInt {
			switch {
			case result.Int() < 0:
				return -1, nil
			case result.Int() > 0:
				return 1, nil
			default:
				return 0, nil
			}
		}
		return 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "comparison of values failed")}
	}
	result, err := s.call(ctx, left, "<=>", []Value{right}, Nil, Nil)
	if err != nil {
		return 0, err
	}
	if result.Kind() != KindInt {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "comparison failed")}
	}
	switch {
	case result.Int() < 0:
		return -1, nil
	case result.Int() > 0:
		return 1, nil
	default:
		return 0, nil
	}
}

func (s *State) arraySValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return self, nil
	}
	switch len(data.values) {
	case 0:
		return Nil, nil
	case 1:
		return data.values[0], nil
	default:
		return self, nil
	}
}

func (s *State) arraySValueEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	value, err := s.arraySValueMethod(ctx, self, nil, Nil, Nil)
	if err != nil {
		return Nil, err
	}
	return BoolValue(valuesEqual(value, args[0])), nil
}

func (s *State) arrayCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || self.obj == nil {
		return False, nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return False, nil
	}
	for _, value := range data.values {
		if class, ok := s.asClass(value); ok && s.isA(args[0], class) {
			return True, nil
		}
		if valuesEqual(value, args[0]) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "map", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	values := make([]Value, 0, len(data.values))
	for _, value := range data.values {
		result, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
		values = append(values, result)
	}
	return ArrayValue(values), nil
}

func (s *State) arrayEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	for _, value := range data.values {
		_, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) procCallMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.callBlock(ctx, self, args, kw)
}

func (s *State) callBlockWithSelf(ctx context.Context, block Value, self Value, args []Value, kw Value) (Value, error) {
	if block.Kind() == KindObject && block.obj != nil {
		if data, ok := block.obj.data.(*blockData); ok {
			copyData := *data
			copyData.self = self
			return s.executeFrame(ctx, copyData.irep, self, copyData.targetClass, args, kw, Nil, copyData.env, copyData.mid, copyData.owner, copyData.strict)
		}
	}
	return s.callBlock(ctx, block, args, kw)
}

func (s *State) callBlock(ctx context.Context, block Value, args []Value, kw Value) (Value, error) {
	if block.Kind() == KindSymbol {
		proc, err := s.call(ctx, block, "to_proc", nil, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		return s.callBlock(ctx, proc, args, kw)
	}
	if block.Kind() != KindObject || block.obj == nil {
		return Nil, fmt.Errorf("mrbvm: no block given")
	}
	if data, ok := block.obj.data.(*nativeBlockData); ok {
		return data.fn(ctx, args)
	}
	data, ok := block.obj.data.(*blockData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: value is not a block")
	}
	return s.executeFrame(ctx, data.irep, data.self, data.targetClass, args, kw, Nil, data.env, data.mid, data.owner, data.strict)
}

func (s *State) kernelPrint(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, arg := range args {
		fmt.Fprint(s.stdout, arg.ToS())
	}
	return Nil, nil
}

func (s *State) kernelPuts(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		fmt.Fprintln(s.stdout)
		return Nil, nil
	}
	for _, arg := range args {
		if arg.Kind() == KindObject && arg.obj != nil {
			if value, ok := arg.obj.data.(*arrayData); ok {
				for _, item := range value.values {
					fmt.Fprintln(s.stdout, item.ToS())
				}
				continue
			}
		}
		fmt.Fprintln(s.stdout, arg.ToS())
	}
	return Nil, nil
}

func (s *State) usesBuiltinMethod(recv Value, name string, builtin *method) bool {
	if builtin == nil {
		return false
	}
	m, ok := s.classOf(recv).lookup(name)
	return ok && m == builtin
}

func (s *State) indexOp(ctx context.Context, recv, index Value) (Value, error) {
	if s.usesBuiltinMethod(recv, "[]", s.arrayIndexBuiltin) {
		return s.index(ctx, recv, index)
	}
	return s.call(ctx, recv, "[]", []Value{index}, Nil, Nil)
}

func (s *State) setIndexOp(ctx context.Context, recv, index, value Value) (Value, error) {
	if s.usesBuiltinMethod(recv, "[]=", s.arraySetBuiltin) {
		return s.setIndex(ctx, recv, index, value)
	}
	return s.call(ctx, recv, "[]=", []Value{index, value}, Nil, Nil)
}
