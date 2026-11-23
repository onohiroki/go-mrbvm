package mrbvm

import (
	"context"
	"fmt"
	"strings"
)

type structData struct {
	values []Value
}

func structDataOf(value Value) (*structData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*structData)
	return data, ok
}

func (s *State) installStructLibrary() {
	class := s.StructClass
	includeModule(class, s.EnumerableClass)

	class.classMethods["new"] = &method{owner: class, name: "new", native: s.structClassNewMethod}
	class.classMethods["[]"] = &method{owner: class, name: "[]", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.classNewMethod(ctx, self, args, kw, block)
	}}

	class.define("initialize", s.structInitializeMethod)
	class.define("initialize_copy", s.structInitializeCopyMethod)
	class.define("members", s.structMembersMethod)
	class.define("size", s.structSizeMethod)
	class.define("length", s.structSizeMethod)
	class.define("[]", s.structIndexMethod)
	class.define("[]=", s.structSetIndexMethod)
	class.define("each", s.structEachMethod)
	class.define("each_pair", s.structEachPairMethod)
	class.define("select", s.structSelectMethod)
	class.define("to_a", s.structToAMethod)
	class.define("values", s.structToAMethod)
	class.define("deconstruct", s.structToAMethod)
	class.define("to_h", s.structToHMethod)
	class.define("values_at", s.structValuesAtMethod)
	class.define("dig", s.structDigMethod)
	class.define("inspect", s.structInspectMethod)
	class.define("to_s", s.structInspectMethod)
	class.define("==", s.structEqualMethod)
	class.define("eql?", s.structEqualMethod)
	class.define("hash", s.structHashMethod)
	class.define("deconstruct_keys", s.structDeconstructKeysMethod)
}

func (s *State) structClassNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a class")}
	}
	if class != s.StructClass {
		return s.classNewMethod(ctx, self, args, kw, block)
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
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "struct member must be a Symbol or String")}
		}
		members = append(members, arg.String())
	}

	var keywordInit Value = Nil
	keywordInitSet := false
	if !kw.IsNil() {
		if data, ok := hashDataOf(kw); ok {
			for _, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "keyword_init" {
					keywordInit = entry.value
					keywordInitSet = true
					continue
				}
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keyword: "+entry.key.ToS())}
			}
		}
	}

	created := newClass("#<Class>", s.StructClass)
	created.isStruct = true
	created.structMembers = append([]string(nil), members...)
	created.structKeywordInit = keywordInit
	created.structKeywordInitSet = keywordInitSet
	s.ensureClassObject(created)
	s.registerSubclass(created, s.StructClass)

	if name != "" {
		if !validConstSegment(name) || name[0] < 'A' || name[0] > 'Z' {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "identifier "+name+" needs to be constant")}
		}
		s.renameClassConstant(created, "Struct::"+name)
		created.lexicalOuter = s.StructClass
		s.StructClass.constants[name] = s.classValue(created)
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
		created.define(member+"=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if len(args) != 1 {
				return Nil, argumentCountError(s, len(args), "1")
			}
			if err := s.ensureMutable(self); err != nil {
				return Nil, err
			}
			data, ok := structDataOf(self)
			if !ok {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
			}
			if index >= len(data.values) {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index out of range")}
			}
			data.values[index] = args[0]
			return args[0], nil
		})
	}

	memberSymbols := make([]Value, len(members))
	for i, member := range members {
		memberSymbols[i] = SymbolValue(member)
	}
	created.classMethods["members"] = &method{owner: created, name: "members", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return ArrayValue(append([]Value(nil), memberSymbols...)), nil
	}}
	created.classMethods["keyword_init?"] = &method{owner: created, name: "keyword_init?", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if !keywordInitSet {
			return Nil, nil
		}
		if keywordInit.IsNil() {
			return Nil, nil
		}
		if keywordInit.Kind() == KindFalse {
			return False, nil
		}
		return True, nil
	}}

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

func (s *State) structMembersFor(class *Class) ([]string, bool) {
	for current := class; current != nil; current = current.super {
		if current.isStruct {
			return current.structMembers, true
		}
	}
	return nil, false
}

func (s *State) structKeywordInitFor(class *Class) (Value, bool) {
	for current := class; current != nil; current = current.super {
		if current.isStruct {
			if current.structKeywordInitSet {
				return current.structKeywordInit, true
			}
			return Nil, false
		}
	}
	return Nil, false
}

func (s *State) structInitializeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	members, ok := s.structMembersFor(s.classOf(self))
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	keywordInit, keywordSet := s.structKeywordInitFor(s.classOf(self))
	keywordMode := keywordSet && keywordInit.IsTruthy()
	positional := append([]Value(nil), args...)
	kwData, hasKw := hashDataOf(kw)
	hasKeywords := hasKw && len(kwData.entries) > 0

	if keywordMode {
		if len(positional) > 0 {
			return Nil, argumentCountError(s, len(positional), "0")
		}
	} else if keywordSet && !keywordInit.IsTruthy() {
		if hasKeywords {
			positional = append(positional, kw)
		}
	} else if hasKeywords && len(positional) == 0 {
		// A bare keyword list names the members.
	} else if hasKeywords {
		positional = append(positional, kw)
	}

	if len(positional) > len(members) {
		return Nil, argumentCountError(s, len(positional), fmt.Sprintf("0..%d", len(members)))
	}
	values := make([]Value, len(members))
	for i := range values {
		values[i] = Nil
	}
	copy(values, positional)

	assignKeyword := func() error {
		var unknown []string
		for _, entry := range kwData.entries {
			name := entry.key.String()
			found := false
			if entry.key.Kind() == KindSymbol {
				for i, member := range members {
					if member == name {
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
			return &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keywords: "+strings.Join(unknown, ", "))}
		}
		return nil
	}
	if hasKeywords && len(positional) == 0 && !(keywordSet && !keywordInit.IsTruthy()) {
		if err := assignKeyword(); err != nil {
			return Nil, err
		}
	}
	if data, ok := structDataOf(self); ok {
		data.values = values
	} else {
		self.obj.data = &structData{values: values}
	}
	return self, nil
}

func (s *State) structInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if identicalValue(self, args[0]) {
		return self, nil
	}
	source, ok := structDataOf(args[0])
	if !ok || s.classOf(self) != s.classOf(args[0]) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize_copy requires a struct of the same type")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if data, ok := structDataOf(self); ok {
		data.values = append([]Value(nil), source.values...)
	} else {
		self.obj.data = &structData{values: append([]Value(nil), source.values...)}
	}
	return self, nil
}

func (s *State) structMembersMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	members, ok := s.structMembersFor(s.classOf(self))
	if !ok {
		return ArrayValue(nil), nil
	}
	values := make([]Value, len(members))
	for i, member := range members {
		values[i] = SymbolValue(member)
	}
	return ArrayValue(values), nil
}

func (s *State) structSizeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return IntValue(0), nil
	}
	return IntValue(int64(len(data.values))), nil
}

func (s *State) structResolveMember(self Value, arg Value) (int, error) {
	members, ok := s.structMembersFor(s.classOf(self))
	if !ok {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
	}
	switch arg.Kind() {
	case KindInt:
		index := int(arg.Int())
		if index < 0 {
			index += len(members)
		}
		if index < 0 || index >= len(members) {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index out of range")}
		}
		return index, nil
	case KindSymbol, KindString:
		name := arg.String()
		for i, member := range members {
			if member == name {
				return i, nil
			}
		}
		return 0, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "no member '"+name+"' in struct")}
	default:
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
}

func (s *State) structIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	index, err := s.structResolveMember(self, args[0])
	if err != nil {
		return Nil, err
	}
	data, ok := structDataOf(self)
	if !ok || index >= len(data.values) {
		return Nil, nil
	}
	return data.values[index], nil
}

func (s *State) structSetIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "2")
	}
	index, err := s.structResolveMember(self, args[0])
	if err != nil {
		return Nil, err
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := structDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
	}
	data.values[index] = args[1]
	return args[1], nil
}

func (s *State) structEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	data, ok := structDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
	}
	for _, value := range data.values {
		if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) structEachPairMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_pair", nil), nil
	}
	members, _ := s.structMembersFor(s.classOf(self))
	data, ok := structDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a struct")}
	}
	for i, value := range data.values {
		if i >= len(members) {
			break
		}
		if _, err := s.callBlock2(ctx, block, SymbolValue(members[i]), value, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) structSelectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "select", nil), nil
	}
	data, ok := structDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	var result []Value
	for _, value := range data.values {
		match, err := s.callBlock1(ctx, block, value, Nil)
		if err != nil {
			return Nil, err
		}
		if match.IsTruthy() {
			result = append(result, value)
		}
	}
	return ArrayValue(result), nil
}

func (s *State) structToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	return ArrayValue(append([]Value(nil), data.values...)), nil
}

func (s *State) structToHMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	members, _ := s.structMembersFor(s.classOf(self))
	data, ok := structDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	if block.IsNil() {
		entries := make([]hashEntry, 0, len(data.values))
		for i, value := range data.values {
			if i >= len(members) {
				break
			}
			entries = append(entries, hashEntry{key: SymbolValue(members[i]), value: value})
		}
		return HashValue(entries), nil
	}
	result := HashValue(nil)
	for i, value := range data.values {
		if i >= len(members) {
			break
		}
		pair, err := s.callBlock2(ctx, block, SymbolValue(members[i]), value, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
		pairData, ok := arrayDataOf(pair)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, fmt.Sprintf("wrong element type %s (expected Array)", s.classOf(pair).name))}
		}
		if len(pairData.values) != 2 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("element has wrong array length (expected 2, was %d)", len(pairData.values)))}
		}
		if _, err := s.setIndex(ctx, result, pairData.values[0], pairData.values[1]); err != nil {
			return Nil, err
		}
	}
	return result, nil
}

func (s *State) structValuesAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	values := make([]Value, len(args))
	for i, arg := range args {
		index, err := s.structResolveMember(self, arg)
		if err != nil {
			return Nil, err
		}
		if index < 0 || index >= len(data.values) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index out of range")}
		}
		values[i] = data.values[index]
	}
	return ArrayValue(values), nil
}

func (s *State) structDigMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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

func (s *State) structInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return StringValue("#<struct>"), nil
	}
	text, err := s.structInspectData(ctx, s.classOf(self), data, map[*structData]bool{}, "struct")
	if err != nil {
		return Nil, err
	}
	return StringValue(text), nil
}

func (s *State) structInspectData(ctx context.Context, class *Class, data *structData, seen map[*structData]bool, kind string) (string, error) {
	if seen[data] {
		return "#<struct ...>", nil
	}
	seen[data] = true
	defer delete(seen, data)
	members, _ := s.structMembersFor(class)
	parts := make([]string, 0, len(data.values))
	for i, value := range data.values {
		name := ""
		if i < len(members) {
			name = members[i]
		}
		if nested, ok := structDataOf(value); ok {
			nestedKind := "struct"
			if s.classOf(value).isData {
				nestedKind = "data"
			}
			text, err := s.structInspectData(ctx, s.classOf(value), nested, seen, nestedKind)
			if err != nil {
				return "", err
			}
			parts = append(parts, name+"="+text)
			continue
		}
		inspected, err := s.call(ctx, value, "inspect", nil, Nil, Nil)
		if err != nil {
			return "", err
		}
		parts = append(parts, name+"="+inspected.ToS())
	}
	return "#<" + kind + " " + strings.Join(parts, ", ") + ">", nil
}

func (s *State) structValueEqual(ctx context.Context, left, right Value, seen map[[2]*structData]bool) (bool, error) {
	leftData, leftOK := structDataOf(left)
	rightData, rightOK := structDataOf(right)
	if leftOK && rightOK {
		return s.structEqualData(ctx, leftData, rightData, seen)
	}
	return s.rubyEqual(ctx, left, right)
}

func (s *State) structEqualData(ctx context.Context, left, right *structData, seen map[[2]*structData]bool) (bool, error) {
	pair := [2]*structData{left, right}
	if seen[pair] {
		return true, nil
	}
	seen[pair] = true
	for i := 0; i < len(right.values); i++ {
		if i >= len(left.values) {
			return false, nil
		}
		equal, err := s.structValueEqual(ctx, left.values[i], right.values[i], seen)
		if err != nil || !equal {
			return false, err
		}
	}
	return len(left.values) == len(right.values), nil
}

func (s *State) structEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	if identicalValue(self, args[0]) {
		return True, nil
	}
	left, ok := structDataOf(self)
	if !ok {
		return False, nil
	}
	right, ok := structDataOf(args[0])
	if !ok || s.classOf(self) != s.classOf(args[0]) {
		return False, nil
	}
	equal, err := s.structEqualData(ctx, left, right, map[[2]*structData]bool{})
	if err != nil {
		return Nil, err
	}
	return BoolValue(equal), nil
}

func (s *State) structHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := structDataOf(self)
	if !ok {
		return IntValue(0), nil
	}
	h := int64(0x9e3779b9)
	for _, value := range data.values {
		valueHash, err := s.hashCode(ctx, value)
		if err != nil {
			return Nil, err
		}
		h = h*31 + valueHash
	}
	return IntValue(h), nil
}

func (s *State) structDeconstructKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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
		index := -1
		switch key.Kind() {
		case KindInt:
			index = int(key.Int())
			if index < 0 {
				index += len(members)
			}
		case KindSymbol, KindString:
			name := key.String()
			for i, member := range members {
				if member == name {
					index = i
					break
				}
			}
		case KindObject:
			if rat, ok := rationalOf(key); ok && rat.IsInt() && rat.Num().IsInt64() {
				index = int(rat.Num().Int64())
				if index < 0 {
					index += len(members)
				}
				break
			}
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		default:
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		if index < 0 || index >= len(members) || index >= len(data.values) {
			break
		}
		if _, err := s.setIndex(ctx, result, key, data.values[index]); err != nil {
			return Nil, err
		}
	}
	return result, nil
}
