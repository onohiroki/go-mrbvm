package mrbvm

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// installCoreLibrary adds the pure-Go part of mruby's core library. The
// methods here are the primitives needed by the macro-language runtime; the
// Ruby-level mrblib methods can be layered on top later.
func (s *State) installCoreLibrary() {
	s.installArrayLibrary()
	s.installHashLibrary()
	s.installStringLibrary()
	s.installIntegerLibrary()
	s.installObjectLibrary()
	s.installRangeLibrary()
}

func (s *State) installArrayLibrary() {
	class := s.ArrayClass
	class.define("empty?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(arrayValues(self)) == 0), nil
	})
	class.define("first", s.arrayFirstMethod)
	class.define("last", s.arrayLastMethod)
	class.define("index", s.arrayIndexMethod2)
	class.define("rindex", s.arrayRindexMethod)
	class.define("reverse", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		values := arrayValues(self)
		result := make([]Value, len(values))
		for i, value := range values {
			result[len(values)-1-i] = value
		}
		return ArrayValue(result), nil
	})
	class.define("reverse!", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		values := arrayValues(self)
		for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
			values[i], values[j] = values[j], values[i]
		}
		return self, nil
	})
	class.define("join", s.arrayJoinMethod)
	class.define("delete_at", s.arrayDeleteAtMethod)
	class.define("delete", s.arrayDeleteMethod)
	class.define("shift", s.arrayShiftMethod)
	class.define("unshift", s.arrayUnshiftMethod)
	class.define("pop", s.arrayPopMethod)
	class.define("concat", s.arrayConcatMethod)
	class.define("flatten", s.arrayFlattenMethod)
	class.define("uniq", s.arrayUniqMethod)
	class.define("compact", s.arrayCompactMethod)
	class.define("each_index", s.arrayEachIndexMethod)
	class.define("slice", s.arraySliceMethod)
	class.define("collect", s.arrayMapMethod)
	class.define("select", s.arraySelectMethod)
	class.define("find_all", s.arraySelectMethod)
	class.define("filter", s.arraySelectMethod)
	class.define("reject", s.arrayRejectMethod)
	class.define("find", s.arrayFindMethod)
	class.define("detect", s.arrayFindMethod)
	class.define("include?", s.arrayIncludeMethod)
	class.define("member?", s.arrayIncludeMethod)
	class.define("each_with_index", s.arrayEachWithIndexMethod)
	class.define("inject", s.arrayInjectMethod)
	class.define("reduce", s.arrayInjectMethod)
	class.define("min", s.arrayMinMethod)
	class.define("max", s.arrayMaxMethod)
	class.define("count", s.arrayCountMethod)
	class.define("all?", s.arrayAllMethod)
	class.define("any?", s.arrayAnyMethod)
	class.define("none?", s.arrayNoneMethod)
	class.define("one?", s.arrayOneMethod)
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
}

func (s *State) installHashLibrary() {
	class := s.HashClass
	class.define("empty?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(hashEntries(self)) == 0), nil
	})
	class.define("keys", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		entries := hashEntries(self)
		values := make([]Value, len(entries))
		for i, entry := range entries {
			values[i] = entry.key
		}
		return ArrayValue(values), nil
	})
	class.define("values", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		entries := hashEntries(self)
		values := make([]Value, len(entries))
		for i, entry := range entries {
			values[i] = entry.value
		}
		return ArrayValue(values), nil
	})
	class.define("fetch", s.hashFetchMethod)
	class.define("key?", s.hashKeyMethod)
	class.define("has_key?", s.hashKeyMethod)
	class.define("include?", s.hashKeyMethod)
	class.define("member?", s.hashKeyMethod)
	class.define("value?", s.hashValueMethod)
	class.define("has_value?", s.hashValueMethod)
	class.define("each", s.hashEachMethod)
	class.define("each_pair", s.hashEachMethod)
	class.define("each_key", s.hashEachKeyMethod)
	class.define("each_value", s.hashEachValueMethod)
	class.define("to_a", s.hashToAMethod)
	class.define("entries", s.hashToAMethod)
	class.define("merge", s.hashMergeMethod)
	class.define("merge!", s.hashMergeBangMethod)
	class.define("update", s.hashMergeBangMethod)
	class.define("delete", s.hashDeleteMethod)
	class.define("__delete", s.hashDeleteMethod)
	class.define("invert", s.hashInvertMethod)
	class.define("values_at", s.hashValuesAtMethod)
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
}

func (s *State) installStringLibrary() {
	class := s.StringClass
	class.define("empty?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.String() == ""), nil
	})
	class.define("strip", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(strings.TrimSpace(self.String())), nil
	})
	class.define("lstrip", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(strings.TrimLeft(self.String(), " \t\r\n\f\v")), nil
	})
	class.define("rstrip", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(strings.TrimRight(self.String(), " \t\r\n\f\v")), nil
	})
	class.define("split", s.stringSplitMethod)
	class.define("include?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		return BoolValue(strings.Contains(self.String(), args[0].String())), nil
	})
	class.define("start_with?", s.stringStartWithMethod)
	class.define("end_with?", s.stringEndWithMethod)
	class.define("index", s.stringIndexMethod)
	class.define("rindex", s.stringRindexMethod)
	class.define("slice", s.stringSliceMethod)
	class.define("[]", s.stringSliceMethod)
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := strings.TrimSpace(self.String())
		if index := strings.IndexFunc(text, func(r rune) bool {
			return !(r == '+' || r == '-' || (r >= '0' && r <= '9') || r == '_')
		}); index >= 0 {
			text = text[:index]
		}
		text = strings.ReplaceAll(text, "_", "")
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return IntValue(0), nil
		}
		return IntValue(value), nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := strings.TrimSpace(self.String())
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return FloatValue(0), nil
		}
		return FloatValue(value), nil
	})
	class.define("chars", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		runes := []rune(self.String())
		values := make([]Value, len(runes))
		for i, r := range runes {
			values[i] = StringValue(string(r))
		}
		return ArrayValue(values), nil
	})
	class.define("bytes", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		bytes := []byte(self.String())
		values := make([]Value, len(bytes))
		for i, b := range bytes {
			values[i] = IntValue(int64(b))
		}
		return ArrayValue(values), nil
	})
	class.define("reverse", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		runes := []rune(self.String())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return StringValue(string(runes)), nil
	})
	class.define("concat", s.stringConcatMethod)
	class.define("<<", s.stringConcatMethod)
	class.define("replace", s.stringReplaceMethod)
	class.define("getbyte", s.stringGetbyteMethod)
	class.define("bytesize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(len([]byte(self.String())))), nil
	})
	class.define("byteslice", s.stringBytesliceMethod)
	class.define("byteindex", s.stringByteindexMethod)
	class.define("__sub_replace", s.stringSubReplaceMethod)
	class.define("sub", s.stringSubMethod)
	class.define("gsub", s.stringGsubMethod)
	class.define("to_sym", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return SymbolValue(self.String()), nil
	})
}

func (s *State) installIntegerLibrary() {
	class := s.IntegerClass
	class.define("times", s.integerTimesMethod)
	class.define("upto", s.integerUptoMethod)
	class.define("downto", s.integerDowntoMethod)
	class.define("step", s.integerStepMethod)
	class.define("abs", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return normalizedBigInt(new(big.Int).Abs(value)), nil
		}
		value := self.Int()
		if value < 0 {
			value = -value
		}
		return IntValue(value), nil
	})
	class.define("even?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return BoolValue(value.Bit(0) == 0), nil
		}
		return BoolValue(self.Int()%2 == 0), nil
	})
	class.define("odd?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return BoolValue(value.Bit(0) == 1), nil
		}
		return BoolValue(self.Int()%2 != 0), nil
	})
	class.define("zero?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Kind() == KindInt && self.Int() == 0), nil
	})
	class.define("positive?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Kind() == KindInt && self.Int() > 0), nil
	})
	class.define("negative?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Kind() == KindInt && self.Int() < 0), nil
	})
	class.define("succ", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(self.Int() + 1), nil
	})
	class.define("next", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(self.Int() + 1), nil
	})
	class.define("pred", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(self.Int() - 1), nil
	})
	class.define("gcd", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "gcd expects an integer")}
		}
		a, b := self.Int(), args[0].Int()
		if a < 0 {
			a = -a
		}
		if b < 0 {
			b = -b
		}
		for b != 0 {
			a, b = b, a%b
		}
		return IntValue(a), nil
	})
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return FloatValue(float64(self.Int())), nil
	})
	class.define("chr", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(string(rune(self.Int()))), nil
	})
}

func (s *State) installObjectLibrary() {
	class := s.ObjectClass
	class.define("is_a?", s.objectIsAMethod)
	class.define("kind_of?", s.objectIsAMethod)
	class.define("instance_of?", s.objectInstanceOfMethod)
	class.define("equal?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		if self.Kind() == KindObject && args[0].Kind() == KindObject {
			return BoolValue(self.obj == args[0].obj), nil
		}
		return BoolValue(valuesEqual(self, args[0])), nil
	})
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && valuesEqual(self, args[0])), nil
	})
	class.define("send", s.objectSendMethod)
	class.define("__send__", s.objectSendMethod)
	class.define("tap", s.objectTapMethod)
}

func (s *State) installRangeLibrary() {
	class := s.RangeClass
	class.define("begin", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := rangeDataOf(self); ok {
			return data.low, nil
		}
		return Nil, nil
	})
	class.define("end", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := rangeDataOf(self); ok {
			return data.high, nil
		}
		return Nil, nil
	})
	class.define("exclude_end?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := rangeDataOf(self); ok {
			return BoolValue(data.exclusive), nil
		}
		return False, nil
	})
	class.define("first", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := rangeDataOf(self); ok {
			if len(args) == 0 {
				return data.low, nil
			}
			return s.rangeTake(data, int(args[0].Int()), false)
		}
		return Nil, nil
	})
	class.define("last", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := rangeDataOf(self); ok {
			if len(args) == 0 {
				return data.high, nil
			}
			return s.rangeTake(data, int(args[0].Int()), true)
		}
		return Nil, nil
	})
	class.define("size", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := rangeDataOf(self)
		if !ok || data.low.Kind() != KindInt || data.high.Kind() != KindInt {
			return Nil, nil
		}
		size := data.high.Int() - data.low.Int()
		if !data.exclusive {
			size++
		}
		if size < 0 {
			size = 0
		}
		return IntValue(size), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
}

func (s *State) rangeTake(data *rangeData, n int, fromEnd bool) (Value, error) {
	if n < 0 || data.low.Kind() != KindInt || data.high.Kind() != KindInt {
		return ArrayValue(nil), nil
	}
	var values []Value
	if fromEnd {
		high := data.high.Int()
		if data.exclusive {
			high--
		}
		start := high - int64(n) + 1
		if start < data.low.Int() {
			start = data.low.Int()
		}
		for i := start; i <= high; i++ {
			values = append(values, IntValue(i))
		}
	} else {
		high := data.low.Int() + int64(n) - 1
		if high > data.high.Int() || (data.exclusive && high >= data.high.Int()) {
			high = data.high.Int()
			if data.exclusive {
				high--
			}
		}
		for i := data.low.Int(); i <= high; i++ {
			values = append(values, IntValue(i))
		}
	}
	return ArrayValue(values), nil
}

func arrayValues(v Value) []Value {
	if v.obj == nil {
		return nil
	}
	if data, ok := v.obj.data.(*arrayData); ok {
		return data.values
	}
	return nil
}

func hashEntries(v Value) []hashEntry {
	if v.obj == nil {
		return nil
	}
	if data, ok := v.obj.data.(*hashData); ok {
		return data.entries
	}
	return nil
}

func rangeDataOf(v Value) (*rangeData, bool) {
	if v.obj == nil {
		return nil, false
	}
	data, ok := v.obj.data.(*rangeData)
	return data, ok
}

// The following methods are deliberately small wrappers so that the public
// class definitions above stay readable.

func (s *State) arrayFirstMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	if len(args) > 0 {
		return s.arrayTake(values, int(args[0].Int()), false)
	}
	if len(values) == 0 {
		return Nil, nil
	}
	return values[0], nil
}

func (s *State) arrayLastMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	if len(args) > 0 {
		return s.arrayTake(values, int(args[0].Int()), true)
	}
	if len(values) == 0 {
		return Nil, nil
	}
	return values[len(values)-1], nil
}

func (s *State) arrayTake(values []Value, n int, fromEnd bool) (Value, error) {
	if n < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
	}
	if n > len(values) {
		n = len(values)
	}
	if fromEnd {
		return ArrayValue(append([]Value(nil), values[len(values)-n:]...)), nil
	}
	return ArrayValue(append([]Value(nil), values[:n]...)), nil
}

func (s *State) arrayIndexMethod2(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	for i, value := range values {
		if len(args) == 0 {
			if block.IsNil() {
				return Nil, nil
			}
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			if match.IsTruthy() {
				return IntValue(int64(i)), nil
			}
			continue
		}
		if len(args) == 1 && valuesEqual(value, args[0]) {
			return IntValue(int64(i)), nil
		}
	}
	return Nil, nil
}

func (s *State) arrayRindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	for i := len(values) - 1; i >= 0; i-- {
		if len(args) == 1 && valuesEqual(values[i], args[0]) {
			return IntValue(int64(i)), nil
		}
	}
	return Nil, nil
}

func (s *State) arrayJoinMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	separator := ""
	if len(args) > 0 && !args[0].IsNil() {
		separator = args[0].ToS()
	}
	values := arrayValues(self)
	parts := make([]string, len(values))
	for i, value := range values {
		if value.obj != nil {
			if arr, ok := value.obj.data.(*arrayData); ok && len(args) == 0 {
				parts[i] = ArrayValue(arr.values).Inspect()
				continue
			}
		}
		parts[i] = value.ToS()
	}
	return StringValue(strings.Join(parts, separator)), nil
}

func (s *State) arrayDeleteAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "delete_at expects an integer")}
	}
	values := arrayValues(self)
	index := int(args[0].Int())
	if index < 0 {
		index += len(values)
	}
	if index < 0 || index >= len(values) {
		return Nil, nil
	}
	value := values[index]
	copy(values[index:], values[index+1:])
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = values[:len(values)-1]
		}
	}
	return value, nil
}

func (s *State) arrayDeleteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "delete expects one argument")}
	}
	values := arrayValues(self)
	result := make([]Value, 0, len(values))
	var deleted Value
	found := false
	for _, value := range values {
		if valuesEqual(value, args[0]) {
			deleted = value
			found = true
			continue
		}
		result = append(result, value)
	}
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = result
		}
	}
	if found {
		return deleted, nil
	}
	if !block.IsNil() {
		return s.callBlock(ctx, block, nil, Nil)
	}
	return Nil, nil
}

func (s *State) arrayShiftMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	if len(values) == 0 {
		return Nil, nil
	}
	value := values[0]
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = values[1:]
		}
	}
	return value, nil
}

func (s *State) arrayUnshiftMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data.values = append(append([]Value(nil), args...), data.values...)
	return self, nil
}

func (s *State) arrayPopMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	if len(values) == 0 {
		return Nil, nil
	}
	value := values[len(values)-1]
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = values[:len(values)-1]
		}
	}
	return value, nil
}

func (s *State) arrayConcatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	for _, arg := range args {
		values = append(values, arrayValues(arg)...)
	}
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = values
		}
	}
	return self, nil
}

func (s *State) arrayFlattenMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	depth := -1
	if len(args) > 0 && args[0].Kind() == KindInt {
		depth = int(args[0].Int())
	}
	var flatten func(values []Value, level int) []Value
	flatten = func(values []Value, level int) []Value {
		var result []Value
		for _, value := range values {
			if value.obj != nil {
				if arr, ok := value.obj.data.(*arrayData); ok && level != 0 {
					result = append(result, flatten(arr.values, level-1)...)
					continue
				}
			}
			result = append(result, value)
		}
		return result
	}
	return ArrayValue(flatten(arrayValues(self), depth)), nil
}

func (s *State) arrayUniqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	result := make([]Value, 0, len(values))
	for _, value := range values {
		duplicate := false
		for _, existing := range result {
			if valuesEqual(existing, value) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayCompactMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	result := make([]Value, 0, len(values))
	for _, value := range values {
		if !value.IsNil() {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayEachIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for i := range arrayValues(self) {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(int64(i))}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arraySliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 1 {
		return s.index(ctx, self, args[0])
	}
	if len(args) == 2 && args[0].Kind() == KindInt && args[1].Kind() == KindInt {
		start := int(args[0].Int())
		length := int(args[1].Int())
		values := arrayValues(self)
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
	return Nil, nil
}

func (s *State) arraySelectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return ArrayValue(nil), nil
	}
	var result []Value
	for _, value := range arrayValues(self) {
		match, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayRejectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return ArrayValue(append([]Value(nil), arrayValues(self)...)), nil
	}
	var result []Value
	for _, value := range arrayValues(self) {
		match, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			return Nil, err
		}
		if !match.IsTruthy() {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayFindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, nil
	}
	for _, value := range arrayValues(self) {
		match, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() {
			return value, nil
		}
	}
	if len(args) > 0 {
		return args[0], nil
	}
	return Nil, nil
}

func (s *State) arrayIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	for _, value := range arrayValues(self) {
		if valuesEqual(value, args[0]) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayEachWithIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for i, value := range arrayValues(self) {
		if _, err := s.callBlock(ctx, block, []Value{value, IntValue(int64(i))}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arrayInjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	var accumulator Value
	start := 0
	if len(args) > 0 {
		if args[0].Kind() == KindSymbol && block.IsNil() {
			name := args[0].String()
			if len(values) == 0 {
				return Nil, nil
			}
			accumulator = values[0]
			start = 1
			for _, value := range values[start:] {
				result, err := s.call(ctx, accumulator, name, []Value{value}, Nil, Nil)
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

func (s *State) arrayMinMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.arrayExtreme(ctx, self, block, true)
}

func (s *State) arrayMaxMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.arrayExtreme(ctx, self, block, false)
}

func (s *State) arrayExtreme(ctx context.Context, self Value, block Value, wantMin bool) (Value, error) {
	values := arrayValues(self)
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

func (s *State) arrayCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := 0
	for _, value := range arrayValues(self) {
		if len(args) == 1 {
			if valuesEqual(value, args[0]) {
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
		} else {
			count++
		}
	}
	return IntValue(int64(count)), nil
}

func (s *State) arrayAllMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, value := range arrayValues(self) {
		test := value.IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			test = match.IsTruthy()
		}
		if !test {
			return False, nil
		}
	}
	return True, nil
}

func (s *State) arrayAnyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, value := range arrayValues(self) {
		test := value.IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			test = match.IsTruthy()
		}
		if test {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayNoneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.arrayAnyMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	return BoolValue(!result.IsTruthy()), nil
}

func (s *State) arrayOneMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := 0
	for _, value := range arrayValues(self) {
		test := value.IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			test = match.IsTruthy()
		}
		if test {
			count++
		}
	}
	return BoolValue(count == 1), nil
}

func (s *State) hashFetchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "fetch expects a key")}
	}
	for _, entry := range hashEntries(self) {
		if valuesEqual(entry.key, args[0]) {
			return entry.value, nil
		}
	}
	if len(args) > 1 {
		return args[1], nil
	}
	if !block.IsNil() {
		return s.callBlock(ctx, block, []Value{args[0]}, Nil)
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.KeyErrorClass, "key not found: "+args[0].Inspect())}
}

func (s *State) hashKeyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	for _, entry := range hashEntries(self) {
		if valuesEqual(entry.key, args[0]) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) hashValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	for _, entry := range hashEntries(self) {
		if valuesEqual(entry.value, args[0]) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) hashEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for _, entry := range hashEntries(self) {
		if _, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) hashEachKeyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for _, entry := range hashEntries(self) {
		if _, err := s.callBlock(ctx, block, []Value{entry.key}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) hashEachValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	for _, entry := range hashEntries(self) {
		if _, err := s.callBlock(ctx, block, []Value{entry.value}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) hashToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	entries := hashEntries(self)
	values := make([]Value, len(entries))
	for i, entry := range entries {
		values[i] = ArrayValue([]Value{entry.key, entry.value})
	}
	return ArrayValue(values), nil
}

func (s *State) hashMergeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	entries := append([]hashEntry(nil), hashEntries(self)...)
	for _, arg := range args {
		var err error
		entries, err = s.mergeHashEntries(ctx, entries, hashEntries(arg), block)
		if err != nil {
			return Nil, err
		}
	}
	return HashValue(entries), nil
}

func (s *State) hashMergeBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	for _, arg := range args {
		var err error
		data.entries, err = s.mergeHashEntries(ctx, data.entries, hashEntries(arg), block)
		if err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) mergeHashEntries(ctx context.Context, left, right []hashEntry, block Value) ([]hashEntry, error) {
	result := append([]hashEntry(nil), left...)
	for _, entry := range right {
		replaced := false
		for i := range result {
			if valuesEqual(result[i].key, entry.key) {
				if !block.IsNil() {
					value, err := s.callBlock(ctx, block, []Value{entry.key, result[i].value, entry.value}, Nil)
					if err != nil {
						return nil, err
					}
					result[i].value = value
				} else {
					result[i].value = entry.value
				}
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *State) hashDeleteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "delete expects one argument")}
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	for i, entry := range data.entries {
		if valuesEqual(entry.key, args[0]) {
			data.entries = append(data.entries[:i], data.entries[i+1:]...)
			return entry.value, nil
		}
	}
	if !block.IsNil() {
		return s.callBlock(ctx, block, []Value{args[0]}, Nil)
	}
	return Nil, nil
}

func (s *State) hashInvertMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	entries := hashEntries(self)
	result := make([]hashEntry, len(entries))
	for i, entry := range entries {
		result[i] = hashEntry{key: entry.value, value: entry.key}
	}
	return HashValue(result), nil
}

func (s *State) hashValuesAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := make([]Value, len(args))
	for i, key := range args {
		values[i] = Nil
		for _, entry := range hashEntries(self) {
			if valuesEqual(entry.key, key) {
				values[i] = entry.value
				break
			}
		}
	}
	return ArrayValue(values), nil
}

func (s *State) stringSplitMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	var parts []string
	if len(args) == 0 || args[0].IsNil() {
		parts = strings.Fields(text)
	} else {
		separator := args[0].String()
		if separator == "" {
			for _, r := range text {
				parts = append(parts, string(r))
			}
		} else {
			parts = strings.Split(text, separator)
		}
	}
	values := make([]Value, len(parts))
	for i, part := range parts {
		values[i] = StringValue(part)
	}
	return ArrayValue(values), nil
}

func (s *State) stringStartWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	for _, arg := range args {
		if strings.HasPrefix(text, arg.String()) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) stringEndWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	for _, arg := range args {
		if strings.HasSuffix(text, arg.String()) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) stringIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, nil
	}
	text := self.String()
	offset := 0
	if len(args) > 1 && args[1].Kind() == KindInt {
		offset = int(args[1].Int())
	}
	runes := []rune(text)
	if offset < 0 {
		offset += len(runes)
	}
	if offset < 0 || offset > len(runes) {
		return Nil, nil
	}
	index := strings.Index(string(runes[offset:]), args[0].String())
	if index < 0 {
		return Nil, nil
	}
	return IntValue(int64(offset + len([]rune(string(runes[offset:])[:index])))), nil
}

func (s *State) stringRindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, nil
	}
	text := self.String()
	index := strings.LastIndex(text, args[0].String())
	if index < 0 {
		return Nil, nil
	}
	return IntValue(int64(len([]rune(text[:index])))), nil
}

func (s *State) stringSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 1 {
		if args[0].Kind() == KindObject && args[0].obj != nil {
			if data, ok := args[0].obj.data.(*rangeData); ok {
				return sliceString(self.String(), data), nil
			}
		}
		return s.index(ctx, self, args[0])
	}
	if len(args) == 2 && args[0].Kind() == KindInt && args[1].Kind() == KindInt {
		runes := []rune(self.String())
		start := int(args[0].Int())
		length := int(args[1].Int())
		if start < 0 {
			start += len(runes)
		}
		if start < 0 || start > len(runes) || length < 0 {
			return Nil, nil
		}
		end := start + length
		if end > len(runes) {
			end = len(runes)
		}
		return StringValue(string(runes[start:end])), nil
	}
	return Nil, nil
}

func sliceString(text string, data *rangeData) Value {
	runes := []rune(text)
	low := 0
	high := len(runes) - 1
	if !data.low.IsNil() && data.low.Kind() == KindInt {
		low = int(data.low.Int())
		if low < 0 {
			low += len(runes)
		}
	}
	if !data.high.IsNil() && data.high.Kind() == KindInt {
		high = int(data.high.Int())
		if high < 0 {
			high += len(runes)
		}
		if data.exclusive {
			high--
		}
	}
	if low < 0 {
		low = 0
	}
	if high >= len(runes) {
		high = len(runes) - 1
	}
	if high < low {
		return StringValue("")
	}
	return StringValue(string(runes[low : high+1]))
}

func (s *State) stringConcatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a string")
	}
	data, ok := self.obj.data.(*stringData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a string")
	}
	for _, arg := range args {
		if arg.Kind() == KindInt {
			data.value += string(rune(arg.Int()))
		} else {
			data.value += arg.ToS()
		}
	}
	return self, nil
}

func (s *State) stringReplaceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "replace expects one argument")}
	}
	data, ok := self.obj.data.(*stringData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a string")
	}
	data.value = args[0].String()
	return self, nil
}

func (s *State) stringGetbyteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, nil
	}
	bytes := []byte(self.String())
	index := int(args[0].Int())
	if index < 0 || index >= len(bytes) {
		return Nil, nil
	}
	return IntValue(int64(bytes[index])), nil
}

func (s *State) stringBytesliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	bytes := []byte(self.String())
	if len(args) == 1 {
		if args[0].Kind() == KindInt {
			index := int(args[0].Int())
			if index < 0 || index >= len(bytes) {
				return Nil, nil
			}
			return StringValue(string(bytes[index])), nil
		}
		if args[0].Kind() == KindObject && args[0].obj != nil {
			if data, ok := args[0].obj.data.(*rangeData); ok {
				return sliceString(string(bytes), data), nil
			}
		}
	}
	if len(args) == 2 && args[0].Kind() == KindInt && args[1].Kind() == KindInt {
		start := int(args[0].Int())
		length := int(args[1].Int())
		if start < 0 {
			start += len(bytes)
		}
		if start < 0 || start > len(bytes) || length < 0 {
			return Nil, nil
		}
		end := start + length
		if end > len(bytes) {
			end = len(bytes)
		}
		return StringValue(string(bytes[start:end])), nil
	}
	return Nil, nil
}

func (s *State) stringByteindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, nil
	}
	bytes := []byte(self.String())
	offset := 0
	if len(args) > 1 && args[1].Kind() == KindInt {
		offset = int(args[1].Int())
	}
	if offset < 0 || offset > len(bytes) {
		return Nil, nil
	}
	index := strings.Index(string(bytes[offset:]), args[0].String())
	if index < 0 {
		return Nil, nil
	}
	return IntValue(int64(offset + index)), nil
}

func (s *State) stringSubReplaceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "__sub_replace expects two arguments")}
	}
	return StringValue(args[0].String()), nil
}

func (s *State) stringSubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "sub expects one or two arguments")}
	}
	text := self.String()
	pattern := args[0].String()
	index := strings.Index(text, pattern)
	if index < 0 {
		return StringValue(text), nil
	}
	var replacement string
	if !block.IsNil() {
		result, err := s.callBlock(ctx, block, []Value{StringValue(pattern)}, Nil)
		if err != nil {
			return Nil, err
		}
		replacement = result.ToS()
	} else if len(args) == 2 {
		replacement = args[1].String()
	}
	return StringValue(text[:index] + replacement + text[index+len(pattern):]), nil
}

func (s *State) stringGsubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "gsub expects one or two arguments")}
	}
	text := self.String()
	pattern := args[0].String()
	if pattern == "" {
		return StringValue(text), nil
	}
	if block.IsNil() && len(args) == 2 {
		return StringValue(strings.ReplaceAll(text, pattern, args[1].String())), nil
	}
	var builder strings.Builder
	offset := 0
	for {
		index := strings.Index(text[offset:], pattern)
		if index < 0 {
			builder.WriteString(text[offset:])
			break
		}
		index += offset
		builder.WriteString(text[offset:index])
		result, err := s.callBlock(ctx, block, []Value{StringValue(pattern)}, Nil)
		if err != nil {
			return Nil, err
		}
		builder.WriteString(result.ToS())
		offset = index + len(pattern)
	}
	return StringValue(builder.String()), nil
}

func (s *State) integerTimesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := self.Int()
	if block.IsNil() {
		if count < 0 {
			return ArrayValue(nil), nil
		}
		values := make([]Value, count)
		for i := int64(0); i < count; i++ {
			values[i] = IntValue(i)
		}
		return ArrayValue(values), nil
	}
	for i := int64(0); i < count; i++ {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(i)}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) integerUptoMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "upto expects an integer")}
	}
	start, end := self.Int(), args[0].Int()
	if block.IsNil() {
		if end < start {
			return ArrayValue(nil), nil
		}
		values := make([]Value, 0, end-start+1)
		for i := start; i <= end; i++ {
			values = append(values, IntValue(i))
		}
		return ArrayValue(values), nil
	}
	for i := start; i <= end; i++ {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(i)}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) integerDowntoMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "downto expects an integer")}
	}
	start, end := self.Int(), args[0].Int()
	if block.IsNil() {
		if end > start {
			return ArrayValue(nil), nil
		}
		values := make([]Value, 0, start-end+1)
		for i := start; i >= end; i-- {
			values = append(values, IntValue(i))
		}
		return ArrayValue(values), nil
	}
	for i := start; i >= end; i-- {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(i)}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) integerStepMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	limit := self.Int()
	step := int64(1)
	if len(args) > 0 && !args[0].IsNil() && args[0].Kind() == KindInt {
		limit = args[0].Int()
	}
	if len(args) > 1 && args[1].Kind() == KindInt {
		step = args[1].Int()
	}
	if step == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "step can't be 0")}
	}
	var values []Value
	for i := self.Int(); (step > 0 && i <= limit) || (step < 0 && i >= limit); i += step {
		values = append(values, IntValue(i))
	}
	if block.IsNil() {
		return ArrayValue(values), nil
	}
	for _, value := range values {
		if _, err := s.callBlock(ctx, block, []Value{value}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) objectIsAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	class, ok := s.asClass(args[0])
	if !ok {
		return False, nil
	}
	return BoolValue(s.isA(self, class)), nil
}

func (s *State) objectInstanceOfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	class, ok := s.asClass(args[0])
	if !ok {
		return False, nil
	}
	return BoolValue(s.classOf(self) == class), nil
}

func (s *State) objectSendMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "send expects a method name")}
	}
	name := args[0].String()
	return s.call(ctx, self, name, args[1:], kw, block)
}

func (s *State) objectTapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if !block.IsNil() {
		if _, err := s.callBlock(ctx, block, []Value{self}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}
