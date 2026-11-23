package mrbvm

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

func validInstanceVarName(name string) bool {
	if len(name) < 2 || name[0] != '@' || name[1] == '@' {
		return false
	}
	for i, r := range name[1:] {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func validClassVarName(name string) bool {
	if len(name) < 3 || !strings.HasPrefix(name, "@@") || name[0] != '@' || name[1] != '@' {
		return false
	}
	for i, r := range name[2:] {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (s *State) installMetaprogLibrary() {
	object := s.ObjectClass
	object.define("instance_variable_defined?", s.metaprogInstanceVariableDefinedMethod)
	object.define("instance_variable_get", s.metaprogInstanceVariableGetMethod)
	object.define("instance_variable_set", s.metaprogInstanceVariableSetMethod)
	object.define("methods", s.metaprogMethodsMethod("any"))
	object.define("public_methods", s.metaprogMethodsMethod("public"))
	object.define("private_methods", s.metaprogMethodsMethod("private"))
	object.define("protected_methods", s.metaprogMethodsMethod("protected"))
	object.define("singleton_methods", s.metaprogSingletonMethodsMethod)
	object.define("global_variables", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		names := make([]string, 0, len(s.globals))
		for name := range s.globals {
			names = append(names, name)
		}
		sort.Strings(names)
		values := make([]Value, len(names))
		for i, name := range names {
			values[i] = SymbolValue(name)
		}
		return ArrayValue(values), nil
	})
	object.define("local_variables", s.metaprogLocalVariablesMethod)
	object.define("define_singleton_method", s.metaprogDefineSingletonMethod)
	object.define("public_send", s.metaprogPublicSendMethod)
	object.define("fail", s.kernelRaise)
	object.define("Integer", s.kernelIntegerMethod)
	object.define("Float", s.kernelFloatMethod)
	object.define("String", s.kernelStringMethod)
	object.define("Array", s.kernelArrayMethod)
	object.define("Hash", s.kernelHashMethod)
	object.define("__method__", s.kernelMethodNameMethod)
	object.define("__callee__", s.kernelMethodNameMethod)

	module := s.ModuleClass
	module.define("class_variable_defined?", s.metaprogClassVariableDefinedMethod)
	module.define("class_variable_get", s.metaprogClassVariableGetMethod)
	module.define("class_variable_set", s.metaprogClassVariableSetMethod)
	module.define("class_variables", s.metaprogClassVariablesMethod)
	module.define("remove_class_variable", s.metaprogRemoveClassVariableMethod)
	module.define("constants", s.metaprogConstantsMethod)
	module.define("included_modules", s.metaprogIncludedModulesMethod)
	module.define("instance_methods", s.metaprogInstanceMethodsMethod("any"))
	module.define("public_instance_methods", s.metaprogInstanceMethodsMethod("public"))
	module.define("private_instance_methods", s.metaprogInstanceMethodsVisibilityMethod("private"))
	module.define("protected_instance_methods", s.metaprogInstanceMethodsVisibilityMethod("protected"))
	module.define("public_method_defined?", s.metaprogMethodDefinedMethod("public"))
	module.define("private_method_defined?", s.metaprogMethodDefinedMethod("private"))
	module.define("protected_method_defined?", s.metaprogMethodDefinedMethod("protected"))
	module.define("remove_method", s.metaprogRemoveMethod)
	module.define("remove_class_variable", s.metaprogRemoveClassVariableMethod)
	module.classMethods["nesting"] = &method{owner: s.ModuleClass, name: "nesting", native: s.metaprogNestingMethod}
	s.ClassClass.define("remove_method", s.metaprogRemoveMethod)
}

func (s *State) metaprogIvarName(args []Value) (string, error) {
	if len(args) != 1 {
		return "", argumentCountError(s, len(args), "1")
	}
	name := args[0].String()
	if !validInstanceVarName(name) {
		return "", &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid instance variable name "+name)}
	}
	return name, nil
}

func (s *State) metaprogInstanceVariableDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	name, err := s.metaprogIvarName(args)
	if err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return False, nil
	}
	_, ok := self.obj.ivars[name]
	return BoolValue(ok), nil
}

func (s *State) metaprogInstanceVariableGetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	name, err := s.metaprogIvarName(args)
	if err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return Nil, nil
	}
	return self.obj.ivar(name), nil
}

func (s *State) metaprogInstanceVariableSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "2")
	}
	name := args[0].String()
	if !validInstanceVarName(name) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid instance variable name "+name)}
	}
	if self.obj == nil || self.Kind() == KindSymbol || self.Kind() == KindInt ||
		self.Kind() == KindTrue || self.Kind() == KindFalse || self.Kind() == KindNil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FrozenErrorClass, "can't modify frozen object")}
	}
	if _, isRange := self.obj.data.(*rangeData); isRange {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "can't modify frozen Range")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	self.obj.setIvar(name, args[1])
	return args[1], nil
}

func (s *State) metaprogNameArg(args []Value) (string, error) {
	if len(args) < 1 {
		return "", argumentCountError(s, len(args), "1")
	}
	if args[0].Kind() != KindSymbol && args[0].Kind() != KindString {
		return "", &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "method name must be a Symbol or String")}
	}
	return args[0].String(), nil
}

func methodVisibility(class *Class, name string) string {
	if class.privateMethods[name] {
		return "private"
	}
	if class.protectedMethods[name] {
		return "protected"
	}
	return "public"
}

// nearestMethods maps each method name reachable from class to the visibility
// of its nearest definition. inherit limits the walk to the class itself.
func nearestMethods(class *Class, inherit bool) map[string]string {
	result := make(map[string]string)
	if class.singletonOf != nil {
		owner := class.singletonOf
		for name, m := range owner.classMethods {
			if m.owner != class {
				continue
			}
			visibility := "public"
			if owner.privateClassMethods[name] {
				visibility = "private"
			} else if owner.protectedClassMethods[name] {
				visibility = "protected"
			}
			result[name] = visibility
		}
		if !inherit {
			return result
		}
	}
	ancestors := []*Class{class}
	if inherit {
		ancestors = classAncestors(class)
	}
	for _, current := range ancestors {
		for name := range current.methods {
			if _, seen := result[name]; seen {
				continue
			}
			if current.undefMethods[name] {
				result[name] = "undef"
				continue
			}
			result[name] = methodVisibility(current, name)
		}
		for name := range current.privateMethods {
			if _, seen := result[name]; !seen {
				result[name] = "private"
			}
		}
		for name := range current.protectedMethods {
			if _, seen := result[name]; !seen {
				result[name] = "protected"
			}
		}
		for name := range current.undefMethods {
			result[name] = "undef"
		}
	}
	return result
}

func symbolsNames(names map[string]string, want string) []Value {
	selected := make([]string, 0, len(names))
	for name, visibility := range names {
		if visibility == "undef" {
			continue
		}
		switch want {
		case "any":
			if visibility == "private" {
				continue
			}
		case "public":
			if visibility != "public" {
				continue
			}
		case "private", "protected":
			if visibility != want {
				continue
			}
		}
		selected = append(selected, name)
	}
	sort.Strings(selected)
	values := make([]Value, len(selected))
	for i, name := range selected {
		values[i] = SymbolValue(name)
	}
	return values
}

func (s *State) metaprogMethodsMethod(want string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		inherit := true
		if len(args) > 0 {
			inherit = args[0].IsTruthy()
		}
		var names map[string]string
		if class, ok := s.asClass(self); ok {
			names = make(map[string]string)
			if inherit {
				for current := class; current != nil; current = current.super {
					for name := range current.classMethods {
						if _, seen := names[name]; !seen {
							if current.privateClassMethods[name] {
								names[name] = "private"
							} else if current.protectedClassMethods[name] {
								names[name] = "protected"
							} else {
								names[name] = "public"
							}
						}
					}
				}
				singleton := s.classOf(self)
				for _, current := range classAncestors(singleton) {
					for name := range current.methods {
						if _, seen := names[name]; !seen {
							names[name] = methodVisibility(current, name)
						}
					}
				}
			} else {
				for name, m := range class.classMethods {
					if m.owner != class {
						continue
					}
					if class.privateClassMethods[name] {
						names[name] = "private"
					} else if class.protectedClassMethods[name] {
						names[name] = "protected"
					} else {
						names[name] = "public"
					}
				}
			}
			return ArrayValue(symbolsNames(names, want)), nil
		}
		names = nearestMethods(s.classOf(self), inherit)
		if inherit {
			if self.obj != nil && self.obj.singleton != nil {
				for _, current := range classAncestors(self.obj.singleton) {
					for name := range current.methods {
						if _, seen := names[name]; !seen {
							names[name] = methodVisibility(current, name)
						}
					}
				}
			}
		}
		return ArrayValue(symbolsNames(names, want)), nil
	}
}

func (s *State) metaprogSingletonMethodsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	names := make(map[string]string)
	if class, ok := s.asClass(self); ok {
		for name, m := range class.classMethods {
			if m.owner != class {
				continue
			}
			if class.privateClassMethods[name] {
				names[name] = "private"
			} else {
				names[name] = "public"
			}
		}
	} else if self.obj != nil && self.obj.singleton != nil {
		for name := range self.obj.singleton.methods {
			names[name] = methodVisibility(self.obj.singleton, name)
		}
	}
	return ArrayValue(symbolsNames(names, "any")), nil
}

func (s *State) metaprogLocalVariablesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	seen := make(map[string]bool)
	var values []Value
	if len(s.frames) > 0 {
		env := s.frames[len(s.frames)-1].env
		for ; env != nil; env = env.parent {
			for _, name := range env.names {
				if name == "" || strings.HasPrefix(name, "__") || name == "self" || seen[name] {
					continue
				}
				seen[name] = true
				values = append(values, SymbolValue(name))
			}
		}
	}
	return ArrayValue(values), nil
}

func (s *State) metaprogDefineSingletonMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	name, err := s.metaprogNameArg(args)
	if err != nil {
		return Nil, err
	}
	if self.obj == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't define singleton")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	body := block
	if len(args) == 2 {
		switch args[1].Kind() {
		case KindObject:
			if data, ok := args[1].obj.data.(*blockData); ok {
				body = args[1]
				_ = data
			} else if _, ok := args[1].obj.data.(*nativeBlockData); ok {
				body = args[1]
			} else {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type")}
			}
		default:
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type")}
		}
	}
	if body.IsNil() {
		return Nil, argumentCountError(s, len(args), "1+")
	}
	singleton := self.obj.singleton
	if singleton == nil {
		singleton = newClass("#<Class:"+s.classOf(self).name+">", s.classOf(self))
		singleton.isSingleton = true
		singleton.singletonOwner = self
		s.ensureClassObject(singleton)
		self.obj.singleton = singleton
	}
	singleton.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := body.obj.data.(*blockData); ok {
			copyData := *data
			copyData.mid = name
			return s.executeFrame(ctx, copyData.irep, self, copyData.targetClass, args, kw, block, copyData.env, name, copyData.owner, copyData.cref, copyData.strict)
		}
		return s.callBlockWithSelfAndBlock(ctx, body, self, args, kw, block)
	})
	if err := s.notifyMethodDefinition(ctx, singleton, name); err != nil {
		return Nil, err
	}
	return SymbolValue(name), nil
}

func (s *State) metaprogPublicSendMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return Nil, argumentCountError(s, 0, "1+")
	}
	name := args[0].String()
	class := s.classOf(self)
	if m, found := class.lookup(name); found {
		owner := m.owner
		if owner == nil {
			owner = class
		}
		if owner.privateMethods[name] || owner.protectedMethods[name] {
			return Nil, &unwindSignal{kind: unwindException, value: s.newVisibilityError("private", name, self)}
		}
	}
	return s.call(ctx, self, name, args[1:], kw, block)
}

func classVariableOwner(class *Class, name string) *Class {
	return classVarOwner(class, name)
}

func (s *State) metaprogClassVariableDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	name := args[0].String()
	if !validClassVarName(name) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid class variable name")}
	}
	return BoolValue(classVariableOwner(class, name) != nil), nil
}

func (s *State) metaprogClassVariableGetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name := args[0].String()
	if !validClassVarName(name) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid class variable name")}
	}
	owner := classVariableOwner(class, name)
	if owner == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "uninitialized class variable "+name)}
	}
	return owner.classVars[name], nil
}

func (s *State) metaprogClassVariableSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "2")
	}
	name := args[0].String()
	if !validClassVarName(name) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid class variable name")}
	}
	owner := classVariableOwner(class, name)
	if owner == nil {
		owner = class
	}
	if err := s.ensureMutable(s.classValue(owner)); err != nil {
		return Nil, err
	}
	owner.classVars[name] = args[1]
	return args[1], nil
}

func (s *State) metaprogClassVariablesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	seen := make(map[string]bool)
	var names []string
	for _, current := range classAncestors(class) {
		for name := range current.classVars {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	values := make([]Value, len(names))
	for i, name := range names {
		values[i] = SymbolValue(name)
	}
	return ArrayValue(values), nil
}

func (s *State) metaprogRemoveClassVariableMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name := args[0].String()
	if !validClassVarName(name) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid class variable name")}
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	value, found := class.classVars[name]
	if !found {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "class variable "+name+" not defined")}
	}
	delete(class.classVars, name)
	return value, nil
}

func (s *State) metaprogConstantsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	if class == s.ModuleClass && len(s.frames) > 0 && s.frames[len(s.frames)-1].cref != nil {
		class = s.frames[len(s.frames)-1].cref
	}
	seen := make(map[string]bool)
	var names []string
	var visit func(*Class)
	visit = func(current *Class) {
		for name := range current.constants {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		for name := range current.autoloads {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		for _, module := range append(append([]*Class(nil), current.prepended...), current.included...) {
			visit(module)
		}
	}
	visit(class)
	sort.Strings(names)
	values := make([]Value, len(names))
	for i, name := range names {
		values[i] = SymbolValue(name)
	}
	return ArrayValue(values), nil
}

func (s *State) metaprogIncludedModulesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	seen := make(map[*Class]bool)
	var modules []*Class
	var visit func(*Class)
	visit = func(current *Class) {
		for _, module := range append(append([]*Class(nil), current.prepended...), current.included...) {
			if seen[module] || module == current {
				continue
			}
			seen[module] = true
			modules = append(modules, module)
			visit(module)
		}
	}
	visit(class)
	for current := class.super; current != nil; current = current.super {
		visit(current)
	}
	values := make([]Value, len(modules))
	for i, module := range modules {
		values[i] = s.classValue(module)
	}
	return ArrayValue(values), nil
}

func (s *State) metaprogInstanceMethodsMethod(want string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		inherit := true
		if len(args) > 0 {
			inherit = args[0].IsTruthy()
		}
		names := nearestMethods(class, inherit)
		return ArrayValue(symbolsNames(names, want)), nil
	}
}

func (s *State) metaprogInstanceMethodsVisibilityMethod(want string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		inherit := true
		if len(args) > 0 {
			inherit = args[0].IsTruthy()
		}
		names := nearestMethods(class, inherit)
		return ArrayValue(symbolsNames(names, want)), nil
	}
}

func (s *State) metaprogMethodDefinedMethod(want string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			return False, nil
		}
		if len(args) < 1 || len(args) > 2 {
			return Nil, argumentCountError(s, len(args), "1..2")
		}
		if args[0].Kind() != KindSymbol && args[0].Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "method name must be a Symbol or String")}
		}
		name := args[0].String()
		inherit := true
		if len(args) == 2 {
			inherit = args[1].IsTruthy()
		}
		if m, found := class.lookup(name); found && m.notImplemented {
			return False, nil
		}
		names := nearestMethods(class, inherit)
		return BoolValue(names[name] == want), nil
	}
}

func (s *State) metaprogRemoveMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) < 1 {
		return Nil, argumentCountError(s, len(args), "1+")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	names := make([]string, 0, len(args))
	for _, arg := range args {
		if arg.Kind() != KindSymbol && arg.Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "method name must be a Symbol or String")}
		}
		name := arg.String()
		if _, found := class.methods[name]; !found {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "method '"+name+"' not defined in "+class.name)}
		}
		names = append(names, name)
	}
	for _, name := range names {
		delete(class.methods, name)
	}
	if _, found := class.lookupClassMethod("method_removed"); found {
		for _, name := range names {
			if _, err := s.call(ctx, s.classValue(class), "method_removed", []Value{SymbolValue(name)}, Nil, Nil); err != nil {
				return Nil, err
			}
		}
	}
	return self, nil
}

func (s *State) metaprogNestingMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(s.frames) == 0 {
		return ArrayValue(nil), nil
	}
	cref := s.frames[len(s.frames)-1].cref
	values := make([]Value, 0, 4)
	seen := make(map[*Class]bool)
	for current := cref; current != nil; current = current.lexicalOuter {
		if current == s.ObjectClass || seen[current] {
			continue
		}
		seen[current] = true
		values = append(values, s.classValue(current))
	}
	return ArrayValue(values), nil
}

func (s *State) metaprogTypeNameError(name string) error {
	return &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, fmt.Sprintf("wrong argument type for %s", name))}
}

var errBadNumber = fmt.Errorf("bad number")

func trimNumberSpace(text string) string {
	return strings.Trim(text, " \t\r\n\f\v")
}

func validUnderscores(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] != '_' {
			continue
		}
		if i == 0 || i+1 == len(text) {
			return false
		}
		if !isHexDigit(text[i-1]) || !isHexDigit(text[i+1]) {
			return false
		}
	}
	return true
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func parseStrictInteger(text string, base int) (Value, error) {
	text = trimNumberSpace(text)
	if text == "" {
		return Nil, errBadNumber
	}
	sign := int64(1)
	negative := false
	if text[0] == '+' || text[0] == '-' {
		negative = text[0] == '-'
		text = text[1:]
	}
	if text == "" {
		return Nil, errBadNumber
	}
	if base == 0 {
		switch {
		case strings.HasPrefix(text, "0x"), strings.HasPrefix(text, "0X"):
			base, text = 16, text[2:]
		case strings.HasPrefix(text, "0b"), strings.HasPrefix(text, "0B"):
			base, text = 2, text[2:]
		case strings.HasPrefix(text, "0o"), strings.HasPrefix(text, "0O"):
			base, text = 8, text[2:]
		case strings.HasPrefix(text, "0d"), strings.HasPrefix(text, "0D"):
			base, text = 10, text[2:]
		case len(text) > 1 && text[0] == '0':
			base = 8
		default:
			base = 10
		}
	} else {
		if base < 0 {
			base = -base
		}
		if base < 2 || base > 36 {
			return Nil, errBadNumber
		}
		if base == 16 && (strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X")) {
			text = text[2:]
		}
		if base == 2 && (strings.HasPrefix(text, "0b") || strings.HasPrefix(text, "0B")) {
			text = text[2:]
		}
		if base == 8 && (strings.HasPrefix(text, "0o") || strings.HasPrefix(text, "0O")) {
			text = text[2:]
		}
	}
	if text == "" || !validUnderscores(text) {
		return Nil, errBadNumber
	}
	value := new(big.Int)
	for i := 0; i < len(text); i++ {
		if text[i] == '_' {
			continue
		}
		digit := -1
		switch {
		case text[i] >= '0' && text[i] <= '9':
			digit = int(text[i] - '0')
		case text[i] >= 'a' && text[i] <= 'z':
			digit = int(text[i]-'a') + 10
		case text[i] >= 'A' && text[i] <= 'Z':
			digit = int(text[i]-'A') + 10
		}
		if digit < 0 || digit >= base {
			return Nil, errBadNumber
		}
		value.Mul(value, big.NewInt(int64(base)))
		value.Add(value, big.NewInt(int64(digit)))
	}
	if negative {
		value.Neg(value)
	}
	if value.IsInt64() {
		return IntValue(value.Int64()), nil
	}
	_ = sign
	return BigIntValue(new(big.Int).Set(value)), nil
}

func parseStrictFloat(text string) (float64, error) {
	text = trimNumberSpace(text)
	if text == "" {
		return 0, errBadNumber
	}
	negative := false
	if text[0] == '+' || text[0] == '-' {
		negative = text[0] == '-'
		text = text[1:]
	}
	if text == "" {
		return 0, errBadNumber
	}
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		body := text[2:]
		if body == "" || !validUnderscores(body) {
			return 0, errBadNumber
		}
		value := new(big.Int)
		for i := 0; i < len(body); i++ {
			if body[i] == '_' {
				continue
			}
			b := body[i]
			digit := -1
			if b >= '0' && b <= '9' {
				digit = int(b - '0')
			} else if b >= 'a' && b <= 'f' {
				digit = int(b-'a') + 10
			} else if b >= 'A' && b <= 'F' {
				digit = int(b-'A') + 10
			}
			if digit < 0 {
				return 0, errBadNumber
			}
			value.Mul(value, big.NewInt(16))
			value.Add(value, big.NewInt(int64(digit)))
		}
		floatValue, _ := new(big.Float).SetInt(value).Float64()
		if negative {
			floatValue = -floatValue
		}
		return floatValue, nil
	}
	if !validUnderscores(text) {
		return 0, errBadNumber
	}
	cleaned := strings.ReplaceAll(text, "_", "")
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, errBadNumber
	}
	if negative {
		value = -value
	}
	return value, nil
}

func (s *State) kernelIntegerMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	exception := true
	if !kw.IsNil() {
		if data, ok := hashDataOf(kw); ok {
			for _, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "exception" {
					exception = entry.value.IsTruthy()
				}
			}
		}
	}
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	base := 0
	if len(args) == 2 {
		if args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "base must be an Integer")}
		}
		base = int(args[1].Int())
	}
	switch args[0].Kind() {
	case KindInt, KindBigInt:
		if len(args) == 2 {
			if exception {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "base specified for non string value")}
			}
			return Nil, nil
		}
		return args[0], nil
	case KindFloat:
		if len(args) == 2 {
			if exception {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "base specified for non string value")}
			}
			return Nil, nil
		}
		return IntValue(int64(args[0].Float())), nil
	case KindString:
		if base == -1 {
			base = 0
		}
		if base != 0 && (base < -36 || base > 36 || base == 1 || base == -1) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid radix")}
		}
		value, err := parseStrictInteger(args[0].String(), base)
		if err != nil {
			if exception {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid value for Integer")}
			}
			return Nil, nil
		}
		return value, nil
	default:
		if exception {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Integer")}
		}
		return Nil, nil
	}
}

func (s *State) kernelFloatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	exception := true
	if !kw.IsNil() {
		if data, ok := hashDataOf(kw); ok {
			for _, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "exception" {
					exception = entry.value.IsTruthy()
				}
			}
		}
	}
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	switch args[0].Kind() {
	case KindFloat:
		return args[0], nil
	case KindInt:
		return FloatValue(float64(args[0].Int())), nil
	case KindBigInt:
		value, _ := args[0].BigInt()
		floatValue, _ := new(big.Float).SetInt(value).Float64()
		return FloatValue(floatValue), nil
	case KindString:
		value, err := parseStrictFloat(args[0].String())
		if err != nil {
			if exception {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid value for Float")}
			}
			return Nil, nil
		}
		return FloatValue(value), nil
	default:
		if exception {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Float")}
		}
		return Nil, nil
	}
}

func (s *State) kernelStringMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if self.obj != nil && s.TopSelf.obj != nil && identicalValue(args[0], s.TopSelf) {
		return StringValue("main"), nil
	}
	if args[0].Kind() == KindString {
		return args[0], nil
	}
	return s.call(ctx, args[0], "to_s", nil, Nil, Nil)
}

func (s *State) kernelArrayMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	value := args[0]
	if value.IsNil() {
		return ArrayValue(nil), nil
	}
	if _, ok := arrayDataOf(value); ok {
		return value, nil
	}
	if _, ok := hashDataOf(value); ok {
		return s.call(ctx, value, "to_a", nil, Nil, Nil)
	}
	if _, found := s.classOf(value).lookup("to_a"); found {
		return s.call(ctx, value, "to_a", nil, Nil, Nil)
	}
	return ArrayValue([]Value{value}), nil
}

func (s *State) kernelHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if !kw.IsNil() {
		if len(args) != 0 {
			return Nil, argumentCountError(s, len(args), "0")
		}
		return kw, nil
	}
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	value := args[0]
	if value.IsNil() {
		return HashValue(nil), nil
	}
	if _, ok := hashDataOf(value); ok {
		return value, nil
	}
	if data, ok := arrayDataOf(value); ok {
		if len(data.values) == 0 {
			return HashValue(nil), nil
		}
		for _, pair := range data.values {
			entry, ok := arrayDataOf(pair)
			if !ok || len(entry.values) != 2 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Hash")}
			}
		}
		var entries []hashEntry
		for _, pair := range data.values {
			entry, _ := arrayDataOf(pair)
			entries = append(entries, hashEntry{key: entry.values[0], value: entry.values[1]})
		}
		return HashValue(entries), nil
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Hash")}
}

func (s *State) kernelMethodNameMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(s.frames) == 0 {
		return Nil, nil
	}
	name := s.frames[len(s.frames)-1].method
	if name == "" {
		return Nil, nil
	}
	return SymbolValue(name), nil
}
