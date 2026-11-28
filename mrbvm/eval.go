package mrbvm

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/onohiroki/go-mrbvm/sandbox"
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
	visibility  string
	args        []Value
	kw          Value
	block       Value
	inMethod    bool
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

	s.topLevelEnv = &environment{}
	s.ObjectClass.constants["TOPLEVEL_BINDING"] = ObjectValue(&Object{class: s.BindingClass, data: &bindingData{
		env:         s.topLevelEnv,
		self:        s.TopSelf,
		targetClass: s.ObjectClass,
		cref:        s.ObjectClass,
		owner:       s.ObjectClass,
		method:      "<main>",
	}})
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
	class.define("dup", s.bindingDupMethod)
	class.define("clone", s.bindingDupMethod)
	class.define("initialize_copy", s.bindingInitializeCopyMethod)
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
		return StringValue(fmt.Sprintf("#<Binding:0x%x>", s.objectID(self))), nil
	})
}

type evalContext struct {
	self        Value
	targetClass *Class
	cref        *Class
	owner       *Class
	env         *environment
	method      string
	visibility  string
	isolate     bool
	args        []Value
	kw          Value
	block       Value
	inMethod    bool
	// recordFrames keeps the eval frames of a binding so writes to its locals
	// reach procs the string made.
	recordFrames bool
}

// evalForwardFor answers what a forwarding super or a yield in evaluated
// source borrows from the frame its env belongs to: the method's arguments,
// keywords, and block.
func (s *State) evalForwardFor(env *environment) ([]Value, Value, Value, bool) {
	methodEnv := nearestMethodEnv(env)
	if methodEnv == nil {
		if env != nil {
			return nil, Nil, env.block, false
		}
		return nil, Nil, Nil, false
	}
	for i := len(s.frames) - 1; i >= 0; i-- {
		frame := &s.frames[i]
		if frame.env == methodEnv {
			return frame.args, frame.kw, methodEnv.block, true
		}
	}
	return nil, Nil, methodEnv.block, true
}

func (s *State) currentEvalContext() evalContext {
	context := evalContext{self: s.TopSelf, targetClass: s.ObjectClass, cref: s.ObjectClass, owner: s.ObjectClass, env: s.topLevelEnv, block: Nil}
	if len(s.frames) == 0 {
		return context
	}
	frame := &s.frames[len(s.frames)-1]
	context.env = frame.env
	context.method = frame.method
	context.args, context.kw, context.block, context.inMethod = s.evalForwardFor(frame.env)
	if frame.method != "" && frame.owner != nil {
		if m, ok := frame.owner.lookup(frame.method); ok && m.hasBodyVisibility {
			context.visibility = m.bodyVisibility
			if context.visibility == "" {
				context.visibility = "public"
			}
		}
	}
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
	if len(filename) > 0xfffe {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "filename too long")}
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
	targetVisibility := ""
	if scope.targetClass != nil {
		targetVisibility = scope.targetClass.defaultVisibility
	}
	if scope.visibility != "" || (scope.self.obj != nil && s.TopSelf.obj != nil && scope.self.obj == s.TopSelf.obj && scope.targetClass == s.ObjectClass) {
		if scope.visibility != "" {
			targetVisibility = scope.visibility
		} else {
			targetVisibility = "private"
		}
	}
	if scope.targetClass != nil {
		savedVisibility := scope.targetClass.defaultVisibility
		scope.targetClass.defaultVisibility = targetVisibility
		if scope.isolate {
			defer func(saved string) { scope.targetClass.defaultVisibility = saved }(savedVisibility)
		}
	}
	compile := func(text string) (*Irep, error) {
		return s.compiler(ctx, []byte(text), CompileOptions{
			FilePath:     filename,
			LocalNames:   names,
			LineOffset:   lineOffset,
			Eval:         true,
			EvalInMethod: scope.inMethod,
		})
	}
	irep, err := compile(source)
	if err != nil && strings.Contains(err.Error(), "reserved for numbered parameters") {
		rewritten := source
		for i := 1; i <= 9; i++ {
			rewritten = strings.ReplaceAll(rewritten, "_"+fmt.Sprint(i), "self._"+fmt.Sprint(i))
		}
		irep, err = compile(rewritten)
	}
	if err != nil {
		message := err.Error()
		if strings.Contains(message, "PostExecutionNode") {
			line := 0
			for index, text := range strings.Split(source, "\n") {
				if strings.HasPrefix(strings.TrimSpace(text), "END") {
					line = index + 1
					break
				}
			}
			if line == 0 {
				line = 1
			}
			message = fmt.Sprintf("file %s line %d: generator error, END not supported", filename, line)
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.SyntaxErrorClass, message)}
	}
	eval := &evalScope{irep: irep, initial: values}
	s.evalScopes = append(s.evalScopes, eval)
	block := scope.block
	if block.obj == nil {
		block = Nil
	}
	forward := &evalForward{args: append([]Value(nil), scope.args...), kw: scope.kw, block: block}
	s.pendingEvalForward = forward
	result, runErr := s.executeFrame(ctx, irep, scope.self, scope.targetClass, nil, Nil, block, scope.env, scope.method, scope.owner, scope.cref, false)
	s.pendingEvalForward = nil
	if scope.recordFrames && scope.env != nil && forward.frameEnv != nil {
		forward.frameEnv.pinned = true
		if !containsEnvironment(scope.env.evalFrames, forward.frameEnv) {
			scope.env.evalFrames = append(scope.env.evalFrames, forward.frameEnv)
		}
	}
	if !scope.isolate && scope.targetClass != nil && scope.targetClass.defaultVisibility != targetVisibility {
		scope.targetClass.visibilityEscaped = true
	}
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
		for name, value := range current.extras {
			if name == "" || seen[name] || strings.HasPrefix(name, "__gomrbc_") {
				continue
			}
			seen[name] = true
			names = append(names, name)
			values[name] = value
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
		if _, ok := current.extras[name]; ok {
			return current, -1, true
		}
	}
	return nil, 0, false
}

func containsEnvironment(envs []*environment, want *environment) bool {
	for _, env := range envs {
		if env == want {
			return true
		}
	}
	return false
}

// syncEvalFrames writes a value a later eval produced into the frames earlier
// evals made from the same scope, so a proc they captured reads the new value.
func syncEvalFrames(env *environment, name string, value Value) {
	if env == nil || name == "" || strings.HasPrefix(name, "__gomrbc_") {
		return
	}
	for current := env; current != nil; current = current.parent {
		for _, frameEnv := range current.evalFrames {
			for slot, localName := range frameEnv.names {
				if localName == name && slot < len(frameEnv.regs) {
					frameEnv.regs[slot] = value
				}
			}
			if frameEnv.extras != nil {
				if _, ok := frameEnv.extras[name]; ok {
					frameEnv.extras[name] = value
				}
			}
		}
	}
}

func writeEvalLocals(env *environment, captured map[string]Value) {
	for name, value := range captured {
		if name == "" || strings.HasPrefix(name, "__gomrbc_") {
			continue
		}
		if owner, slot, ok := findEvalLocal(env, name); ok {
			if slot < 0 {
				owner.extras[name] = value
			} else if slot < len(owner.regs) {
				owner.regs[slot] = value
			}
			syncEvalFrames(env, name, value)
			continue
		}
		if env == nil {
			continue
		}
		if env.extras == nil {
			env.extras = make(map[string]Value)
		}
		env.extras[name] = value
		syncEvalFrames(env, name, value)
	}
}

func (s *State) kernelEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.requireCapability(sandbox.Eval); err != nil {
		return Nil, err
	}
	if len(args) < 1 || len(args) > 4 {
		return Nil, argumentCountError(s, len(args), "1..4")
	}
	if args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(args[0]).name+" into String")}
	}
	filename := "(eval)"
	line := 1
	scope := s.currentEvalContext()
	hasBinding := false
	if len(args) > 1 && !args[1].IsNil() {
		binding, ok := bindingOf(args[1])
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type "+s.classOf(args[1]).name+" (expected Binding)")}
		}
		hasBinding = true
		scope = evalContext{
			self:         binding.self,
			targetClass:  binding.targetClass,
			cref:         binding.cref,
			owner:        binding.owner,
			env:          binding.env,
			method:       binding.method,
			visibility:   binding.visibility,
			args:         binding.args,
			kw:           binding.kw,
			block:        binding.block,
			inMethod:     binding.inMethod,
			recordFrames: true,
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
	if !hasBinding {
		scope.isolate = true
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
	if err := s.requireCapability(sandbox.Eval); err != nil {
		return Nil, err
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
	if err := s.requireCapability(sandbox.Eval); err != nil {
		return Nil, err
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
	if err := s.requireCapability(sandbox.Load); err != nil {
		return Nil, err
	}
	path := args[0].String()
	wrapped := len(args) > 1 && args[1].IsTruthy()
	return s.loadFile(ctx, path, wrapped, false)
}

func (s *State) loadFile(ctx context.Context, path string, wrapped, required bool) (Value, error) {
	data, err := s.sourceReadFile(path)
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

	if s.isBytecodeSource(path, data) {
		irep, err := DecodeBytecode(data)
		if err != nil {
			return Nil, err
		}
		_, err = s.executeFrame(ctx, irep, scope.self, scope.targetClass, nil, Nil, Nil, nil, "<top (required)>", scope.owner, scope.cref, false)
		if err != nil {
			return Nil, err
		}
	} else {
		if s.compiler == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NotImplementedErrorClass, "no compiler is configured for load")}
		}
		if _, err := s.evalString(ctx, string(data), path, 1, scope); err != nil {
			return Nil, err
		}
	}
	if required {
		s.requiredFiles[path] = true
	}
	return True, nil
}

// isBytecodeSource reports whether data should be decoded as RITE bytecode
// rather than compiled as Ruby source.
func (s *State) isBytecodeSource(path string, data []byte) bool {
	if s.sourceExt(path) == ".mrb" {
		return true
	}
	return bytes.HasPrefix(data, []byte("RITE"))
}

func (s *State) resolveRequirePath(ctx context.Context, path string, relative bool) (string, bool) {
	for _, candidate := range s.requireCandidates(path, relative) {
		if info, err := s.sourceStat(candidate); err == nil && !info.IsDir() {
			return s.canonicalSourcePath(candidate), true
		}
	}
	return "", false
}

// requireCandidates returns the search order for a require name: the name
// itself and .rb/.mrb variants, for each of the load-path roots. Relative
// requires only search the directory of the requiring file.
func (s *State) requireCandidates(name string, relative bool) []string {
	names := []string{name}
	if s.sourceExt(name) == "" {
		names = append(names, name+".rb", name+".mrb")
	}

	var roots []string
	if relative {
		base := ""
		if len(s.frames) > 0 {
			base = s.sourceDir(s.frames[len(s.frames)-1].file)
		}
		roots = []string{base}
	} else {
		roots = append([]string{""}, s.loadPathRoots()...)
	}

	seen := make(map[string]bool, len(roots)*len(names))
	candidates := make([]string, 0, len(roots)*len(names))
	for _, root := range roots {
		for _, candidate := range names {
			joined := s.sourceJoin(root, candidate)
			if seen[joined] {
				continue
			}
			seen[joined] = true
			candidates = append(candidates, joined)
		}
	}
	return candidates
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
	if err := s.requireCapability(sandbox.Require); err != nil {
		return Nil, err
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
	for env := scope.env; env != nil; env = env.parent {
		env.pinned = true
	}
	visibility := scope.visibility
	if visibility == "" && scope.targetClass != nil {
		visibility = scope.targetClass.defaultVisibility
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
		visibility:  visibility,
		args:        scope.args,
		kw:          scope.kw,
		block:       scope.block,
		inMethod:    scope.inMethod,
	}
	return ObjectValue(&Object{class: s.BindingClass, data: data}), nil
}

func copyEnvironmentChain(env *environment) *environment {
	if env == nil {
		return nil
	}
	copyEnv := &environment{
		parent:      copyEnvironmentChain(env.parent),
		regs:        append([]Value(nil), env.regs...),
		block:       env.block,
		methodFrame: env.methodFrame,
		pinned:      true,
		names:       append([]string(nil), env.names...),
	}
	return copyEnv
}

func (s *State) bindingDupMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a binding")}
	}
	copyData := *data
	copyData.env = copyEnvironmentChain(data.env)
	return ObjectValue(&Object{class: s.classOf(self), data: &copyData}), nil
}

func (s *State) bindingInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	source, ok := bindingOf(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize_copy requires a Binding")}
	}
	copyData := *source
	copyData.env = copyEnvironmentChain(source.env)
	if self.obj != nil {
		self.obj.data = &copyData
	}
	return self, nil
}

func (s *State) bindingEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := bindingOf(self)
	if !ok {
		if self.obj == nil || !s.classIsSubclass(s.classOf(self), s.BindingClass) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a binding")}
		}
		data = &bindingData{env: s.topLevelEnv, self: s.TopSelf, targetClass: s.ObjectClass, cref: s.ObjectClass, owner: s.ObjectClass}
	}
	if len(args) < 1 || len(args) > 3 || args[0].Kind() != KindString {
		return Nil, argumentCountError(s, len(args), "1..3")
	}
	if err := s.requireCapability(sandbox.Eval); err != nil {
		return Nil, err
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
	scope := evalContext{self: data.self, targetClass: data.targetClass, cref: data.cref, owner: data.owner, env: data.env, method: data.method, visibility: data.visibility, args: data.args, kw: data.kw, block: data.block, inMethod: data.inMethod, recordFrames: true}
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
		for name := range env.extras {
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
	if slot < 0 {
		return env.extras[name], nil
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
