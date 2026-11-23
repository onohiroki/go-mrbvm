package mrbvm

import "context"

// installEnumeratorLibrary provides the minimal Enumerator used by blockless
// collection methods. Enumeration is eager: next/peek materialize the source
// on first use, then serve items from the cached list.
func (s *State) installEnumeratorLibrary() {
	class := s.EnumeratorClass
	class.define("each", s.enumeratorEachMethod)
	class.define("next", s.enumeratorNextMethod)
	class.define("peek", s.enumeratorPeekMethod)
	class.define("rewind", s.enumeratorRewindMethod)
	class.define("size", s.enumeratorSizeMethod)
	class.define("with_index", s.enumeratorWithIndexMethod)
	class.define("each_with_index", s.enumeratorWithIndexMethod)
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue("#<Enumerator>"), nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue("#<Enumerator>"), nil
	})

	s.ObjectClass.define("to_enum", s.objectToEnumMethod)
	s.ObjectClass.define("enum_for", s.objectToEnumMethod)
}

func (s *State) newEnumerator(collect func(ctx context.Context, yield func([]Value) (Value, error)) error) Value {
	return ObjectValue(&Object{class: s.EnumeratorClass, data: &enumeratorData{collect: collect}})
}

func (s *State) toEnum(receiver Value, method string, args []Value) Value {
	// The enumerator outlives the call, so detach from the caller's argument
	// buffer (which may be pooled).
	argsCopy := append([]Value(nil), args...)
	var data *enumeratorData
	enumerator := s.newEnumerator(func(ctx context.Context, yield func([]Value) (Value, error)) error {
		block := nativeBlockValue(s.ProcClass, func(ctx context.Context, raw []Value) (Value, error) {
			return yield(raw)
		})
		result, err := s.call(ctx, receiver, method, argsCopy, Nil, block)
		if data != nil {
			data.result = result
		}
		return err
	})
	data, _ = enumeratorOf(enumerator)
	return enumerator
}

func enumeratorOf(value Value) (*enumeratorData, bool) {
	if value.Kind() == KindObject && value.obj != nil {
		data, ok := value.obj.data.(*enumeratorData)
		return data, ok
	}
	return nil, false
}

func (s *State) enumeratorItems(ctx context.Context, data *enumeratorData) error {
	if data.loaded {
		return nil
	}
	data.items = nil
	if err := data.collect(ctx, func(raw []Value) (Value, error) {
		copied := make([]Value, len(raw))
		copy(copied, raw)
		data.items = append(data.items, copied)
		return Nil, nil
	}); err != nil {
		return err
	}
	data.loaded = true
	return nil
}

func (s *State) objectToEnumMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "to_enum expects a method name")}
	}
	return s.toEnum(self, args[0].String(), args[1:]), nil
}

func (s *State) enumeratorEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if block.IsNil() {
		return self, nil
	}
	err := data.collect(ctx, func(raw []Value) (Value, error) {
		_, err := s.callBlock(ctx, block, raw, Nil)
		return Nil, err
	})
	if err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) enumeratorNextMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if err := s.enumeratorItems(ctx, data); err != nil {
		return Nil, err
	}
	if data.index >= len(data.items) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.StopIterationClass, "iteration reached an end")}
	}
	value := svalueOf(data.items[data.index])
	data.index++
	return value, nil
}

func (s *State) enumeratorPeekMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if err := s.enumeratorItems(ctx, data); err != nil {
		return Nil, err
	}
	if data.index >= len(data.items) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.StopIterationClass, "iteration reached an end")}
	}
	return svalueOf(data.items[data.index]), nil
}

func (s *State) enumeratorRewindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	data.index = 0
	return self, nil
}

func (s *State) enumeratorSizeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if err := s.enumeratorItems(ctx, data); err != nil {
		return Nil, err
	}
	return IntValue(int64(len(data.items))), nil
}

func (s *State) enumeratorWithIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	offset := 0
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
				"no implicit conversion into Integer")}
		}
		offset = int(args[0].Int())
	}
	if block.IsNil() {
		return s.newEnumerator(func(ctx context.Context, yield func([]Value) (Value, error)) error {
			index := offset
			return data.collect(ctx, func(raw []Value) (Value, error) {
				combined := append(append([]Value(nil), raw...), IntValue(int64(index)))
				index++
				return yield(combined)
			})
		}), nil
	}
	index := offset
	err := data.collect(ctx, func(raw []Value) (Value, error) {
		combined := append(append([]Value(nil), raw...), IntValue(int64(index)))
		index++
		return s.callBlock(ctx, block, combined, Nil)
	})
	if err != nil {
		return Nil, err
	}
	return data.result, nil
}
