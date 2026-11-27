package mrbvm

import (
	"context"
	"fmt"
)

// installHashExtensionLibrary adds the methods mruby implements in
// mruby-hash-ext.
func (s *State) installHashExtensionLibrary() {
	s.HashClass.classMethods["[]"] = &method{owner: s.HashClass, name: "[]", native: s.hashClassBracketMethod}
	class := s.HashClass
	class.define("values_at", s.hashExtValuesAtMethod)
	class.define("compact", s.hashExtCompactMethod)
	class.define("compact!", s.hashExtCompactBangMethod)
	class.define("delete_if", s.hashExtDeleteIfMethod)
	class.define("keep_if", s.hashExtKeepIfMethod)
	class.define("flatten", s.hashExtFlattenMethod)
	class.define("invert", s.hashExtInvertMethod)
	class.define("key", s.hashExtKeyMethod)
	class.define("transform_values", s.hashExtTransformValuesMethod)
	class.define("transform_values!", s.hashExtTransformValuesBangMethod)
	class.define("transform_keys", s.hashExtTransformKeysMethod)
	class.define("transform_keys!", s.hashExtTransformKeysBangMethod)
	class.define("to_h", s.hashExtToHMethod)
	class.define("slice", s.hashExtSliceMethod)
	class.define("slice!", s.hashExtSliceBangMethod)
	class.define("except", s.hashExtExceptMethod)
	class.define("<", s.hashExtCompareMethod('<'))
	class.define("<=", s.hashExtCompareMethod('l'))
	class.define(">", s.hashExtCompareMethod('>'))
	class.define(">=", s.hashExtCompareMethod('g'))
}

func (s *State) hashExtNewResult(class *Class, identity bool) Value {
	result := HashValue(nil)
	if result.obj != nil {
		result.obj.class = class
		if data, ok := result.obj.data.(*hashData); ok {
			data.identity = identity
		}
	}
	return result
}

func (s *State) hashExtSet(ctx context.Context, hash Value, key, value Value) error {
	_, err := s.setIndex(ctx, hash, key, value)
	return err
}

func (s *State) hashExtDelete(ctx context.Context, data *hashData, key Value) (Value, bool, error) {
	index, found, err := s.hashLookupKey(ctx, data, key)
	if err != nil || !found {
		return Nil, false, err
	}
	value := data.entries[index].value
	data.entries = append(data.entries[:index], data.entries[index+1:]...)
	data.version++
	data.index = nil
	data.indexBuilt = 0
	return value, true, nil
}

func (s *State) hashClassBracketMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, _ := s.asClass(self)
	var entries []hashEntry
	add := func(key, value Value) {
		for i := range entries {
			if arrayEql(entries[i].key, key) {
				entries[i].value = value
				return
			}
		}
		entries = append(entries, hashEntry{key: key, value: value})
	}
	identity := false
	fromHash := false
	single := args
	if len(args) == 0 && !kw.IsNil() {
		single = []Value{kw}
	}
	switch {
	case len(single) == 0:
	case len(single) == 1:
		if source, ok := hashDataOf(single[0]); ok {
			entries = append(entries, source.entries...)
			identity = source.identity
			fromHash = true
			break
		}
		pairs, ok := arrayDataOf(single[0])
		if !ok {
			return Nil, s.newArrayError(s.ArgumentErrorClass, "odd number of arguments for Hash")
		}
		for _, pairValue := range pairs.values {
			pair, ok := arrayDataOf(pairValue)
			if !ok {
				return Nil, s.newArrayError(s.ArgumentErrorClass, "wrong element type (expected array)")
			}
			if len(pair.values) != 2 {
				return Nil, s.newArrayError(s.ArgumentErrorClass, fmt.Sprintf("wrong array length (expected 2, was %d)", len(pair.values)))
			}
			add(pair.values[0], pair.values[1])
		}
	default:
		if len(single)%2 != 0 {
			return Nil, s.newArrayError(s.ArgumentErrorClass, "odd number of arguments for Hash")
		}
		for i := 0; i < len(single); i += 2 {
			add(single[i], single[i+1])
		}
	}
	_ = fromHash
	if err := s.checkHashSize(len(entries)); err != nil {
		return Nil, err
	}
	result := HashValue(entries)
	if result.obj != nil {
		result.obj.class = class
		if data, ok := result.obj.data.(*hashData); ok {
			data.identity = identity
		}
	}
	return result, nil
}

func (s *State) hashExtLookup(ctx context.Context, self Value, data *hashData, key Value) (Value, error) {
	index, found, err := s.hashLookupKey(ctx, data, key)
	if err != nil {
		return Nil, err
	}
	if found {
		return data.entries[index].value, nil
	}
	if !data.defaultProc.IsNil() {
		return s.callBlock(ctx, data.defaultProc, []Value{self, key}, Nil)
	}
	return data.defaultValue, nil
}

func (s *State) hashExtValuesAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	values := make([]Value, len(args))
	for i, key := range args {
		value, err := s.hashExtLookup(ctx, self, data, key)
		if err != nil {
			return Nil, err
		}
		values[i] = value
	}
	return ArrayValue(values), nil
}

func (s *State) hashExtCompactEntries(entries []hashEntry) []hashEntry {
	result := make([]hashEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.value.IsNil() {
			result = append(result, entry)
		}
	}
	return result
}

func (s *State) hashExtCompactMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.classOf(self), data.identity)
	resultData, _ := hashDataOf(result)
	resultData.entries = s.hashExtCompactEntries(data.entries)
	resultData.defaultValue = data.defaultValue
	resultData.defaultProc = data.defaultProc
	return result, nil
}

func (s *State) hashExtCompactBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not a hash")
	}
	entries := s.hashExtCompactEntries(data.entries)
	if len(entries) == len(data.entries) {
		return Nil, nil
	}
	data.entries = entries
	data.version++
	data.index = nil
	data.indexBuilt = 0
	return self, nil
}

func (s *State) hashExtFilter(ctx context.Context, self Value, block Value, keep bool) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not a hash")
	}
	snapshot := append([]hashEntry(nil), data.entries...)
	keepKeys := make([]Value, 0, len(snapshot))
	for _, entry := range snapshot {
		value, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
		if err != nil {
			return Nil, err
		}
		if value.IsTruthy() == keep {
			keepKeys = append(keepKeys, entry.key)
		}
	}
	for _, entry := range snapshot {
		found := false
		for _, key := range keepKeys {
			if arrayEql(key, entry.key) {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if _, _, err := s.hashExtDelete(ctx, data, entry.key); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) hashExtDeleteIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "delete_if", nil), nil
	}
	return s.hashExtFilter(ctx, self, block, false)
}

func (s *State) hashExtKeepIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "keep_if", nil), nil
	}
	return s.hashExtFilter(ctx, self, block, true)
}

func (s *State) hashExtFlattenMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	level := 1
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, s.newArrayError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
		level = int(args[0].Int())
	}
	entries := hashEntries(self)
	pairs := make([]Value, len(entries))
	for i, entry := range entries {
		pairs[i] = ArrayValue([]Value{entry.key, entry.value})
	}
	values, err := s.arrayFlattenValues(pairs, level, map[*arrayData]bool{})
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) hashExtInvertMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.classOf(self), false)
	for _, entry := range data.entries {
		if err := s.hashExtSet(ctx, result, entry.value, entry.key); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) hashValueEqual(ctx context.Context, left, right Value) (bool, error) {
	if identicalValue(left, right) {
		return true, nil
	}
	return s.rubyEqual(ctx, left, right)
}

func (s *State) hashExtKeyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	entries := append([]hashEntry(nil), hashEntries(self)...)
	for _, entry := range entries {
		equal, err := s.hashValueEqual(ctx, entry.value, args[0])
		if err != nil {
			return Nil, err
		}
		if equal {
			return entry.key, nil
		}
	}
	return Nil, nil
}

func (s *State) hashExtTransformValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "transform_values", nil), nil
	}
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.classOf(self), data.identity)
	resultData, _ := hashDataOf(result)
	resultData.entries = make([]hashEntry, len(data.entries))
	for i, entry := range data.entries {
		value, err := s.callBlock1(ctx, block, entry.value, Nil)
		if err != nil {
			return Nil, err
		}
		resultData.entries[i] = hashEntry{key: entry.key, value: value}
	}
	return result, nil
}

func (s *State) hashExtTransformValuesBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "transform_values!", nil), nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not a hash")
	}
	values := make([]Value, len(data.entries))
	for i, entry := range data.entries {
		value, err := s.callBlock1(ctx, block, entry.value, Nil)
		if err != nil {
			return Nil, err
		}
		values[i] = value
	}
	for i := range data.entries {
		data.entries[i].value = values[i]
	}
	data.version++
	return self, nil
}

func (s *State) hashExtTransformKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "transform_keys", nil), nil
	}
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.classOf(self), false)
	for _, entry := range data.entries {
		key, err := s.callBlock1(ctx, block, entry.key, Nil)
		if err != nil {
			return Nil, err
		}
		if err := s.hashExtSet(ctx, result, key, entry.value); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) hashExtTransformKeysBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "transform_keys!", nil), nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not a hash")
	}
	result := s.hashExtNewResult(s.classOf(self), data.identity)
	for _, entry := range data.entries {
		key, err := s.callBlock1(ctx, block, entry.key, Nil)
		if err != nil {
			return Nil, err
		}
		if err := s.hashExtSet(ctx, result, key, entry.value); err != nil {
			return Nil, err
		}
	}
	resultData, _ := hashDataOf(result)
	data.entries = resultData.entries
	data.version++
	data.index = nil
	data.indexBuilt = 0
	return self, nil
}

func (s *State) hashExtToHMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return self, nil
	}
	result := HashValue(nil)
	entries := append([]hashEntry(nil), hashEntries(self)...)
	for _, entry := range entries {
		pairValue, err := s.callBlock(ctx, block, []Value{entry.key, entry.value}, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
		pair, ok := arrayDataOf(pairValue)
		if !ok {
			return Nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("wrong element type %s (expected Array)", s.classOf(pairValue).name))
		}
		if len(pair.values) != 2 {
			return Nil, s.newArrayError(s.ArgumentErrorClass, fmt.Sprintf("element has wrong array length (expected 2, was %d)", len(pair.values)))
		}
		if err := s.hashExtSet(ctx, result, pair.values[0], pair.values[1]); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) hashExtSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.HashClass, data.identity)
	for _, key := range args {
		index, found, err := s.hashLookupKey(ctx, data, key)
		if err != nil {
			return Nil, err
		}
		if !found {
			continue
		}
		if err := s.hashExtSet(ctx, result, key, data.entries[index].value); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) hashExtSliceBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := hashDataOf(self)
	if !ok {
		return Nil, s.newArrayError(s.TypeErrorClass, "not a hash")
	}
	keepKeys := append([]Value(nil), args...)
	result := HashValue(nil)
	entries := append([]hashEntry(nil), data.entries...)
	for _, entry := range entries {
		keep := false
		for _, key := range keepKeys {
			if arrayEql(key, entry.key) {
				keep = true
				break
			}
		}
		if keep {
			continue
		}
		value, found, err := s.hashExtDelete(ctx, data, entry.key)
		if err != nil {
			return Nil, err
		}
		if !found {
			continue
		}
		if err := s.hashExtSet(ctx, result, entry.key, value); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) hashExtExceptMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := s.hashExtNewResult(s.HashClass, data.identity)
	for _, entry := range data.entries {
		excluded := false
		for _, key := range args {
			if arrayEql(key, entry.key) {
				excluded = true
				break
			}
		}
		if !excluded {
			if err := s.hashExtSet(ctx, result, entry.key, entry.value); err != nil {
				return Nil, err
			}
		}
	}
	return result, nil
}

func (s *State) hashExtSubset(ctx context.Context, self Value, other Value) (bool, error) {
	selfData, ok := hashDataOf(self)
	if !ok {
		return false, nil
	}
	otherData, ok := hashDataOf(other)
	if !ok {
		return false, nil
	}
	for _, entry := range selfData.entries {
		index, found, err := s.hashLookupKey(ctx, otherData, entry.key)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		equal, err := s.hashValueEqual(ctx, entry.value, otherData.entries[index].value)
		if err != nil {
			return false, err
		}
		if !equal {
			return false, nil
		}
	}
	return true, nil
}

func (s *State) hashExtCompareMethod(op byte) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		if _, ok := hashDataOf(args[0]); !ok {
			return Nil, s.newArrayError(s.TypeErrorClass, fmt.Sprintf("can't convert %s to Hash", s.classOf(args[0]).name))
		}
		selfSize := len(hashEntries(self))
		otherSize := len(hashEntries(args[0]))
		switch op {
		case '<':
			if selfSize >= otherSize {
				return False, nil
			}
			subset, err := s.hashExtSubset(ctx, self, args[0])
			if err != nil {
				return Nil, err
			}
			return BoolValue(subset), nil
		case 'l':
			if selfSize > otherSize {
				return False, nil
			}
			subset, err := s.hashExtSubset(ctx, self, args[0])
			if err != nil {
				return Nil, err
			}
			return BoolValue(subset), nil
		case '>':
			if selfSize <= otherSize {
				return False, nil
			}
			subset, err := s.hashExtSubset(ctx, args[0], self)
			if err != nil {
				return Nil, err
			}
			return BoolValue(subset), nil
		default:
			if selfSize < otherSize {
				return False, nil
			}
			subset, err := s.hashExtSubset(ctx, args[0], self)
			if err != nil {
				return Nil, err
			}
			return BoolValue(subset), nil
		}
	}
}
