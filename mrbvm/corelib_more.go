package mrbvm

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (s *State) installAdvancedCoreLibrary() {
	s.installAdvancedArrayLibrary()
	s.installAdvancedHashLibrary()
	s.installAdvancedStringLibrary()
	s.installAdvancedIntegerLibrary()
	s.installFloatLibrary()
}

func (s *State) installAdvancedArrayLibrary() {
	class := s.ArrayClass
	class.define("take", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.arrayTake(arrayValues(self), intArg(args, 0, 0), false)
	})
	class.define("drop", s.arrayDropMethod)
	class.define("take_while", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.arrayWhileMethod(ctx, self, block, true)
	})
	class.define("drop_while", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.arrayWhileMethod(ctx, self, block, false)
	})
	class.define("each_slice", s.arrayEachSliceMethod)
	class.define("each_cons", s.arrayEachConsMethod)
	class.define("zip", s.arrayZipMethod)
	class.define("flat_map", s.arrayFlatMapMethod)
	class.define("collect_concat", s.arrayFlatMapMethod)
	class.define("group_by", s.arrayGroupByMethod)
	class.define("partition", s.arrayPartitionMethod)
	class.define("each_with_object", s.arrayEachWithObjectMethod)
	class.define("sort_by", s.arraySortByMethod)
	class.define("minmax", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		min, err := s.arrayExtreme(ctx, self, block, true)
		if err != nil {
			return Nil, err
		}
		max, err := s.arrayExtreme(ctx, self, block, false)
		if err != nil {
			return Nil, err
		}
		return ArrayValue([]Value{min, max}), nil
	})
	class.define("sum", s.arraySumMethod)
	class.define("dig", s.arrayDigMethod)
	class.define("transpose", s.arrayTransposeMethod)
	class.define("each_entry", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return self, nil
		}
		for _, value := range arrayValues(self) {
			if _, err := s.callBlock(ctx, block, []Value{value}, Nil); err != nil {
				return Nil, err
			}
		}
		return self, nil
	})
}

func (s *State) installAdvancedHashLibrary() {
	class := s.HashClass
	class.define("map", s.hashMapMethod)
	class.define("collect", s.hashMapMethod)
	class.define("select", s.hashSelectMethod)
	class.define("filter", s.hashSelectMethod)
	class.define("reject", s.hashRejectMethod)
	class.define("find", s.hashFindMethod)
	class.define("detect", s.hashFindMethod)
	class.define("sort_by", s.hashSortByMethod)
	class.define("group_by", s.hashGroupByMethod)
	class.define("each_with_object", s.hashEachWithObjectMethod)
	class.define("key", s.hashKeyOfMethod)
	class.define("dig", s.hashDigMethod)
	class.define("transform_values", s.hashTransformValuesMethod)
	class.define("transform_keys", s.hashTransformKeysMethod)
	class.define("select!", s.hashSelectBangMethod)
	class.define("reject!", s.hashRejectBangMethod)
	class.define("any?", s.hashAnyMethod)
	class.define("all?", s.hashAllMethod)
}

func (s *State) installAdvancedStringLibrary() {
	class := s.StringClass
	class.define("chomp", s.stringChompMethod)
	class.define("chop", s.stringChopMethod)
	class.define("capitalize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		runes := []rune(strings.ToLower(self.String()))
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		return StringValue(string(runes)), nil
	})
	class.define("swapcase", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(strings.Map(func(r rune) rune {
			if unicode.IsUpper(r) {
				return unicode.ToLower(r)
			}
			if unicode.IsLower(r) {
				return unicode.ToUpper(r)
			}
			return r
		}, self.String())), nil
	})
	class.define("ljust", s.stringLjustMethod)
	class.define("rjust", s.stringRjustMethod)
	class.define("center", s.stringCenterMethod)
	class.define("*", s.stringRepeatMethod)
	class.define("%", s.stringFormatMethod)
	class.define("ord", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		runes := []rune(self.String())
		if len(runes) == 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "empty string")}
		}
		return IntValue(int64(runes[0])), nil
	})
	class.define("count", s.stringCountMethod)
	class.define("delete", s.stringDeleteMethod)
	class.define("tr", s.stringTrMethod)
	class.define("lines", s.stringLinesMethod)
	class.define("succ", s.stringSuccMethod)
	class.define("next", s.stringSuccMethod)
	class.define("squeeze", s.stringSqueezeMethod)
	class.define("upcase!", s.stringUpcaseBangMethod)
	class.define("downcase!", s.stringDowncaseBangMethod)
}

func (s *State) installAdvancedIntegerLibrary() {
	class := s.IntegerClass
	class.define("div", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "div expects an integer")}
		}
		return IntValue(floorDiv(self.Int(), args[0].Int())), nil
	})
	class.define("divmod", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "divmod expects an integer")}
		}
		divisor := args[0].Int()
		quotient := floorDiv(self.Int(), divisor)
		remainder := self.Int() - quotient*divisor
		return ArrayValue([]Value{IntValue(quotient), IntValue(remainder)}), nil
	})
	class.define("fdiv", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "fdiv expects an integer")}
		}
		if args[0].Int() == 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
		return FloatValue(float64(self.Int()) / float64(args[0].Int())), nil
	})
	class.define("bit_length", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		value := self.Int()
		if value < 0 {
			value = -value
		}
		length := 0
		for value > 0 {
			length++
			value >>= 1
		}
		return IntValue(int64(length)), nil
	})
	class.define("digits", s.integerDigitsMethod)
	class.define("pow", s.integerPowMethod)
	class.define("between?", s.numericBetweenMethod)
	class.define("clamp", s.numericClampMethod)
	class.define("floor", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("ceil", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("round", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("truncate", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
}

func (s *State) installFloatLibrary() {
	class := s.FloatClass
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(self.Float())), nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("abs", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return FloatValue(math.Abs(self.Float())), nil
	})
	class.define("floor", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(math.Floor(self.Float()))), nil
	})
	class.define("ceil", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(math.Ceil(self.Float()))), nil
	})
	class.define("round", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(math.Round(self.Float()))), nil
	})
	class.define("truncate", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(int64(self.Float())), nil
	})
	class.define("nan?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(math.IsNaN(self.Float())), nil
	})
	class.define("infinite?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		value := self.Float()
		if math.IsInf(value, 1) {
			return IntValue(1), nil
		}
		if math.IsInf(value, -1) {
			return IntValue(-1), nil
		}
		return Nil, nil
	})
	class.define("zero?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Float() == 0), nil
	})
	class.define("positive?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Float() > 0), nil
	})
	class.define("negative?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.Float() < 0), nil
	})
	class.define("between?", s.numericBetweenMethod)
	class.define("clamp", s.numericClampMethod)
}

func (s *State) arrayDropMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	n := intArg(args, 0, 0)
	values := arrayValues(self)
	if n < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "attempt to drop negative size")}
	}
	if n > len(values) {
		n = len(values)
	}
	return ArrayValue(append([]Value(nil), values[n:]...)), nil
}

func (s *State) arrayWhileMethod(ctx context.Context, self Value, block Value, take bool) (Value, error) {
	var result []Value
	dropping := !take
	for _, value := range arrayValues(self) {
		matched := true
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if take {
			if !matched {
				break
			}
			result = append(result, value)
		} else {
			if dropping {
				if matched {
					continue
				}
				dropping = false
			}
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayEachSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size := intArg(args, 0, 1)
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid slice size")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each_slice", args), nil
	}
	values := arrayValues(self)
	for i := 0; i < len(values); i += size {
		end := i + size
		if end > len(values) {
			end = len(values)
		}
		if _, err := s.callBlock(ctx, block, []Value{ArrayValue(append([]Value(nil), values[i:end]...))}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arrayEachConsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	size := intArg(args, 0, 1)
	if size <= 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid size")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each_cons", args), nil
	}
	values := arrayValues(self)
	for i := 0; i+size <= len(values); i++ {
		if _, err := s.callBlock(ctx, block, []Value{ArrayValue(append([]Value(nil), values[i:i+size]...))}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arrayZipMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	arrays := make([][]Value, len(args))
	length := len(values)
	for i, arg := range args {
		arrays[i] = arrayValues(arg)
		if len(arrays[i]) > length {
			length = len(arrays[i])
		}
	}
	result := make([]Value, length)
	for i := 0; i < length; i++ {
		row := make([]Value, 0, 1+len(arrays))
		if i < len(values) {
			row = append(row, values[i])
		} else {
			row = append(row, Nil)
		}
		for _, array := range arrays {
			if i < len(array) {
				row = append(row, array[i])
			} else {
				row = append(row, Nil)
			}
		}
		result[i] = ArrayValue(row)
	}
	if !block.IsNil() {
		for _, row := range result {
			if _, err := s.callBlock(ctx, block, []Value{row}, Nil); err != nil {
				return Nil, err
			}
		}
		return Nil, nil
	}
	return ArrayValue(result), nil
}

func (s *State) arrayFlatMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	var result []Value
	for _, value := range arrayValues(self) {
		mapped := value
		if !block.IsNil() {
			m, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			mapped = m
		}
		if mapped.obj != nil {
			if data, ok := mapped.obj.data.(*arrayData); ok {
				result = append(result, data.values...)
				continue
			}
		}
		result = append(result, mapped)
	}
	return ArrayValue(result), nil
}

func (s *State) arrayGroupByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	groups := make([]hashEntry, 0)
	for _, value := range arrayValues(self) {
		key := value
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			key = k
		}
		found := false
		for i := range groups {
			if valuesEqual(groups[i].key, key) {
				data := groups[i].value.obj.data.(*arrayData)
				data.values = append(data.values, value)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, hashEntry{key: key, value: ArrayValue([]Value{value})})
		}
	}
	return HashValue(groups), nil
}

func (s *State) arrayPartitionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	var yes, no []Value
	for _, value := range arrayValues(self) {
		matched := value.IsTruthy()
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if matched {
			yes = append(yes, value)
		} else {
			no = append(no, value)
		}
	}
	return ArrayValue([]Value{ArrayValue(yes), ArrayValue(no)}), nil
}

func (s *State) arrayEachWithObjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "each_with_object expects a memo and a block")}
	}
	memo := args[0]
	for _, value := range arrayValues(self) {
		if _, err := s.callBlock(ctx, block, []Value{value, memo}, Nil); err != nil {
			return Nil, err
		}
	}
	return memo, nil
}

func (s *State) arraySortByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	type pair struct {
		key   Value
		value Value
	}
	pairs := make([]pair, len(values))
	for i, value := range values {
		key := value
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			key = k
		}
		pairs[i] = pair{key: key, value: value}
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

func (s *State) arraySumMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	accumulator := IntValue(0)
	if len(args) > 0 {
		accumulator = args[0]
	}
	for _, value := range arrayValues(self) {
		if !block.IsNil() {
			mapped, err := s.callBlock(ctx, block, []Value{value}, Nil)
			if err != nil {
				return Nil, err
			}
			value = mapped
		}
		sum, err := s.call(ctx, accumulator, "+", []Value{value}, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		accumulator = sum
	}
	return accumulator, nil
}

func (s *State) arrayDigMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	current := self
	for _, key := range args {
		value, err := s.digValue(ctx, current, key)
		if err != nil {
			return Nil, err
		}
		if value.IsNil() {
			return Nil, nil
		}
		current = value
	}
	return current, nil
}

func (s *State) digValue(ctx context.Context, receiver, key Value) (Value, error) {
	if receiver.obj != nil {
		switch data := receiver.obj.data.(type) {
		case *arrayData:
			if key.Kind() != KindInt {
				return Nil, nil
			}
			index := int(key.Int())
			if index < 0 || index >= len(data.values) {
				return Nil, nil
			}
			return data.values[index], nil
		case *hashData:
			for _, entry := range data.entries {
				if valuesEqual(entry.key, key) {
					return entry.value, nil
				}
			}
			return Nil, nil
		}
	}
	return Nil, nil
}

func (s *State) arrayTransposeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	rows := arrayValues(self)
	if len(rows) == 0 {
		return ArrayValue(nil), nil
	}
	width := len(arrayValues(rows[0]))
	for _, row := range rows {
		if len(arrayValues(row)) != width {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "element size differs")}
		}
	}
	result := make([]Value, width)
	for column := 0; column < width; column++ {
		values := make([]Value, len(rows))
		for row := range rows {
			values[row] = arrayValues(rows[row])[column]
		}
		result[column] = ArrayValue(values)
	}
	return ArrayValue(result), nil
}

func (s *State) hashMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "map", nil), nil
	}
	var result []Value
	for _, entry := range hashEntries(self) {
		if block.IsNil() {
			result = append(result, ArrayValue([]Value{entry.key, entry.value}))
			continue
		}
		value, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
		if err != nil {
			return Nil, err
		}
		result = append(result, value)
	}
	return ArrayValue(result), nil
}

func (s *State) hashSelectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "select", nil), nil
	}
	var result []hashEntry
	for _, entry := range hashEntries(self) {
		matched := true
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if matched {
			result = append(result, entry)
		}
	}
	return HashValue(result), nil
}

func (s *State) hashRejectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "reject", nil), nil
	}
	var result []hashEntry
	for _, entry := range hashEntries(self) {
		matched := false
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if !matched {
			result = append(result, entry)
		}
	}
	return HashValue(result), nil
}

func (s *State) hashFindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "find", nil), nil
	}
	for _, entry := range hashEntries(self) {
		match, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() {
			return ArrayValue([]Value{entry.key, entry.value}), nil
		}
	}
	return Nil, nil
}

func (s *State) hashSortByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "sort_by", nil), nil
	}
	type pair struct {
		key   Value
		entry hashEntry
	}
	entries := hashEntries(self)
	pairs := make([]pair, len(entries))
	for i, entry := range entries {
		key := ArrayValue([]Value{entry.key, entry.value})
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			key = k
		}
		pairs[i] = pair{key: key, entry: entry}
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
		result[i] = ArrayValue([]Value{p.entry.key, p.entry.value})
	}
	return ArrayValue(result), nil
}

func (s *State) hashGroupByMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "group_by", nil), nil
	}
	groups := make([]hashEntry, 0)
	for _, entry := range hashEntries(self) {
		key := ArrayValue([]Value{entry.key, entry.value})
		if !block.IsNil() {
			k, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			key = k
		}
		found := false
		for i := range groups {
			if valuesEqual(groups[i].key, key) {
				data := groups[i].value.obj.data.(*arrayData)
				data.values = append(data.values, ArrayValue([]Value{entry.key, entry.value}))
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, hashEntry{key: key, value: ArrayValue([]Value{ArrayValue([]Value{entry.key, entry.value})})})
		}
	}
	return HashValue(groups), nil
}

func (s *State) hashEachWithObjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "each_with_object expects a memo and a block")}
	}
	memo := args[0]
	for _, entry := range hashEntries(self) {
		if _, err := s.callBlock(ctx, block, []Value{ArrayValue([]Value{entry.key, entry.value}), memo}, Nil); err != nil {
			return Nil, err
		}
	}
	return memo, nil
}

func (s *State) hashKeyOfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	for _, entry := range hashEntries(self) {
		if valuesEqual(entry.value, args[0]) {
			return entry.key, nil
		}
	}
	return Nil, nil
}

func (s *State) hashDigMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.arrayDigMethod(ctx, self, args, kw, block)
}

func (s *State) hashTransformValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return HashValue(append([]hashEntry(nil), hashEntries(self)...)), nil
	}
	entries := hashEntries(self)
	result := make([]hashEntry, len(entries))
	for i, entry := range entries {
		value, err := s.callBlock(ctx, block, []Value{entry.value}, Nil)
		if err != nil {
			return Nil, err
		}
		result[i] = hashEntry{key: entry.key, value: value}
	}
	return HashValue(result), nil
}

func (s *State) hashTransformKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return HashValue(append([]hashEntry(nil), hashEntries(self)...)), nil
	}
	entries := hashEntries(self)
	result := make([]hashEntry, len(entries))
	for i, entry := range entries {
		key, err := s.callBlock(ctx, block, []Value{entry.key}, Nil)
		if err != nil {
			return Nil, err
		}
		result[i] = hashEntry{key: key, value: entry.value}
	}
	return HashValue(result), nil
}

func (s *State) hashSelectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	selected, err := s.hashSelectMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return self, nil
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return self, nil
	}
	data.entries = hashEntries(selected)
	return self, nil
}

func (s *State) hashRejectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	rejected, err := s.hashRejectMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return self, nil
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return self, nil
	}
	data.entries = hashEntries(rejected)
	return self, nil
}

func (s *State) hashAnyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, entry := range hashEntries(self) {
		matched := true
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if matched {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) hashAllMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, entry := range hashEntries(self) {
		matched := true
		if !block.IsNil() {
			match, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
			if err != nil {
				return Nil, err
			}
			matched = match.IsTruthy()
		}
		if !matched {
			return False, nil
		}
	}
	return True, nil
}

func (s *State) stringChompMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	separator := "\n"
	if len(args) > 0 {
		if args[0].IsNil() {
			return StringValue(text), nil
		}
		separator = args[0].String()
	}
	if separator == "" {
		text = strings.TrimRight(text, "\r\n")
	} else if strings.HasSuffix(text, separator) {
		text = text[:len(text)-len(separator)]
	} else if separator == "\n" && strings.HasSuffix(text, "\r\n") {
		text = text[:len(text)-2]
	}
	return StringValue(text), nil
}

func (s *State) stringChopMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	if strings.HasSuffix(text, "\r\n") {
		return StringValue(text[:len(text)-2]), nil
	}
	_, size := utf8.DecodeLastRuneInString(text)
	if size == 0 {
		return StringValue(""), nil
	}
	return StringValue(text[:len(text)-size]), nil
}

func (s *State) stringPad(text string, width int, pad string, left, right bool) string {
	if width <= len([]rune(text)) {
		return text
	}
	if pad == "" {
		pad = " "
	}
	missing := width - len([]rune(text))
	padRunes := []rune(pad)
	build := func(n int) string {
		var builder strings.Builder
		for i := 0; i < n; i++ {
			builder.WriteRune(padRunes[i%len(padRunes)])
		}
		return builder.String()
	}
	leftCount, rightCount := 0, 0
	if left && right {
		leftCount = missing / 2
		rightCount = missing - leftCount
	} else if right {
		rightCount = missing
	} else {
		leftCount = missing
	}
	return build(leftCount) + text + build(rightCount)
}

func (s *State) stringLjustMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	return StringValue(s.stringPad(self.String(), width, pad, false, true)), nil
}

func (s *State) stringRjustMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	return StringValue(s.stringPad(self.String(), width, pad, true, false)), nil
}

func (s *State) stringCenterMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	return StringValue(s.stringPad(self.String(), width, pad, true, true)), nil
}

func (s *State) stringRepeatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := intArg(args, 0, 0)
	if count < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative argument")}
	}
	return StringValue(strings.Repeat(self.String(), count)), nil
}

func (s *State) stringFormatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := args
	if len(args) == 1 && args[0].obj != nil {
		if data, ok := args[0].obj.data.(*arrayData); ok {
			values = data.values
		}
	}
	formatted, err := formatString(self.String(), values)
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, err.Error())}
	}
	return StringValue(formatted), nil
}

func formatString(format string, args []Value) (string, error) {
	var builder strings.Builder
	argIndex := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			builder.WriteByte(format[i])
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			builder.WriteByte('%')
			i++
			continue
		}
		start := i
		i++
		flags := ""
		for i < len(format) && strings.ContainsRune("-+ 0#", rune(format[i])) {
			flags += string(format[i])
			i++
		}
		width := 0
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			width = width*10 + int(format[i]-'0')
			i++
		}
		precision := -1
		if i < len(format) && format[i] == '.' {
			i++
			precision = 0
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				precision = precision*10 + int(format[i]-'0')
				i++
			}
		}
		if i >= len(format) {
			builder.WriteString(format[start:])
			break
		}
		specifier := format[i]
		if argIndex >= len(args) {
			return "", fmt.Errorf("too few arguments")
		}
		value := args[argIndex]
		argIndex++
		var text string
		switch specifier {
		case 'd', 'i', 'u':
			text = strconv.FormatInt(value.Int(), 10)
		case 'x':
			text = strconv.FormatInt(value.Int(), 16)
		case 'X':
			text = strings.ToUpper(strconv.FormatInt(value.Int(), 16))
		case 'o':
			text = strconv.FormatInt(value.Int(), 8)
		case 'b':
			text = strconv.FormatInt(value.Int(), 2)
		case 'f':
			precisionValue := 6
			if precision >= 0 {
				precisionValue = precision
			}
			text = strconv.FormatFloat(value.Float(), 'f', precisionValue, 64)
		case 'e':
			precisionValue := 6
			if precision >= 0 {
				precisionValue = precision
			}
			text = strconv.FormatFloat(value.Float(), 'e', precisionValue, 64)
		case 'g':
			precisionValue := -1
			if precision >= 0 {
				precisionValue = precision
			}
			text = strconv.FormatFloat(value.Float(), 'g', precisionValue, 64)
		case 'c':
			text = string(rune(value.Int()))
		case 'p':
			text = value.Inspect()
		case 's':
			text = value.ToS()
		default:
			return "", fmt.Errorf("malformed format string - %%%c", specifier)
		}
		if width > len([]rune(text)) {
			missing := width - len([]rune(text))
			if strings.Contains(flags, "-") {
				text += strings.Repeat(" ", missing)
			} else if strings.Contains(flags, "0") && strings.IndexByte("diuxXob", specifier) >= 0 {
				sign := ""
				if strings.HasPrefix(text, "-") && !strings.Contains(flags, "+") {
					sign = "-"
					text = text[1:]
				}
				text = sign + strings.Repeat("0", missing) + text
			} else {
				text = strings.Repeat(" ", missing) + text
			}
		}
		if strings.Contains(flags, "+") && value.Kind() == KindInt && value.Int() >= 0 &&
			strings.IndexByte("di", specifier) >= 0 {
			text = "+" + text
		}
		builder.WriteString(text)
	}
	return builder.String(), nil
}

func (s *State) stringCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return IntValue(0), nil
	}
	set := expandCharSet(args[0].String())
	count := 0
	for _, r := range self.String() {
		if set[r] {
			count++
		}
	}
	return IntValue(int64(count)), nil
}

func (s *State) stringDeleteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return StringValue(self.String()), nil
	}
	set := expandCharSet(args[0].String())
	var builder strings.Builder
	for _, r := range self.String() {
		if !set[r] {
			builder.WriteRune(r)
		}
	}
	return StringValue(builder.String()), nil
}

func (s *State) stringTrMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return StringValue(self.String()), nil
	}
	from := expandCharList(args[0].String())
	to := expandCharList(args[1].String())
	mapping := make(map[rune]rune, len(from))
	for i, r := range from {
		if len(to) == 0 {
			continue
		}
		if i < len(to) {
			mapping[r] = to[i]
		} else {
			mapping[r] = to[len(to)-1]
		}
	}
	var builder strings.Builder
	for _, r := range self.String() {
		if mapped, ok := mapping[r]; ok {
			builder.WriteRune(mapped)
		} else {
			builder.WriteRune(r)
		}
	}
	return StringValue(builder.String()), nil
}

func (s *State) stringLinesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	var lines []string
	start := 0
	for i, r := range text {
		if r == '\n' {
			lines = append(lines, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		lines = append(lines, text[start:])
	}
	values := make([]Value, len(lines))
	for i, line := range lines {
		values[i] = StringValue(line)
	}
	return ArrayValue(values), nil
}

func (s *State) stringSuccMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	runes := []rune(self.String())
	if len(runes) == 0 {
		return StringValue(""), nil
	}
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		switch {
		case r >= '0' && r < '9':
			runes[i] = r + 1
			return StringValue(string(runes)), nil
		case r == '9':
			runes[i] = '0'
			continue
		case (r >= 'a' && r < 'z') || (r >= 'A' && r < 'Z'):
			runes[i] = r + 1
			return StringValue(string(runes)), nil
		case r == 'z':
			runes[i] = 'a'
			continue
		case r == 'Z':
			runes[i] = 'A'
			continue
		default:
			runes[i] = r + 1
			return StringValue(string(runes)), nil
		}
	}
	prefix := "a"
	if len(runes) > 0 && runes[0] >= 'A' && runes[0] <= 'Z' {
		prefix = "A"
	}
	return StringValue(prefix + string(runes)), nil
}

func (s *State) stringSqueezeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	set := map[rune]bool(nil)
	if len(args) > 0 {
		set = expandCharSet(args[0].String())
	}
	var builder strings.Builder
	var previous rune
	first := true
	for _, r := range self.String() {
		if !first && r == previous && (set == nil || set[r]) {
			continue
		}
		builder.WriteRune(r)
		previous = r
		first = false
	}
	return StringValue(builder.String()), nil
}

func (s *State) stringUpcaseBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, nil
	}
	if data, ok := self.obj.data.(*stringData); ok {
		updated := strings.ToUpper(data.value)
		if updated == data.value {
			return Nil, nil
		}
		data.value = updated
		return self, nil
	}
	return Nil, nil
}

func (s *State) stringDowncaseBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, nil
	}
	if data, ok := self.obj.data.(*stringData); ok {
		updated := strings.ToLower(data.value)
		if updated == data.value {
			return Nil, nil
		}
		data.value = updated
		return self, nil
	}
	return Nil, nil
}

func expandCharSet(spec string) map[rune]bool {
	set := make(map[rune]bool)
	runes := []rune(spec)
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' {
			start, end := runes[i], runes[i+2]
			if start <= end {
				for r := start; r <= end; r++ {
					set[r] = true
				}
			}
			i += 2
			continue
		}
		set[runes[i]] = true
	}
	return set
}

func expandCharList(spec string) []rune {
	var list []rune
	runes := []rune(spec)
	for i := 0; i < len(runes); i++ {
		if i+2 < len(runes) && runes[i+1] == '-' {
			start, end := runes[i], runes[i+2]
			if start <= end {
				for r := start; r <= end; r++ {
					list = append(list, r)
				}
			}
			i += 2
			continue
		}
		list = append(list, runes[i])
	}
	return list
}

func (s *State) integerDigitsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	base := int64(10)
	if len(args) > 0 && args[0].Kind() == KindInt {
		base = args[0].Int()
	}
	if base < 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid radix")}
	}
	value := self.Int()
	if value < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative number")}
	}
	var values []Value
	if value == 0 {
		values = append(values, IntValue(0))
	}
	for value > 0 {
		values = append(values, IntValue(value%base))
		value /= base
	}
	return ArrayValue(values), nil
}

func (s *State) integerPowMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "pow expects an exponent")}
	}
	if len(args) == 1 {
		result, ok := binaryValue(self, "**", args[0])
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "pow failed")}
		}
		return result, nil
	}
	base, exponent, modulus := big.NewInt(self.Int()), big.NewInt(args[0].Int()), big.NewInt(args[1].Int())
	if exponent.Sign() < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative exponent")}
	}
	return normalizedBigInt(new(big.Int).Exp(base, exponent, modulus)), nil
}

func (s *State) numericBetweenMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return False, nil
	}
	lower, err := s.compareValues(ctx, self, args[0], Nil)
	if err != nil {
		return Nil, err
	}
	if lower < 0 {
		return False, nil
	}
	upper, err := s.compareValues(ctx, self, args[1], Nil)
	if err != nil {
		return Nil, err
	}
	return BoolValue(upper <= 0), nil
}

func (s *State) numericClampMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 1 {
		if data, ok := rangeDataOf(args[0]); ok {
			args = []Value{data.low, data.high}
		}
	}
	if len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "clamp expects a range or two bounds")}
	}
	lower, err := s.compareValues(ctx, self, args[0], Nil)
	if err != nil {
		return Nil, err
	}
	if lower < 0 {
		return args[0], nil
	}
	upper, err := s.compareValues(ctx, self, args[1], Nil)
	if err != nil {
		return Nil, err
	}
	if upper > 0 {
		return args[1], nil
	}
	return self, nil
}

func intArg(args []Value, index int, fallback int) int {
	if index < len(args) && args[index].Kind() == KindInt {
		return int(args[index].Int())
	}
	return fallback
}

func floorDiv(left, right int64) int64 {
	if right == 0 {
		return 0
	}
	quotient := left / right
	remainder := left % right
	if remainder != 0 && (remainder < 0) != (right < 0) {
		quotient--
	}
	return quotient
}
