package mrbvm

import (
	"context"
	"fmt"

	"github.com/onohiroki/go-mrbc/bytecode"
)

// installProcLibrary adds the Proc extension methods that mruby provides in
// mruby-proc-ext. The methods are implemented natively so that the VM core
// remains bytecode-only.
func (s *State) installProcLibrary() {
	class := s.ProcClass
	class.define("lambda?", s.procLambdaMethod)
	class.define("===", s.procCallMethod)
	class.define("yield", s.procCallMethod)
	class.define("to_proc", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("source_location", s.procSourceLocationMethod)
	class.define("inspect", s.procInspectMethod)
	class.define("to_s", s.procInspectMethod)
	class.define("parameters", s.procParametersMethod)
	class.define("curry", s.procCurryMethod)
	class.define("<<", s.procComposeMethod(false))
	class.define(">>", s.procComposeMethod(true))
}

func procNativeBlockData(value Value) (*nativeBlockData, bool) {
	if value.Kind() == KindObject && value.obj != nil {
		data, ok := value.obj.data.(*nativeBlockData)
		return data, ok
	}
	return nil, false
}

func (s *State) procLambdaMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if data, ok := procBlockData(self); ok {
		return BoolValue(data.strict), nil
	}
	if data, ok := procNativeBlockData(self); ok {
		return BoolValue(data.strict), nil
	}
	return False, nil
}

func (s *State) procSourceLocationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := procBlockData(self)
	if !ok || data.irep == nil || data.irep.Filename == "" || len(data.irep.Lines) == 0 {
		return Nil, nil
	}
	return ArrayValue([]Value{StringValue(data.irep.Filename), IntValue(int64(data.irep.Lines[0].Line))}), nil
}

func (s *State) procInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if data, ok := procBlockData(self); ok {
		text := "#<Proc:" + fmt.Sprintf("%p", data)
		if data.irep != nil && data.irep.Filename != "" && len(data.irep.Lines) > 0 {
			text += " " + data.irep.Filename + ":" + fmt.Sprint(data.irep.Lines[0].Line)
		}
		if data.strict {
			text += " (lambda)"
		}
		return StringValue(text + ">"), nil
	}
	if data, ok := procNativeBlockData(self); ok {
		text := "#<Proc:" + fmt.Sprintf("%p", data)
		if data.strict {
			text += " (lambda)"
		}
		return StringValue(text + ">"), nil
	}
	return StringValue(self.Inspect()), nil
}

type procParameter struct {
	kind string
	name string
}

// procParameters decodes the ENTER aspec and the local variable names of an
// irep. The slot layout matches the compiler: positional parameters come
// first, then the keyword dictionary, the internal block slot, the named
// block slot and finally the keyword parameters.
func procParameters(data *blockData) []procParameter {
	if data == nil || data.irep == nil || len(data.irep.Code) == 0 {
		return nil
	}
	first, err := bytecode.DecodeInstruction(data.irep.Code, 0)
	if err != nil || first.Op != bytecode.OpENTER || len(first.Operands) == 0 {
		return nil
	}
	aspec := first.Operands[0]
	req := int((aspec >> 18) & 0x1f)
	opt := int((aspec >> 13) & 0x1f)
	rest := (aspec>>12)&1 != 0
	post := int((aspec >> 7) & 0x1f)
	key := int((aspec >> 2) & 0x1f)
	kdict := (aspec>>1)&1 != 0
	hasBlock := aspec&1 != 0

	names := data.irep.LocalNames
	nameAt := func(index int) string {
		slot := index + 1
		if slot >= 0 && slot < len(names) {
			return names[slot]
		}
		return ""
	}

	var parameters []procParameter
	positionalKind := "req"
	if !data.strict {
		positionalKind = "opt"
	}
	index := 0
	for i := 0; i < req; i++ {
		parameters = append(parameters, procParameter{kind: positionalKind, name: nameAt(index)})
		index++
	}
	for i := 0; i < opt; i++ {
		parameters = append(parameters, procParameter{kind: "opt", name: nameAt(index)})
		index++
	}
	if rest {
		parameters = append(parameters, procParameter{kind: "rest", name: nameAt(index)})
		index++
	}
	for i := 0; i < post; i++ {
		parameters = append(parameters, procParameter{kind: positionalKind, name: nameAt(index)})
		index++
	}
	hasKeywords := key > 0 || kdict
	var keyRestParameter *procParameter
	if hasKeywords {
		if kdict {
			parameter := procParameter{kind: "keyrest", name: nameAt(index)}
			keyRestParameter = &parameter
		}
		index++
	}
	var blockParameter *procParameter
	if hasBlock {
		parameter := procParameter{kind: "block", name: nameAt(index + 1)}
		blockParameter = &parameter
	}
	// The internal block slot is present for every scope.
	index++
	if hasBlock {
		index++
	}
	for i := 0; i < key; i++ {
		parameters = append(parameters, procParameter{kind: "key", name: nameAt(index)})
		index++
	}
	if keyRestParameter != nil {
		parameters = append(parameters, *keyRestParameter)
	}
	if blockParameter != nil {
		parameters = append(parameters, *blockParameter)
	}
	return parameters
}

func (s *State) procParametersMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := procBlockData(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	parameters := procParameters(data)
	values := make([]Value, len(parameters))
	for i, parameter := range parameters {
		values[i] = ArrayValue([]Value{SymbolValue(parameter.kind), SymbolValue(parameter.name)})
	}
	return ArrayValue(values), nil
}

func procAbsArity(arity int64) int64 {
	if arity < 0 {
		return -arity - 1
	}
	return arity
}

func (s *State) procCurryMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	data, ok := procBlockData(self)
	if !ok {
		if _, native := procNativeBlockData(self); native {
			return self, nil
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a proc")}
	}
	arity := procArity(data)
	if len(args) == 1 {
		arity = args[0].Int()
	}
	required := procAbsArity(arity)
	if data.strict {
		selfArity := procArity(data)
		minRequired := procAbsArity(selfArity)
		if selfArity < 0 {
			maxArity := int64(0)
			hasRest := false
			for _, parameter := range procParameters(data) {
				switch parameter.kind {
				case "rest", "keyrest":
					hasRest = true
				case "req", "opt":
					maxArity++
				}
			}
			if required < minRequired || (!hasRest && required > maxArity) {
				expected := fmt.Sprint(minRequired)
				if !hasRest && maxArity != minRequired {
					expected = fmt.Sprintf("%d..%d", minRequired, maxArity)
				}
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected %s)", required, expected))}
			}
		} else if required != selfArity {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected %d)", required, selfArity))}
		}
	}

	var makeCurry func(given []Value) (Value, error)
	makeCurry = func(given []Value) (Value, error) {
		value := nativeBlockValue(s.ProcClass, func(ctx context.Context, callArgs []Value) (Value, error) {
			merged := make([]Value, 0, len(given)+len(callArgs))
			merged = append(merged, given...)
			merged = append(merged, callArgs...)
			if int64(len(merged)) >= required {
				return s.procCallMethod(ctx, self, merged, Nil, Nil)
			}
			return makeCurry(merged)
		})
		if native, ok := value.obj.data.(*nativeBlockData); ok {
			native.strict = data.strict
			native.arity = -1
			native.hasArity = true
		}
		return value, nil
	}
	return makeCurry(nil)
}

func (s *State) procComposeMethod(reverse bool) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		other := args[0]
		value := nativeBlockValue(s.ProcClass, func(ctx context.Context, callArgs []Value) (Value, error) {
			first, second := other, self
			if reverse {
				first, second = self, other
			}
			result, err := s.call(ctx, first, "call", callArgs, Nil, Nil)
			if err != nil {
				return Nil, err
			}
			return s.call(ctx, second, "call", []Value{result}, Nil, Nil)
		})
		if native, ok := value.obj.data.(*nativeBlockData); ok {
			native.strict = true
			native.arity = -1
			native.hasArity = true
		}
		return value, nil
	}
}
