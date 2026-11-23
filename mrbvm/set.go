package mrbvm

import (
	"context"
	"fmt"
	"strings"
)

// setData stores elements in insertion order. Lookup always goes through the
// Ruby hash/eql? protocol, even for primitive values, so a user-defined
// #eql? is consulted exactly as mruby's hash-backed Set consults it.
type setData struct {
	entries *hashData
}

func newSetData() *setData {
	return &setData{entries: &hashData{}}
}

func setDataOf(value Value) (*setData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*setData)
	return data, ok
}

func newSetOfClass(class *Class, data *setData) Value {
	return ObjectValue(&Object{class: class, data: data})
}

func (s *State) setValue(class *Class) Value {
	if class == nil {
		class = s.SetClass
	}
	return newSetOfClass(class, newSetData())
}

func (s *State) setLookup(ctx context.Context, data *setData, key Value) (int, bool, error) {
	version := data.entries.version
	for i, entry := range data.entries.entries {
		version = data.entries.version
		equal, err := s.call(ctx, entry.key, "eql?", []Value{key}, Nil, Nil)
		if err != nil {
			return -1, false, err
		}
		if data.entries.version != version {
			return -1, false, s.hashModifiedError()
		}
		if equal.IsTruthy() {
			return i, true, nil
		}
	}
	return -1, false, nil
}

func (s *State) setContains(ctx context.Context, data *setData, key Value) (bool, error) {
	_, found, err := s.setLookup(ctx, data, key)
	return found, err
}

func (s *State) setAddElement(ctx context.Context, data *setData, key Value) (bool, error) {
	_, found, err := s.setLookup(ctx, data, key)
	if err != nil || found {
		return false, err
	}
	if err := s.checkHashSize(len(data.entries.entries) + 1); err != nil {
		return false, err
	}
	data.entries.entries = appendHashEntry(data.entries.entries, hashEntry{key: key, value: True})
	data.entries.version++
	return true, nil
}

func (s *State) setDeleteElement(ctx context.Context, data *setData, key Value) (bool, error) {
	index, found, err := s.setLookup(ctx, data, key)
	if err != nil || !found {
		return false, err
	}
	data.entries.entries = append(data.entries.entries[:index], data.entries.entries[index+1:]...)
	data.entries.version++
	return true, nil
}

func (s *State) setClearData(data *setData) {
	if len(data.entries.entries) == 0 {
		return
	}
	data.entries.entries = nil
	data.entries.version++
}

func setEntries(data *setData) []hashEntry {
	return data.entries.entries
}

func (s *State) installSetLibrary() {
	class := s.SetClass
	includeModule(class, s.EnumerableClass)
	classMethods := class
	classMethods.classMethods["[]"] = &method{owner: class, name: "[]", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		target, ok := s.asClass(self)
		if !ok {
			target = s.SetClass
		}
		data := newSetData()
		for _, arg := range args {
			if _, err := s.setAddElement(ctx, data, arg); err != nil {
				return Nil, err
			}
		}
		return newSetOfClass(target, data), nil
	}}

	class.define("initialize", s.setInitializeMethod)
	class.define("initialize_copy", s.setInitializeCopyMethod)
	class.define("each", s.setEachMethod)
	class.define("size", s.setSizeMethod)
	class.define("length", s.setSizeMethod)
	class.define("empty?", s.setEmptyMethod)
	class.define("clear", s.setClearMethod)
	class.define("replace", s.setReplaceMethod)
	class.define("to_a", s.setToAMethod)
	class.define("include?", s.setIncludeMethod)
	class.define("member?", s.setIncludeMethod)
	class.define("===", s.setIncludeMethod)
	class.define("add", s.setAddMethod)
	class.define("<<", s.setAddMethod)
	class.define("add?", s.setAddPredicateMethod)
	class.define("delete", s.setDeleteMethod)
	class.define("delete?", s.setDeletePredicateMethod)
	class.define("merge", s.setMergeMethod)
	class.define("subtract", s.setSubtractMethod)
	class.define("replace", s.setReplaceMethod)
	class.define("union", s.setUnionMethod)
	class.define("|", s.setUnionMethod)
	class.define("+", s.setUnionMethod)
	class.define("intersection", s.setIntersectionMethod)
	class.define("&", s.setIntersectionMethod)
	class.define("difference", s.setDifferenceMethod)
	class.define("-", s.setDifferenceMethod)
	class.define("^", s.setXorMethod)
	class.define("superset?", s.setSupersetMethod)
	class.define(">=", s.setSupersetMethod)
	class.define("proper_superset?", s.setProperSupersetMethod)
	class.define(">", s.setProperSupersetMethod)
	class.define("subset?", s.setSubsetMethod)
	class.define("<=", s.setSubsetMethod)
	class.define("proper_subset?", s.setProperSubsetMethod)
	class.define("<", s.setProperSubsetMethod)
	class.define("intersect?", s.setIntersectPredicateMethod)
	class.define("disjoint?", s.setDisjointPredicateMethod)
	class.define("delete_if", s.setDeleteIfMethod)
	class.define("keep_if", s.setKeepIfMethod)
	class.define("reject!", s.setRejectBangMethod)
	class.define("select!", s.setSelectBangMethod)
	class.define("filter!", s.setSelectBangMethod)
	class.define("collect!", s.setCollectBangMethod)
	class.define("map!", s.setCollectBangMethod)
	class.define("classify", s.setClassifyMethod)
	class.define("divide", s.setDivideMethod)
	class.define("flatten", s.setFlattenMethod)
	class.define("flatten!", s.setFlattenBangMethod)
	class.define("inspect", s.setInspectMethod)
	class.define("to_s", s.setInspectMethod)
	class.define("join", s.setJoinMethod)
	class.define("hash", s.setHashMethod)
	class.define("==", s.setEqualMethod)
	class.define("eql?", s.setEqualMethod)
	class.define("to_set", s.setToSetMethod)

	s.EnumerableClass.define("to_set", s.enumerableToSetMethod)
	s.ArrayClass.define("to_set", s.enumerableToSetMethod)
	s.HashClass.define("to_set", s.enumerableToSetMethod)
	s.RangeClass.define("to_set", s.enumerableToSetMethod)
}

func (s *State) setClassValue(self Value) *Class {
	if class, ok := s.asClass(self); ok {
		return class
	}
	return s.SetClass
}

func (s *State) setInitializeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	data := newSetData()
	self.obj.data = data
	if len(args) == 0 || args[0].IsNil() {
		return self, nil
	}
	if !block.IsNil() {
		err := s.setEachEnumValue(ctx, args[0], func(value Value) error {
			mapped, err := s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return err
			}
			_, err = s.setAddElement(ctx, data, mapped)
			return err
		})
		if err != nil {
			return Nil, err
		}
		return self, nil
	}
	if err := s.setMergeInto(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if identicalValue(self, args[0]) {
		return self, nil
	}
	source, ok := setDataOf(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize_copy requires a Set")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data := newSetData()
	data.entries.entries = append([]hashEntry(nil), source.entries.entries...)
	self.obj.data = data
	return self, nil
}

func (s *State) setEachEnumValue(ctx context.Context, enum Value, fn func(Value) error) error {
	if enum.IsNil() {
		return nil
	}
	class := s.classOf(enum)
	if _, found := class.lookup("each"); !found {
		return &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "value must be enumerable")}
	}
	return s.eachRaw(ctx, enum, func(raw []Value) error {
		return fn(svalueOf(raw))
	})
}

func (s *State) setMergeInto(ctx context.Context, data *setData, enum Value) error {
	if source, ok := setDataOf(enum); ok {
		for _, entry := range source.entries.entries {
			if _, err := s.setAddElement(ctx, data, entry.key); err != nil {
				return err
			}
		}
		return nil
	}
	return s.setEachEnumValue(ctx, enum, func(value Value) error {
		_, err := s.setAddElement(ctx, data, value)
		return err
	})
}

func (s *State) setSubtractFrom(ctx context.Context, data *setData, enum Value) error {
	if source, ok := setDataOf(enum); ok {
		for _, entry := range source.entries.entries {
			if _, err := s.setDeleteElement(ctx, data, entry.key); err != nil {
				return err
			}
		}
		return nil
	}
	return s.setEachEnumValue(ctx, enum, func(value Value) error {
		_, err := s.setDeleteElement(ctx, data, value)
		return err
	})
}

func (s *State) setSizeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	return IntValue(int64(len(data.entries.entries))), nil
}

func (s *State) setEmptyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	return BoolValue(ok && len(data.entries.entries) == 0), nil
}

func (s *State) setClearMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	s.setClearData(data)
	return self, nil
}

func (s *State) setReplaceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	s.setClearData(data)
	if err := s.setMergeInto(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	values := make([]Value, len(data.entries.entries))
	for i, entry := range data.entries.entries {
		values[i] = entry.key
	}
	return ArrayValue(values), nil
}

func (s *State) setEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	values := make([]Value, len(data.entries.entries))
	for i, entry := range data.entries.entries {
		values[i] = entry.key
	}
	for _, value := range values {
		if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) setIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	data, ok := setDataOf(self)
	if !ok {
		return False, nil
	}
	found, err := s.setContains(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	return BoolValue(found), nil
}

func (s *State) setAddMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	found, err := s.setContains(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	if found {
		return self, nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if _, err := s.setAddElement(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setAddPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	found, err := s.setContains(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	if found {
		return Nil, nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if _, err := s.setAddElement(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setDeleteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if _, err := s.setDeleteElement(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setDeletePredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	found, err := s.setContains(ctx, data, args[0])
	if err != nil {
		return Nil, err
	}
	if !found {
		return Nil, nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if _, err := s.setDeleteElement(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setMergeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	if err := s.setMergeInto(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setSubtractMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	if err := s.setSubtractFrom(ctx, data, args[0]); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) setUnionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	copyValue, err := s.objectDupMethod(ctx, self, nil, Nil, Nil)
	if err != nil {
		return Nil, err
	}
	copyData, _ := setDataOf(copyValue)
	if err := s.setMergeInto(ctx, copyData, args[0]); err != nil {
		return Nil, err
	}
	return copyValue, nil
}

func (s *State) setDifferenceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	copyValue, err := s.objectDupMethod(ctx, self, nil, Nil, Nil)
	if err != nil {
		return Nil, err
	}
	copyData, _ := setDataOf(copyValue)
	if err := s.setSubtractFrom(ctx, copyData, args[0]); err != nil {
		return Nil, err
	}
	return copyValue, nil
}

func (s *State) setIntersectionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	other, ok := setDataOf(args[0])
	if ok {
		result := newSetOfClass(s.classOf(self), newSetData())
		resultData, _ := setDataOf(result)
		for _, entry := range data.entries.entries {
			found, err := s.setContains(ctx, other, entry.key)
			if err != nil {
				return Nil, err
			}
			if found {
				if _, err := s.setAddElement(ctx, resultData, entry.key); err != nil {
					return Nil, err
				}
			}
		}
		return result, nil
	}
	// An enumerable argument is collected first.
	temp := newSetData()
	if err := s.setMergeInto(ctx, temp, args[0]); err != nil {
		return Nil, err
	}
	result := newSetOfClass(s.classOf(self), newSetData())
	resultData, _ := setDataOf(result)
	for _, entry := range data.entries.entries {
		found, err := s.setContains(ctx, temp, entry.key)
		if err != nil {
			return Nil, err
		}
		if found {
			if _, err := s.setAddElement(ctx, resultData, entry.key); err != nil {
				return Nil, err
			}
		}
	}
	return result, nil
}

func (s *State) setXorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	// Build the result through the receiver's class so an overridden
	// initialize runs before the receiver is copied into it.
	class := s.classOf(self)
	value, err := s.call(ctx, s.classValue(class), "new", nil, Nil, Nil)
	if err != nil {
		return Nil, err
	}
	if _, ok := setDataOf(value); !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize did not produce a Set")}
	}
	if _, err := s.setInitializeCopyMethod(ctx, value, []Value{self}, Nil, Nil); err != nil {
		return Nil, err
	}
	resultData, _ := setDataOf(value)
	other := newSetData()
	if err := s.setMergeInto(ctx, other, args[0]); err != nil {
		return Nil, err
	}
	for _, entry := range other.entries.entries {
		found, err := s.setContains(ctx, resultData, entry.key)
		if err != nil {
			return Nil, err
		}
		if found {
			if _, err := s.setDeleteElement(ctx, resultData, entry.key); err != nil {
				return Nil, err
			}
		} else if _, err := s.setAddElement(ctx, resultData, entry.key); err != nil {
			return Nil, err
		}
	}
	return value, nil
}

func (s *State) setOtherData(self Value, arg Value) (*setData, error) {
	if _, ok := setDataOf(self); !ok {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	other, ok := setDataOf(arg)
	if !ok {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "value must be a Set")}
	}
	return other, nil
}

func (s *State) setSubsetOf(ctx context.Context, selfData, other *setData) (bool, error) {
	if len(selfData.entries.entries) > len(other.entries.entries) {
		return false, nil
	}
	for _, entry := range selfData.entries.entries {
		found, err := s.setContains(ctx, other, entry.key)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func (s *State) setSupersetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, _ := setDataOf(self)
	other, err := s.setOtherData(self, args[0])
	if err != nil {
		return Nil, err
	}
	subset, err := s.setSubsetOf(ctx, other, data)
	if err != nil {
		return Nil, err
	}
	return BoolValue(subset), nil
}

func (s *State) setProperSupersetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, _ := setDataOf(self)
	other, err := s.setOtherData(self, args[0])
	if err != nil {
		return Nil, err
	}
	if len(data.entries.entries) <= len(other.entries.entries) {
		return False, nil
	}
	subset, err := s.setSubsetOf(ctx, other, data)
	if err != nil {
		return Nil, err
	}
	return BoolValue(subset), nil
}

func (s *State) setSubsetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, _ := setDataOf(self)
	other, err := s.setOtherData(self, args[0])
	if err != nil {
		return Nil, err
	}
	subset, err := s.setSubsetOf(ctx, data, other)
	if err != nil {
		return Nil, err
	}
	return BoolValue(subset), nil
}

func (s *State) setProperSubsetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, _ := setDataOf(self)
	other, err := s.setOtherData(self, args[0])
	if err != nil {
		return Nil, err
	}
	if len(data.entries.entries) >= len(other.entries.entries) {
		return False, nil
	}
	subset, err := s.setSubsetOf(ctx, data, other)
	if err != nil {
		return Nil, err
	}
	return BoolValue(subset), nil
}

func (s *State) setIntersectPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, _ := setDataOf(self)
	other, err := s.setOtherData(self, args[0])
	if err != nil {
		return Nil, err
	}
	for _, entry := range data.entries.entries {
		found, err := s.setContains(ctx, other, entry.key)
		if err != nil {
			return Nil, err
		}
		if found {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) setDisjointPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.setIntersectPredicateMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	return BoolValue(!result.IsTruthy()), nil
}

func (s *State) setFilterBang(ctx context.Context, self Value, block Value, keep bool, reportChange bool) (Value, error) {
	if block.IsNil() {
		name := "delete_if"
		if keep {
			name = "keep_if"
		}
		return s.toEnum(self, name, nil), nil
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	snapshot := append([]hashEntry(nil), data.entries.entries...)
	var remove []Value
	for _, entry := range snapshot {
		match, err := s.callBlock1(ctx, block, entry.key, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() != keep {
			remove = append(remove, entry.key)
		}
	}
	if len(remove) == 0 {
		if reportChange {
			return Nil, nil
		}
		return self, nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	for _, key := range remove {
		if _, err := s.setDeleteElement(ctx, data, key); err != nil {
			return Nil, err
		}
	}
	if reportChange {
		return self, nil
	}
	return self, nil
}

func (s *State) setDeleteIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.setFilterBang(ctx, self, block, false, false)
}

func (s *State) setKeepIfMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.setFilterBang(ctx, self, block, true, false)
}

func (s *State) setRejectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.setFilterBang(ctx, self, block, false, true)
}

func (s *State) setSelectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.setFilterBang(ctx, self, block, true, true)
}

func (s *State) setCollectBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "collect!", nil), nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	snapshot := append([]hashEntry(nil), data.entries.entries...)
	result := newSetData()
	for _, entry := range snapshot {
		mapped, err := s.callBlock1(ctx, block, entry.key, Nil)
		if err != nil {
			return Nil, err
		}
		if _, err := s.setAddElement(ctx, result, mapped); err != nil {
			return Nil, err
		}
	}
	data.entries.entries = result.entries.entries
	data.entries.version++
	return self, nil
}

func (s *State) setClassifyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "classify", nil), nil
	}
	data, ok := setDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := HashValue(nil)
	for _, entry := range data.entries.entries {
		key, err := s.callBlock1(ctx, block, entry.key, Nil)
		if err != nil {
			return Nil, err
		}
		index, found, err := s.hashLookupKey(ctx, result.obj.data.(*hashData), key)
		if err != nil {
			return Nil, err
		}
		var group Value
		if found {
			group = result.obj.data.(*hashData).entries[index].value
		} else {
			group = s.setValue(s.classOf(self))
			if _, err := s.setIndex(ctx, result, key, group); err != nil {
				return Nil, err
			}
		}
		groupData, _ := setDataOf(group)
		if _, err := s.setAddElement(ctx, groupData, entry.key); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) setDivideMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "divide", nil), nil
	}
	arity := int64(-1)
	if data, ok := block.obj.data.(*blockData); ok {
		arity = procArity(data)
	}
	if arity == 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NotImplementedErrorClass, "Set#divide with 2 arity block is not implemented.")}
	}
	classified, err := s.setClassifyMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	groups := classified.obj.data.(*hashData).entries
	result := newSetOfClass(s.SetClass, newSetData())
	resultData, _ := setDataOf(result)
	for _, entry := range groups {
		if _, err := s.setAddElement(ctx, resultData, entry.value); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) setFlattenEntries(ctx context.Context, data *setData, ancestors map[*setData]bool, output *setData) error {
	for _, entry := range data.entries.entries {
		if nested, ok := setDataOf(entry.key); ok {
			if ancestors[nested] {
				return &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "tried to flatten recursive Set")}
			}
			ancestors[nested] = true
			err := s.setFlattenEntries(ctx, nested, ancestors, output)
			delete(ancestors, nested)
			if err != nil {
				return err
			}
			continue
		}
		if _, err := s.setAddElement(ctx, output, entry.key); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) setFlattenMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	result := newSetOfClass(s.classOf(self), newSetData())
	resultData, _ := setDataOf(result)
	if err := s.setFlattenEntries(ctx, data, map[*setData]bool{data: true}, resultData); err != nil {
		return Nil, err
	}
	return result, nil
}

func (s *State) setFlattenBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a Set")}
	}
	result := newSetData()
	if err := s.setFlattenEntries(ctx, data, map[*setData]bool{data: true}, result); err != nil {
		return Nil, err
	}
	if len(result.entries.entries) == len(data.entries.entries) {
		same := true
		for i := range result.entries.entries {
			if !identicalValue(result.entries.entries[i].key, data.entries.entries[i].key) {
				same = false
				break
			}
		}
		if same {
			return Nil, nil
		}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data.entries.entries = result.entries.entries
	data.entries.version++
	return self, nil
}

func (s *State) setInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return StringValue("#<Set>"), nil
	}
	text, err := s.setInspectData(ctx, data, map[*setData]bool{})
	if err != nil {
		return Nil, err
	}
	return StringValue(text), nil
}

func (s *State) setInspectData(ctx context.Context, data *setData, seen map[*setData]bool) (string, error) {
	if seen[data] {
		return "Set:[...]", nil
	}
	seen[data] = true
	defer delete(seen, data)
	parts := make([]string, 0, len(data.entries.entries))
	for _, entry := range data.entries.entries {
		if nested, ok := setDataOf(entry.key); ok {
			text, err := s.setInspectData(ctx, nested, seen)
			if err != nil {
				return "", err
			}
			parts = append(parts, text)
			continue
		}
		inspected, err := s.call(ctx, entry.key, "inspect", nil, Nil, Nil)
		if err != nil {
			return "", err
		}
		parts = append(parts, inspected.ToS())
	}
	return "Set[" + strings.Join(parts, ", ") + "]", nil
}

func (s *State) setJoinMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	separator := ""
	if len(args) > 0 {
		separator = args[0].ToS()
	}
	data, ok := setDataOf(self)
	if !ok {
		return StringValue(""), nil
	}
	parts := make([]string, 0, len(data.entries.entries))
	for _, entry := range data.entries.entries {
		value, err := s.call(ctx, entry.key, "to_s", nil, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		parts = append(parts, value.ToS())
	}
	return StringValue(strings.Join(parts, separator)), nil
}

func (s *State) setHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := setDataOf(self)
	if !ok {
		return IntValue(0), nil
	}
	h := int64(0x9e3779b9) ^ int64(len(data.entries.entries))
	for _, entry := range data.entries.entries {
		elementHash, err := s.hashCode(ctx, entry.key)
		if err != nil {
			return Nil, err
		}
		h ^= elementHash * 0x100000001b3
	}
	return IntValue(h), nil
}

func (s *State) setEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	if identicalValue(self, args[0]) {
		return True, nil
	}
	left, ok := setDataOf(self)
	if !ok {
		return False, nil
	}
	right, ok := setDataOf(args[0])
	if !ok || len(left.entries.entries) != len(right.entries.entries) {
		return False, nil
	}
	for _, entry := range left.entries.entries {
		found, err := s.setContains(ctx, right, entry.key)
		if err != nil {
			return Nil, err
		}
		if !found {
			return False, nil
		}
	}
	return True, nil
}

func (s *State) setToSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() && s.classOf(self) == s.SetClass {
		return self, nil
	}
	data := newSetData()
	err := s.setEachEnumValue(ctx, self, func(value Value) error {
		if !block.IsNil() {
			mapped, err := s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return err
			}
			value = mapped
		}
		_, err := s.setAddElement(ctx, data, value)
		return err
	})
	if err != nil {
		return Nil, err
	}
	return newSetOfClass(s.SetClass, data), nil
}

func (s *State) enumerableToSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data := newSetData()
	err := s.setEachEnumValue(ctx, self, func(value Value) error {
		if !block.IsNil() {
			mapped, err := s.callBlock1(ctx, block, value, Nil)
			if err != nil {
				return err
			}
			value = mapped
		}
		_, err := s.setAddElement(ctx, data, value)
		return err
	})
	if err != nil {
		return Nil, err
	}
	return newSetOfClass(s.SetClass, data), nil
}

var _ = fmt.Sprint
