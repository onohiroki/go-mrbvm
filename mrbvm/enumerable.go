package mrbvm

import "context"

// installEnumerableLibrary defines the Ruby-level Enumerable protocol on top
// of a native block adapter. Classes that define #each and include
// Enumerable get the collection methods for free.
func (s *State) installEnumerableLibrary() {
	class := s.EnumerableClass
	class.define("to_a", s.enumerableToAMethod)
	class.define("entries", s.enumerableToAMethod)
	class.define("map", s.enumerableMapMethod)
	class.define("collect", s.enumerableMapMethod)
	class.define("select", s.enumerableSelectMethod)
	class.define("find_all", s.enumerableSelectMethod)
	class.define("filter", s.enumerableSelectMethod)
	class.define("reject", s.enumerableRejectMethod)
	class.define("find", s.enumerableFindMethod)
	class.define("detect", s.enumerableFindMethod)
	class.define("include?", s.enumerableIncludeMethod)
	class.define("grep", s.enumerableGrepMethod)
	class.define("member?", s.enumerableIncludeMethod)
	class.define("inject", s.enumerableInjectMethod)
	class.define("reduce", s.enumerableInjectMethod)
	class.define("each_with_index", s.enumerableEachWithIndexMethod)
	class.define("each_with_object", s.enumerableEachWithObjectMethod)
	class.define("count", s.enumerableCountMethod)
	class.define("all?", s.enumerableAllMethod)
	class.define("any?", s.enumerableAnyMethod)
	class.define("none?", s.enumerableNoneMethod)
	class.define("one?", s.enumerableOneMethod)
	class.define("min", s.enumerableMinMethod)
	class.define("max", s.enumerableMaxMethod)
	class.define("minmax", s.enumerableMinmaxMethod)
	class.define("min_by", s.enumerableMinByMethod)
	class.define("max_by", s.enumerableMaxByMethod)
	class.define("sort", s.enumerableSortMethod)
	class.define("sort_by", s.enumerableSortByMethod)
	class.define("group_by", s.enumerableGroupByMethod)
	class.define("partition", s.enumerablePartitionMethod)
	class.define("flat_map", s.enumerableFlatMapMethod)
	class.define("collect_concat", s.enumerableFlatMapMethod)
	class.define("first", s.enumerableFirstMethod)
	class.define("take", s.enumerableTakeMethod)
	class.define("drop", s.enumerableDropMethod)
	class.define("each_slice", s.enumerableEachSliceMethod)
	class.define("each_cons", s.enumerableEachConsMethod)
	class.define("sum", s.enumerableSumMethod)
}

func nativeBlockValue(class *Class, fn func(ctx context.Context, args []Value) (Value, error)) Value {
	return ObjectValue(&Object{class: class, data: &nativeBlockData{fn: fn, arity: -1, hasArity: true}})
}

func (s *State) eachRaw(ctx context.Context, self Value, fn func(args []Value) error) error {
	block := nativeBlockValue(s.ProcClass, func(ctx context.Context, args []Value) (Value, error) {
		return Nil, fn(args)
	})
	_, err := s.call(ctx, self, "each", nil, Nil, block)
	return err
}

func svalueOf(args []Value) Value {
	switch len(args) {
	case 0:
		return Nil
	case 1:
		return args[0]
	default:
		return ArrayValue(append([]Value(nil), args...))
	}
}

func (s *State) enumerableCollect(ctx context.Context, self Value, block Value) ([]Value, error) {
	var values []Value
	err := s.eachRaw(ctx, self, func(args []Value) error {
		values = append(values, svalueOf(args))
		return nil
	})
	return values, err
}

func (s *State) enumerableToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) enumerableMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "map", nil), nil
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		mapped, err := s.callBlock(ctx, block, raw, Nil)
		if err != nil {
			return err
		}
		result = append(result, mapped)
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableSelectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "select", nil), nil
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		matched := true
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			matched = match.IsTruthy()
		}
		if matched {
			result = append(result, svalueOf(raw))
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableRejectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "reject", nil), nil
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		matched := false
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			matched = match.IsTruthy()
		}
		if !matched {
			result = append(result, svalueOf(raw))
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableFindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "find", args), nil
	}
	var ifnone Value
	if len(args) > 0 {
		// Nested blocks may reuse a pooled argument slice, so copy before
		// iterating.
		ifnone = args[0]
	}
	var found Value
	matched := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		if matched || block.IsNil() {
			return nil
		}
		result, err := s.callBlock(ctx, block, raw, Nil)
		if err != nil {
			return err
		}
		if result.IsTruthy() {
			found = svalueOf(raw)
			matched = true
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	if matched {
		return found, nil
	}
	if !ifnone.IsNil() {
		return s.call(ctx, ifnone, "call", nil, Nil, Nil)
	}
	return Nil, nil
}

func (s *State) enumerableGrepMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	pattern := args[0]
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		match, err := s.call(ctx, pattern, "===", []Value{value}, Nil, Nil)
		if err != nil {
			return err
		}
		if !match.IsTruthy() {
			return nil
		}
		if !block.IsNil() {
			value, err = s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return err
			}
		}
		result = append(result, value)
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	target := args[0]
	found := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		equal, err := s.rubyEqual(ctx, svalueOf(raw), target)
		if err != nil {
			return err
		}
		if equal {
			found = true
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(found), nil
}

func (s *State) enumerableInjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	var accumulator Value
	start := 0
	if len(args) > 0 {
		if args[0].Kind() == KindSymbol && block.IsNil() {
			if len(values) == 0 {
				return Nil, nil
			}
			accumulator = values[0]
			start = 1
			for _, value := range values[start:] {
				result, err := s.call(ctx, accumulator, args[0].String(), []Value{value}, Nil, Nil)
				if err != nil {
					return Nil, err
				}
				accumulator = result
			}
			return accumulator, nil
		}
		accumulator = args[0]
	} else {
		if len(values) == 0 {
			return Nil, nil
		}
		accumulator = values[0]
		start = 1
	}
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for _, value := range values[start:] {
		result, err := s.callBlock(ctx, block, []Value{accumulator, value}, Nil)
		if err != nil {
			return Nil, err
		}
		accumulator = result
	}
	return accumulator, nil
}

func (s *State) enumerableEachWithIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_with_index", nil), nil
	}
	index := 0
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		args := append(append([]Value(nil), raw...), IntValue(int64(index)))
		index++
		_, err := s.callBlock(ctx, block, args, Nil)
		return err
	})
	if err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) enumerableEachWithObjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "each_with_object expects a memo")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each_with_object", args), nil
	}
	memo := args[0]
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		_, err := s.callBlock(ctx, block, []Value{svalueOf(raw), memo}, Nil)
		return err
	})
	if err != nil {
		return Nil, err
	}
	return memo, nil
}

func (s *State) enumerableCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := 0
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		if len(args) == 1 {
			if valuesEqual(svalueOf(raw), args[0]) {
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
		} else {
			count++
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return IntValue(int64(count)), nil
}

func (s *State) enumerableAllMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result := true
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		test := svalueOf(raw).IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			test = match.IsTruthy()
		}
		if !test {
			result = false
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(result), nil
}

func (s *State) enumerableAnyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		test := svalueOf(raw).IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			test = match.IsTruthy()
		}
		if test {
			result = true
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(result), nil
}

func (s *State) enumerableNoneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.enumerableAnyMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	return BoolValue(!result.IsTruthy()), nil
}

func (s *State) enumerableOneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := 0
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		test := svalueOf(raw).IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			test = match.IsTruthy()
		}
		if test {
			count++
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return BoolValue(count == 1), nil
}

func (s *State) enumerableMinMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.enumerableExtreme(ctx, self, block, true)
}

func (s *State) enumerableMaxMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.enumerableExtreme(ctx, self, block, false)
}

func (s *State) enumerableExtreme(ctx context.Context, self Value, block Value, wantMin bool) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	if len(values) == 0 {
		return Nil, nil
	}
	best := values[0]
	for _, value := range values[1:] {
		cmp, err := s.compareValues(ctx, value, best, block)
		if err != nil {
			return Nil, err
		}
		if (wantMin && cmp < 0) || (!wantMin && cmp > 0) {
			best = value
		}
	}
	return best, nil
}

func (s *State) enumerableMinmaxMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	min, err := s.enumerableExtreme(ctx, self, block, true)
	if err != nil {
		return Nil, err
	}
	max, err := s.enumerableExtreme(ctx, self, block, false)
	if err != nil {
		return Nil, err
	}
	return ArrayValue([]Value{min, max}), nil
}

func (s *State) enumerableMinByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "min_by", nil), nil
	}
	return s.enumerableExtremeBy(ctx, self, block, true)
}

func (s *State) enumerableMaxByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "max_by", nil), nil
	}
	return s.enumerableExtremeBy(ctx, self, block, false)
}

func (s *State) enumerableExtremeBy(ctx context.Context, self Value, block Value, wantMin bool) (Value, error) {
	var best Value
	var bestKey Value
	found := false
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		key := svalueOf(raw)
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			key = k
		}
		if !found {
			best, bestKey, found = svalueOf(raw), key, true
			return nil
		}
		cmp, err := s.compareValues(ctx, key, bestKey, Nil)
		if err != nil {
			return err
		}
		if (wantMin && cmp < 0) || (!wantMin && cmp > 0) {
			best, bestKey = svalueOf(raw), key
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	if !found {
		return Nil, nil
	}
	return best, nil
}

func (s *State) enumerableSortMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	for i := 1; i < len(values); i++ {
		for j := i; j > 0; j-- {
			cmp, err := s.compareValues(ctx, values[j-1], values[j], block)
			if err != nil {
				return Nil, err
			}
			if cmp <= 0 {
				break
			}
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
	return ArrayValue(values), nil
}

func (s *State) enumerableSortByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "sort_by", nil), nil
	}
	type pair struct {
		key   Value
		value Value
	}
	var pairs []pair
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		key := svalueOf(raw)
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			key = k
		}
		pairs = append(pairs, pair{key: key, value: svalueOf(raw)})
		return nil
	})
	if err != nil {
		return Nil, err
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
	result := make([]Value, len(pairs))
	for i, p := range pairs {
		result[i] = p.value
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableGroupByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "group_by", nil), nil
	}
	var groups []hashEntry
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		key := value
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			key = k
		}
		for i := range groups {
			if valuesEqual(groups[i].key, key) {
				data := groups[i].value.obj.data.(*arrayData)
				data.values = append(data.values, value)
				return nil
			}
		}
		groups = append(groups, hashEntry{key: key, value: ArrayValue([]Value{value})})
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return HashValue(groups), nil
}

func (s *State) enumerablePartitionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "partition", nil), nil
	}
	var yes, no []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		matched := value.IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			matched = match.IsTruthy()
		}
		if matched {
			yes = append(yes, value)
		} else {
			no = append(no, value)
		}
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue([]Value{ArrayValue(yes), ArrayValue(no)}), nil
}

func (s *State) enumerableFlatMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "flat_map", nil), nil
	}
	var result []Value
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		mapped := svalueOf(raw)
		if !block.IsNil() {
			m, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			mapped = m
		}
		if mapped.obj != nil {
			if data, ok := mapped.obj.data.(*arrayData); ok {
				result = append(result, data.values...)
				return nil
			}
		}
		result = append(result, mapped)
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(result), nil
}

func (s *State) enumerableFirstMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	if len(args) == 0 {
		if len(values) == 0 {
			return Nil, nil
		}
		return values[0], nil
	}
	n := int(args[0].Int())
	if n < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
	}
	if n > len(values) {
		n = len(values)
	}
	return ArrayValue(values[:n]), nil
}

func (s *State) enumerableTakeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	n := intArg(args, 0, 0)
	if n < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "attempt to take negative size")}
	}
	if n > len(values) {
		n = len(values)
	}
	return ArrayValue(append([]Value(nil), values[:n]...)), nil
}

func (s *State) enumerableDropMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	n := intArg(args, 0, 0)
	if n < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "attempt to drop negative size")}
	}
	if n > len(values) {
		n = len(values)
	}
	return ArrayValue(append([]Value(nil), values[n:]...)), nil
}

func (s *State) enumerableEachSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size := intArg(args, 0, 1)
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid slice size")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each_slice", args), nil
	}
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	var slices []Value
	for i := 0; i < len(values); i += size {
		end := i + size
		if end > len(values) {
			end = len(values)
		}
		slices = append(slices, ArrayValue(append([]Value(nil), values[i:end]...)))
	}
	if block.IsNil() {
		return ArrayValue(slices), nil
	}
	for _, slice := range slices {
		if _, err := s.callBlock(ctx, block, []Value{slice}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) enumerableEachConsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size := intArg(args, 0, 1)
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid size")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each_cons", args), nil
	}
	values, err := s.enumerableCollect(ctx, self, block)
	if err != nil {
		return Nil, err
	}
	var slices []Value
	for i := 0; i+size <= len(values); i++ {
		slices = append(slices, ArrayValue(append([]Value(nil), values[i:i+size]...)))
	}
	if block.IsNil() {
		return ArrayValue(slices), nil
	}
	for _, slice := range slices {
		if _, err := s.callBlock(ctx, block, []Value{slice}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) enumerableSumMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	accumulator := IntValue(0)
	if len(args) > 0 {
		accumulator = args[0]
	}
	err := s.eachRaw(ctx, self, func(raw []Value) error {
		value := svalueOf(raw)
		if !block.IsNil() {
			mapped, err := s.callBlock(ctx, block, raw, Nil)
			if err != nil {
				return err
			}
			value = mapped
		}
		sum, err := s.call(ctx, accumulator, "+", []Value{value}, Nil, Nil)
		if err != nil {
			return err
		}
		accumulator = sum
		return nil
	})
	if err != nil {
		return Nil, err
	}
	return accumulator, nil
}
