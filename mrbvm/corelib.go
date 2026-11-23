package mrbvm

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"
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
	s.installSymbolLibrary()
	s.installProcLibrary()
	s.installRangeLibrary()
	s.installAdvancedCoreLibrary()
}

func (s *State) installArrayLibrary() {
	class := s.ArrayClass
	class.define("==", s.arrayEqualMethod)
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		other, ok := arrayDataOf(args[0])
		if !ok {
			return False, nil
		}
		values := arrayValues(self)
		if len(values) != len(other.values) {
			return False, nil
		}
		if self.obj != nil && args[0].obj != nil {
			if self.obj == args[0].obj {
				return True, nil
			}
			pair := objectPair{left: self.obj, right: args[0].obj}
			if s.eqSeen[pair] {
				return True, nil
			}
			s.eqSeen[pair] = true
			defer delete(s.eqSeen, pair)
		}
		for i := range values {
			equal, err := s.rubyEql(ctx, values[i], other.values[i])
			if err != nil {
				return Nil, err
			}
			if !equal {
				return False, nil
			}
		}
		return True, nil
	})
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
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
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
	class.define("flatten!", s.arrayFlattenBangMethod)
	class.define("uniq", s.arrayUniqMethod)
	class.define("compact", s.arrayCompactMethod)
	class.define("each_index", s.arrayEachIndexMethod)
	class.define("clear", s.arrayClearMethod)
	class.define("map!", s.arrayMapBangMethod)
	class.define("collect!", s.arrayMapBangMethod)
	class.define("replace", s.arrayReplaceMethod)
	class.define("initialize", s.arrayInitializeMethod)
	class.define("initialize_copy", s.arrayInitializeCopyMethod)
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
	class.define("hash", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(hashValue(self, map[*Object]bool{})), nil
	})
	class.define("<=>", s.arrayCompareMethod)
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
	class.define("default", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) > 1 {
			return Nil, argumentCountError(s, len(args), "0..1")
		}
		data, ok := hashDataOf(self)
		if !ok {
			return Nil, nil
		}
		if !data.defaultProc.IsNil() {
			if len(args) == 0 {
				return Nil, nil
			}
			return s.callBlock(ctx, data.defaultProc, []Value{self, args[0]}, Nil)
		}
		return data.defaultValue, nil
	})
	class.define("default_proc", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if !ok || data.defaultProc.IsNil() {
			return Nil, nil
		}
		return data.defaultProc, nil
	})
	class.define("default_proc=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if ok && len(args) == 1 {
			data.defaultProc = args[0]
		}
		if len(args) == 1 {
			return args[0], nil
		}
		return Nil, nil
	})
	class.define("default=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		if self.obj != nil && len(args) == 1 {
			if data, ok := self.obj.data.(*hashData); ok {
				data.defaultValue = args[0]
			}
		}
		if len(args) == 1 {
			return args[0], nil
		}
		return Nil, nil
	})
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
	class.define("compare_by_identity", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if ok && data.identity {
			return self, nil
		}
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		if !ok {
			return self, nil
		}
		data.identity = true
		data.version++
		data.index = nil
		data.indexBuilt = 0
		return self, nil
	})
	class.define("compare_by_identity?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		return BoolValue(ok && data.identity), nil
	})
	class.define("rehash", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if !ok {
			return self, nil
		}
		snapshot := append([]hashEntry(nil), data.entries...)
		version := data.version
		rebuilt := &hashData{entries: make([]hashEntry, 0, len(snapshot))}
		for _, entry := range snapshot {
			index, found, err := s.hashLookupKey(ctx, rebuilt, entry.key)
			if err != nil {
				return Nil, err
			}
			if data.version != version {
				return Nil, s.hashModifiedError()
			}
			if found {
				rebuilt.entries[index] = entry
				continue
			}
			rebuilt.entries = appendHashEntry(rebuilt.entries, entry)
			addHashIndex(rebuilt, len(rebuilt.entries)-1)
		}
		if data.version != version {
			return Nil, s.hashModifiedError()
		}
		data.entries = rebuilt.entries
		data.version++
		data.index = nil
		data.indexBuilt = 0
		return self, nil
	})
	class.define("merge!", s.hashMergeBangMethod)
	class.define("update", s.hashMergeBangMethod)
	class.define("store", s.hashSetMethod)
	class.define("assoc", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		for _, entry := range data.entries {
			version := data.version
			equal, err := s.rubyEql(ctx, entry.key, args[0])
			if err != nil {
				return Nil, err
			}
			if data.version != version {
				return Nil, s.hashModifiedError()
			}
			if equal {
				return ArrayValue([]Value{entry.key, entry.value}), nil
			}
		}
		return Nil, nil
	})
	class.define("rassoc", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := hashDataOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		for _, entry := range data.entries {
			version := data.version
			equal, err := s.rubyEql(ctx, entry.value, args[0])
			if err != nil {
				return Nil, err
			}
			if data.version != version {
				return Nil, s.hashModifiedError()
			}
			if equal {
				return ArrayValue([]Value{entry.key, entry.value}), nil
			}
		}
		return Nil, nil
	})
	class.define("shift", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		data, ok := hashDataOf(self)
		if !ok || len(data.entries) == 0 {
			return Nil, nil
		}
		entry := data.entries[0]
		data.entries = data.entries[1:]
		data.version++
		data.index = nil
		data.indexBuilt = 0
		return ArrayValue([]Value{entry.key, entry.value}), nil
	})
	class.define("replace", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "replace expects one argument")}
		}
		data, ok := hashDataOf(self)
		other, otherOK := hashDataOf(args[0])
		if !ok || !otherOK {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a hash")}
		}
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		data.entries = append([]hashEntry(nil), other.entries...)
		data.version++
		data.defaultValue = other.defaultValue
		data.defaultProc = other.defaultProc
		data.identity = other.identity
		data.index = nil
		data.indexBuilt = 0
		return self, nil
	})
	class.define("clear", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		if data, ok := hashDataOf(self); ok {
			data.entries = nil
			data.version++
			data.index = nil
			data.indexBuilt = 0
		}
		return self, nil
	})
	class.define("==", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		equal, err := s.hashCompare(ctx, self, args[0], false)
		if err != nil {
			return Nil, err
		}
		return BoolValue(equal), nil
	})
	class.define("delete", s.hashDeleteMethod)
	class.define("__delete", s.hashDeleteMethod)
	class.define("invert", s.hashInvertMethod)
	class.define("values_at", s.hashValuesAtMethod)
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.Inspect()), nil
	})
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		equal, err := s.hashCompare(ctx, self, args[0], true)
		if err != nil {
			return Nil, err
		}
		return BoolValue(equal), nil
	})
	class.define("hash", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(hashValue(self, map[*Object]bool{})), nil
	})
	class.define("<=>", s.arrayCompareMethod)
	class.define("inspect", s.hashInspectMethod)
}

func (s *State) installStringLibrary() {
	class := s.StringClass
	class.define("inspect", s.stringInspectMethod)
	class.define("==", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() == KindSymbol {
			return False, nil
		}
		left, ok := stringPayload(self)
		if !ok {
			return False, nil
		}
		right, ok := stringPayload(args[0])
		if !ok {
			return False, nil
		}
		return BoolValue(left == right), nil
	})
	class.define("!=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() == KindSymbol {
			return True, nil
		}
		left, ok := stringPayload(self)
		if !ok {
			return True, nil
		}
		right, ok := stringPayload(args[0])
		if !ok {
			return True, nil
		}
		return BoolValue(left != right), nil
	})
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
	class.define("each_char", s.stringEachCharMethod)
	class.define("each_byte", s.stringEachByteMethod)
	class.define("casecmp", s.stringCasecmpMethod)
	class.define("casecmp?", s.stringCasecmpPredicateMethod)
	class.define("start_with?", s.stringStartWithMethod)
	class.define("end_with?", s.stringEndWithMethod)
	class.define("index", s.stringIndexMethod)
	class.define("rindex", s.stringRindexMethod)
	class.define("slice", s.stringSliceMethod)
	class.define("[]", s.stringSliceMethod)
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		base := 10
		if len(args) > 0 {
			if args[0].Kind() != KindInt {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "base must be an integer")}
			}
			base = int(args[0].Int())
			if base != 0 && (base < 2 || base > 36) {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid radix")}
			}
		}
		return parseRubyInteger(self.String(), base), nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return FloatValue(parseRubyFloat(self.String())), nil
	})
	class.define("chars", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := self.String()
		spans := stringCharSpans(text)
		values := make([]Value, len(spans))
		for i, span := range spans {
			values[i] = StringValue(text[span.start:span.end])
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
		text := self.String()
		spans := stringCharSpans(text)
		var builder strings.Builder
		for i := len(spans) - 1; i >= 0; i-- {
			builder.WriteString(text[spans[i].start:spans[i].end])
		}
		return StringValue(builder.String()), nil
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
	class.define("=~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, nil
		}
		if _, ok := regexpOf(args[0]); !ok {
			return Nil, nil
		}
		return s.regexpMatchOperatorMethod(ctx, args[0], []Value{self}, kw, block)
	})
	class.define("match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 || len(args) > 2 {
			return Nil, nil
		}
		if _, ok := regexpOf(args[0]); !ok {
			return Nil, nil
		}
		callArgs := append([]Value{self}, args[1:]...)
		return s.regexpMatchMethod(ctx, args[0], callArgs, kw, block)
	})
	class.define("scan", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return ArrayValue(nil), nil
		}
		if _, ok := regexpOf(args[0]); !ok {
			return ArrayValue(nil), nil
		}
		data, err := s.regexpCompiled(args[0])
		if err != nil {
			return Nil, err
		}
		matches, err := s.regexpAllMatches(args[0], data, self.String())
		if err != nil {
			return Nil, err
		}
		if len(matches) > 0 {
			s.regexpSetGlobals(matches[len(matches)-1])
		} else {
			s.regexpClearGlobals()
		}
		values := make([]Value, 0, len(matches))
		for _, match := range matches {
			var value Value
			if len(match.groups) > 2 {
				captures := make([]Value, len(match.groups)-1)
				for i := 1; i < len(match.groups); i++ {
					if match.matched[i] {
						captures[i-1] = StringValue(match.groups[i])
					} else {
						captures[i-1] = Nil
					}
				}
				value = ArrayValue(captures)
			} else if len(match.groups) == 2 {
				if match.matched[1] {
					value = StringValue(match.groups[1])
				} else {
					value = Nil
				}
			} else if match.matched[0] {
				value = StringValue(match.groups[0])
			} else {
				value = Nil
			}
			if !block.IsNil() {
				if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
					return Nil, err
				}
				continue
			}
			values = append(values, value)
		}
		if !block.IsNil() {
			return self, nil
		}
		return ArrayValue(values), nil
	})
}

func (s *State) installIntegerLibrary() {
	class := s.IntegerClass
	class.define("times", s.integerTimesMethod)
	class.define("upto", s.integerUptoMethod)
	class.define("downto", s.integerDowntoMethod)
	s.NumericClass.define("step", s.numericStepMethod)
	class.define("abs", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return normalizedBigInt(new(big.Int).Abs(value)), nil
		}
		value := self.Int()
		if value == math.MinInt64 {
			return normalizedBigInt(new(big.Int).Neg(big.NewInt(value))), nil
		}
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
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return normalizedBigInt(new(big.Int).Add(value, big.NewInt(1))), nil
		}
		if value, ok := addInt64(self.Int(), 1); ok {
			return IntValue(value), nil
		}
		return normalizedBigInt(new(big.Int).Add(big.NewInt(self.Int()), big.NewInt(1))), nil
	})
	class.define("next", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return normalizedBigInt(new(big.Int).Add(value, big.NewInt(1))), nil
		}
		if value, ok := addInt64(self.Int(), 1); ok {
			return IntValue(value), nil
		}
		return normalizedBigInt(new(big.Int).Add(big.NewInt(self.Int()), big.NewInt(1))), nil
	})
	class.define("pred", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return normalizedBigInt(new(big.Int).Sub(value, big.NewInt(1))), nil
		}
		if value, ok := subInt64(self.Int(), 1); ok {
			return IntValue(value), nil
		}
		return normalizedBigInt(new(big.Int).Sub(big.NewInt(self.Int()), big.NewInt(1))), nil
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
	class.define("lcm", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "lcm expects an integer")}
		}
		a, b := self.Int(), args[0].Int()
		if a == 0 || b == 0 {
			return IntValue(0), nil
		}
		absA := new(big.Int).Abs(big.NewInt(a))
		absB := new(big.Int).Abs(big.NewInt(b))
		gcd := new(big.Int).GCD(nil, nil, absA, absB)
		quotient := new(big.Int).Quo(big.NewInt(a), gcd)
		lcm := new(big.Int).Abs(new(big.Int).Mul(quotient, big.NewInt(b)))
		if !lcm.IsInt64() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "integer overflow in lcm")}
		}
		return IntValue(lcm.Int64()), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) == 0 {
			return StringValue(self.ToS()), nil
		}
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "base must be an integer")}
		}
		var base int
		switch args[0].Kind() {
		case KindInt:
			base = int(args[0].Int())
		case KindBigInt:
			value, _ := args[0].BigInt()
			if !value.IsInt64() {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid radix")}
			}
			base = int(value.Int64())
		case KindFloat:
			value := args[0].Float()
			if math.IsNaN(value) || math.IsInf(value, 0) || value < math.MinInt32 || value > math.MaxInt32 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "base must be an integer")}
			}
			base = int(value)
		default:
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "base must be an integer")}
		}
		if base < 2 || base > 36 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("invalid radix %d", base))}
		}
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			return StringValue(value.Text(base)), nil
		}
		return StringValue(strconv.FormatInt(self.Int(), base)), nil
	})
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.Kind() == KindBigInt {
			value, _ := self.BigInt()
			floatValue, _ := new(big.Float).SetInt(value).Float64()
			return FloatValue(floatValue), nil
		}
		return FloatValue(float64(self.Int())), nil
	})
	class.define("chr", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(string(rune(self.Int()))), nil
	})
	class.classMethods["__ensure"] = &method{owner: class, name: "__ensure", native: s.integerEnsureMethod}
}

// integerEnsureMethod implements Integer.__ensure, the non-dispatching
// conversion used by mrblib. It must not call to_int or __to_int.
func (s *State) integerEnsureMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	value := args[0]
	switch value.Kind() {
	case KindInt, KindBigInt:
		return value, nil
	case KindFloat:
		f := value.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FloatDomainErrorClass, "Infinity")}
		}
		integer, _ := new(big.Float).SetFloat64(f).Int(nil)
		return normalizedBigInt(integer), nil
	}
	name := "Object"
	if class := s.classOf(value); class != nil && class.name != "" {
		name = class.name
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert "+name+" into Integer")}
}

func (s *State) objectDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if s.definedSet {
		kind := s.definedKind
		isNil := s.definedNil
		s.definedSet = false
		s.definedKind = ""
		s.definedNil = false
		if isNil {
			return Nil, nil
		}
		if kind != "" {
			return StringValue(kind), nil
		}
	}
	if len(args) == 0 || args[0].IsNil() {
		return Nil, nil
	}
	return StringValue("expression"), nil
}

func (s *State) installObjectLibrary() {
	class := s.ObjectClass
	class.define("defined?", s.objectDefinedMethod)
	s.BasicObjectClass.define("defined?", s.objectDefinedMethod)
	class.define("is_a?", s.objectIsAMethod)
	class.define("kind_of?", s.objectIsAMethod)
	class.define("instance_of?", s.objectInstanceOfMethod)
	class.define("equal?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		return BoolValue(identicalValue(self, args[0])), nil
	})
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && eqlValues(self, args[0])), nil
	})
	class.define("hash", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(hashValue(self, map[*Object]bool{})), nil
	})
	class.define("==", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		if self.Kind() == KindObject && args[0].Kind() == KindObject {
			return BoolValue(self.obj == args[0].obj), nil
		}
		return BoolValue(valuesEqual(self, args[0])), nil
	})
	class.define("!=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return True, nil
		}
		if self.Kind() == KindObject && args[0].Kind() == KindObject {
			return BoolValue(self.obj != args[0].obj), nil
		}
		return BoolValue(!valuesEqual(self, args[0])), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(self.ToS()), nil
	})
	class.define("inspect", s.objectInspectMethod)
	class.define("<=>", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, nil
		}
		if cmp, ok := compareNative(self, args[0]); ok {
			return IntValue(int64(cmp)), nil
		}
		return Nil, nil
	})
	class.define("__id__", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 0 || !kw.IsNil() {
			return Nil, argumentCountError(s, len(args), "0")
		}
		return IntValue(objectIdentityID(self)), nil
	})
	class.define("object_id", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 0 {
			return Nil, argumentCountError(s, len(args), "0")
		}
		return IntValue(objectIdentityID(self)), nil
	})
	class.define("instance_eval", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return self, nil
		}
		return s.callBlockWithSelf(ctx, block, self, nil, Nil)
	})
	class.define("instance_exec", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
		}
		return s.callBlockWithSelf(ctx, block, self, args, kw)
	})
	class.define("_str_match?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return False, nil
		}
		return BoolValue(globMatch(args[0].String(), args[1].String())), nil
	})
	class.define("send", s.objectSendMethod)
	class.define("__send__", s.objectSendMethod)
	class.define("!~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return True, nil
		}
		result, err := s.call(ctx, self, "=~", args, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		return BoolValue(!result.IsTruthy()), nil
	})
	class.define("tap", s.objectTapMethod)
	s.SymbolClass.define("to_sym", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.SymbolClass.define("to_proc", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		name := self.String()
		return nativeBlockValue(s.ProcClass, func(ctx context.Context, args []Value) (Value, error) {
			if len(args) == 0 {
				return Nil, nil
			}
			return s.call(ctx, args[0], name, args[1:], Nil, Nil)
		}), nil
	})
	s.ProcClass.define("[]", s.procCallMethod)
	s.ProcClass.define("arity", s.procArityMethod)
	s.ProcClass.define("==", s.procEqualMethod)
	s.ProcClass.define("eql?", s.procEqualMethod)
	s.ProcClass.define("hash", s.procHashMethod)
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
		return StringValue(formatRange(self, false)), nil
	})
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		left, ok := rangeDataOf(self)
		right, ok2 := rangeDataOf(args[0])
		if !ok || !ok2 || left.exclusive != right.exclusive {
			return False, nil
		}
		return BoolValue(eqlValues(left.low, right.low) && eqlValues(left.high, right.high)), nil
	})
	class.define("hash", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(hashValue(self, map[*Object]bool{})), nil
	})
	class.define("<=>", s.arrayCompareMethod)
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(formatRange(self, true)), nil
	})
	class.define("initialize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "'initialize' called twice")}
	})
}

func formatRange(self Value, inspect bool) string {
	data, ok := rangeDataOf(self)
	if !ok {
		return "#<Range>"
	}
	op := ".."
	if data.exclusive {
		op = "..."
	}
	low := ""
	if !data.low.IsNil() {
		if inspect {
			low = data.low.Inspect()
		} else {
			low = data.low.ToS()
		}
	}
	high := ""
	if !data.high.IsNil() {
		if inspect {
			high = data.high.Inspect()
		} else {
			high = data.high.ToS()
		}
	}
	return low + op + high
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
	if len(args) > 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	if len(args) > 0 {
		n, err := s.arrayLengthArg(args[0], "first")
		if err != nil {
			return Nil, err
		}
		return s.arrayTake(values, n, false)
	}
	if len(values) == 0 {
		return Nil, nil
	}
	return values[0], nil
}

func (s *State) arrayLastMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := arrayValues(self)
	if len(args) > 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	if len(args) > 0 {
		n, err := s.arrayLengthArg(args[0], "last")
		if err != nil {
			return Nil, err
		}
		return s.arrayTake(values, n, true)
	}
	if len(values) == 0 {
		return Nil, nil
	}
	return values[len(values)-1], nil
}

func (s *State) arrayLengthArg(value Value, method string) (int, error) {
	n, err := s.indexInteger(value)
	if err != nil {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, method+": no implicit conversion to integer")}
	}
	if n < 0 {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
	}
	return n, nil
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
		if len(args) == 1 {
			if identicalValue(value, args[0]) {
				return IntValue(int64(i)), nil
			}
			equal, err := s.rubyEqual(ctx, value, args[0])
			if err != nil {
				return Nil, err
			}
			if equal {
				return IntValue(int64(i)), nil
			}
		}
	}
	return Nil, nil
}

func (s *State) arrayRindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, _ := arrayDataOf(self)
	for attempt := 0; attempt < 2; attempt++ {
		values := arrayValues(self)
		for i := len(values) - 1; i >= 0; i-- {
			if len(args) == 1 {
				if identicalValue(values[i], args[0]) {
					return IntValue(int64(i)), nil
				}
				equal, err := s.rubyEqual(ctx, values[i], args[0])
				if err != nil {
					return Nil, err
				}
				if equal {
					return IntValue(int64(i)), nil
				}
				continue
			}
			if !block.IsNil() {
				match, err := s.callBlock(ctx, block, []Value{values[i]}, Nil)
				if err != nil {
					return Nil, err
				}
				if match.IsTruthy() {
					return IntValue(int64(i)), nil
				}
			}
		}
		if data == nil || len(data.values) == len(values) {
			break
		}
	}
	return Nil, nil
}

func (s *State) arrayJoinMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	separator := ""
	if len(args) > 0 && !args[0].IsNil() {
		separator = args[0].ToS()
	}
	joined, err := s.joinArrayValues(ctx, arrayDataOfValue(self), separator, map[*arrayData]bool{})
	if err != nil {
		return Nil, err
	}
	return StringValue(joined), nil
}

func (s *State) arrayDeleteAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
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
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "delete expects one argument")}
	}
	data, _ := arrayDataOf(self)
	values := arrayValues(self)
	originalLen := len(values)
	result := make([]Value, 0, len(values))
	var deleted Value
	found := false
	for _, value := range values {
		equal, err := s.rubyEqual(ctx, value, args[0])
		if err != nil {
			return Nil, err
		}
		if data != nil && len(data.values) != originalLen {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "can't modify array during iteration")}
		}
		if equal {
			deleted = value
			found = true
			continue
		}
		result = append(result, value)
	}
	if data != nil {
		data.values = result
	}
	if found {
		return deleted, nil
	}
	if !block.IsNil() {
		return s.callBlock(ctx, block, []Value{args[0]}, Nil)
	}
	return Nil, nil
}

func (s *State) arrayShiftMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	values := arrayValues(self)
	if len(args) == 0 {
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
	n, err := s.arrayLengthArg(args[0], "shift")
	if err != nil {
		return Nil, err
	}
	if n > len(values) {
		n = len(values)
	}
	result := append([]Value(nil), values[:n]...)
	if self.obj != nil {
		if data, ok := self.obj.data.(*arrayData); ok {
			data.values = values[n:]
		}
	}
	return ArrayValue(result), nil
}

func (s *State) arrayUnshiftMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	if err := s.checkArraySize(len(data.values) + len(args)); err != nil {
		return Nil, err
	}
	data.values = append(append([]Value(nil), args...), data.values...)
	return self, nil
}

func (s *State) arrayPopMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
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
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	values := arrayValues(self)
	total := len(values)
	for _, arg := range args {
		total += len(arrayValues(arg))
	}
	if err := s.checkArraySize(total); err != nil {
		return Nil, err
	}
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

func (s *State) arrayClearMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if data, ok := arrayDataOf(self); ok {
		data.values = nil
	}
	return self, nil
}

func (s *State) arrayMapBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	if block.IsNil() {
		return s.toEnum(self, "map!", nil), nil
	}
	for i, value := range data.values {
		mapped, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err != nil {
			return Nil, err
		}
		data.values[i] = mapped
	}
	return self, nil
}

func (s *State) arrayReplaceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "replace expects one array")}
	}
	source := append([]Value(nil), arrayValues(args[0])...)
	data.values = source
	return self, nil
}

func (s *State) arrayInitializeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	switch len(args) {
	case 0:
		data.values = nil
	case 1:
		count, err := s.arrayLengthArg(args[0], "initialize")
		if err != nil {
			return Nil, err
		}
		data.values = make([]Value, count)
		if !block.IsNil() {
			for i := range data.values {
				value, err := s.callBlock(ctx, block, []Value{IntValue(int64(i))}, Nil)
				if err != nil {
					return Nil, err
				}
				data.values[i] = value
			}
		}
	case 2:
		count, err := s.arrayLengthArg(args[0], "initialize")
		if err != nil {
			return Nil, err
		}
		data.values = make([]Value, count)
		for i := range data.values {
			data.values[i] = args[1]
		}
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	return self, nil
}

func (s *State) arrayInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "initialize_copy expects one array")}
	}
	data.values = append([]Value(nil), arrayValues(args[0])...)
	return self, nil
}

func (s *State) arrayFlattenValues(values []Value, level int, ancestors map[*arrayData]bool) ([]Value, error) {
	result := make([]Value, 0, len(values))
	for _, value := range values {
		if value.obj != nil {
			if arr, ok := value.obj.data.(*arrayData); ok && level != 0 {
				tracked := false
				if level < 0 {
					if ancestors[arr] {
						return nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "tried to flatten recursive array")}
					}
					ancestors[arr] = true
					tracked = true
				}
				nested, err := s.arrayFlattenValues(arr.values, level-1, ancestors)
				if tracked {
					delete(ancestors, arr)
				}
				if err != nil {
					return nil, err
				}
				result = append(result, nested...)
				continue
			}
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *State) arrayFlattenMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	depth := -1
	if len(args) > 0 && args[0].Kind() == KindInt {
		depth = int(args[0].Int())
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an array")}
	}
	ancestors := map[*arrayData]bool{data: true}
	values, err := s.arrayFlattenValues(data.values, depth, ancestors)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) arrayFlattenBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an array")}
	}
	depth := -1
	if len(args) > 0 && args[0].Kind() == KindInt {
		depth = int(args[0].Int())
	}
	ancestors := map[*arrayData]bool{data: true}
	values, err := s.arrayFlattenValues(data.values, depth, ancestors)
	if err != nil {
		return Nil, err
	}
	changed := len(values) != len(data.values)
	if !changed {
		for i := range values {
			if !identicalValue(values[i], data.values[i]) {
				changed = true
				break
			}
		}
	}
	if !changed {
		return Nil, nil
	}
	data.values = values
	return self, nil
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
		return s.toEnum(self, "each_index", nil), nil
	}
	for i := range arrayValues(self) {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(int64(i))}, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) arraySliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.arrayIndexMethod(ctx, self, args, kw, block)
}

func (s *State) arraySelectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "select", nil), nil
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
		return s.toEnum(self, "reject", nil), nil
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
		return s.toEnum(self, "find", args), nil
	}
	var ifnone Value
	if len(args) > 0 {
		ifnone = args[0]
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
	if !ifnone.IsNil() {
		return s.call(ctx, ifnone, "call", nil, Nil, Nil)
	}
	return Nil, nil
}

func (s *State) arrayIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	for _, value := range arrayValues(self) {
		if identicalValue(value, args[0]) {
			return True, nil
		}
		equal, err := s.rubyEqual(ctx, value, args[0])
		if err != nil {
			return Nil, err
		}
		if equal {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayEachWithIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_with_index", nil), nil
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
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a hash")}
	}
	if index, found, err := s.hashLookupKey(ctx, data, args[0]); err != nil {
		return Nil, err
	} else if found {
		return data.entries[index].value, nil
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
	data, ok := hashDataOf(self)
	if !ok {
		return False, nil
	}
	_, found, err := s.hashLookupKey(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	return BoolValue(found), nil
}

func (s *State) hashValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	for _, entry := range hashEntries(self) {
		equal, err := s.rubyEqual(ctx, args[0], entry.value)
		if err != nil {
			return Nil, err
		}
		if equal {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) hashEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	data, ok := hashDataOf(self)
	if !ok {
		return self, nil
	}
	for i := 0; i < len(data.entries); {
		entry := data.entries[i]
		if _, err := s.callHashPairBlock(ctx, block, entry, Nil); err != nil {
			return Nil, err
		}
		if i < len(data.entries) && valuesEqual(data.entries[i].key, entry.key) {
			i++
		}
	}
	return self, nil
}

func (s *State) hashEachKeyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_key", nil), nil
	}
	data, ok := hashDataOf(self)
	if !ok {
		return self, nil
	}
	for i := 0; i < len(data.entries); {
		entry := data.entries[i]
		if _, err := s.callBlock(ctx, block, []Value{entry.key}, Nil); err != nil {
			return Nil, err
		}
		if i < len(data.entries) && valuesEqual(data.entries[i].key, entry.key) {
			i++
		}
	}
	return self, nil
}

func (s *State) hashEachValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_value", nil), nil
	}
	data, ok := hashDataOf(self)
	if !ok {
		return self, nil
	}
	for i := 0; i < len(data.entries); {
		entry := data.entries[i]
		if _, err := s.callBlock(ctx, block, []Value{entry.value}, Nil); err != nil {
			return Nil, err
		}
		if i < len(data.entries) && valuesEqual(data.entries[i].key, entry.key) {
			i++
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
	source, ok := hashDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a hash")}
	}
	entries := append([]hashEntry(nil), source.entries...)
	for _, arg := range args {
		other, ok := hashDataOf(arg)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a hash")}
		}
		var err error
		entries, err = s.mergeHashEntries(ctx, entries, other, block)
		if err != nil {
			return Nil, err
		}
	}
	result := HashValue(entries)
	if result.obj != nil {
		result.obj.class = self.obj.class
		result.obj.ivars = copyObjectIvars(self.obj)
		if data, ok := result.obj.data.(*hashData); ok {
			data.defaultValue = source.defaultValue
			data.defaultProc = source.defaultProc
			data.identity = source.identity
		}
	}
	return result, nil
}

func (s *State) hashMergeBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if len(args) == 0 {
		return Nil, argumentCountError(s, 0, "1+")
	}
	for _, arg := range args {
		other, ok := hashDataOf(arg)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a hash")}
		}
		entries, err := s.mergeHashEntries(ctx, data.entries, other, block)
		if err != nil {
			return Nil, err
		}
		data.entries = entries
		data.version++
		data.index = nil
		data.indexBuilt = 0
	}
	return self, nil
}

func (s *State) mergeHashEntries(ctx context.Context, left []hashEntry, right *hashData, block Value) ([]hashEntry, error) {
	result := append([]hashEntry(nil), left...)
	merged := &hashData{entries: result}
	for _, entry := range right.entries {
		index, found, err := s.hashLookupKey(ctx, merged, entry.key)
		if err != nil {
			return nil, err
		}
		if found {
			if !block.IsNil() {
				value, err := s.callBlock(ctx, block, []Value{entry.key, merged.entries[index].value, entry.value}, Nil)
				if err != nil {
					return nil, err
				}
				merged.entries[index].value = value
			} else {
				merged.entries[index].value = entry.value
			}
			continue
		}
		merged.entries = appendHashEntry(merged.entries, entry)
		addHashIndex(merged, len(merged.entries)-1)
	}
	return merged.entries, nil
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
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	index, found, err := s.hashLookupKey(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	if found {
		value := data.entries[index].value
		data.entries = append(data.entries[:index], data.entries[index+1:]...)
		data.version++
		data.index = nil
		return value, nil
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
	data, ok := hashDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	values := make([]Value, len(args))
	for i, key := range args {
		values[i] = Nil
		if index, found, err := s.hashLookupKey(ctx, data, key); err != nil {
			return Nil, err
		} else if found {
			values[i] = data.entries[index].value
		}
	}
	return ArrayValue(values), nil
}

func (s *State) stringSplitMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	limit := 0
	if len(args) > 1 && args[1].Kind() == KindInt {
		limit = int(args[1].Int())
	}
	var parts []string
	switch {
	case len(args) == 0 || args[0].IsNil():
		parts = strings.Fields(text)
	case args[0].String() == " ":
		parts = splitWhitespace(text, limit)
	case args[0].String() == "":
		parts = splitRunes(text, limit)
	default:
		parts = splitSeparator(text, args[0].String(), limit)
	}
	values := make([]Value, len(parts))
	for i, part := range parts {
		values[i] = StringValue(part)
	}
	return ArrayValue(values), nil
}

func splitSeparator(text, separator string, limit int) []string {
	if text == "" {
		return nil
	}
	if limit > 0 {
		parts := make([]string, 0, limit)
		remaining := text
		for len(parts) < limit-1 {
			index := strings.Index(remaining, separator)
			if index < 0 {
				break
			}
			parts = append(parts, remaining[:index])
			remaining = remaining[index+len(separator):]
		}
		parts = append(parts, remaining)
		return parts
	}
	parts := strings.Split(text, separator)
	if limit == 0 {
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
	}
	return parts
}

func splitRunes(text string, limit int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if limit > 0 {
		parts := make([]string, 0, limit)
		for i := 0; i < limit-1 && i < len(runes); i++ {
			parts = append(parts, string(runes[i]))
		}
		if len(parts) < limit {
			parts = append(parts, string(runes[len(parts):]))
		}
		return parts
	}
	parts := make([]string, len(runes))
	for i, r := range runes {
		parts[i] = string(r)
	}
	if limit < 0 {
		parts = append(parts, "")
	}
	return parts
}

func splitWhitespace(text string, limit int) []string {
	runes := []rune(text)
	position := 0
	for position < len(runes) && unicode.IsSpace(runes[position]) {
		position++
	}
	if limit > 0 {
		parts := make([]string, 0, limit)
		for len(parts) < limit-1 && position < len(runes) {
			start := position
			for position < len(runes) && !unicode.IsSpace(runes[position]) {
				position++
			}
			parts = append(parts, string(runes[start:position]))
			for position < len(runes) && unicode.IsSpace(runes[position]) {
				position++
			}
		}
		if position < len(runes) {
			parts = append(parts, string(runes[position:]))
		}
		return parts
	}
	parts := strings.Fields(text)
	if limit < 0 && len(runes) > 0 && unicode.IsSpace(runes[len(runes)-1]) {
		parts = append(parts, "")
	}
	return parts
}

func (s *State) stringInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(inspectUTF8String(self.String())), nil
}

func inspectUTF8String(text string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for i := 0; i < len(text); {
		b := text[i]
		if b < 0x80 {
			switch b {
			case '\a':
				builder.WriteString(`\a`)
			case '\b':
				builder.WriteString(`\b`)
			case '\t':
				builder.WriteString(`\t`)
			case '\n':
				builder.WriteString(`\n`)
			case '\v':
				builder.WriteString(`\v`)
			case '\f':
				builder.WriteString(`\f`)
			case '\r':
				builder.WriteString(`\r`)
			case 0x1b:
				builder.WriteString(`\e`)
			case '"':
				builder.WriteString(`\"`)
			case '\\':
				builder.WriteString(`\\`)
			default:
				if b < 0x20 || b == 0x7f {
					fmt.Fprintf(&builder, "\\x%02X", b)
				} else {
					builder.WriteByte(b)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&builder, "\\x%02X", b)
			i++
			continue
		}
		builder.WriteString(text[i : i+size])
		i += size
	}
	builder.WriteByte('"')
	return builder.String()
}

func (s *State) stringEachCharMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_char", nil), nil
	}
	text := self.String()
	for _, span := range stringCharSpans(text) {
		if _, err := s.callBlock1(ctx, block, StringValue(text[span.start:span.end]), Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) stringEachByteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_byte", nil), nil
	}
	for i := 0; i < len(self.String()); i++ {
		if _, err := s.callBlock1(ctx, block, IntValue(int64(self.String()[i])), Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) stringCasecmpMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	if args[0].Kind() != KindString {
		return Nil, nil
	}
	left := strings.ToLower(self.String())
	right := strings.ToLower(args[0].String())
	switch {
	case left < right:
		return IntValue(-1), nil
	case left > right:
		return IntValue(1), nil
	default:
		return IntValue(0), nil
	}
}

func (s *State) stringCasecmpPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.stringCasecmpMethod(ctx, self, args, kw, block)
	if err != nil || result.IsNil() {
		return result, err
	}
	return BoolValue(result.Int() == 0), nil
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
	if len(args) < 1 || len(args) > 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	if _, ok := regexpOf(args[0]); ok {
		data, err := s.regexpCompiled(args[0])
		if err != nil {
			return Nil, err
		}
		text := self.String()
		count := stringCharCount(text)
		offset := 0
		if len(args) > 1 {
			if args[1].Kind() != KindInt {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "offset must be an integer")}
			}
			offset = int(args[1].Int())
			if offset < 0 {
				offset += count
			}
		}
		if offset < 0 || offset > count {
			s.regexpClearGlobals()
			return Nil, nil
		}
		byteOffset, _ := stringCharIndexToByte(text, offset)
		match, err := s.regexpSearch(args[0], data, text, byteOffset)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			s.regexpClearGlobals()
			return Nil, nil
		}
		s.regexpSetGlobals(match)
		return IntValue(int64(charOffsetOfByte(text, match.start))), nil
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
			"no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	text := self.String()
	count := stringCharCount(text)
	offset := 0
	if len(args) > 1 {
		if args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "offset must be an integer")}
		}
		offset = int(args[1].Int())
		if offset < 0 {
			offset += count
		}
	}
	if offset < 0 || offset > count {
		return Nil, nil
	}
	needle := args[0].String()
	if needle == "" {
		return IntValue(int64(offset)), nil
	}
	if !utf8.ValidString(needle) {
		return Nil, nil
	}
	byteOffset, _ := stringCharIndexToByte(text, offset)
	if index, ok := stringCharIndex(text, needle, byteOffset); ok {
		return IntValue(int64(index)), nil
	}
	return Nil, nil
}

func (s *State) stringRindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	if _, ok := regexpOf(args[0]); ok {
		data, err := s.regexpCompiled(args[0])
		if err != nil {
			return Nil, err
		}
		text := self.String()
		count := stringCharCount(text)
		position := count
		if len(args) > 1 {
			if args[1].Kind() != KindInt {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "position must be an integer")}
			}
			position = int(args[1].Int())
			if position < 0 {
				position += count
			}
		}
		if position < 0 {
			s.regexpClearGlobals()
			return Nil, nil
		}
		if position > count {
			position = count
		}
		byteLimit, _ := stringCharIndexToByte(text, position)
		matches, err := s.regexpAllMatches(args[0], data, text)
		if err != nil {
			return Nil, err
		}
		var found *matchData
		for _, match := range matches {
			if match.start <= byteLimit {
				found = match
			}
		}
		if found == nil {
			s.regexpClearGlobals()
			return Nil, nil
		}
		s.regexpSetGlobals(found)
		return IntValue(int64(charOffsetOfByte(text, found.start))), nil
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
			"no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	text := self.String()
	count := stringCharCount(text)
	position := count
	if len(args) > 1 {
		if args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "position must be an integer")}
		}
		position = int(args[1].Int())
		if position < 0 {
			position += count
		}
	}
	if position < 0 {
		return Nil, nil
	}
	if position > count {
		position = count
	}
	needle := args[0].String()
	if needle == "" {
		return IntValue(int64(position)), nil
	}
	if !utf8.ValidString(needle) {
		return Nil, nil
	}
	byteLimit, _ := stringCharIndexToByte(text, position)
	if index, ok := stringCharRindex(text, needle, byteLimit); ok {
		return IntValue(int64(index)), nil
	}
	return Nil, nil
}

func (s *State) stringSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	if len(args) == 1 {
		if args[0].Kind() == KindObject && args[0].obj != nil {
			if data, ok := args[0].obj.data.(*rangeData); ok {
				return sliceStringChars(text, data), nil
			}
		}
		if args[0].Kind() == KindString {
			pattern := args[0].String()
			if strings.Contains(text, pattern) {
				return StringValue(pattern), nil
			}
			return Nil, nil
		}
		if args[0].Kind() == KindInt {
			index := int(args[0].Int())
			count := stringCharCount(text)
			if index < 0 {
				index += count
			}
			if index < 0 || index >= count {
				return Nil, nil
			}
			span := stringCharSpans(text)[index]
			return StringValue(text[span.start:span.end]), nil
		}
		return s.index(ctx, self, args[0])
	}
	if len(args) == 2 && args[0].Kind() == KindInt && args[1].Kind() == KindInt {
		if sliced, ok := stringCharSlice(text, int(args[0].Int()), int(args[1].Int())); ok {
			return StringValue(sliced), nil
		}
	}
	return Nil, nil
}

func sliceStringChars(text string, data *rangeData) Value {
	spans := stringCharSpans(text)
	length := len(spans)
	low := 0
	high := length - 1
	originalLow := 0
	if !data.low.IsNil() && data.low.Kind() == KindInt {
		originalLow = int(data.low.Int())
		low = originalLow
		if low < 0 {
			low += length
		}
	}
	if !data.high.IsNil() && data.high.Kind() == KindInt {
		high = int(data.high.Int())
		if high < 0 {
			high += length
		}
		if data.exclusive {
			high--
		}
	}
	if originalLow < -length || originalLow > length {
		return Nil
	}
	if low < 0 {
		low = 0
	}
	if high >= length {
		high = length - 1
	}
	if high < low || length == 0 {
		return StringValue("")
	}
	return StringValue(text[spans[low].start:spans[high].end])
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
		addition := arg.ToS()
		if arg.Kind() == KindInt {
			addition = string(rune(arg.Int()))
		}
		if err := s.checkStringSize(data.len() + len(addition)); err != nil {
			return Nil, err
		}
		data.appendString(addition)
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
	data.set(args[0].String())
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
				return sliceStringBytes(string(bytes), data), nil
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

func sliceStringBytes(text string, data *rangeData) Value {
	length := len(text)
	low := 0
	high := length - 1
	originalLow := 0
	if !data.low.IsNil() && data.low.Kind() == KindInt {
		originalLow = int(data.low.Int())
		low = originalLow
		if low < 0 {
			low += length
		}
	}
	if !data.high.IsNil() && data.high.Kind() == KindInt {
		high = int(data.high.Int())
		if high < 0 {
			high += length
		}
		if data.exclusive {
			high--
		}
	}
	if originalLow < -length || originalLow > length {
		return Nil
	}
	if low < 0 {
		low = 0
	}
	if high >= length {
		high = length - 1
	}
	if high < low || length == 0 {
		return StringValue("")
	}
	return StringValue(text[low : high+1])
}

func (s *State) stringByteindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, nil
	}
	text := self.String()
	needle := args[0].String()
	if !utf8.ValidString(needle) {
		return Nil, nil
	}
	offset := 0
	if len(args) > 1 && args[1].Kind() == KindInt {
		offset = int(args[1].Int())
		if offset < 0 {
			offset += len(text)
		}
	}
	if offset < 0 || offset > len(text) {
		return Nil, nil
	}
	if !stringIsCharBoundary(text, offset) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "offset is not a character boundary")}
	}
	index := strings.Index(text[offset:], needle)
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
	if _, ok := regexpOf(args[0]); ok {
		data, err := s.regexpCompiled(args[0])
		if err != nil {
			return Nil, err
		}
		matches, err := s.regexpAllMatches(args[0], data, text)
		if err != nil {
			return Nil, err
		}
		if len(matches) == 0 {
			s.regexpClearGlobals()
			return StringValue(text), nil
		}
		match := matches[0]
		s.regexpSetGlobals(match)
		var replacement string
		if !block.IsNil() {
			result, err := s.callBlock(ctx, block, []Value{StringValue(match.groups[0])}, Nil)
			if err != nil {
				return Nil, err
			}
			replacement = result.ToS()
		} else if len(args) == 2 {
			replacement = expandRegexpReplacement(args[1].String(), match)
		}
		return StringValue(text[:match.start] + replacement + text[match.end:]), nil
	}
	pattern := args[0].String()
	if pattern == "" {
		var replacement string
		if !block.IsNil() {
			result, err := s.callBlock(ctx, block, []Value{StringValue("")}, Nil)
			if err != nil {
				return Nil, err
			}
			replacement = result.ToS()
		} else if len(args) == 2 {
			replacement = expandStringReplacement(args[1].String(), "", "", text)
		}
		return StringValue(replacement + text), nil
	}
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
		replacement = expandStringReplacement(args[1].String(), pattern, text[:index], text[index+len(pattern):])
	}
	return StringValue(text[:index] + replacement + text[index+len(pattern):]), nil
}

func (s *State) stringGsubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "gsub expects one or two arguments")}
	}
	text := self.String()
	if _, ok := regexpOf(args[0]); ok {
		data, err := s.regexpCompiled(args[0])
		if err != nil {
			return Nil, err
		}
		matches, err := s.regexpAllMatches(args[0], data, text)
		if err != nil {
			return Nil, err
		}
		if len(matches) == 0 {
			s.regexpClearGlobals()
			return StringValue(text), nil
		}
		var builder strings.Builder
		position := 0
		for _, match := range matches {
			builder.WriteString(text[position:match.start])
			if !block.IsNil() {
				result, err := s.callBlock(ctx, block, []Value{StringValue(match.groups[0])}, Nil)
				if err != nil {
					return Nil, err
				}
				builder.WriteString(result.ToS())
			} else if len(args) == 2 {
				builder.WriteString(expandRegexpReplacement(args[1].String(), match))
			}
			position = match.end
		}
		builder.WriteString(text[position:])
		s.regexpSetGlobals(matches[len(matches)-1])
		return StringValue(builder.String()), nil
	}
	pattern := args[0].String()
	if pattern == "" {
		runes := []rune(text)
		var builder strings.Builder
		for i := 0; i <= len(runes); i++ {
			if !block.IsNil() {
				result, err := s.callBlock(ctx, block, []Value{StringValue("")}, Nil)
				if err != nil {
					return Nil, err
				}
				builder.WriteString(result.ToS())
			} else if len(args) == 2 {
				builder.WriteString(expandStringReplacement(args[1].String(), "", string(runes[:i]), string(runes[i:])))
			}
			if i < len(runes) {
				builder.WriteRune(runes[i])
			}
		}
		return StringValue(builder.String()), nil
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
		if !block.IsNil() {
			result, err := s.callBlock(ctx, block, []Value{StringValue(pattern)}, Nil)
			if err != nil {
				return Nil, err
			}
			builder.WriteString(result.ToS())
		} else if len(args) == 2 {
			builder.WriteString(expandStringReplacement(args[1].String(), text[index:index+len(pattern)], text[:index], text[index+len(pattern):]))
		}
		offset = index + len(pattern)
	}
	return StringValue(builder.String()), nil
}

func (s *State) integerTimesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	count := self.Int()
	if block.IsNil() {
		return s.toEnum(self, "times", nil), nil
	}
	for i := int64(0); i < count; i++ {
		if _, err := s.callBlock(ctx, block, []Value{IntValue(i)}, Nil); err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
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
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
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
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) numericStepMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "0..2")
	}
	if block.IsNil() {
		return s.toEnum(self, "step", args), nil
	}
	num := Nil
	if len(args) > 0 {
		num = args[0]
	}
	step := IntValue(1)
	if len(args) > 1 && !args[1].IsNil() {
		step = args[1]
	}
	if isNumericZero(step) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "step can't be 0")}
	}
	i := self
	if step.Kind() == KindFloat {
		i = FloatValue(valueFloat(self))
	}
	stepCmp := 0
	if cmp, ok := numericCompare(step, IntValue(0)); ok {
		stepCmp = cmp
	} else if step.Kind() == KindFloat {
		stepCmp = 0
		if step.Float() > 0 {
			stepCmp = 1
		} else if step.Float() < 0 {
			stepCmp = -1
		}
	}
	stepInfinite := step.Kind() == KindFloat && math.IsInf(step.Float(), 0)
	yield := func(value Value) (bool, error) {
		_, err := s.callBlock(ctx, block, []Value{value}, Nil)
		if err == nil {
			return true, nil
		}
		if sig, ok := err.(*unwindSignal); ok && sig.kind == unwindBreak {
			return false, nil
		}
		return false, err
	}
	emit := func(value Value) (bool, error) {
		keep, err := yield(value)
		if err != nil || !keep {
			return false, err
		}
		return true, nil
	}
	equalSelf := false
	if num.IsNil() {
		// handled by the branches below
	} else if cmp, ok := numericCompare(num, self); ok {
		equalSelf = cmp == 0
	}
	if equalSelf || stepInfinite {
		limit := i
		if !num.IsNil() {
			limit = num
		} else if stepCmp < 0 {
			limit = IntValue(0)
			if neg, err := s.numericNegate(i); err == nil {
				limit = neg
			}
		}
		cmp, _ := numericCompare(i, limit)
		emitValue := stepCmp > 0 && cmp <= 0 || stepCmp < 0 && cmp >= 0
		if emitValue {
			if _, err := emit(i); err != nil {
				return Nil, err
			}
		}
		return self, nil
	}
	iterations := 0
	for {
		iterations++
		if iterations > 1000000 {
			return self, nil
		}
		if !num.IsNil() {
			cmp, ok := numericCompare(i, num)
			if !ok {
				break
			}
			if stepCmp > 0 && cmp > 0 {
				break
			}
			if stepCmp < 0 && cmp < 0 {
				break
			}
		}
		keep, err := emit(i)
		if err != nil {
			return Nil, err
		}
		if !keep {
			return self, nil
		}
		next, ok := binaryValue(i, "+", step)
		if !ok {
			var err error
			next, err = s.call(ctx, i, "+", []Value{step}, Nil, Nil)
			if err != nil {
				return Nil, err
			}
		}
		i = next
	}
	return self, nil
}

func valueFloat(v Value) float64 {
	if v.Kind() == KindFloat {
		return v.Float()
	}
	if v.Kind() == KindBigInt {
		n, _ := v.BigInt()
		f, _ := new(big.Float).SetInt(n).Float64()
		return f
	}
	return float64(v.Int())
}

func (s *State) objectIsAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	class, ok := s.asClass(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "class or module required")}
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
		return Nil, argumentCountError(s, 0, "1+")
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

func (s *State) arrayCompareMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	other := arrayValues(args[0])
	values := arrayValues(self)
	length := len(values)
	if len(other) < length {
		length = len(other)
	}
	for i := 0; i < length; i++ {
		result, err := s.call(ctx, values[i], "<=>", []Value{other[i]}, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		if result.IsNil() {
			return Nil, nil
		}
		if result.Kind() != KindInt {
			return Nil, nil
		}
		if result.Int() < 0 {
			return IntValue(-1), nil
		}
		if result.Int() > 0 {
			return IntValue(1), nil
		}
	}
	switch {
	case len(values) < len(other):
		return IntValue(-1), nil
	case len(values) > len(other):
		return IntValue(1), nil
	default:
		return IntValue(0), nil
	}
}

func hashValue(v Value, seen map[*Object]bool) int64 {
	const fnvOffset = int64(1469598103934665603)
	const fnvPrime = int64(1099511628211)
	mix := func(h int64, value Value) int64 {
		return (h ^ hashValue(value, seen)) * fnvPrime
	}
	switch v.Kind() {
	case KindNil:
		return 1
	case KindFalse:
		return 2
	case KindTrue:
		return 3
	case KindInt:
		return v.Int()
	case KindBigInt:
		value, _ := v.BigInt()
		return int64(fnvString(value.String(), fnvOffset, fnvPrime))
	case KindFloat:
		return int64(math.Float64bits(v.Float()))
	case KindString, KindSymbol:
		return int64(fnvString(v.String(), fnvOffset, fnvPrime))
	case KindObject:
		if v.obj == nil {
			return 0
		}
		if seen[v.obj] {
			return 0x9e3779b9
		}
		switch data := v.obj.data.(type) {
		case *arrayData:
			seen[v.obj] = true
			h := int64(17)
			for _, item := range data.values {
				h = mix(h, item)
			}
			delete(seen, v.obj)
			return h
		case *hashData:
			seen[v.obj] = true
			h := int64(29)
			for _, entry := range data.entries {
				h ^= mix(h, entry.key) ^ mix(h, entry.value)
			}
			delete(seen, v.obj)
			return h
		case *rangeData:
			h := int64(41)
			h = mix(h, data.low)
			h = mix(h, data.high)
			if data.exclusive {
				h++
			}
			return h
		case *stringData:
			return int64(fnvString(data.str(), fnvOffset, fnvPrime))
		default:
			return int64(fnvString(fmt.Sprintf("%p", v.obj), fnvOffset, fnvPrime))
		}
	}
	return 0
}

func fnvString(text string, offset, prime int64) int64 {
	h := offset
	for i := 0; i < len(text); i++ {
		h ^= int64(text[i])
		h *= prime
	}
	return h
}

type eqlPair struct {
	left  *Object
	right *Object
}

func eqlValues(left, right Value) bool {
	return eqlValuesSeen(left, right, make(map[eqlPair]bool))
}

func eqlValuesSeen(left, right Value, seen map[eqlPair]bool) bool {
	if leftString, leftOK := stringPayload(left); leftOK {
		if rightString, rightOK := stringPayload(right); rightOK {
			if (left.Kind() == KindSymbol) != (right.Kind() == KindSymbol) {
				return false
			}
			return leftString == rightString
		}
	}
	if left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case KindInt:
		return left.Int() == right.Int()
	case KindBigInt:
		li, _ := left.BigInt()
		ri, _ := right.BigInt()
		return li.Cmp(ri) == 0
	case KindFloat:
		return left.Float() == right.Float()
	case KindString, KindSymbol:
		return left.String() == right.String()
	case KindNil, KindTrue, KindFalse:
		return true
	case KindObject:
		if left.obj == nil || right.obj == nil {
			return left.obj == right.obj
		}
		pair := eqlPair{left: left.obj, right: right.obj}
		if seen[pair] {
			return true
		}
		leftArray, leftOK := left.obj.data.(*arrayData)
		rightArray, rightOK := right.obj.data.(*arrayData)
		if leftOK || rightOK {
			if !leftOK || !rightOK || len(leftArray.values) != len(rightArray.values) {
				return false
			}
			seen[pair] = true
			defer delete(seen, pair)
			for i := range leftArray.values {
				if !eqlValuesSeen(leftArray.values[i], rightArray.values[i], seen) {
					return false
				}
			}
			return true
		}
		leftHash, leftOK := left.obj.data.(*hashData)
		rightHash, rightOK := right.obj.data.(*hashData)
		if leftOK || rightOK {
			if !leftOK || !rightOK || len(leftHash.entries) != len(rightHash.entries) {
				return false
			}
			return valuesEqual(left, right)
		}
		return left.obj == right.obj
	}
	return false
}

func (s *State) arrayEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	other, ok := arrayDataOf(args[0])
	if !ok {
		return False, nil
	}
	values := arrayValues(self)
	if len(values) != len(other.values) {
		return False, nil
	}
	if self.obj != nil && args[0].obj != nil {
		if self.obj == args[0].obj {
			return True, nil
		}
		pair := objectPair{left: self.obj, right: args[0].obj}
		if s.eqSeen[pair] {
			return True, nil
		}
		s.eqSeen[pair] = true
		defer delete(s.eqSeen, pair)
	}
	for i := range values {
		equal, err := s.rubyEqual(ctx, values[i], other.values[i])
		if err != nil {
			return Nil, err
		}
		if !equal {
			return False, nil
		}
	}
	return True, nil
}

func (s *State) rubyEql(ctx context.Context, left, right Value) (bool, error) {
	if left.obj != nil && right.obj != nil && left.obj == right.obj {
		return true, nil
	}
	if left.Kind() != KindObject || right.Kind() != KindObject {
		return eqlValues(left, right), nil
	}
	result, err := s.call(ctx, left, "eql?", []Value{right}, Nil, Nil)
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}

func (s *State) rubyEqual(ctx context.Context, left, right Value) (bool, error) {
	if identicalValue(left, right) {
		return true, nil
	}
	result, err := s.call(ctx, left, "==", []Value{right}, Nil, Nil)
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}

func rubyReplacement(text string) string {
	replacer := strings.NewReplacer(`\0`, `${0}`, `\1`, `${1}`, `\2`, `${2}`, `\3`, `${3}`, `\4`, `${4}`, `\5`, `${5}`, `\6`, `${6}`, `\7`, `${7}`, `\8`, `${8}`, `\9`, `${9}`)
	return replacer.Replace(text)
}

func digitValue(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r >= 'a' && r <= 'z':
		return int(r-'a') + 10
	case r >= 'A' && r <= 'Z':
		return int(r-'A') + 10
	}
	return -1
}

func parseRubyInteger(text string, base int) Value {
	runes := []rune(strings.TrimLeft(text, " \t\r\n\f\v"))
	position := 0
	negative := false
	if position < len(runes) && (runes[position] == '+' || runes[position] == '-') {
		negative = runes[position] == '-'
		position++
	}
	detected := base
	if detected == 0 {
		detected = 10
		if position+1 < len(runes) && runes[position] == '0' {
			switch runes[position+1] {
			case 'x', 'X':
				detected = 16
				position += 2
			case 'b', 'B':
				detected = 2
				position += 2
			case 'o', 'O':
				detected = 8
				position += 2
			default:
				detected = 8
				position++
			}
		}
	} else if position+1 < len(runes) && runes[position] == '0' {
		switch {
		case detected == 16 && (runes[position+1] == 'x' || runes[position+1] == 'X'):
			position += 2
		case detected == 2 && (runes[position+1] == 'b' || runes[position+1] == 'B'):
			position += 2
		case detected == 8 && (runes[position+1] == 'o' || runes[position+1] == 'O'):
			position += 2
		}
	}
	value := new(big.Int)
	previousDigit := false
	for position < len(runes) {
		r := runes[position]
		if r == '_' {
			if !previousDigit {
				break
			}
			if position+1 < len(runes) && runes[position+1] == '_' {
				break
			}
			position++
			continue
		}
		digit := digitValue(r)
		if digit < 0 || digit >= detected {
			break
		}
		value.Mul(value, big.NewInt(int64(detected)))
		value.Add(value, big.NewInt(int64(digit)))
		previousDigit = true
		position++
	}
	if negative {
		value.Neg(value)
	}
	return normalizedBigInt(value)
}

func parseRubyFloat(text string) float64 {
	runes := []rune(strings.TrimLeft(text, " \t\r\n\f\v"))
	position := 0
	var builder strings.Builder
	if position < len(runes) && (runes[position] == '+' || runes[position] == '-') {
		builder.WriteRune(runes[position])
		position++
	}
	previousDigit := false
	for position < len(runes) {
		r := runes[position]
		if r == '_' {
			if !previousDigit {
				break
			}
			if position+1 < len(runes) && runes[position+1] == '_' {
				break
			}
			position++
			continue
		}
		if r < '0' || r > '9' {
			break
		}
		builder.WriteRune(r)
		previousDigit = true
		position++
	}
	if position < len(runes) && runes[position] == '.' &&
		position+1 < len(runes) && runes[position+1] >= '0' && runes[position+1] <= '9' {
		builder.WriteByte('.')
		position++
		previousDigit = false
		for position < len(runes) {
			r := runes[position]
			if r == '_' {
				if !previousDigit {
					break
				}
				if position+1 < len(runes) && runes[position+1] == '_' {
					break
				}
				position++
				continue
			}
			if r < '0' || r > '9' {
				break
			}
			builder.WriteRune(r)
			previousDigit = true
			position++
		}
	}
	if position < len(runes) && (runes[position] == 'e' || runes[position] == 'E') {
		next := position + 1
		if next < len(runes) && (runes[next] == '+' || runes[next] == '-') {
			next++
		}
		if next < len(runes) && runes[next] >= '0' && runes[next] <= '9' {
			builder.WriteByte('e')
			position++
			if runes[position] == '+' || runes[position] == '-' {
				builder.WriteRune(runes[position])
				position++
			}
			for position < len(runes) && runes[position] >= '0' && runes[position] <= '9' {
				builder.WriteRune(runes[position])
				position++
			}
		}
	}
	if builder.Len() == 0 || builder.String() == "+" || builder.String() == "-" {
		return 0
	}
	value, err := strconv.ParseFloat(builder.String(), 64)
	if err != nil {
		if value != 0 || strings.Contains(err.Error(), "range") {
			return value
		}
		return 0
	}
	return value
}

func globMatch(pattern, text string) bool {
	p := []rune(pattern)
	t := []rune(text)
	pi, ti := 0, 0
	star, match := -1, 0
	for ti < len(t) {
		if pi < len(p) && (p[pi] == '?' || p[pi] == t[ti]) {
			pi++
			ti++
			continue
		}
		if pi < len(p) && p[pi] == '*' {
			star = pi
			match = ti
			pi++
			continue
		}
		if star >= 0 {
			pi = star + 1
			match++
			ti = match
			continue
		}
		return false
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}

func expandStringReplacement(replacement, match, pre, post string) string {
	var builder strings.Builder
	for i := 0; i < len(replacement); i++ {
		if replacement[i] != '\\' || i+1 >= len(replacement) {
			builder.WriteByte(replacement[i])
			continue
		}
		i++
		switch replacement[i] {
		case '0', '&':
			builder.WriteString(match)
		case '`':
			builder.WriteString(pre)
		case '\'':
			builder.WriteString(post)
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			// String patterns have no capture groups.
		case '\\':
			builder.WriteByte('\\')
		default:
			builder.WriteByte(replacement[i])
		}
	}
	return builder.String()
}

func objectIdentityID(value Value) int64 {
	if value.obj != nil && (value.Kind() == KindObject || value.Kind() == KindString) {
		return int64(uintptr(unsafe.Pointer(value.obj)))
	}
	return hashValue(value, map[*Object]bool{})
}

func (s *State) objectInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj != nil && s.TopSelf.obj != nil && self.obj == s.TopSelf.obj {
		return StringValue("main"), nil
	}
	if self.obj == nil {
		return StringValue(self.Inspect()), nil
	}
	if _, ok := self.obj.data.(*Class); ok {
		return StringValue(self.Inspect()), nil
	}
	names := make([]string, 0, len(self.obj.ivars))
	for name := range self.obj.ivars {
		if strings.HasPrefix(name, "@") {
			names = append(names, name)
		}
	}
	className := s.classOf(self).name
	if className == "" {
		className = "Object"
	}
	text := "#<" + className
	if len(names) > 0 {
		sort.Strings(names)
		for _, name := range names {
			value, ok := self.obj.ivars[name]
			if !ok {
				continue
			}
			rendered, err := s.call(ctx, value, "inspect", nil, Nil, Nil)
			if err != nil {
				return Nil, err
			}
			text += " " + name + "=" + rendered.ToS()
		}
	}
	return StringValue(text + ">"), nil
}
