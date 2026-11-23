package mrbvm

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (s *State) installEnumerableExtensionLibrary() {
	class := s.EnumerableClass
	class.define("each_slice", s.enumerableExtEachSliceMethod)
	class.define("each_cons", s.enumerableExtEachConsMethod)
	class.define("take", s.enumerableExtTakeMethod)
	class.define("drop", s.enumerableExtDropMethod)
	class.define("all?", s.enumerableExtAllMethod)
	class.define("any?", s.enumerableExtAnyMethod)
	class.define("none?", s.enumerableExtNoneMethod)
	class.define("one?", s.enumerableExtOneMethod)
	class.define("count", s.enumerableExtCountMethod)
	class.define("minmax_by", s.enumerableExtMinmaxByMethod)
	class.define("reverse_each", s.enumerableExtReverseEachMethod)
	class.define("cycle", s.enumerableExtCycleMethod)
	class.define("find_index", s.enumerableExtFindIndexMethod)
	class.define("to_h", s.enumerableExtToHMethod)
	class.define("filter_map", s.enumerableExtFilterMapMethod)
	class.define("tally", s.enumerableExtTallyMethod)
	class.define("grep_v", s.enumerableExtGrepVMethod)
	class.define("each_entry", s.enumerableExtEachEntryMethod)
	class.define("zip", s.enumerableExtZipMethod)
	class.define("chunk", s.enumerableExtChunkMethod)
	class.define("chunk_while", s.enumerableExtChunkWhileMethod)

	array := s.ArrayClass
	array.define("each_slice", s.enumerableExtEachSliceMethod)
	array.define("each_cons", s.enumerableExtEachConsMethod)
	array.define("all?", s.enumerableExtAllMethod)
	array.define("any?", s.enumerableExtAnyMethod)
	array.define("none?", s.enumerableExtNoneMethod)
	array.define("one?", s.enumerableExtOneMethod)
	array.define("count", s.arrayExtCountMethod)
	array.define("sort_by!", s.arrayExtSortByBangMethod)
	array.define("zip", s.arrayExtZipMethod)
	array.define("chunk", s.enumerableExtChunkMethod)
	array.define("chunk_while", s.enumerableExtChunkWhileMethod)

	// The built-in classes copy Enumerable's methods when they include it, so
	// the methods defined above have to be placed on them explicitly.
	names := []string{"each_slice", "each_cons", "take", "drop", "none?", "one?",
		"minmax_by", "reverse_each", "cycle", "find_index", "filter_map", "tally",
		"grep_v", "each_entry", "zip"}
	s.ObjectClass.define("loop", s.kernelLoopMethod)
	for _, name := range names {
		if m, ok := class.methods[name]; ok {
			s.RangeClass.methods[name] = m
			s.EnumeratorClass.methods[name] = m
			if _, exists := s.HashClass.methods[name]; !exists {
				s.HashClass.methods[name] = m
			}
		}
	}
	for _, name := range []string{"count", "all?", "any?"} {
		if m, ok := class.methods[name]; ok {
			s.RangeClass.methods[name] = m
		}
	}
}

func (s *State) enumStrictInt(args []Value, index int) (int, error) {
	if index >= len(args) {
		return 0, argumentCountError(s, len(args), "1")
	}
	if args[index].Kind() != KindInt {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
			fmt.Sprintf("no implicit conversion of %s into Integer", s.classOf(args[index]).name))}
	}
	return int(args[index].Int()), nil
}

func (s *State) enumCollectValues(ctx context.Context, self Value) ([]Value, error) {
	var values []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		values = append(values, svalueOf(raw))
		return nil
	})
	return values, err
}

func (s *State) enumerableExtEachSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size, err := s.enumStrictInt(args, 0)
	if err != nil {
		return Nil, err
	}
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid slice size")}
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	if block.IsNil() {
		return s.toEnum(self, "each_slice", args), nil
	}
	for i := 0; i < len(values); i += size {
		end := i + size
		if end > len(values) {
			end = len(values)
		}
		if _, err := s.callBlock1(ctx, block, ArrayValue(append([]Value(nil), values[i:end]...)), Nil); err != nil {
			return Nil, err
		}
	}
	return Nil, nil
}

func (s *State) enumerableExtEachConsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size, err := s.enumStrictInt(args, 0)
	if err != nil {
		return Nil, err
	}
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid size")}
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	if block.IsNil() {
		return s.toEnum(self, "each_cons", args), nil
	}
	for i := 0; i+size <= len(values); i++ {
		if _, err := s.callBlock1(ctx, block, ArrayValue(append([]Value(nil), values[i:i+size]...)), Nil); err != nil {
			return Nil, err
		}
	}
	return Nil, nil
}

func (s *State) enumerableExtTakeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count, err := s.enumStrictInt(args, 0)
	if err != nil {
		return Nil, err
	}
	if count < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "attempt to take negative size")}
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	if count > len(values) {
		count = len(values)
	}
	return ArrayValue(append([]Value(nil), values[:count]...)), nil
}

func (s *State) enumerableExtDropMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count, err := s.enumStrictInt(args, 0)
	if err != nil {
		return Nil, err
	}
	if count < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "attempt to drop negative size")}
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	if count > len(values) {
		count = len(values)
	}
	return ArrayValue(append([]Value(nil), values[count:]...)), nil
}

func (s *State) enumValueMatches(ctx context.Context, args []Value, block Value, raw []Value) (bool, error) {
	value := svalueOf(raw)
	if len(args) == 1 {
		match, err := s.call(ctx, args[0], "===", []Value{value}, Nil, Nil)
		if err != nil {
			return false, err
		}
		return match.IsTruthy(), nil
	}
	if !block.IsNil() {
		match, err := s.callBlock(ctx, block, raw, Nil)
		if err != nil {
			return false, err
		}
		return match.IsTruthy(), nil
	}
	return value.IsTruthy(), nil
}

func (s *State) enumerableExtAllMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	result := true
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		match, err := s.enumValueMatches(ctx, args, block, raw)
		if err != nil {
			return err
		}
		if !match {
			result = false
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(result), nil
}

func (s *State) enumerableExtAnyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	result := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		match, err := s.enumValueMatches(ctx, args, block, raw)
		if err != nil {
			return err
		}
		if match {
			result = true
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(result), nil
}

func (s *State) enumerableExtNoneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.enumerableExtAnyMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	return BoolValue(!result.IsTruthy()), nil
}

func (s *State) enumerableExtOneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	count := 0
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		match, err := s.enumValueMatches(ctx, args, block, raw)
		if err != nil {
			return err
		}
		if match {
			count++
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(count == 1), nil
}

func (s *State) enumerableExtCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	count := 0
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		if len(args) == 1 {
			value := svalueOf(raw)
			if identicalValue(value, args[0]) {
				count++
				return nil
			}
			equal, err := s.rubyEqual(ctx, value, args[0])
			if err != nil {
				return err
			}
			if equal {
				count++
			}
			return nil
		}
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			if match.IsTruthy() {
				count++
			}
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return IntValue(int64(count)), nil
}

func (s *State) arrayExtCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	count := 0
	for index := 0; index < len(data.values); index++ {
		value := data.values[index]
		if len(args) == 1 {
			if identicalValue(value, args[0]) {
				count++
				continue
			}
			equal, err := s.rubyEqual(ctx, value, args[0])
			if err != nil {
				return Nil, err
			}
			if equal {
				count++
			}
			continue
		}
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			if match.IsTruthy() {
				count++
			}
			continue
		}
		count++
	}
	return IntValue(int64(count)), nil
}

func (s *State) enumerableExtMinmaxByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "minmax_by", nil), nil
	}
	var minimum, maximum Value
	var minKey, maxKey Value
	found := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		key, err := s.callBlock(ctx, block, raw, Nil)
		if err != nil {
			return err
		}
		if !found {
			minimum, maximum = value, value
			minKey, maxKey = key, key
			found = true
			return nil
		}
		cmp, err := s.compareValues(ctx, key, minKey, Nil)
		if err != nil {
			return err
		}
		if cmp < 0 {
			minimum, minKey = value, key
		}
		cmp, err = s.compareValues(ctx, key, maxKey, Nil)
		if err != nil {
			return err
		}
		if cmp > 0 {
			maximum, maxKey = value, key
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	if !found {
		return ArrayValue([]Value{Nil, Nil}), nil
	}
	return ArrayValue([]Value{minimum, maximum}), nil
}

func (s *State) enumerableExtReverseEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "reverse_each", nil), nil
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	for i := len(values) - 1; i >= 0; i-- {
		if _, err := s.callBlock1(ctx, block, values[i], Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) enumerableExtCycleMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	if block.IsNil() {
		return s.toEnum(self, "cycle", args), nil
	}
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	if len(values) == 0 {
		return Nil, nil
	}
	count := -1
	if len(args) == 1 {
		count, err = s.enumStrictInt(args, 0)
		if err != nil {
			return Nil, err
		}
	}
	for round := 0; count < 0 || round < count; round++ {
		for _, value := range values {
			if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
				if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
					return br.value, nil
				}
				return Nil, err
			}
		}
	}
	return Nil, nil
}

func (s *State) enumerableExtFindIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	index := 0
	found := -1
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		if found >= 0 {
			return nil
		}
		match := false
		if len(args) == 1 {
			value := svalueOf(raw)
			if identicalValue(value, args[0]) {
				match = true
			} else {
				equal, err := s.rubyEqual(ctx, value, args[0])
				if err != nil {
					return err
				}
				match = equal
			}
		} else if !block.IsNil() {
			value, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			match = value.IsTruthy()
		}
		if match {
			found = index
		}
		index++
		return nil
	})
	if err != nil {
		return Nil, err
	}
	if found < 0 {
		return Nil, nil
	}
	return IntValue(int64(found)), nil
}

func (s *State) enumerableExtToHMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result := HashValue(nil)
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		var pairValue Value
		if block.IsNil() {
			pairValue = svalueOf(raw)
		} else {
			value, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			pairValue = value
		}
		pair, ok := arrayDataOf(pairValue)
		if !ok {
			return s.newArrayError(s.TypeErrorClass, "wrong element type (expected Array)")
		}
		if len(pair.values) != 2 {
			return s.newArrayError(s.ArgumentErrorClass, fmt.Sprintf("wrong array length (expected 2, was %d)", len(pair.values)))
		}
		_, err := s.setIndex(ctx, result, pair.values[0], pair.values[1])
		return err
	})
	if err != nil {
		return Nil, err
	}
	return result, nil
}

func (s *State) enumerableExtFilterMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "filter_map", nil), nil
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value, err := s.callBlock(ctx, block, raw, Nil)
		if err != nil {
			return err
		}
		if value.IsTruthy() {
			result = append(result, value)
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableExtTallyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	var entries []hashEntry
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		for i := range entries {
			if arrayEql(entries[i].key, value) {
				entries[i].value = IntValue(entries[i].value.Int() + 1)
				return nil
			}
		}
		entries = append(entries, hashEntry{key: value, value: IntValue(1)})
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return HashValue(entries), nil
}

func (s *State) enumerableExtGrepVMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		match, err := s.call(ctx, args[0], "===", []Value{value}, Nil, Nil)
		if err != nil {
			return err
		}
		if match.IsTruthy() {
			return nil
		}
		if !block.IsNil() {
			mapped, err := s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return err
			}
			result = append(result, mapped)
			return nil
		}
		result = append(result, value)
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableExtEachEntryMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_entry", nil), nil
	}
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		var value Value
		switch len(raw) {
		case 0:
			value = Nil
		case 1:
			value = raw[0]
		default:
			value = ArrayValue(append([]Value(nil), raw...))
		}
		_, err := s.callBlock1(ctx, block, value, Nil)
		return err
	})
	if err != nil {
		return Nil, err
	}
	return self, nil
}

var errStopEnumeration = errors.New("stop enumeration")

func (s *State) enumZipArgument(ctx context.Context, arg Value, limit int) ([]Value, error) {
	if data, ok := arrayDataOf(arg); ok {
		return data.values, nil
	}
	if _, ok := hashDataOf(arg); ok {
		return nil, s.newArrayError(s.TypeErrorClass, "wrong argument type Hash (must respond to to_ary)")
	}
	if _, ok := s.asClass(arg); ok {
		return nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("wrong argument type %s (must respond to to_ary)", s.classOf(arg).name))
	}
	if _, ok := s.classOf(arg).lookup("each"); !ok {
		return nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("wrong argument type %s (must respond to to_ary)", s.classOf(arg).name))
	}
	var values []Value
	err := s.eachRaw(ctx, arg, func(raw []Value) error {
		values = append(values, svalueOf(raw))
		if len(values) >= limit {
			return errStopEnumeration
		}
		return nil
	})
	if err != nil && err != errStopEnumeration {
		if _, ok := err.(*unwindSignal); ok {
			return nil, err
		}
		return nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("wrong argument type %s (must respond to to_ary)", s.classOf(arg).name))
	}
	return values, nil
}

func (s *State) enumZipValues(ctx context.Context, values []Value, args []Value) ([]Value, error) {
	lists := make([][]Value, len(args))
	for i, arg := range args {
		list, err := s.enumZipArgument(ctx, arg, len(values))
		if err != nil {
			return nil, err
		}
		lists[i] = list
	}
	length := len(values)
	result := make([]Value, length)
	for i := 0; i < length; i++ {
		row := make([]Value, 0, len(lists)+1)
		if i < len(values) {
			row = append(row, values[i])
		} else {
			row = append(row, Nil)
		}
		for _, list := range lists {
			if i < len(list) {
				row = append(row, list[i])
			} else {
				row = append(row, Nil)
			}
		}
		result[i] = ArrayValue(row)
	}
	return result, nil
}

func (s *State) enumerableExtZipMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumCollectValues(ctx, self)
	if err != nil {
		return Nil, err
	}
	rows, err := s.enumZipValues(ctx, values, args)
	if err != nil {
		return Nil, err
	}
	if !block.IsNil() {
		for _, row := range rows {
			if _, err := s.callBlock1(ctx, block, row, Nil); err != nil {
				return Nil, err
			}
		}
		return Nil, nil
	}
	return ArrayValue(rows), nil
}

func (s *State) arrayExtZipMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	rows, err := s.enumZipValues(ctx, arrayValues(self), args)
	if err != nil {
		return Nil, err
	}
	if !block.IsNil() {
		for _, row := range rows {
			if _, err := s.callBlock1(ctx, block, row, Nil); err != nil {
				return Nil, err
			}
		}
		return Nil, nil
	}
	return ArrayValue(rows), nil
}

func (s *State) arrayExtSortByBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "sort_by!", nil), nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	type sortPair struct {
		key   Value
		value Value
	}
	pairs := make([]sortPair, len(data.values))
	for i, value := range data.values {
		key, err := s.callBlock1(ctx, block, value, Nil)
		if err != nil {
			return Nil, err
		}
		pairs[i] = sortPair{key: key, value: value}
	}
	for i := 1; i < len(pairs); i++ {
		for j := i; j > 0; j-- {
			cmp, err := s.compareValues(ctx, pairs[j-1].key, pairs[j].key, Nil)
			if err != nil {
				return Nil, err
			}
			if cmp <= 0 {
				break
			}
			pairs[j-1], pairs[j] = pairs[j], pairs[j-1]
		}
	}
	for i := range pairs {
		data.values[i] = pairs[i].value
	}
	return self, nil
}

func (s *State) kernelLoopMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "loop", nil), nil
	}
	for {
		_, err := s.callBlock(ctx, block, nil, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			if s.isStopIteration(err) {
				return Nil, nil
			}
			return Nil, err
		}
	}
}

func (s *State) enumerableExtChunkMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 0 {
		return Nil, argumentCountError(s, len(args), "0")
	}
	if block.IsNil() {
		return s.toEnum(self, "chunk", nil), nil
	}
	return s.newEnumerator(func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error) {
		var currentKey Value
		var current []Value
		have := false
		flush := func() error {
			if !have {
				return nil
			}
			have = false
			_, err := yield([]Value{ArrayValue([]Value{currentKey, ArrayValue(current)})})
			current = nil
			return err
		}
		err := s.eachRaw(ctx, self, func(raw []Value) error {
			value := svalueOf(raw)
			key, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			if key.Kind() == KindSymbol {
				switch key.String() {
				case "_alone":
					if err := flush(); err != nil {
						return err
					}
					_, err := yield([]Value{ArrayValue([]Value{key, ArrayValue([]Value{value})})})
					return err
				case "_separator":
					return flush()
				default:
					if strings.HasPrefix(key.String(), "_") {
						return &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "invalid chunk state")}
					}
				}
			}
			if key.IsNil() {
				return flush()
			}
			if have && arrayEql(currentKey, key) {
				current = append(current, value)
				return nil
			}
			if err := flush(); err != nil {
				return err
			}
			currentKey = key
			current = []Value{value}
			have = true
			return nil
		})
		if err != nil {
			return Nil, err
		}
		if err := flush(); err != nil {
			return Nil, err
		}
		return Nil, nil
	}), nil
}

func (s *State) enumerableExtChunkWhileMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 0 {
		return Nil, argumentCountError(s, len(args), "0")
	}
	if block.IsNil() {
		return s.toEnum(self, "chunk_while", nil), nil
	}
	return s.newEnumerator(func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error) {
		var current []Value
		var previous Value
		havePrevious := false
		flush := func() error {
			if len(current) == 0 {
				return nil
			}
			chunk := ArrayValue(append([]Value(nil), current...))
			current = nil
			_, err := yield([]Value{chunk})
			return err
		}
		err := s.eachRaw(ctx, self, func(raw []Value) error {
			value := svalueOf(raw)
			if havePrevious {
				join, err := s.callBlock2(ctx, block, previous, value, Nil)
				if err != nil {
					return err
				}
				if !join.IsTruthy() {
					if err := flush(); err != nil {
						return err
					}
				}
			}
			current = append(current, value)
			previous = value
			havePrevious = true
			return nil
		})
		if err != nil {
			return Nil, err
		}
		if err := flush(); err != nil {
			return Nil, err
		}
		return Nil, nil
	}), nil
}
