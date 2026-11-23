package mrbvm

import (
	"context"
	"fmt"
	"strings"
)

func (s *State) installDataLibrary() {
	class := s.DataClass
	class.classMethods["define"] = &method{owner: class, name: "define", native: s.dataDefineMethod}
}

func classIsData(class *Class) bool {
	for current := class; current != nil; current = current.super {
		if current.isData {
			return true
		}
	}
	return false
}

func (s *State) dataDefineMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	base, ok := s.asClass(self)
	if !ok || !classIsData(base) && base != s.DataClass {
		base = s.DataClass
	}
	name := ""
	members := make([]string, 0, len(args))
	start := 0
	if len(args) > 0 && args[0].Kind() == KindString {
		name = args[0].String()
		start = 1
	}
	for _, arg := range args[start:] {
		if arg.Kind() != KindSymbol && arg.Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "data member must be a Symbol or String")}
		}
		members = append(members, arg.String())
	}
	created := newClass("#<Class>", s.DataClass)
	created.isStruct = true
	created.isData = true
	created.structMembers = append([]string(nil), members...)
	s.ensureClassObject(created)
	s.registerSubclass(created, s.DataClass)

	if name != "" {
		if !validConstSegment(name) || name[0] < 'A' || name[0] > 'Z' {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "identifier "+name+" needs to be constant")}
		}
		s.renameClassConstant(created, "Data::"+name)
		created.lexicalOuter = s.DataClass
		s.DataClass.constants[name] = s.classValue(created)
	}

	for index, member := range members {
		index, member := index, member
		created.define(member, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			data, ok := structDataOf(self)
			if !ok || index >= len(data.values) {
				return Nil, nil
			}
			return data.values[index], nil
		})
	}

	memberSymbols := make([]Value, len(members))
	for i, member := range members {
		memberSymbols[i] = SymbolValue(member)
	}
	created.classMethods["members"] = &method{owner: created, name: "members", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return ArrayValue(append([]Value(nil), memberSymbols...)), nil
	}}
	created.classMethods["new"] = &method{owner: created, name: "new", native: s.dataNewMethod}

	if !block.IsNil() {
		saved := created.defaultVisibility
		created.defaultVisibility = ""
		_, err := s.callBlockWithSelf(ctx, block, s.classValue(created), []Value{s.classValue(created)}, Nil)
		created.defaultVisibility = saved
		if err != nil {
			return Nil, err
		}
	}
	return s.classValue(created), nil
}

func (s *State) dataDefaultInitialize(class *Class) bool {
	m, found := class.lookup("initialize")
	if !found {
		return true
	}
	return m.owner == s.DataClass && m.name == "initialize" && m.native != nil
}

func (s *State) dataNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a class")}
	}
	members, ok := s.structMembersFor(class)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a data class")}
	}
	if !s.dataDefaultInitialize(class) {
		if len(args) > len(members) {
			return Nil, argumentCountError(s, len(args), fmt.Sprintf("0..%d", len(members)))
		}
		var entries []hashEntry
		for i, value := range args {
			entries = append(entries, hashEntry{key: SymbolValue(members[i]), value: value})
		}
		if kwData, ok := hashDataOf(kw); ok {
			for _, entry := range kwData.entries {
				replaced := false
				for i := range entries {
					if valuesEqual(entries[i].key, entry.key) {
						entries[i].value = entry.value
						replaced = true
						break
					}
				}
				if !replaced {
					entries = append(entries, entry)
				}
			}
		}
		instance := ObjectValue(&Object{class: class})
		if _, err := s.call(ctx, instance, "initialize", nil, HashValue(entries), Nil); err != nil {
			return Nil, err
		}
		return instance, nil
	}

	values := make([]Value, len(members))
	for i := range values {
		values[i] = Nil
	}
	if kwData, ok := hashDataOf(kw); ok && len(kwData.entries) > 0 {
		var unknown []string
		for _, entry := range kwData.entries {
			found := false
			if entry.key.Kind() == KindSymbol {
				for i, member := range members {
					if member == entry.key.String() {
						values[i] = entry.value
						found = true
						break
					}
				}
			}
			if !found {
				unknown = append(unknown, entry.key.ToS())
			}
		}
		if len(unknown) > 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keywords: "+strings.Join(unknown, ", "))}
		}
	} else {
		if len(args) != len(members) {
			return Nil, argumentCountError(s, len(args), fmt.Sprint(len(members)))
		}
		copy(values, args)
	}
	value := ObjectValue(&Object{class: class, data: &structData{values: values}})
	value.obj.frozen = true
	return value, nil
}

func (s *State) dataInitializeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 0 {
		return Nil, argumentCountError(s, len(args), "0")
	}
	members, ok := s.structMembersFor(s.classOf(self))
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a data class")}
	}
	values := make([]Value, len(members))
	for i := range values {
		values[i] = Nil
	}
	kwData, _ := hashDataOf(kw)
	seen := make(map[string]bool)
	if kwData != nil {
		for _, entry := range kwData.entries {
			name := entry.key.String()
			found := false
			if entry.key.Kind() == KindSymbol {
				for i, member := range members {
					if member == name {
						values[i] = entry.value
						seen[name] = true
						found = true
						break
					}
				}
			}
			if !found {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keyword: "+entry.key.ToS())}
			}
		}
	}
	for _, member := range members {
		if !seen[member] {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "missing keyword: "+member)}
		}
	}
	self.obj.data = &structData{values: values}
	self.obj.frozen = true
	return self, nil
}

func (s *State) dataWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 0 {
		return Nil, argumentCountError(s, len(args), "0")
	}
	data, ok := structDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a data")}
	}
	kwData, hasKw := hashDataOf(kw)
	if !hasKw || len(kwData.entries) == 0 {
		return self, nil
	}
	members, _ := s.structMembersFor(s.classOf(self))
	values := append([]Value(nil), data.values...)
	for _, entry := range kwData.entries {
		found := false
		if entry.key.Kind() == KindSymbol {
			for i, member := range members {
				if member == entry.key.String() {
					values[i] = entry.value
					found = true
					break
				}
			}
		}
		if !found {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keyword: "+entry.key.ToS())}
		}
	}
	result := ObjectValue(&Object{class: s.classOf(self), data: &structData{values: values}})
	result.obj.frozen = true
	return result, nil
}

func (s *State) dataInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return StringValue("#<data >"), nil
	}
	text, err := s.structInspectData(ctx, s.classOf(self), data, map[*structData]bool{}, "data")
	if err != nil {
		return Nil, err
	}
	return StringValue(text), nil
}

func (s *State) dataDeconstructKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := structDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	members, _ := s.structMembersFor(s.classOf(self))
	if args[0].IsNil() {
		entries := make([]hashEntry, 0, len(data.values))
		for i, value := range data.values {
			if i >= len(members) {
				break
			}
			entries = append(entries, hashEntry{key: SymbolValue(members[i]), value: value})
		}
		return HashValue(entries), nil
	}
	keys, ok := arrayDataOf(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type "+s.classOf(args[0]).name+" (expected Array or nil)")}
	}
	if len(keys.values) > len(members) {
		return HashValue(nil), nil
	}
	result := HashValue(nil)
	for i := 0; i < len(keys.values); i++ {
		key := keys.values[i]
		var name string
		switch key.Kind() {
		case KindSymbol, KindString:
			name = key.String()
		default:
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, key.Inspect()+" is not a symbol nor a string")}
		}
		index := -1
		for j, member := range members {
			if member == name {
				index = j
				break
			}
		}
		if index < 0 {
			break
		}
		if _, err := s.setIndex(ctx, result, key, data.values[index]); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) installDataInstanceMethods() {
	class := s.DataClass
	class.define("initialize", s.dataInitializeMethod)
	class.define("members", s.structMembersMethod)
	class.define("size", s.structSizeMethod)
	class.define("length", s.structSizeMethod)
	class.define("to_a", s.structToAMethod)
	class.define("values", s.structToAMethod)
	class.define("deconstruct", s.structToAMethod)
	class.define("to_h", s.structToHMethod)
	class.define("inspect", s.dataInspectMethod)
	class.define("to_s", s.dataInspectMethod)
	class.define("==", s.structEqualMethod)
	class.define("eql?", s.structEqualMethod)
	class.define("hash", s.structHashMethod)
	class.define("deconstruct_keys", s.dataDeconstructKeysMethod)
	class.define("with", s.dataWithMethod)
}
