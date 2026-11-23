package mrbvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type evalScope struct {
	irep     *Irep
	initial  map[string]Value
	captured map[string]Value
	applied  bool
}

type bindingData struct {
	env         *environment
	self        Value
	targetClass *Class
	cref        *Class
	owner       *Class
	method      string
	file        string
	line        int
}

func (s *State) installEvalLibrary() {
	s.ObjectClass.define("eval", s.kernelEvalMethod)
	s.KernelClass.classMethods["eval"] = &method{owner: s.KernelClass, name: "eval", native: s.kernelEvalMethod}
	s.ObjectClass.define("load", s.kernelLoadMethod)
	s.KernelClass.classMethods["load"] = &method{owner: s.KernelClass, name: "load", native: s.kernelLoadMethod}
	s.ObjectClass.define("require", s.kernelRequireMethod)
	s.KernelClass.classMethods["require"] = &method{owner: s.KernelClass, name: "require", native: s.kernelRequireMethod}
	s.ObjectClass.define("require_relative", s.kernelRequireRelativeMethod)
	s.ObjectClass.define("binding", s.kernelBindingMethod)
	s.KernelClass.classMethods["binding"] = &method{owner: s.KernelClass, name: "binding", native: s.kernelBindingMethod}

	s.ObjectClass.define("instance_eval", s.instanceEvalMethod)
	s.BasicObjectClass.define("instance_eval", s.instanceEvalMethod)
	s.ClassClass.define("class_eval", s.classEvalStringMethod)
	s.ClassClass.define("module_eval", s.classEvalStringMethod)
	s.ClassClass.define("class_exec", s.classEvalStringMethod)
	s.ModuleClass.define("class_eval", s.classEvalStringMethod)
	s.ModuleClass.define("module_eval", s.classEvalStringMethod)
	s.ModuleClass.define("class_exec", s.classEvalStringMethod)

	class := s.BindingClass
	class.classMethods["new"] = &method{owner: class, name: "new", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NoMethodErrorClass, "undefined method 'new' for Class")}
	}}
	class.define("eval", s.bindingEvalMethod)
	class.define("local_variables", s.bindingLocalVariablesMethod)
	class.define("local_variable_get", s.bindingLocalVariableGetMethod)
	class.define("local_variable_set", s.bindingLocalVariableSetMethod)
	class.define("local_variable_defined?", s.bindingLocalVariableDefinedMethod)
	class.define("receiver", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := bindingOf(self); ok {
			return data.self, nil
		}
		return Nil, nil
	})
	class.define("source_location", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := bindingOf(self); ok && data.file != "" {
			return ArrayValue([]Value{StringValue(data.file), IntValue(int64(data.line))}), nil
		}
		return Nil, nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(fmt.Sprintf("#<Binding:0x%x>", objectIdentityID(self))), nil
	})
}

type evalContext struct {
	self        Value
	targetClass *Class
	cref        *Class
	owner       *Class
	env         *environment
	method      string
}

func (s *State) currentEvalContext() evalContext {
	context := evalContext{self: s.TopSelf, targetClass: s.ObjectClass, cref: s.ObjectClass, owner: s.ObjectClass}
	if len(s.frames) == 0 {
		return context
	}
	frame := &s.frames[len(s.frames)-1]
	context.env = frame.env
	context.method = frame.method
	context.cref = frame.cref
	context.owner = frame.owner
	context.targetClass = frame.targetClass
	context.self = frame.self
	if context.cref == nil {
		context.cref = s.ObjectClass
	}
	if context.targetClass == nil {
		context.targetClass = s.ObjectClass
	}
	if context.owner == nil {
		context.owner = context.targetClass
	}
	return context
}

func (s *State) evalString(ctx context.Context, source, filename string, line int, scope evalContext) (Value, error) {
	if s.compiler == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NotImplementedErrorClass, "no compiler is configured for eval")}
	}
	if filename == "" {
		filename = "(eval)"
	}
	if line < 1 {
		line = 1
	}
	names, values := evalVisibleLocals(scope.env)
	// Prism decides what a bare name means while it parses, and it has no
	// API here for seeding its local table. A self-assignment prefix makes
	// every caller local known without changing the value that is injected
	// into the frame before it runs.
	prefix := ""
	for _, name := range names {
		if validLocalName(name) {
			prefix += name + " = " + name + "; "
		}
	}
	lineOffset := line - 1
	if prefix != "" {
		source = prefix + "\n" + source
		lineOffset = line - 2
	}
	irep, err := s.compiler(ctx, []byte(source), CompileOptions{
		FilePath:   filename,
		LocalNames: names,
		LineOffset: lineOffset,
	})
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.SyntaxErrorClass, err.Error())}
	}
	eval := &evalScope{irep: irep, initial: values}
	s.evalScopes = append(s.evalScopes, eval)
	result, runErr := s.executeFrame(ctx, irep, scope.self, scope.targetClass, nil, Nil, Nil, scope.env, scope.method, scope.owner, scope.cref, false)
	s.evalScopes = s.evalScopes[:len(s.evalScopes)-1]
	if eval.captured != nil {
		writeEvalLocals(scope.env, eval.captured)
	}
	if runErr != nil {
		return Nil, runErr
	}
	return result, nil
}

func evalVisibleLocals(env *environment) ([]string, map[string]Value) {
	values := make(map[string]Value)
	seen := make(map[string]bool)
	var names []string
	for current := env; current != nil; current = current.parent {
		for slot, name := range current.names {
			if name == "" || seen[name] || strings.HasPrefix(name, "__gomrbc_") {
				continue
			}
			seen[name] = true
			names = append(names, name)
			if slot < len(current.regs) {
				values[name] = current.regs[slot]
			} else {
				values[name] = Nil
			}
		}
	}
	return names, values
}

func findEvalLocal(env *environment, name string) (*environment, int, bool) {
	for current := env; current != nil; current = current.parent {
		for slot, localName := range current.names {
			if localName == name {
				return current, slot, true
			}
		}
	}
	return nil, 0, false
}

func writeEvalLocals(env *environment, captured map[string]Value) {
	for name, value := range captured {
		if name == "" || strings.HasPrefix(name, "__gomrbc_") {
			continue
		}
		if owner, slot, ok := findEvalLocal(env, name); ok {
			if slot < len(owner.regs) {
				owner.regs[slot] = value
			}
			continue
		}
		if env == nil {
			continue
		}
		slot := len(env.names)
		env.names = append(env.names, name)
		if slot < len(env.regs) {
			env.regs[slot] = value
		} else {
			env.regs = append(env.regs, value)
		}
	}
}

func (s *State) kernelEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 4 {
		return Nil, argumentCountError(s, len(args), "1..4")
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	filename := "(eval)"
	line := 1
	scope := s.currentEvalContext()
	if len(args) > 1 && !args[1].IsNil() {
		binding, ok := bindingOf(args[1])
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type "+s.classOf(args[1]).name+" (expected Binding)")}
		}
		scope = evalContext{
			self:        binding.self,
			targetClass: binding.targetClass,
			cref:        binding.cref,
			owner:       binding.owner,
			env:         binding.env,
			method:      binding.method,
		}
	}
	if len(args) > 2 && !args[2].IsNil() {
		if args[2].Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
		}
		filename = args[2].String()
	}
	if len(args) > 3 && !args[3].IsNil() {
		if args[3].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		line = int(args[3].Int())
	}
	return s.evalString(ctx, args[0].String(), filename, line, scope)
}

func singletonEvalClass(s *State, receiver Value) *Class {
	if class, ok := s.asClass(receiver); ok {
		return s.ensureSingletonOfClass(class)
	}
	if receiver.obj == nil {
		return s.classOf(receiver)
	}
	if receiver.obj.singleton == nil {
		original := s.classOf(receiver)
		receiver.obj.singleton = newClass("#<Class:"+original.name+">", original)
		receiver.obj.singleton.singletonOwner = receiver
		receiver.obj.singleton.isSingleton = true
		s.ensureClassObject(receiver.obj.singleton)
	}
	return receiver.obj.singleton
}

func (s *State) instanceEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if !block.IsNil() {
		if len(args) > 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
		}
		return s.callBlockWithSelf(ctx, block, self, args, Nil)
	}
	if len(args) < 1 || len(args) > 3 {
		return Nil, argumentCountError(s, len(args), "1..3")
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	filename := "(eval)"
	line := 1
	if len(args) > 1 && !args[1].IsNil() {
		if args[1].Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
		}
		filename = args[1].String()
	}
	if len(args) > 2 && !args[2].IsNil() {
		if args[2].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		line = int(args[2].Int())
	}
	target := singletonEvalClass(s, self)
	scope := evalContext{self: self, targetClass: target, cref: target, owner: target, env: s.currentEvalContext().env, method: s.currentEvalContext().method}
	return s.evalString(ctx, args[0].String(), filename, line, scope)
}

func (s *State) classEvalStringMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a class/module")}
	}
	if !block.IsNil() {
		saved := class.defaultVisibility
		savedModuleFunction := class.moduleFunction
		class.defaultVisibility = ""
		class.moduleFunction = false
		defer func() {
			class.defaultVisibility = saved
			class.moduleFunction = savedModuleFunction
		}()
		return s.callBlockWithSelf(ctx, block, self, args, Nil)
	}
	if len(args) < 1 || len(args) > 3 {
		return Nil, argumentCountError(s, len(args), "1..3")
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	filename := "(eval)"
	line := 1
	if len(args) > 1 && !args[1].IsNil() {
		if args[1].Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
		}
		filename = args[1].String()
	}
	if len(args) > 2 && !args[2].IsNil() {
		if args[2].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		line = int(args[2].Int())
	}
	saved := class.defaultVisibility
	savedModuleFunction := class.moduleFunction
	class.defaultVisibility = ""
	class.moduleFunction = false
	defer func() {
		class.defaultVisibility = saved
		class.moduleFunction = savedModuleFunction
	}()
	scope := evalContext{self: self, targetClass: class, cref: class, owner: class, env: s.currentEvalContext().env, method: s.currentEvalContext().method}
	return s.evalString(ctx, args[0].String(), filename, line, scope)
}

func (s *State) kernelLoadMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 || args[0].Kind() != KindString {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	path := args[0].String()
	wrapped := len(args) > 1 && args[1].IsTruthy()
	return s.loadFile(ctx, path, wrapped, false)
}

func (s *State) loadFile(ctx context.Context, path string, wrapped, required bool) (Value, error) {
	if s.compiler == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NotImplementedErrorClass, "no compiler is configured for load")}
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.LoadErrorClass, "cannot load such file -- "+path)}
	}
	scope := evalContext{self: s.TopSelf, targetClass: s.ObjectClass, cref: s.ObjectClass, owner: s.ObjectClass, method: "<top (required)>"}
	if wrapped {
		module := newClass("#<Module>", nil)
		module.isModule = true
		s.ensureClassObject(module)
		scope = evalContext{self: s.classValue(module), targetClass: module, cref: module, owner: module, method: "<top (required)>"}
	}
	_, err = s.evalString(ctx, string(source), path, 1, scope)
	if err != nil {
		return Nil, err
	}
	if required {
		s.requiredFiles[path] = true
	}
	return True, nil
}

func (s *State) resolveRequirePath(ctx context.Context, path string, relative bool) (string, bool) {
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		candidates = append(candidates, path+".rb")
	}
	if relative {
		base := ""
		if len(s.frames) > 0 {
			base = filepath.Dir(s.frames[len(s.frames)-1].file)
		}
		for i, candidate := range candidates {
			candidates[i] = filepath.Join(base, candidate)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			absolute, err := filepath.Abs(candidate)
			if err == nil {
				return absolute, true
			}
			return candidate, true
		}
	}
	return "", false
}

func (s *State) kernelRequireMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.requirePath(ctx, args, false)
}

func (s *State) kernelRequireRelativeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.requirePath(ctx, args, true)
}

func (s *State) requirePath(ctx context.Context, args []Value, relative bool) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindString {
		return Nil, argumentCountError(s, len(args), "1")
	}
	path, found := s.resolveRequirePath(ctx, args[0].String(), relative)
	if !found {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.LoadErrorClass, "cannot load such file -- "+args[0].String())}
	}
	if s.requiredFiles[path] {
		return False, nil
	}
	s.requiredFiles[path] = true
	if _, err := s.loadFile(ctx, path, false, false); err != nil {
		delete(s.requiredFiles, path)
		return Nil, err
	}
	return True, nil
}

func bindingOf(value Value) (*bindingData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*bindingData)
	return data, ok
}

func (s *State) kernelBindingMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	scope := s.currentEvalContext()
	file := ""
	line := 0
	if len(s.frames) > 0 {
		file = s.frames[len(s.frames)-1].file
		line = s.frames[len(s.frames)-1].line
	}
	data := &bindingData{
		env:         scope.env,
		self:        scope.self,
		targetClass: scope.targetClass,
		cref:        scope.cref,
		owner:       scope.owner,
		method:      scope.method,
		file:        file,
		line:        line,
	}
	return ObjectValue(&Object{class: s.BindingClass, data: data}), nil
}

func (s *State) bindingEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a binding")}
	}
	if len(args) < 1 || len(args) > 3 || args[0].Kind() != KindString {
		return Nil, argumentCountError(s, len(args), "1..3")
	}
	filename := "(eval)"
	line := 1
	if len(args) > 1 && !args[1].IsNil() {
		if args[1].Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
		}
		filename = args[1].String()
	}
	if len(args) > 2 && !args[2].IsNil() {
		if args[2].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		line = int(args[2].Int())
	}
	scope := evalContext{self: data.self, targetClass: data.targetClass, cref: data.cref, owner: data.owner, env: data.env, method: data.method}
	return s.evalString(ctx, args[0].String(), filename, line, scope)
}

func bindingVariableName(s *State, value Value) (string, error) {
	if value.Kind() != KindSymbol && value.Kind() != KindString {
		return "", &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "local variable name must be a Symbol or String")}
	}
	name := value.String()
	if name == "" || !validLocalName(name) {
		return "", &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid local variable name")}
	}
	return name, nil
}

func validLocalName(name string) bool {
	for i, char := range name {
		if char == '_' || (char >= 'a' && char <= 'z') || (i > 0 && ((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9'))) {
			continue
		}
		return false
	}
	return name != ""
}

func (s *State) bindingLocalVariablesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	var values []Value
	seen := make(map[string]bool)
	for env := data.env; env != nil; env = env.parent {
		for _, name := range env.names {
			if name == "" || seen[name] || strings.HasPrefix(name, "__gomrbc_") {
				continue
			}
			seen[name] = true
			values = append(values, SymbolValue(name))
		}
	}
	return ArrayValue(values), nil
}

func (s *State) bindingLocalVariableGetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok || len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name, err := bindingVariableName(s, args[0])
	if err != nil {
		return Nil, err
	}
	env, slot, found := findEvalLocal(data.env, name)
	if !found {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "local variable "+name+" not defined")}
	}
	if slot >= len(env.regs) {
		return Nil, nil
	}
	return env.regs[slot], nil
}

func (s *State) bindingLocalVariableSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok || len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "2")
	}
	name, err := bindingVariableName(s, args[0])
	if err != nil {
		return Nil, err
	}
	writeEvalLocals(data.env, map[string]Value{name: args[1]})
	return args[1], nil
}

func (s *State) bindingLocalVariableDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok || len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	name, err := bindingVariableName(s, args[0])
	if err != nil {
		return Nil, err
	}
	_, _, found := findEvalLocal(data.env, name)
	return BoolValue(found), nil
}
