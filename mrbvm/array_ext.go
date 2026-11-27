package mrbvm

import (
	"context"
	"fmt"
)

// installArrayExtensionLibrary adds the methods mruby implements in
// mruby-array-ext. Several of them are also used by the core array tests.
func (s *State) installArrayExtensionLibrary() {
	class := s.ArrayClass
	class.define("assoc", s.arrayAssocMethod)
	class.define("rassoc", s.arrayRassocMethod)
	class.define("at", s.arrayAtMethod)
	class.define("uniq", s.arrayUniqMethod)
	class.define("uniq!", s.arrayUniqBangMethod)
	class.define("-", s.arraySubtractMethod)
	class.define("|", s.arrayUnionOpMethod)
	class.define("&", s.arrayIntersectMethod)
	class.define("union", s.arrayUnionMethod)
	class.define("difference", s.arrayDifferenceMethod)
	class.define("intersection", s.arrayIntersectionMethod)
	class.define("intersect?", s.arrayIntersectPMethod)
	class.define("compact!", s.arrayCompactBangMethod)
	class.define("fetch", s.arrayFetchMethod)
	class.define("fetch_values", s.arrayFetchValuesMethod)
	class.define("fill", s.arrayFillMethod)
	class.define("reverse_each", s.arrayReverseEachMethod)
	class.define("rotate", s.arrayRotateMethod)
	class.define("rotate!", s.arrayRotateBangMethod)
	class.define("delete_if", s.arrayDeleteIfMethod)
	class.define("reject!", s.arrayRejectBangMethod)
	class.define("keep_if", s.arrayKeepIfMethod)
	class.define("select!", s.arraySelectBangMethod)
	class.define("insert", s.arrayInsertMethod)
	class.define("bsearch", s.arrayBsearchMethod)
	class.define("values_at", s.arrayValuesAtMethod)
	class.define("to_h", s.arrayToHMethod)
	class.define("slice!", s.arraySliceBangMethod)
	class.define("permutation", s.arrayPermutationMethod)
	class.define("combination", s.arrayCombinationMethod)
	class.define("repeated_combination", s.arrayRepeatedCombinationMethod)
	class.define("repeated_permutation", s.arrayRepeatedPermutationMethod)
	class.define("product", s.arrayProductMethod)
	class.define("rfind", s.arrayRfindMethod)
	class.define("__unshift_from_c", s.arrayUnshiftMethod)
}

func (s *State) newArrayError(class *Class, message string) error {
	return &unwindSignal{kind: unwindException, value: newException(class, message)}
}

// arrayEql compares two elements the way a Hash keyed on them does: identity
// first, then eql?. The identity shortcut is what lets a NaN find itself.
func arrayEql(left, right Value) bool {
	if identicalValue(left, right) {
		return true
	}
	return arrayEqlValues(left, right)
}

func arrayEqlValues(left, right Value) bool {
	if left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case KindNil, KindTrue, KindFalse:
		return true
	case KindInt:
		return left.Int() == right.Int()
	case KindBigInt:
		leftValue, leftOK := left.BigInt()
		rightValue, rightOK := right.BigInt()
		return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
	case KindFloat:
		return left.Float() == right.Float()
	case KindSymbol:
		return left.String() == right.String()
	case KindString:
		return left.String() == right.String()
	case KindObject:
		return valuesEqual(left, right)
	}
	return identicalValue(left, right)
}

func (s *State) arrayAssocMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	for _, value := range arrayValues(self) {
		entry, ok := arrayDataOf(value)
		if !ok || len(entry.values) == 0 {
			continue
		}
		if valuesEqual(entry.values[0], args[0]) {
			return value, nil
		}
	}
	return Nil, nil
}

func (s *State) arrayRassocMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	for _, value := range arrayValues(self) {
		entry, ok := arrayDataOf(value)
		if !ok || len(entry.values) < 2 {
			continue
		}
		if valuesEqual(entry.values[1], args[0]) {
			return value, nil
		}
	}
	return Nil, nil
}

func (s *State) arrayAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	values := arrayValues(self)
	index := int(args[0].Int())
	if index < 0 {
		index += len(values)
	}
	if index < 0 || index >= len(values) {
		return Nil, nil
	}
	return values[index], nil
}

func (s *State) arrayUniqValues(ctx context.Context, values []Value, block Value) ([]Value, error) {
	result := make([]Value, 0, len(values))
	keys := make([]Value, 0, len(values))
	for _, value := range values {
		key := value
		if !block.IsNil() {
			var err error
			key, err = s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return nil, err
			}
		}
		duplicate := false
		for _, existing := range keys {
			if arrayEql(existing, key) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			keys = append(keys, key)
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *State) arrayUniqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values, err := s.arrayUniqValues(ctx, arrayValues(self), block)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) arrayUniqBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	values, err := s.arrayUniqValues(ctx, data.values, block)
	if err != nil {
		return Nil, err
	}
	if len(values) == len(data.values) {
		return Nil, nil
	}
	data.values = values
	return self, nil
}

func (s *State) arrayArgumentValues(arg Value) ([]Value, error) {
	data, ok := arrayDataOf(arg)
	if !ok {
		return nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Array")
	}
	return data.values, nil
}

func (s *State) arrayContainsEql(values []Value, target Value) bool {
	for _, value := range values {
		if arrayEql(value, target) {
			return true
		}
	}
	return false
}

func (s *State) arraySubtractMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	other, err := s.arrayArgumentValues(args[0])
	if err != nil {
		return Nil, err
	}
	var result []Value
	for _, value := range arrayValues(self) {
		if !s.arrayContainsEql(other, value) {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayUnionOpMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	other, err := s.arrayArgumentValues(args[0])
	if err != nil {
		return Nil, err
	}
	all := make([]Value, 0, len(arrayValues(self))+len(other))
	all = append(all, arrayValues(self)...)
	all = append(all, other...)
	values, err := s.arrayUniqValues(ctx, all, Nil)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) arrayIntersectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	other, err := s.arrayArgumentValues(args[0])
	if err != nil {
		return Nil, err
	}
	var result []Value
	for _, value := range arrayValues(self) {
		if s.arrayContainsEql(other, value) && !s.arrayContainsEql(result, value) {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayUnionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	all := append([]Value(nil), arrayValues(self)...)
	for _, arg := range args {
		values, err := s.arrayArgumentValues(arg)
		if err != nil {
			return Nil, err
		}
		all = append(all, values...)
	}
	values, err := s.arrayUniqValues(ctx, all, Nil)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) arrayDifferenceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	excluded := make([][]Value, len(args))
	for i, arg := range args {
		values, err := s.arrayArgumentValues(arg)
		if err != nil {
			return Nil, err
		}
		excluded[i] = values
	}
	var result []Value
	for _, value := range arrayValues(self) {
		drop := false
		for _, values := range excluded {
			if s.arrayContainsEql(values, value) {
				drop = true
				break
			}
		}
		if !drop {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayIntersectionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return ArrayValue(append([]Value(nil), arrayValues(self)...)), nil
	}
	others := make([][]Value, len(args))
	for i, arg := range args {
		values, err := s.arrayArgumentValues(arg)
		if err != nil {
			return Nil, err
		}
		others[i] = values
	}
	var result []Value
	for _, value := range arrayValues(self) {
		present := true
		for _, values := range others {
			if !s.arrayContainsEql(values, value) {
				present = false
				break
			}
		}
		if present && !s.arrayContainsEql(result, value) {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayIntersectPMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	other, err := s.arrayArgumentValues(args[0])
	if err != nil {
		return Nil, err
	}
	for _, value := range arrayValues(self) {
		if s.arrayContainsEql(other, value) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayCompactBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	result := make([]Value, 0, len(data.values))
	for _, value := range data.values {
		if !value.IsNil() {
			result = append(result, value)
		}
	}
	if len(result) == len(data.values) {
		return Nil, nil
	}
	data.values = result
	return self, nil
}

func (s *State) arrayFetchIndex(values []Value, index int) (Value, bool) {
	if index < 0 {
		index += len(values)
	}
	if index < 0 || index >= len(values) {
		return Nil, false
	}
	return values[index], true
}

func (s *State) arrayFetchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	values := arrayValues(self)
	index := int(args[0].Int())
	if value, ok := s.arrayFetchIndex(values, index); ok {
		return value, nil
	}
	if !block.IsNil() {
		return s.callBlock1(ctx, block, args[0], Nil)
	}
	if len(args) == 2 {
		return args[1], nil
	}
	return Nil, s.newArrayError(s.IndexErrorClass, fmt.Sprintf("index %d outside of array bounds: %d...%d", index, -len(values), len(values)))
}

func (s *State) arrayFetchValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	result := make([]Value, len(args))
	for i, arg := range args {
		if arg.Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		index := int(arg.Int())
		if value, ok := s.arrayFetchIndex(values, index); ok {
			result[i] = value
			continue
		}
		if block.IsNil() {
			return Nil, s.newArrayError(s.IndexErrorClass, fmt.Sprintf("index %d outside of array bounds: %d...%d", index, -len(values), len(values)))
		}
		value, err := s.callBlock1(ctx, block, arg, Nil)
		if err != nil {
			return Nil, err
		}
		result[i] = value
	}
	return ArrayValue(result), nil
}

func (s *State) arrayFillMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	rest := args
	fillValue := Nil
	haveValue := false
	if len(args) > 0 {
		first := args[0]
		isPosition := first.Kind() == KindInt
		if _, ok := rangeDataOf(first); ok {
			isPosition = true
		}
		if block.IsNil() || !isPosition {
			fillValue = first
			haveValue = true
			rest = args[1:]
		}
	}
	if !haveValue && block.IsNil() {
		return Nil, s.newArrayError(s.ArgumentErrorClass, "wrong number of arguments (given 0, expected 1+)")
	}
	start := 0
	length := -1
	if len(rest) > 0 {
		if rangeValue, ok := rangeDataOf(rest[0]); ok {
			if len(rest) > 1 {
				return Nil, argumentCountError(s, len(rest), "1")
			}
			length = len(data.values)
			rangeStart, rangeLength := rangeBounds(rangeValue, length)
			start = rangeStart
			length = rangeLength
			if start < 0 {
				start = 0
			}
			if length < 0 {
				length = 0
			}
		} else {
			if rest[0].Kind() != KindInt {
				return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
			}
			start = int(rest[0].Int())
			if len(rest) >= 2 {
				if rest[1].Kind() != KindInt {
					return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
				}
				length = int(rest[1].Int())
			}
			if len(rest) > 2 {
				return Nil, argumentCountError(s, len(rest), "1..2")
			}
		}
	}
	if start < 0 {
		start += len(data.values)
		if start < 0 {
			start = 0
		}
	}
	end := 0
	if length < 0 {
		end = len(data.values)
		if start >= end {
			end = start + 1
		}
	} else {
		if start > int(^uint(0)>>1)-length {
			return Nil, s.newArrayError(s.RangeErrorClass, "index too big for array")
		}
		end = start + length
	}
	if end > len(data.values)+1<<20 {
		return Nil, s.newArrayError(s.RangeErrorClass, "index too big for array")
	}
	if err := s.checkArraySize(end); err != nil {
		return Nil, err
	}
	for len(data.values) < end {
		data.values = append(data.values, Nil)
	}
	if block.IsNil() {
		for i := start; i < end; i++ {
			data.values[i] = fillValue
		}
		return self, nil
	}
	for i := start; i < end; i++ {
		value, err := s.callBlock1(ctx, block, IntValue(int64(i)), Nil)
		if err != nil {
			return Nil, err
		}
		data.values[i] = value
	}
	return self, nil
}

func (s *State) arrayReverseEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "reverse_each", nil), nil
	}
	values := arrayValues(self)
	for i := len(values) - 1; i >= 0; i-- {
		if _, err := s.callBlock1(ctx, block, values[i], Nil); err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arrayRotateValues(values []Value, amount int) []Value {
	if len(values) == 0 {
		return nil
	}
	amount %= len(values)
	if amount < 0 {
		amount += len(values)
	}
	result := make([]Value, 0, len(values))
	result = append(result, values[amount:]...)
	result = append(result, values[:amount]...)
	return result
}

func (s *State) arrayRotateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	amount := 1
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		amount = int(args[0].Int())
	}
	return ArrayValue(s.arrayRotateValues(arrayValues(self), amount)), nil
}

func (s *State) arrayRotateBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	amount := 1
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		amount = int(args[0].Int())
	}
	data.values = s.arrayRotateValues(data.values, amount)
	return self, nil
}

func (s *State) arrayFilterBang(ctx context.Context, self Value, block Value, keep bool, reportChange bool) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	result := make([]Value, 0, len(data.values))
	for _, value := range data.values {
		match, err := s.callBlock1(ctx, block, value, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() == keep {
			result = append(result, value)
		}
	}
	if reportChange && len(result) == len(data.values) {
		return Nil, nil
	}
	data.values = result
	return self, nil
}

func (s *State) arrayDeleteIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "delete_if", nil), nil
	}
	return s.arrayFilterBang(ctx, self, block, false, false)
}

func (s *State) arrayRejectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "reject!", nil), nil
	}
	return s.arrayFilterBang(ctx, self, block, false, true)
}

func (s *State) arrayKeepIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "keep_if", nil), nil
	}
	return s.arrayFilterBang(ctx, self, block, true, false)
}

func (s *State) arraySelectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "select!", nil), nil
	}
	return s.arrayFilterBang(ctx, self, block, true, true)
}

func (s *State) arrayInsertMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	if len(args) == 0 {
		return Nil, argumentCountError(s, 0, "1+")
	}
	if args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	index := int(args[0].Int())
	if index < 0 {
		index += len(data.values) + 1
		if index < 0 {
			return Nil, s.newArrayError(s.IndexErrorClass, "index too small for array")
		}
	}
	if index > int(^uint(0)>>1)-len(args) {
		return Nil, s.newArrayError(s.RangeErrorClass, "index too big for array")
	}
	finalLen := index
	if len(data.values) > finalLen {
		finalLen = len(data.values)
	}
	finalLen += len(args) - 1
	if err := s.checkArraySize(finalLen); err != nil {
		return Nil, err
	}
	if index > len(data.values) {
		if index-len(data.values) > 1<<20 {
			return Nil, s.newArrayError(s.IndexErrorClass, "index too big for array")
		}
		for len(data.values) < index {
			data.values = append(data.values, Nil)
		}
	}
	inserted := append([]Value(nil), args[1:]...)
	rest := append([]Value(nil), data.values[index:]...)
	data.values = append(data.values[:index], inserted...)
	data.values = append(data.values, rest...)
	return self, nil
}

func (s *State) arrayBsearchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "bsearch", nil), nil
	}
	values := arrayValues(self)
	low, high := 0, len(values)
	findAny := false
	for low < high {
		mid := low + (high-low)/2
		result, err := s.callBlock1(ctx, block, values[mid], Nil)
		if err != nil {
			return Nil, err
		}
		switch result.Kind() {
		case KindTrue, KindFalse, KindNil:
			if findAny {
				return Nil, s.newArrayError(s.TypeErrorClass, "invalid block result (must be numeric, true, false or nil)")
			}
			if result.IsTruthy() {
				high = mid
			} else {
				low = mid + 1
			}
		case KindInt, KindFloat, KindBigInt:
			findAny = true
			cmp := 0
			switch result.Kind() {
			case KindInt:
				if result.Int() < 0 {
					cmp = -1
				} else if result.Int() > 0 {
					cmp = 1
				}
			case KindFloat:
				if result.Float() < 0 {
					cmp = -1
				} else if result.Float() > 0 {
					cmp = 1
				}
			}
			if cmp == 0 {
				return values[mid], nil
			}
			if cmp < 0 {
				high = mid
			} else {
				low = mid + 1
			}
		default:
			return Nil, s.newArrayError(s.TypeErrorClass, "invalid block result (must be numeric, true, false or nil)")
		}
	}
	if findAny || low >= len(values) {
		return Nil, nil
	}
	return values[low], nil
}

func (s *State) arrayValuesAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	var result []Value
	for _, arg := range args {
		if arg.Kind() == KindInt {
			value, _ := s.arrayFetchIndex(values, int(arg.Int()))
			result = append(result, value)
			continue
		}
		rangeValue, ok := rangeDataOf(arg)
		if !ok {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		start, end := 0, len(values)-1
		if !rangeValue.low.IsNil() && rangeValue.low.Kind() == KindInt {
			start = int(rangeValue.low.Int())
			if start < 0 {
				start += len(values)
			}
		}
		if !rangeValue.high.IsNil() && rangeValue.high.Kind() == KindInt {
			end = int(rangeValue.high.Int())
			if end < 0 {
				end += len(values)
			}
			if rangeValue.exclusive {
				end--
			}
		}
		for i := start; i <= end; i++ {
			value, _ := s.arrayFetchIndex(values, i)
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayToHMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	var entries []hashEntry
	for _, value := range arrayValues(self) {
		pair, ok := arrayDataOf(value)
		if !ok {
			return Nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("wrong element type %s (expected array)", s.classOf(value).name))
		}
		if len(pair.values) != 2 {
			return Nil, s.newArrayError(s.ArgumentErrorClass, fmt.Sprintf("wrong array length (expected 2, was %d)", len(pair.values)))
		}
		found := false
		for i := range entries {
			if valuesEqual(entries[i].key, pair.values[0]) {
				entries[i].value = pair.values[1]
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, hashEntry{key: pair.values[0], value: pair.values[1]})
		}
	}
	return HashValue(entries), nil
}

func (s *State) arraySliceBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not an array")
	}
	if len(args) == 0 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	start, length := 0, 0
	if len(args) == 1 {
		if rangeValue, ok := rangeDataOf(args[0]); ok {
			start, length = rangeBounds(rangeValue, len(data.values))
			if start < 0 || start > len(data.values) {
				return Nil, nil
			}
			if length == 0 {
				return ArrayValue(nil), nil
			}
		} else {
			if args[0].Kind() != KindInt {
				return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
			}
			index := int(args[0].Int())
			if index < 0 {
				index += len(data.values)
			}
			if index < 0 || index >= len(data.values) {
				return Nil, nil
			}
			removed := data.values[index]
			data.values = append(data.values[:index], data.values[index+1:]...)
			return removed, nil
		}
	} else {
		if args[0].Kind() != KindInt || args[1].Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		start = int(args[0].Int())
		length = int(args[1].Int())
		if length < 0 {
			return Nil, nil
		}
		if start < 0 {
			start += len(data.values)
		}
		if start < 0 || start > len(data.values) {
			return Nil, nil
		}
	}
	if start+length > len(data.values) {
		length = len(data.values) - start
	}
	if length < 0 {
		length = 0
	}
	removed := append([]Value(nil), data.values[start:start+length]...)
	data.values = append(data.values[:start], data.values[start+length:]...)
	if len(args) == 1 {
		return ArrayValue(removed), nil
	}
	return ArrayValue(removed), nil
}

func (s *State) arrayPermutationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "permutation", nil), nil
	}
	values := arrayValues(self)
	count := len(values)
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		count = int(args[0].Int())
	}
	if count < 0 || count > len(values) {
		return self, nil
	}
	var emit func(prefix, used []int)
	emit = func(prefix, used []int) {
		if len(prefix) == count {
			out := make([]Value, len(prefix))
			for i, index := range prefix {
				out[i] = values[index]
			}
			_, _ = s.callBlock1(ctx, block, ArrayValue(out), Nil)
			return
		}
		for i := range values {
			skip := false
			for _, existing := range used {
				if existing == i {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			emit(append(prefix, i), append(used, i))
		}
	}
	emit(nil, nil)
	return self, nil
}

func (s *State) arrayCombinationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "combination", nil), nil
	}
	if len(args) != 1 || args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	values := arrayValues(self)
	count := int(args[0].Int())
	if count < 0 || count > len(values) {
		return self, nil
	}
	var emit func(start int, prefix []Value)
	emit = func(start int, prefix []Value) {
		if len(prefix) == count {
			_, _ = s.callBlock1(ctx, block, ArrayValue(append([]Value(nil), prefix...)), Nil)
			return
		}
		for i := start; i < len(values); i++ {
			emit(i+1, append(prefix, values[i]))
		}
	}
	emit(0, nil)
	return self, nil
}

func (s *State) arrayRepeatedCombinationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	if block.IsNil() {
		return s.toEnum(self, "repeated_combination", nil), nil
	}
	values := arrayValues(self)
	count := int(args[0].Int())
	if count < 0 {
		return self, nil
	}
	var emit func(start int, prefix []Value)
	emit = func(start int, prefix []Value) {
		if len(prefix) == count {
			_, _ = s.callBlock1(ctx, block, ArrayValue(append([]Value(nil), prefix...)), Nil)
			return
		}
		for i := start; i < len(values); i++ {
			emit(i, append(prefix, values[i]))
		}
	}
	emit(0, nil)
	return self, nil
}

func (s *State) arrayRepeatedPermutationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if args[0].Kind() != KindInt {
		return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	if block.IsNil() {
		return s.toEnum(self, "repeated_permutation", nil), nil
	}
	values := arrayValues(self)
	count := int(args[0].Int())
	if count < 0 {
		return self, nil
	}
	prefix := make([]Value, 0, count)
	var emit func(remaining int)
	emit = func(remaining int) {
		if remaining == 0 {
			_, _ = s.callBlock1(ctx, block, ArrayValue(append([]Value(nil), prefix...)), Nil)
			return
		}
		for _, value := range values {
			prefix = append(prefix, value)
			emit(remaining - 1)
			prefix = prefix[:len(prefix)-1]
		}
	}
	emit(count)
	return self, nil
}

func (s *State) arrayProductMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	lists := make([][]Value, 0, len(args)+1)
	lists = append(lists, arrayValues(self))
	for _, arg := range args {
		values, err := s.arrayArgumentValues(arg)
		if err != nil {
			return Nil, err
		}
		lists = append(lists, values)
	}
	var result []Value
	current := make([]Value, len(lists))
	var emit func(depth int)
	emit = func(depth int) {
		if depth == len(lists) {
			combination := append([]Value(nil), current...)
			if block.IsNil() {
				result = append(result, ArrayValue(combination))
			} else {
				_, _ = s.callBlock1(ctx, block, ArrayValue(combination), Nil)
			}
			return
		}
		for _, value := range lists[depth] {
			current[depth] = value
			emit(depth + 1)
		}
	}
	emit(0)
	if block.IsNil() {
		return ArrayValue(result), nil
	}
	return self, nil
}

func (s *State) arrayRfindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "rfind", nil), nil
	}
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	values := arrayValues(self)
	for i := len(values) - 1; i >= 0; i-- {
		match, err := s.callBlock1(ctx, block, values[i], Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() {
			return values[i], nil
		}
	}
	if len(args) == 1 && !args[0].IsNil() {
		return s.call(ctx, args[0], "call", nil, Nil, Nil)
	}
	return Nil, nil
}
