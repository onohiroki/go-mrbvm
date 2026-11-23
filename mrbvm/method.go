package mrbvm

import (
	"context"
	"fmt"
	"strings"
)

type methodObjectData struct {
	method    *method
	receiver  Value
	owner     *Class
	name      string
	missing   bool
	superBase *Class
}

func methodObjectOf(value Value) (*methodObjectData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*methodObjectData)
	return data, ok
}

func (s *State) installMethodClasses() {
	methodClass := s.MethodClass
	unboundClass := s.UnboundMethodClass
	noNew := func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NoMethodErrorClass, "undefined method 'new' for Class")}
	}
	methodClass.classMethods["new"] = &method{owner: methodClass, name: "new", native: noNew}
	unboundClass.classMethods["new"] = &method{owner: unboundClass, name: "new", native: noNew}

	methodClass.define("call", s.methodCallMethod)
	methodClass.define("[]", s.methodCallMethod)
	methodClass.define("receiver", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := methodObjectOf(self); ok {
			return data.receiver, nil
		}
		return Nil, nil
	})
	for _, class := range []*Class{methodClass, unboundClass} {
		class.define("name", s.methodNameMethod)
		class.define("owner", s.methodOwnerMethod)
		class.define("arity", s.methodArityMethod)
		class.define("parameters", s.methodParametersMethod)
		class.define("source_location", s.methodSourceLocationMethod)
		class.define("super_method", s.methodSuperMethod)
		class.define("unbind", s.methodUnbindMethod)
		class.define("bind", s.methodBindMethod)
		class.define("bind_call", s.methodBindCallMethod)
		class.define("to_s", s.methodInspectMethod)
		class.define("inspect", s.methodInspectMethod)
		class.define("==", s.methodEqualMethod)
		class.define("eql?", s.methodEqualMethod)
		class.define("clone", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return self, nil
		})
		class.define("dup", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return self, nil
		})
		class.define("initialize_copy", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return self, nil
		})
	}
	methodClass.define("to_proc", s.methodToProcMethod)
	methodClass.define("<<", s.methodComposeMethod(false))
	methodClass.define(">>", s.methodComposeMethod(true))

	s.ObjectClass.define("method", s.kernelMethodMethod)
	s.ObjectClass.define("singleton_method", s.kernelSingletonMethodMethod)
	s.ObjectClass.define("public_method", s.kernelPublicMethodMethod)
	s.ClassClass.define("instance_method", s.classInstanceMethod(false))
	s.ClassClass.define("public_instance_method", s.classInstanceMethod(true))
}

func (s *State) newMethodObject(data methodObjectData, unbound bool) Value {
	class := s.MethodClass
	if unbound {
		class = s.UnboundMethodClass
		data.receiver = Nil
	}
	return ObjectValue(&Object{class: class, data: &data})
}

func lookupInstanceMethod(class *Class, name string) (*method, bool) {
	if class.singletonOf != nil {
		if m, found := class.singletonOf.classMethods[name]; found && m.owner == class {
			return m, true
		}
	}
	return class.lookup(name)
}

func validMethodNameArg(s *State, arg Value) (string, error) {
	if arg.Kind() != KindSymbol && arg.Kind() != KindString {
		return "", &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "method name must be a Symbol or String")}
	}
	return arg.String(), nil
}

func (s *State) respondToMissingTrue(ctx context.Context, receiver Value, name string, includePrivate bool) (bool, error) {
	hook, found := s.classOf(receiver).lookup("respond_to_missing?")
	if !found || hook.notImplemented {
		return false, nil
	}
	result, err := s.invoke(ctx, hook, receiver, []Value{SymbolValue(name), BoolValue(includePrivate)}, Nil, Nil)
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}

func (s *State) kernelMethodMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name, err := validMethodNameArg(s, args[0])
	if err != nil {
		return Nil, err
	}
	if classRecv, isClass := s.asClass(self); isClass {
		if m, found := classRecv.lookupClassMethod(name); found {
			defining := classRecv
			for current := classRecv.super; current != nil; current = current.super {
				if candidate, ok := current.classMethods[name]; ok && candidate == m {
					defining = current
				}
			}
			owner := s.ensureSingletonOfClass(defining)
			return s.newMethodObject(methodObjectData{method: m, receiver: self, owner: owner, name: name}, false), nil
		}
	}
	class := s.classOf(self)
	if m, found := lookupInstanceMethod(class, name); found {
		owner := m.owner
		if owner == nil {
			owner = class
		}
		if class.singletonOf != nil && owner == class.singletonOf {
			owner = class
		}
		if self.obj != nil && class == self.obj.singleton {
			owner = class
		}
		return s.newMethodObject(methodObjectData{method: m, receiver: self, owner: owner, name: name}, false), nil
	}
	if ok, err := s.respondToMissingTrue(ctx, self, name, false); err != nil {
		return Nil, err
	} else if ok {
		return s.newMethodObject(methodObjectData{receiver: self, owner: class, name: name, missing: true}, false), nil
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
}

func (s *State) kernelPublicMethodMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	value, err := s.kernelMethodMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	data, _ := methodObjectOf(value)
	if data != nil && data.method != nil {
		owner := data.method.owner
		if owner == nil {
			owner = s.classOf(self)
		}
		if owner.privateMethods[data.name] || owner.protectedMethods[data.name] {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+data.name+"'")}
		}
	}
	return value, nil
}

func (s *State) kernelSingletonMethodMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name, err := validMethodNameArg(s, args[0])
	if err != nil {
		return Nil, err
	}
	if class, ok := s.asClass(self); ok {
		if m, found := class.classMethods[name]; found {
			return s.newMethodObject(methodObjectData{method: m, receiver: self, owner: class, name: name}, false), nil
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
	}
	if self.obj != nil && self.obj.singleton != nil {
		if m, found := self.obj.singleton.methods[name]; found {
			return s.newMethodObject(methodObjectData{method: m, receiver: self, owner: self.obj.singleton, name: name}, false), nil
		}
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
}

func (s *State) classInstanceMethod(publicOnly bool) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok || len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		name, err := validMethodNameArg(s, args[0])
		if err != nil {
			return Nil, err
		}
		lookupClass := class
		if class == s.KernelClass {
			lookupClass = s.ObjectClass
		}
		m, found := lookupInstanceMethod(lookupClass, name)
		if !found {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
		}
		owner := m.owner
		if owner == nil {
			owner = class
		}
		if class == s.KernelClass {
			owner = s.KernelClass
		}
		superBase := (*Class)(nil)
		if !class.isModule {
			superBase = class
		}
		if class.singletonOf != nil && owner == class.singletonOf {
			owner = class
		}
		if publicOnly && (owner.privateMethods[name] || owner.protectedMethods[name]) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
		}
		return s.newMethodObject(methodObjectData{method: m, owner: owner, name: name, superBase: superBase}, true), nil
	}
}

func (s *State) methodNameMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if data, ok := methodObjectOf(self); ok {
		return SymbolValue(data.name), nil
	}
	return Nil, nil
}

func (s *State) methodOwnerMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if data, ok := methodObjectOf(self); ok {
		if data.owner == nil {
			return Nil, nil
		}
		return s.classValue(data.owner), nil
	}
	return Nil, nil
}

func methodArityValue(data *methodObjectData) int64 {
	if data == nil || data.missing || data.method == nil {
		return -1
	}
	if data.method.irep != nil {
		return procArity(&blockData{irep: data.method.irep, strict: true})
	}
	nativeName := data.name
	if data.method != nil && data.method.name != "" {
		nativeName = data.method.name
	}
	switch nativeName {
	case "upcase", "downcase", "object_id", "size", "inspect", "to_s":
		return 0
	case "+", "-", "*", "/":
		return 1
	case "push", "first", "each", "map", "select":
		return -1
	case "__send__":
		return -2
	}
	return -1
}

func (s *State) methodArityMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, _ := methodObjectOf(self)
	return IntValue(methodArityValue(data)), nil
}

func nativeMethodParameters(name string) []procParameter {
	switch name {
	case "push":
		return []procParameter{{kind: "rest", name: ""}}
	case "first":
		return []procParameter{{kind: "opt", name: ""}}
	case "+", "-", "*", "/":
		return []procParameter{{kind: "req", name: ""}}
	}
	return nil
}

func (s *State) methodParametersMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, _ := methodObjectOf(self)
	if data == nil || data.method == nil {
		return ArrayValue([]Value{ArrayValue([]Value{SymbolValue("rest")})}), nil
	}
	var parameters []procParameter
	if data.method.irep != nil {
		parameters = procParameters(&blockData{irep: data.method.irep, strict: true})
	} else {
		parameters = nativeMethodParameters(data.method.name)
	}
	values := make([]Value, len(parameters))
	for i, parameter := range parameters {
		values[i] = ArrayValue([]Value{SymbolValue(parameter.kind), SymbolValue(parameter.name)})
	}
	return ArrayValue(values), nil
}

func (s *State) methodSourceLocationMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, _ := methodObjectOf(self)
	if data == nil || data.method == nil || data.method.irep == nil || data.method.irep.Filename == "" || len(data.method.irep.Lines) == 0 {
		return Nil, nil
	}
	return ArrayValue([]Value{StringValue(data.method.irep.Filename), IntValue(int64(data.method.irep.Lines[0].Line))}), nil
}

func (s *State) methodSuperMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok || data.method == nil || data.owner == nil {
		return Nil, nil
	}
	name := data.name
	owner := data.owner
	searchClass := owner
	if owner.isModule && !data.receiver.IsNil() {
		searchClass = s.classOf(data.receiver)
	}
	ancestors := classAncestors(searchClass)
	start := len(ancestors)
	for i, ancestor := range ancestors {
		if ancestor == owner {
			start = i + 1
			break
		}
	}
	for i := 0; i < len(ancestors); i++ {
		if i < start && ancestors[i] != owner {
			continue
		}
		if m, found := ancestors[i].methods[name]; found {
			if m == data.method {
				continue
			}
			return s.newMethodObject(methodObjectData{method: m, receiver: data.receiver, owner: ancestors[i], name: name}, data.receiver.IsNil()), nil
		}
	}
	return Nil, nil
}

func (s *State) methodUnbindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a method")}
	}
	if data.receiver.IsNil() {
		return self, nil
	}
	copyData := *data
	copyData.receiver = Nil
	return s.newMethodObject(copyData, true), nil
}

func methodCompatible(s *State, owner *Class, receiver Value) bool {
	if owner == nil {
		return true
	}
	if owner.isModule && owner.singletonOf == nil {
		return true
	}
	if owner.singletonOf != nil {
		attached := s.classValue(owner.singletonOf)
		return identicalValue(attached, receiver)
	}
	return s.isA(receiver, owner)
}

func (s *State) methodBindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a method")}
	}
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if !methodCompatible(s, data.owner, args[0]) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "bind argument must be an instance of "+data.owner.name)}
	}
	copyData := *data
	copyData.receiver = args[0]
	return s.newMethodObject(copyData, false), nil
}

func (s *State) methodBindCallMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, argumentCountError(s, len(args), "1+")
	}
	data, ok := methodObjectOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a method")}
	}
	if !methodCompatible(s, data.owner, args[0]) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "bind argument must be an instance of "+data.owner.name)}
	}
	return s.callMethodObject(ctx, data, args[0], args[1:], kw, block)
}

func (s *State) callMethodObject(ctx context.Context, data *methodObjectData, receiver Value, args []Value, kw Value, block Value) (Value, error) {
	if data.missing || data.method == nil {
		return s.invokeMethodMissing(ctx, receiver, data.name, args, kw, block)
	}
	if data.superBase != nil {
		s.pendingSuperBase = data.superBase
	}
	result, err := s.invoke(ctx, data.method, receiver, args, kw, block)
	s.pendingSuperBase = nil
	return result, err
}

func (s *State) methodCallMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a method")}
	}
	return s.callMethodObject(ctx, data, data.receiver, args, kw, block)
}

func (s *State) methodToProcMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a method")}
	}
	value := nativeBlockValue(s.ProcClass, func(ctx context.Context, args []Value) (Value, error) {
		return s.callMethodObject(ctx, data, data.receiver, args, Nil, Nil)
	})
	if native, ok := value.obj.data.(*nativeBlockData); ok {
		native.fnFull = func(ctx context.Context, args []Value, kw Value, block Value) (Value, error) {
			receiver := data.receiver
			return s.callMethodObject(ctx, data, receiver, args, kw, block)
		}
		native.strict = true
		native.arity = int(methodArityValue(data))
		native.hasArity = true
	}
	return value, nil
}

func (s *State) methodComposeMethod(reverse bool) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		other := args[0]
		value := nativeBlockValue(s.ProcClass, func(ctx context.Context, callArgs []Value) (Value, error) {
			first, second := other, self
			if reverse {
				first, second = self, other
			}
			result, err := s.call(ctx, first, "call", callArgs, Nil, Nil)
			if err != nil {
				return Nil, err
			}
			return s.call(ctx, second, "call", []Value{result}, Nil, Nil)
		})
		if native, ok := value.obj.data.(*nativeBlockData); ok {
			native.strict = true
			native.arity = -1
			native.hasArity = true
		}
		return value, nil
	}
}

func (s *State) methodInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := methodObjectOf(self)
	if !ok {
		return StringValue("#<Method>"), nil
	}
	prefix := "Method"
	if self.Kind() == KindObject && self.obj != nil {
		if _, ok := self.obj.data.(*methodObjectData); ok && self.obj.class == s.UnboundMethodClass {
			prefix = "UnboundMethod"
		}
	}
	owner := ""
	if data.owner != nil {
		if data.owner.singletonOf != nil {
			owner = s.classValue(data.owner.singletonOf).Inspect()
		} else if data.owner.name != "" && !strings.HasPrefix(data.owner.name, "#<") {
			owner = data.owner.name
		} else {
			owner = classDisplayName(data.owner)
		}
	} else if data.receiver.obj != nil {
		owner = data.receiver.Inspect()
	}
	return StringValue(fmt.Sprintf("#<%s: %s#%s>", prefix, owner, data.name)), nil
}

func (s *State) methodEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	left, leftOK := methodObjectOf(self)
	right, rightOK := methodObjectOf(args[0])
	if !leftOK || !rightOK {
		return False, nil
	}
	if left.owner != right.owner {
		return False, nil
	}
	if left.method != right.method {
		aliasPair := (left.name == "==" && right.name == "eql?") || (left.name == "eql?" && right.name == "==")
		if !aliasPair {
			return False, nil
		}
	}
	if !left.receiver.IsNil() || !right.receiver.IsNil() {
		return BoolValue(identicalValue(left.receiver, right.receiver)), nil
	}
	return True, nil
}

// methodFromBody builds a method that runs a captured method body under a new
// owner. It is used by define_method with a Method or UnboundMethod argument.
func (s *State) methodFromBody(body Value, target *Class, name string) (*method, error) {
	data, ok := methodObjectOf(body)
	if !ok || data.method == nil {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type (expected Method or UnboundMethod)")}
	}
	owner := data.owner
	if owner == nil {
		owner = target
	}
	compatible := owner == target || (owner.isModule && owner.singletonOf == nil)
	if !compatible && owner != target {
		compatible = s.classIsSubclass(target, owner)
	}
	if data.receiver.obj != nil && !data.receiver.IsNil() {
		receiverClass := s.classOf(data.receiver)
		compatible = target == receiverClass || s.classIsSubclass(target, receiverClass)
		if data.owner != nil && data.owner.singletonOf != nil {
			compatible = target == data.owner
		}
	}
	if !compatible {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "bind argument must be an instance of "+owner.name)}
	}
	copied := *data.method
	copied.owner = target
	copied.name = name
	if copied.originalName == "" {
		copied.originalName = data.method.name
	}
	return &copied, nil
}

func (s *State) methodNameMatchesPrefix(name string) bool {
	return !strings.HasPrefix(name, "__")
}
