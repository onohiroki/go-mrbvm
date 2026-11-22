package mrbvm

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/onohiroki/go-mrbvm/sandbox"
)

// Options controls VM construction.
type Options struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// LoadMrblib loads a precompiled subset of mruby's Ruby-level core
	// library (10error, compar, enum, array, hash, string, range) at VM
	// creation.
	LoadMrblib bool

	// Policy is the capability allow-list exposed to embeddings.
	Policy *sandbox.Policy

	// Execution limits. Zero means unlimited, except MaxDepth, where zero
	// selects the default depth.
	MaxInstructions uint64
	MaxDepth        int
	MaxTime         time.Duration
	MaxOutputBytes  int
	MaxArraySize    int
	MaxHashSize     int
	MaxStringSize   int
}

// State is one mruby VM instance. A State is owned by a single goroutine.
type State struct {
	BasicObjectClass *Class
	ObjectClass      *Class
	ModuleClass      *Class
	ClassClass       *Class
	NumericClass     *Class
	NilClass         *Class
	TrueClass        *Class
	FalseClass       *Class
	IntegerClass     *Class
	FloatClass       *Class
	StringClass      *Class
	SymbolClass      *Class
	ArrayClass       *Class
	HashClass        *Class
	RangeClass       *Class
	ProcClass        *Class
	EnumerableClass  *Class
	EnumeratorClass  *Class
	ComparableClass  *Class
	GCClass          *Class
	TimeClass        *Class
	JSONClass        *Class
	IOClass          *Class
	FileClass        *Class
	RegexpClass      *Class
	MatchDataClass   *Class
	FiberClass       *Class
	RootFiber        Value

	ExceptionClass              *Class
	StandardErrorClass          *Class
	RuntimeErrorClass           *Class
	ArgumentErrorClass          *Class
	TypeErrorClass              *Class
	NameErrorClass              *Class
	NoMethodErrorClass          *Class
	ZeroDivisionErrorClass      *Class
	LocalJumpErrorClass         *Class
	NoMatchingPatternErrorClass *Class
	IndexErrorClass             *Class
	StopIterationClass          *Class
	KeyErrorClass               *Class
	NotImplementedErrorClass    *Class
	RangeErrorClass             *Class
	FloatDomainErrorClass       *Class
	FrozenErrorClass            *Class
	SecurityErrorClass          *Class
	IOErrorClass                *Class
	FiberErrorClass             *Class
	RegexpErrorClass            *Class
	ScriptErrorClass            *Class
	SyntaxErrorClass            *Class
	SystemStackErrorClass       *Class

	TopSelf Value

	globals          map[string]Value
	constants        map[string]Value
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
	currentException Value
	initErr          error

	arrayIndexBuiltin *method
	arraySetBuiltin   *method

	frames            []callFrame
	callDepth         int
	programs          map[*Irep]*irepProgram
	stringPlusBuiltin *method
	objectEqBuiltin   *method
	regFree           [][]Value
	argFree           [][]Value
	envFree           []*environment
	gcEnabled         bool

	policy             *sandbox.Policy
	maxInstructions    uint64
	maxDepth           int
	maxTime            time.Duration
	startTime          time.Time
	instructionCount   uint64
	limitsEnabled      bool
	privateMethodCount int
	protectedCount     int
	maxArraySize       int
	maxHashSize        int
	maxStringSize      int
	output             *limitWriter
}

// New creates a VM state with the minimal core needed to execute VM-0
// bytecode.
func New(opts Options) *State {
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	s := &State{
		globals:   make(map[string]Value),
		constants: make(map[string]Value),
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,

		policy:          opts.Policy,
		maxInstructions: opts.MaxInstructions,
		maxDepth:        opts.MaxDepth,
		maxTime:         opts.MaxTime,
		maxArraySize:    opts.MaxArraySize,
		maxHashSize:     opts.MaxHashSize,
		maxStringSize:   opts.MaxStringSize,
		startTime:       time.Now(),
		gcEnabled:       true,
	}
	if s.maxDepth == 0 {
		s.maxDepth = 200
	}
	s.programs = make(map[*Irep]*irepProgram)
	if opts.MaxOutputBytes > 0 {
		s.output = &limitWriter{writer: stdout, remaining: opts.MaxOutputBytes}
		s.stdout = s.output
	}
	s.limitsEnabled = s.maxInstructions > 0 || s.maxTime > 0 || s.output != nil
	s.BasicObjectClass = newClass("BasicObject", nil)
	s.ObjectClass = newClass("Object", s.BasicObjectClass)
	s.ModuleClass = newClass("Module", s.ObjectClass)
	s.ClassClass = newClass("Class", s.ModuleClass)
	s.NumericClass = newClass("Numeric", s.ObjectClass)
	s.NilClass = newClass("NilClass", s.ObjectClass)
	s.TrueClass = newClass("TrueClass", s.ObjectClass)
	s.FalseClass = newClass("FalseClass", s.ObjectClass)
	s.IntegerClass = newClass("Integer", s.NumericClass)
	s.FloatClass = newClass("Float", s.NumericClass)
	s.StringClass = newClass("String", s.ObjectClass)
	s.SymbolClass = newClass("Symbol", s.ObjectClass)
	s.ArrayClass = newClass("Array", s.ObjectClass)
	s.HashClass = newClass("Hash", s.ObjectClass)
	s.RangeClass = newClass("Range", s.ObjectClass)
	s.ProcClass = newClass("Proc", s.ObjectClass)
	s.EnumerableClass = newClass("Enumerable", nil)
	s.EnumerableClass.isModule = true
	s.EnumeratorClass = newClass("Enumerator", s.ObjectClass)
	s.ComparableClass = newClass("Comparable", nil)
	s.ComparableClass.isModule = true
	s.GCClass = newClass("GC", nil)
	s.GCClass.isModule = true
	s.TimeClass = newClass("Time", s.ObjectClass)
	s.JSONClass = newClass("JSON", nil)
	s.JSONClass.isModule = true
	s.IOClass = newClass("IO", s.ObjectClass)
	s.FileClass = newClass("File", s.IOClass)
	s.RegexpClass = newClass("Regexp", s.ObjectClass)
	s.MatchDataClass = newClass("MatchData", s.ObjectClass)
	s.FiberClass = newClass("Fiber", s.ObjectClass)
	s.ExceptionClass = newClass("Exception", s.ObjectClass)
	s.StandardErrorClass = newClass("StandardError", s.ExceptionClass)
	s.RuntimeErrorClass = newClass("RuntimeError", s.StandardErrorClass)
	s.ArgumentErrorClass = newClass("ArgumentError", s.StandardErrorClass)
	s.TypeErrorClass = newClass("TypeError", s.StandardErrorClass)
	s.NameErrorClass = newClass("NameError", s.StandardErrorClass)
	s.NoMethodErrorClass = newClass("NoMethodError", s.NameErrorClass)
	s.ZeroDivisionErrorClass = newClass("ZeroDivisionError", s.StandardErrorClass)
	s.LocalJumpErrorClass = newClass("LocalJumpError", s.StandardErrorClass)
	s.NoMatchingPatternErrorClass = newClass("NoMatchingPatternError", s.StandardErrorClass)
	s.IndexErrorClass = newClass("IndexError", s.StandardErrorClass)
	s.StopIterationClass = newClass("StopIteration", s.IndexErrorClass)
	s.KeyErrorClass = newClass("KeyError", s.IndexErrorClass)
	s.NotImplementedErrorClass = newClass("NotImplementedError", s.StandardErrorClass)
	s.RangeErrorClass = newClass("RangeError", s.StandardErrorClass)
	s.FloatDomainErrorClass = newClass("FloatDomainError", s.RangeErrorClass)
	s.FrozenErrorClass = newClass("FrozenError", s.RuntimeErrorClass)
	s.RegexpErrorClass = newClass("RegexpError", s.StandardErrorClass)
	s.SecurityErrorClass = newClass("SecurityError", s.StandardErrorClass)
	s.IOErrorClass = newClass("IOError", s.StandardErrorClass)
	s.FiberErrorClass = newClass("FiberError", s.StandardErrorClass)
	s.ScriptErrorClass = newClass("ScriptError", s.ExceptionClass)
	s.SyntaxErrorClass = newClass("SyntaxError", s.ScriptErrorClass)
	s.SystemStackErrorClass = newClass("SystemStackError", s.ExceptionClass)

	for _, class := range []*Class{
		s.BasicObjectClass, s.ObjectClass, s.ModuleClass, s.ClassClass,
		s.NumericClass, s.NilClass,
		s.TrueClass, s.FalseClass, s.IntegerClass, s.FloatClass,
		s.StringClass, s.SymbolClass, s.ArrayClass, s.HashClass,
		s.RangeClass, s.ProcClass, s.EnumerableClass, s.EnumeratorClass,
		s.ComparableClass, s.GCClass, s.TimeClass, s.JSONClass,
		s.IOClass, s.FileClass, s.RegexpClass, s.MatchDataClass, s.FiberClass,
		s.ExceptionClass, s.StandardErrorClass, s.RuntimeErrorClass,
		s.ArgumentErrorClass, s.TypeErrorClass, s.NameErrorClass,
		s.NoMethodErrorClass, s.ZeroDivisionErrorClass, s.LocalJumpErrorClass,
		s.NoMatchingPatternErrorClass,
		s.IndexErrorClass, s.StopIterationClass, s.KeyErrorClass,
		s.NotImplementedErrorClass, s.RangeErrorClass, s.FloatDomainErrorClass,
		s.FrozenErrorClass,
		s.SecurityErrorClass, s.IOErrorClass, s.FiberErrorClass,
		s.RegexpErrorClass, s.ScriptErrorClass, s.SyntaxErrorClass,
		s.SystemStackErrorClass,
	} {
		s.ensureClassObject(class)
	}

	s.ClassClass.define("new", s.classNewMethod)
	s.ClassClass.define("superclass", s.classSuperclassMethod)
	s.ClassClass.define("name", s.classNameMethod)
	s.ClassClass.define("include", s.classIncludeMethod)
	s.ClassClass.define("include?", s.classIncludePredicateMethod)
	s.ClassClass.define("class_eval", s.classEvalMethod)
	s.ClassClass.define("module_eval", s.classEvalMethod)
	s.ClassClass.define("class_exec", s.classEvalMethod)
	s.ClassClass.define("alias_method", s.classAliasMethod)
	s.ClassClass.define("remove_method", s.classRemoveMethod)
	s.ClassClass.define("private", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if class, ok := s.asClass(self); ok {
			if len(args) == 0 {
				class.defaultVisibility = "private"
			}
			for _, arg := range args {
				name := arg.String()
				if !class.privateMethods[name] {
					class.privateMethods[name] = true
					s.privateMethodCount++
				}
				if class.protectedMethods[name] {
					delete(class.protectedMethods, name)
					if s.protectedCount > 0 {
						s.protectedCount--
					}
				}
			}
		}
		return namesResult(args), nil
	})
	s.ClassClass.define("protected", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if class, ok := s.asClass(self); ok {
			if len(args) == 0 {
				class.defaultVisibility = "protected"
			}
			for _, arg := range args {
				name := arg.String()
				if !class.protectedMethods[name] {
					class.protectedMethods[name] = true
					s.protectedCount++
				}
				if class.privateMethods[name] {
					delete(class.privateMethods, name)
					if s.privateMethodCount > 0 {
						s.privateMethodCount--
					}
				}
			}
		}
		return namesResult(args), nil
	})
	s.ClassClass.define("public", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if class, ok := s.asClass(self); ok {
			if len(args) == 0 {
				class.defaultVisibility = "public"
			}
			for _, arg := range args {
				name := arg.String()
				if class.privateMethods[name] {
					delete(class.privateMethods, name)
					if s.privateMethodCount > 0 {
						s.privateMethodCount--
					}
				}
				if class.protectedMethods[name] {
					delete(class.protectedMethods, name)
					if s.protectedCount > 0 {
						s.protectedCount--
					}
				}
			}
		}
		return namesResult(args), nil
	})
	s.ClassClass.define("module_function", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			return Nil, nil
		}
		if len(args) == 0 {
			class.moduleFunction = true
			return self, nil
		}
		for _, arg := range args {
			name := arg.String()
			m, ok := class.lookup(name)
			if !ok {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+name+"'")}
			}
			class.classMethods[name] = m
			if !class.privateMethods[name] {
				class.privateMethods[name] = true
				s.privateMethodCount++
			}
		}
		return namesResult(args), nil
	})
	s.ClassClass.define("prepend", s.classPrependMethod)
	s.ClassClass.define("remove_const", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok || len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "remove_const expects one name")}
		}
		name := args[0].String()
		value, found := class.constants[name]
		if !found {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "constant "+name+" not defined")}
		}
		delete(class.constants, name)
		delete(s.constants, name)
		return value, nil
	})
	s.ClassClass.define("undef_method", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			return self, nil
		}
		for _, arg := range args {
			delete(class.methods, arg.String())
		}
		return self, nil
	})
	s.ClassClass.define("private_class_method", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		return Nil, nil
	})
	s.ClassClass.define("define_method", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok || len(args) < 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "define_method expects a name")}
		}
		name := args[0].String()
		var body Value = block
		if len(args) > 1 {
			body = args[1]
		}
		if body.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
		}
		class.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return s.callBlockWithSelf(ctx, body, self, args, kw)
		})
		return SymbolValue(name), nil
	})
	s.ClassClass.define("private_constant", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.ensureMutable(self); err != nil {
			return Nil, err
		}
		return namesResult(args), nil
	})
	s.ClassClass.define("public_constant", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return namesResult(args), nil
	})
	s.ClassClass.define("deprecate_constant", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return namesResult(args), nil
	})
	s.ClassClass.define("append_features", s.classAppendFeaturesMethod)
	s.ClassClass.define("prepend_features", s.classPrependFeaturesMethod)
	s.ClassClass.define("inherited", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.ClassClass.define("included", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.ClassClass.define("prepended", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.ClassClass.define("===", s.classCaseEqMethod)
	s.ClassClass.define("attr_reader", s.classAttrReaderMethod)
	s.ClassClass.define("attr", s.classAttrReaderMethod)
	s.ClassClass.define("attr_writer", s.classAttrWriterMethod)
	s.ClassClass.define("attr_accessor", s.classAttrAccessorMethod)
	s.ClassClass.define("const_defined?", s.classConstDefinedMethod)
	s.ClassClass.define("method_defined?", s.classMethodDefinedMethod)
	s.ClassClass.define("const_get", s.classConstGetMethod)
	s.ClassClass.define("const_set", s.classConstSetMethod)
	s.ClassClass.define("ancestors", s.classAncestorsMethod)
	s.ObjectClass.define("class", s.objectClassMethod)
	s.ObjectClass.define("singleton_class", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if class, ok := s.asClass(self); ok {
			if class.singleton == nil {
				class.singleton = newClass("#<Class:"+class.name+">", nil)
				class.singleton.singletonOf = class
				s.ensureClassObject(class.singleton)
			}
			return s.classValue(class.singleton), nil
		}
		if self.obj == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't define singleton")}
		}
		if self.obj.singleton == nil {
			original := s.classOf(self)
			self.obj.singleton = newClass("#<Class:"+original.name+">", original)
			s.ensureClassObject(self.obj.singleton)
		}
		return s.classValue(self.obj.singleton), nil
	})
	s.ObjectClass.define("extend", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj == nil {
			return self, nil
		}
		if self.obj.singleton == nil {
			original := s.classOf(self)
			self.obj.singleton = newClass("#<Class:"+original.name+">", original)
			s.ensureClassObject(self.obj.singleton)
		}
		for _, arg := range args {
			module, ok := s.asClass(arg)
			if !ok {
				continue
			}
			for name, m := range module.methods {
				if _, exists := self.obj.singleton.methods[name]; !exists {
					self.obj.singleton.methods[name] = m
				}
			}
		}
		return self, nil
	})
	s.ObjectClass.define("freeze", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			self.obj.frozen = true
		}
		return self, nil
	})
	s.ObjectClass.define("frozen?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.obj != nil && self.obj.frozen), nil
	})
	s.ObjectClass.define("initialize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, nil
	})

	for name, class := range map[string]*Class{
		"BasicObject": s.BasicObjectClass, "Object": s.ObjectClass,
		"Module": s.ModuleClass, "Class": s.ClassClass,
		"Numeric":  s.NumericClass,
		"NilClass": s.NilClass, "TrueClass": s.TrueClass, "FalseClass": s.FalseClass,
		"Integer": s.IntegerClass,
		"Float":   s.FloatClass, "String": s.StringClass, "Symbol": s.SymbolClass,
		"Array": s.ArrayClass, "Hash": s.HashClass, "Range": s.RangeClass,
		"Proc": s.ProcClass, "Enumerable": s.EnumerableClass,
		"Enumerator":    s.EnumeratorClass,
		"Comparable":    s.ComparableClass,
		"GC":            s.GCClass,
		"Time":          s.TimeClass,
		"JSON":          s.JSONClass,
		"IO":            s.IOClass,
		"File":          s.FileClass,
		"Regexp":        s.RegexpClass,
		"MatchData":     s.MatchDataClass,
		"Fiber":         s.FiberClass,
		"Exception":     s.ExceptionClass,
		"StandardError": s.StandardErrorClass, "RuntimeError": s.RuntimeErrorClass,
		"ArgumentError": s.ArgumentErrorClass, "TypeError": s.TypeErrorClass,
		"NameError": s.NameErrorClass, "NoMethodError": s.NoMethodErrorClass,
		"ZeroDivisionError":      s.ZeroDivisionErrorClass,
		"LocalJumpError":         s.LocalJumpErrorClass,
		"NoMatchingPatternError": s.NoMatchingPatternErrorClass,
		"IndexError":             s.IndexErrorClass,
		"StopIteration":          s.StopIterationClass,
		"KeyError":               s.KeyErrorClass,
		"NotImplementedError":    s.NotImplementedErrorClass,
		"RangeError":             s.RangeErrorClass,
		"FrozenError":            s.FrozenErrorClass,
		"SecurityError":          s.SecurityErrorClass,
		"IOError":                s.IOErrorClass,
		"FiberError":             s.FiberErrorClass,
		"RegexpError":            s.RegexpErrorClass,
		"ScriptError":            s.ScriptErrorClass,
		"SyntaxError":            s.SyntaxErrorClass,
		"SystemStackError":       s.SystemStackErrorClass,
	} {
		value := s.classValue(class)
		s.constants[name] = value
		s.ObjectClass.constants[name] = value
	}

	s.FloatClass.constants["INFINITY"] = FloatValue(math.Inf(1))
	s.FloatClass.constants["NAN"] = FloatValue(math.NaN())
	s.constants["MRUBY_VERSION"] = frozenString("4.0.0")
	s.constants["MRUBY_REVISION"] = frozenString("HEAD")
	s.constants["MRUBY_PLATFORM"] = frozenString(runtime.GOARCH + "-" + runtime.GOOS)
	s.TopSelf = ObjectValue(&Object{class: s.ObjectClass})
	s.ObjectClass.define("puts", s.kernelPuts)
	s.ObjectClass.define("print", s.kernelPrint)
	s.ObjectClass.define("p", s.kernelP)
	s.ObjectClass.define("raise", s.kernelRaise)
	s.ObjectClass.define("fail", s.kernelRaise)
	s.ExceptionClass.classMethods["exception"] = &method{owner: s.ExceptionClass, name: "exception", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		class, ok := s.asClass(self)
		if !ok {
			class = s.ExceptionClass
		}
		message := ""
		if len(args) > 0 {
			message = args[0].ToS()
		}
		return newException(class, message), nil
	}}
	s.ExceptionClass.define("exception", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) == 0 || (len(args) == 1 && args[0].obj == self.obj) {
			return self, nil
		}
		message := exceptionMessage(self)
		if len(args) > 0 && !args[0].IsNil() {
			message = args[0].ToS()
		}
		return newException(s.classOf(self), message), nil
	})
	s.ExceptionClass.define("message", s.exceptionMessageMethod)
	s.ExceptionClass.define("to_s", s.exceptionMessageMethod)
	s.ExceptionClass.define("inspect", s.exceptionInspectMethod)
	s.ExceptionClass.define("initialize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				if len(args) > 0 {
					data.message = args[0].ToS()
				}
				return self, nil
			}
		}
		return self, nil
	})
	s.ExceptionClass.define("set_backtrace", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil && len(args) > 0 {
			if data, ok := self.obj.data.(*exceptionData); ok {
				data.backtrace = nil
				if array, ok := args[0].obj.data.(*arrayData); ok {
					for _, item := range array.values {
						data.backtrace = append(data.backtrace, item.ToS())
					}
				}
			}
		}
		return args[0], nil
	})
	s.ExceptionClass.define("full_message", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := self.Inspect()
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok && len(data.backtrace) > 0 {
				for _, line := range data.backtrace {
					text += "\n\tfrom " + line
				}
			}
		}
		return StringValue(text), nil
	})
	s.ExceptionClass.define("backtrace", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		var values []Value
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				for _, line := range data.backtrace {
					values = append(values, StringValue(line))
				}
			}
		}
		return ArrayValue(values), nil
	})
	s.NameErrorClass.define("name", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				return data.name, nil
			}
		}
		return Nil, nil
	})
	s.NameErrorClass.define("receiver", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				if data.receiverSet {
					return data.receiver, nil
				}
			}
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no receiver is available")}
	})
	s.NoMethodErrorClass.define("args", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				return data.args, nil
			}
		}
		return Nil, nil
	})
	s.ExceptionClass.define("cause", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if self.obj != nil {
			if data, ok := self.obj.data.(*exceptionData); ok {
				return data.cause, nil
			}
		}
		return Nil, nil
	})

	s.ArrayClass.define("[]", s.arrayIndexMethod)
	s.arrayIndexBuiltin = s.ArrayClass.methods["[]"]
	s.ArrayClass.define("+", s.arrayPlusMethod)
	s.ArrayClass.define("*", s.arrayRepeatMethod)
	s.ArrayClass.classMethods["[]"] = &method{owner: s.ArrayClass, name: "[]", native: s.arrayClassBracketMethod}
	s.ArrayClass.define("length", s.arrayLengthMethod)
	s.ArrayClass.define("size", s.arrayLengthMethod)
	s.ArrayClass.define("to_a", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.HashClass.define("[]", s.hashIndexMethod)
	s.HashClass.define("length", s.hashLengthMethod)
	s.HashClass.define("size", s.hashLengthMethod)
	s.HashClass.define("deconstruct_keys", s.hashDeconstructKeysMethod)
	s.HashClass.define("__pat_values", s.hashPatValuesMethod)
	s.HashClass.define("__except", s.hashExceptMethod)
	s.RangeClass.define("to_a", s.rangeToAMethod)
	s.RangeClass.define("===", s.rangeCaseEqMethod)
	s.RangeClass.define("include?", s.rangeCaseEqMethod)
	s.RangeClass.define("member?", s.rangeCaseEqMethod)
	s.RangeClass.define("each", s.rangeEachMethod)
	s.StringClass.define("length", s.stringLengthMethod)
	s.StringClass.define("size", s.stringLengthMethod)
	s.StringClass.define("upcase", s.stringUpcaseMethod)
	s.StringClass.define("downcase", s.stringDowncaseMethod)
	s.ArrayClass.define("each", s.arrayEachMethod)
	s.ArrayClass.define("deconstruct", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	s.ArrayClass.define("map", s.arrayMapMethod)
	s.ArrayClass.define("sort", s.arraySortMethod)
	s.ArrayClass.define("sort!", s.arraySortBangMethod)
	s.ArrayClass.define("__case_eqq", s.arrayCaseEqMethod)
	s.ArrayClass.define("__svalue", s.arraySValueMethod)
	s.ArrayClass.define("__svalue_eq", s.arraySValueEqMethod)
	s.ArrayClass.define("<<", s.arrayPushMethod)
	s.ArrayClass.define("push", s.arrayPushMethod)
	s.ArrayClass.define("[]=", s.arraySetMethod)
	s.arraySetBuiltin = s.ArrayClass.methods["[]="]
	s.HashClass.define("[]=", s.hashSetMethod)
	s.ProcClass.define("call", s.procCallMethod)
	s.ProcClass.classMethods["new"] = &method{owner: s.ProcClass, name: "new", native: s.kernelProcMethod}
	s.ObjectClass.define("respond_to?", s.objectRespondToMethod)
	s.ObjectClass.define("dup", s.objectDupMethod)
	s.ObjectClass.define("instance_variables", s.objectInstanceVariablesMethod)
	s.ObjectClass.define("instance_variable_get", s.objectInstanceVariableGetMethod)
	s.ObjectClass.define("instance_variable_set", s.objectInstanceVariableSetMethod)
	s.ObjectClass.define("instance_variable_defined?", s.objectInstanceVariableDefinedMethod)
	s.ObjectClass.define("methods", s.objectMethodsMethod)
	s.ObjectClass.define("public_methods", s.objectMethodsMethod)
	s.ObjectClass.define("private_methods", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return ArrayValue(nil), nil
	})
	s.ObjectClass.define("clone", s.objectDupMethod)
	s.ObjectClass.define("__ENCODING__", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue("ASCII-8BIT"), nil
	})
	s.ObjectClass.define("proc", s.kernelProcMethod)
	s.ObjectClass.define("lambda", s.kernelProcMethod)

	s.installEnumerableLibrary()
	s.installEnumeratorLibrary()
	s.installComparableLibrary()
	includeModule(s.NumericClass, s.ComparableClass)
	includeModule(s.StringClass, s.ComparableClass)
	includeModule(s.EnumeratorClass, s.EnumerableClass)
	for _, class := range []*Class{s.ArrayClass, s.HashClass, s.RangeClass} {
		includeModule(class, s.EnumerableClass)
	}
	s.installImmediateLibrary()
	s.installCoreLibrary()
	s.installTimeClass()
	s.installJSONModule()
	s.installGCLibrary()
	s.installIOLibrary()
	s.installFileLibrary()
	s.installRegexpClass()
	s.installMatchDataClass()
	s.installFiberClass()
	s.stringPlusBuiltin = s.StringClass.methods["+"]
	s.objectEqBuiltin = s.ObjectClass.methods["=="]
	for name, m := range s.ClassClass.methods {
		if name == "new" {
			continue
		}
		if _, ok := s.ModuleClass.methods[name]; !ok {
			s.ModuleClass.methods[name] = m
		}
	}
	rootData := &fiberData{resumeCh: make(chan fiberMessage), outCh: make(chan fiberMessage)}
	s.RootFiber = ObjectValue(&Object{class: s.FiberClass, data: rootData})
	rootData.self = s.RootFiber
	if opts.LoadMrblib {
		irep, err := s.LoadBytecode(mrblibBytecode)
		if err == nil {
			_, err = s.Execute(context.Background(), irep)
		}
		s.initErr = err
	}
	s.instructionCount = 0
	s.startTime = time.Now()
	return s
}

func (s *State) classConstDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	name := args[0].String()
	for current := class; current != nil; current = current.super {
		if _, ok := current.constants[name]; ok {
			return True, nil
		}
	}
	return False, nil
}

func validAttrName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (s *State) classAttrReaderMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: attr_reader receiver is not a class")
	}
	names := make([]Value, 0, len(args))
	for _, arg := range args {
		name := arg.String()
		if !validAttrName(name) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid attribute name '"+name+"'")}
		}
		ivar := "@" + strings.TrimPrefix(name, "@")
		class.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if self.obj == nil {
				return Nil, nil
			}
			return self.obj.ivar(ivar), nil
		})
		names = append(names, SymbolValue(name))
	}
	return ArrayValue(names), nil
}

func (s *State) classAttrWriterMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: attr_writer receiver is not a class")
	}
	names := make([]Value, 0, len(args))
	for _, arg := range args {
		name := arg.String()
		if !validAttrName(name) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "invalid attribute name '"+name+"'")}
		}
		ivar := "@" + strings.TrimPrefix(name, "@")
		setter := name + "="
		class.define(setter, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if len(args) != 1 {
				return Nil, fmt.Errorf("mrbvm: %s expects one argument", setter)
			}
			if self.obj != nil {
				self.obj.setIvar(ivar, args[0])
			}
			return args[0], nil
		})
		names = append(names, SymbolValue(setter))
	}
	return ArrayValue(names), nil
}

func (s *State) classAttrAccessorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	readers, err := s.classAttrReaderMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	writers, err := s.classAttrWriterMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	result := make([]Value, 0, len(args)*2)
	for i := range args {
		result = append(result, readers.obj.data.(*arrayData).values[i])
		result = append(result, writers.obj.data.(*arrayData).values[i])
	}
	return ArrayValue(result), nil
}

func includeModule(class, module *Class) {
	for name, method := range module.methods {
		if _, exists := class.methods[name]; !exists {
			class.methods[name] = method
		}
	}
	for name, value := range module.constants {
		if _, exists := class.constants[name]; !exists {
			class.constants[name] = value
		}
	}
	found := false
	for _, included := range class.included {
		if included == module {
			found = true
			break
		}
	}
	if !found {
		class.included = append(class.included, module)
	}
}

func (s *State) classMethodDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	_, found := class.lookup(args[0].String())
	return BoolValue(found), nil
}

func (s *State) classIncludePredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	module, ok := s.asClass(args[0])
	if !ok {
		return False, nil
	}
	for current := class; current != nil; current = current.super {
		for _, included := range current.included {
			if included == module {
				return True, nil
			}
		}
		for _, prepended := range current.prepended {
			if prepended == module {
				return True, nil
			}
		}
	}
	return False, nil
}

func (s *State) arrayClassBracketMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	values := append([]Value(nil), args...)
	class, ok := s.asClass(self)
	if ok && class != s.ArrayClass {
		return ObjectValue(&Object{class: class, data: &arrayData{values: values}}), nil
	}
	return ArrayValue(values), nil
}

func (s *State) arrayPlusMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Array#+ expects one argument")}
	}
	values := append([]Value(nil), arrayValues(self)...)
	values = append(values, arrayValues(args[0])...)
	return ArrayValue(values), nil
}

func (s *State) arrayRepeatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Array#* expects one argument")}
	}
	values := arrayValues(self)
	if args[0].Kind() == KindInt {
		count := args[0].Int()
		if count < 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative argument")}
		}
		result := make([]Value, 0, len(values)*int(count))
		for i := int64(0); i < count; i++ {
			result = append(result, values...)
		}
		return ArrayValue(result), nil
	}
	joined, err := s.joinArrayValues(ctx, arrayDataOfValue(self), args[0].ToS(), map[*arrayData]bool{})
	if err != nil {
		return Nil, err
	}
	return StringValue(joined), nil
}

func (s *State) joinArrayValues(ctx context.Context, data *arrayData, separator string, seen map[*arrayData]bool) (string, error) {
	var parts []string
	for i := 0; i < len(data.values); i++ {
		value := data.values[i]
		if value.obj != nil {
			if nested, ok := value.obj.data.(*arrayData); ok {
				if seen[nested] {
					return "", &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "recursive array join")}
				}
				seen[nested] = true
				joined, err := s.joinArrayValues(ctx, nested, separator, seen)
				delete(seen, nested)
				if err != nil {
					return "", err
				}
				parts = append(parts, joined)
				continue
			}
		}
		if value.Kind() == KindObject {
			text, err := s.call(ctx, value, "to_s", nil, Nil, Nil)
			if err != nil {
				return "", err
			}
			parts = append(parts, text.ToS())
			continue
		}
		parts = append(parts, value.ToS())
	}
	return strings.Join(parts, separator), nil
}

func arrayDataOfValue(value Value) *arrayData {
	if value.obj == nil {
		return &arrayData{}
	}
	if data, ok := value.obj.data.(*arrayData); ok {
		return data
	}
	return &arrayData{}
}

func normalizeIvarName(name string) string {
	if strings.HasPrefix(name, "@") {
		return name
	}
	return "@" + name
}

func (s *State) objectInstanceVariablesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return ArrayValue(nil), nil
	}
	names := make([]Value, 0, len(self.obj.ivars))
	for name := range self.obj.ivars {
		names = append(names, SymbolValue(name))
	}
	return ArrayValue(names), nil
}

func (s *State) objectInstanceVariableGetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil || len(args) != 1 {
		return Nil, nil
	}
	return self.obj.ivar(normalizeIvarName(args[0].String())), nil
}

func (s *State) objectInstanceVariableSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil || len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "instance_variable_set expects two arguments")}
	}
	name := normalizeIvarName(args[0].String())
	self.obj.setIvar(name, args[1])
	return args[1], nil
}

func (s *State) objectInstanceVariableDefinedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil || len(args) != 1 {
		return False, nil
	}
	name := normalizeIvarName(args[0].String())
	_, ok := self.obj.ivars[name]
	return BoolValue(ok), nil
}

func (s *State) objectMethodsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	names := make(map[string]bool)
	if class, ok := s.asClass(self); ok {
		for current := class; current != nil; current = current.super {
			for name := range current.classMethods {
				names[name] = true
			}
		}
		for name := range s.ClassClass.methods {
			names[name] = true
		}
	} else {
		for current := s.classOf(self); current != nil; current = current.super {
			for name := range current.methods {
				names[name] = true
			}
		}
	}
	values := make([]Value, 0, len(names))
	for name := range names {
		values = append(values, SymbolValue(name))
	}
	return ArrayValue(values), nil
}

func (s *State) classConstGetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "const_get expects a name")}
	}
	name := args[0].String()
	for current := class; current != nil; current = current.super {
		if value, ok := current.constants[name]; ok {
			return value, nil
		}
	}
	return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "uninitialized constant "+name)}
}

func (s *State) classConstSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "const_set expects a name and value")}
	}
	class.constants[args[0].String()] = args[1]
	return args[1], nil
}

func (s *State) classAncestorsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	var values []Value
	seen := make(map[*Class]bool)
	var appendClass func(*Class)
	appendClass = func(current *Class) {
		if current == nil || seen[current] {
			return
		}
		seen[current] = true
		if current != class {
			values = append(values, s.classValue(current))
		}
		for i := len(current.included) - 1; i >= 0; i-- {
			appendClass(current.included[i])
		}
		for current.super != nil {
			appendClass(current.super)
			break
		}
	}
	for i := len(class.prepended) - 1; i >= 0; i-- {
		appendClass(class.prepended[i])
	}
	values = append(values, s.classValue(class))
	for i := len(class.included) - 1; i >= 0; i-- {
		appendClass(class.included[i])
	}
	if class.super != nil {
		appendClass(class.super)
	}
	return ArrayValue(values), nil
}

func frozenString(text string) Value {
	value := StringValue(text)
	if value.obj != nil {
		value.obj.frozen = true
	}
	return value
}

func (s *State) classNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#new receiver is not a class")
	}
	if class == s.ModuleClass {
		created := newClass("#<Module>", nil)
		created.isModule = true
		s.ensureClassObject(created)
		createdValue := s.classValue(created)
		if !block.IsNil() {
			if _, err := s.callBlockWithSelf(ctx, block, createdValue, []Value{createdValue}, Nil); err != nil {
				return Nil, err
			}
		}
		return createdValue, nil
	}
	if class == s.ClassClass {
		super := s.ObjectClass
		if len(args) > 0 {
			parent, ok := s.asClass(args[0])
			if !ok {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
					fmt.Sprintf("%s is not class/module", args[0].Inspect()))}
			}
			if parent != s.BasicObjectClass && parent.super == nil {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
					fmt.Sprintf("superclass must be a Class (%s given)", args[0].Inspect()))}
			}
			super = parent
		}
		created := newClass("#<Class>", super)
		s.ensureClassObject(created)
		createdValue := s.classValue(created)
		if !block.IsNil() {
			if _, err := s.callBlockWithSelf(ctx, block, createdValue, []Value{createdValue}, Nil); err != nil {
				return Nil, err
			}
		}
		return createdValue, nil
	}

	if class == s.RangeClass {
		if len(args) < 2 || len(args) > 3 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
		}
		exclusive := len(args) == 3 && args[2].IsTruthy()
		if err := s.validateRangeBound(args[0], args[1]); err != nil {
			return Nil, err
		}
		return RangeValue(args[0], args[1], exclusive), nil
	}

	var instance Value
	switch {
	case s.classIsSubclass(class, s.ArrayClass):
		values := []Value(nil)
		if len(args) == 1 {
			if args[0].Kind() == KindInt {
				count := args[0].Int()
				if count < 0 {
					return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
				}
				values = make([]Value, count)
				if !block.IsNil() {
					for i := range values {
						item, err := s.callBlock1(ctx, block, IntValue(int64(i)), Nil)
						if err != nil {
							return Nil, err
						}
						values[i] = item
					}
				}
			} else if argValues := arrayValues(args[0]); args[0].obj != nil {
				if _, ok := args[0].obj.data.(*arrayData); ok {
					values = append([]Value(nil), argValues...)
				}
			}
		} else if len(args) >= 2 && args[0].Kind() == KindInt {
			count := args[0].Int()
			if count < 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative array size")}
			}
			values = make([]Value, count)
			for i := range values {
				values[i] = args[1]
			}
		}
		instance = ObjectValue(&Object{class: class, data: &arrayData{values: values}})
	case s.classIsSubclass(class, s.HashClass):
		data := &hashData{}
		if !block.IsNil() {
			data.defaultProc = block
		} else if len(args) > 0 {
			data.defaultValue = args[0]
		}
		instance = ObjectValue(&Object{class: class, data: data})
	case s.classIsSubclass(class, s.StringClass):
		value := ""
		if len(args) > 0 {
			value = args[0].ToS()
		}
		instance = ObjectValue(&Object{class: class, data: &stringData{raw: value}})
	case s.classIsSubclass(class, s.ExceptionClass):
		message := ""
		if len(args) > 0 {
			message = args[0].ToS()
		}
		instance = newException(class, message)
		if data, ok := instance.obj.data.(*exceptionData); ok {
			if s.classIsSubclass(class, s.NameErrorClass) && len(args) > 1 {
				data.name = args[1]
			}
			if s.classIsSubclass(class, s.NoMethodErrorClass) && len(args) > 2 {
				data.args = args[2]
			}
			if kw.Kind() == KindObject && kw.obj != nil {
				if hash, ok := kw.obj.data.(*hashData); ok {
					if index, found, err := s.hashLookupKey(ctx, hash, SymbolValue("receiver")); err == nil && found {
						data.receiver = hash.entries[index].value
						data.receiverSet = true
					}
				}
			}
			if kw.Kind() == KindObject && kw.obj != nil {
				if entries, ok := kw.obj.data.(*hashData); ok {
					for _, entry := range entries.entries {
						if entry.key.Kind() == KindSymbol && entry.key.String() == "receiver" {
							data.receiver = entry.value
							data.receiverSet = true
						} else {
							return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "unknown keyword: "+entry.key.Inspect())}
						}
					}
				}
			}
		}
	default:
		instance = ObjectValue(&Object{class: class})
	}
	if m, ok := class.lookup("initialize"); ok {
		if _, err := s.invoke(ctx, m, instance, args, kw, block); err != nil {
			return Nil, err
		}
	}
	return instance, nil
}

func (s *State) classEvalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return self, nil
	}
	return s.callBlockWithSelf(ctx, block, self, nil, Nil)
}

func (s *State) classAliasMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 2 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "alias_method expects two names")}
	}
	newName := args[0].String()
	oldName := args[1].String()
	m, ok := class.lookup(oldName)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "undefined method '"+oldName+"'")}
	}
	class.methods[newName] = m
	return SymbolValue(newName), nil
}

func (s *State) classRemoveMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "remove_method expects one name")}
	}
	name := args[0].String()
	if _, ok := class.methods[name]; !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NameErrorClass, "method '"+name+"' not defined in "+class.name)}
	}
	delete(class.methods, name)
	return self, nil
}

func (s *State) classIsSubclass(class, super *Class) bool {
	for current := class; current != nil; current = current.super {
		if current == super {
			return true
		}
	}
	return false
}

func (s *State) classSuperclassMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#superclass receiver is not a class")
	}
	if class.super == nil {
		return Nil, nil
	}
	return s.classValue(class.super), nil
}

func (s *State) classNameMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#name receiver is not a class")
	}
	return StringValue(class.name), nil
}

func (s *State) classCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return False, nil
	}
	return BoolValue(s.isA(args[0], class)), nil
}

func (s *State) isA(value Value, class *Class) bool {
	for current := s.classOf(value); current != nil; current = current.super {
		if current == class {
			return true
		}
	}
	return false
}

// classOp implements OP_CLASS / OP_MODULE: resolve the outer namespace,
// create or reopen the class, and validate the superclass.
func (s *State) classOp(ctx context.Context, outerValue, superValue Value, name string, isClass bool) (Value, error) {
	outer := s.ObjectClass
	if !outerValue.IsNil() {
		class, ok := s.asClass(outerValue)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
				fmt.Sprintf("%s is not class/module", outerValue.Inspect()))}
		}
		outer = class
	}
	super := s.ObjectClass
	if !isClass {
		super = nil
	}
	if isClass && !superValue.IsNil() {
		class, ok := s.asClass(superValue)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
				fmt.Sprintf("superclass must be a Class (%s given)", superValue.Inspect()))}
		}
		if class.isModule {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
				fmt.Sprintf("superclass must be a Class (%s given)", superValue.Inspect()))}
		}
		super = class
	}
	var class *Class
	if existing := s.lookupConstant(outer, name); !existing.IsNil() {
		if c, ok := s.asClass(existing); ok {
			class = c
		}
	}
	if isClass && class != nil && !superValue.IsNil() {
		if candidate, ok := s.asClass(superValue); ok && candidate == class {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "superclass must not be the class itself")}
		}
	}
	if class == nil {
		class = newClass(name, super)
		class.isModule = !isClass
		s.ensureClassObject(class)
		outer.constants[name] = s.classValue(class)
		s.constants[name] = s.classValue(class)
		if isClass && super != nil {
			if _, err := s.call(ctx, s.classValue(super), "inherited", []Value{s.classValue(class)}, Nil, Nil); err != nil {
				return Nil, err
			}
		}
	}
	return s.classValue(class), nil
}

func namesResult(args []Value) Value {
	if len(args) == 0 {
		return Nil
	}
	if len(args) == 1 {
		return args[0]
	}
	return ArrayValue(append([]Value(nil), args...))
}

func (s *State) classIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class, ok := s.asClass(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: Class#include receiver is not a class")
	}
	if len(args) == 0 {
		return Nil, fmt.Errorf("mrbvm: include expects a module")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	for _, arg := range args {
		if _, ok := s.asClass(arg); !ok {
			return Nil, fmt.Errorf("mrbvm: include expects a module")
		}
		if _, err := s.call(ctx, arg, "append_features", []Value{s.classValue(class)}, Nil, Nil); err != nil {
			return Nil, err
		}
		if _, err := s.call(ctx, arg, "included", []Value{s.classValue(class)}, Nil, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) classAppendFeaturesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	module, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "append_features expects one argument")}
	}
	base, ok := s.asClass(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a class")}
	}
	s.includeFeatures(base, module, false)
	return args[0], nil
}

func (s *State) classPrependFeaturesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	module, ok := s.asClass(self)
	if !ok || len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "prepend_features expects one argument")}
	}
	base, ok := s.asClass(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a class")}
	}
	s.includeFeatures(base, module, true)
	return args[0], nil
}

func (s *State) classPrependMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, ok := s.asClass(self); !ok {
		return Nil, fmt.Errorf("mrbvm: prepend receiver is not a class")
	}
	if len(args) == 0 {
		return Nil, fmt.Errorf("mrbvm: prepend expects a module")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	for i := len(args) - 1; i >= 0; i-- {
		if _, ok := s.asClass(args[i]); !ok {
			return Nil, fmt.Errorf("mrbvm: prepend expects a module")
		}
		if _, err := s.call(ctx, args[i], "prepend_features", []Value{self}, Nil, Nil); err != nil {
			return Nil, err
		}
		if _, err := s.call(ctx, args[i], "prepended", []Value{self}, Nil, Nil); err != nil {
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) includeFeatures(base, module *Class, prepend bool) {
	for name, m := range module.methods {
		if prepend {
			copied := *m
			copied.owner = base
			base.methods[name] = &copied
		} else if _, exists := base.methods[name]; !exists {
			copied := *m
			copied.owner = base
			base.methods[name] = &copied
		}
	}
	for name, value := range module.constants {
		if _, exists := base.constants[name]; !exists {
			base.constants[name] = value
		}
	}
	for name := range module.privateMethods {
		base.privateMethods[name] = true
	}
	if prepend {
		found := false
		for _, current := range base.prepended {
			if current == module {
				found = true
				break
			}
		}
		if !found {
			base.prepended = append(base.prepended, module)
		}
		return
	}
	found := false
	for _, included := range base.included {
		if included == module {
			found = true
			break
		}
	}
	if !found {
		base.included = append(base.included, module)
	}
}

func (s *State) objectClassMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.classValue(s.classOf(self)), nil
}

func (s *State) index(ctx context.Context, recv, index Value) (Value, error) {
	if recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if err := s.ensureMutable(recv); err != nil {
				return Nil, err
			}
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					return sliceArray(data.values, rangeIndex), nil
				}
			}
			pos, err := s.indexInteger(index)
			if err != nil {
				return Nil, err
			}
			if pos < 0 {
				pos += len(data.values)
			}
			if pos < 0 || pos >= len(data.values) {
				return Nil, nil
			}
			return data.values[pos], nil
		case *hashData:
			if i, ok, err := s.hashLookupKey(ctx, data, index); err != nil {
				return Nil, err
			} else if ok {
				return data.entries[i].value, nil
			}
			return s.call(ctx, recv, "default", []Value{index}, Nil, Nil)
		case *stringData:
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					return sliceString(data.str(), rangeIndex), nil
				}
			}
			pos, err := s.indexInteger(index)
			if err != nil {
				return Nil, err
			}
			runes := []rune(data.str())
			if pos < 0 {
				pos += len(runes)
			}
			if pos < 0 || pos >= len(runes) {
				return Nil, nil
			}
			return StringValue(string(runes[pos])), nil
		}
	}
	return Nil, fmt.Errorf("mrbvm: %s is not indexable", recv.Inspect())
}

func sliceArray(values []Value, data *rangeData) Value {
	length := len(values)
	low := 0
	high := length
	originalLow := 0
	if !data.low.IsNil() {
		if data.low.Kind() == KindInt {
			originalLow = int(data.low.Int())
			low = originalLow
			if low < 0 {
				low += length
			}
		}
	}
	if !data.high.IsNil() {
		if data.high.Kind() == KindInt {
			high = int(data.high.Int())
			if high < 0 {
				high += length
			}
			if data.exclusive {
				high--
			}
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
	if high < low {
		return ArrayValue(nil)
	}
	result := make([]Value, high-low+1)
	copy(result, values[low:high+1])
	return ArrayValue(result)
}

func (s *State) indexInteger(index Value) (int, error) {
	switch index.Kind() {
	case KindInt:
		return int(index.Int()), nil
	case KindFloat:
		f := index.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || f >= math.MaxInt64 || f <= math.MinInt64 {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "float out of range")}
		}
		return int(f), nil
	default:
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "index must be an integer")}
	}
}

func (s *State) arrayIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	switch len(args) {
	case 0:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments (given 0, expected 1..2)")}
	case 1:
		return s.index(ctx, self, args[0])
	case 2:
		start, err := s.indexInteger(args[0])
		if err != nil {
			return Nil, err
		}
		length, err := s.indexInteger(args[1])
		if err != nil {
			return Nil, err
		}
		return s.arraySliceAt(self, start, length)
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))}
	}
}

func (s *State) arraySliceAt(self Value, start, length int) (Value, error) {
	values := arrayValues(self)
	if length < 0 {
		return Nil, nil
	}
	if start < 0 {
		start += len(values)
	}
	if start < 0 || start > len(values) {
		return Nil, nil
	}
	end := start + length
	if end > len(values) {
		end = len(values)
	}
	if end < start {
		end = start
	}
	return ArrayValue(append([]Value(nil), values[start:end]...)), nil
}

func (s *State) hashIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: Hash#[] expects one argument")
	}
	return s.index(ctx, self, args[0])
}

func (s *State) arrayLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	return IntValue(int64(len(data.values))), nil
}

func (s *State) hashLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return IntValue(0), nil
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	return IntValue(int64(len(data.entries))), nil
}

func (s *State) stringLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	count := 0
	for len(text) > 0 {
		_, size := utf8.DecodeRuneInString(text)
		if size == 0 {
			break
		}
		text = text[size:]
		count++
	}
	return IntValue(int64(count)), nil
}

func hashHasKey(hash Value, name string) bool {
	if hash.Kind() != KindObject || hash.obj == nil {
		return false
	}
	data, ok := hash.obj.data.(*hashData)
	if !ok {
		return false
	}
	key := SymbolValue(name)
	for _, entry := range data.entries {
		if valuesEqual(entry.key, key) {
			return true
		}
	}
	return false
}

func hashDeleteKey(hash Value, name string) (Value, bool) {
	if hash.Kind() != KindObject || hash.obj == nil {
		return Nil, false
	}
	data, ok := hash.obj.data.(*hashData)
	if !ok {
		return Nil, false
	}
	key := SymbolValue(name)
	for i, entry := range data.entries {
		if valuesEqual(entry.key, key) {
			data.entries = append(data.entries[:i], data.entries[i+1:]...)
			return entry.value, true
		}
	}
	return Nil, false
}

func hashDataOf(self Value) (*hashData, bool) {
	if self.Kind() != KindObject || self.obj == nil {
		return nil, false
	}
	data, ok := self.obj.data.(*hashData)
	return data, ok
}

func (s *State) hashEntries(self Value) ([]hashEntry, bool) {
	if self.Kind() != KindObject || self.obj == nil {
		return nil, false
	}
	data, ok := self.obj.data.(*hashData)
	if !ok {
		return nil, false
	}
	return data.entries, true
}

func (s *State) hashDeconstructKeysMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	entries, ok := s.hashEntries(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects one argument")
	}
	if args[0].IsNil() {
		return HashValue(append([]hashEntry(nil), entries...)), nil
	}
	if args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects an array or nil")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: deconstruct_keys expects an array or nil")
	}
	data, ok := hashDataOf(self)
	if !ok {
		return HashValue(nil), nil
	}
	result := make([]hashEntry, 0, len(keys.values))
	for _, key := range keys.values {
		if index, found, err := s.hashLookupKey(ctx, data, key); err != nil {
			return Nil, err
		} else if found {
			result = append(result, data.entries[index])
		}
	}
	return HashValue(result), nil
}

func (s *State) hashPatValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, ok := s.hashEntries(self); !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 || args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: __pat_values expects an array")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: __pat_values expects an array")
	}
	data, ok := hashDataOf(self)
	if !ok {
		return False, nil
	}
	values := make([]Value, 0, len(keys.values))
	for _, key := range keys.values {
		index, found, err := s.hashLookupKey(ctx, data, key)
		if err != nil {
			return Nil, err
		}
		if !found {
			return False, nil
		}
		values = append(values, data.entries[index].value)
	}
	return ArrayValue(values), nil
}

func (s *State) hashExceptMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	entries, ok := s.hashEntries(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a hash")
	}
	if len(args) != 1 || args[0].Kind() != KindObject || args[0].obj == nil {
		return Nil, fmt.Errorf("mrbvm: __except expects an array")
	}
	keys, ok := args[0].obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: __except expects an array")
	}
	var result []hashEntry
	for _, entry := range entries {
		excluded := false
		for _, key := range keys.values {
			equal, err := s.call(ctx, entry.key, "eql?", []Value{key}, Nil, Nil)
			if err != nil {
				return Nil, err
			}
			if equal.IsTruthy() {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, entry)
		}
	}
	return HashValue(result), nil
}

func (s *State) rangeCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: Range#=== expects one argument")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if !data.low.IsNil() {
		if low, ok := binaryValue(args[0], ">=", data.low); !ok || !low.IsTruthy() {
			return False, nil
		}
	}
	if !data.high.IsNil() {
		op := "<="
		if data.exclusive {
			op = "<"
		}
		if high, ok := binaryValue(args[0], op, data.high); ok {
			return high, nil
		}
		return False, nil
	}
	return True, nil
}

func (s *State) stringUpcaseMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(strings.ToUpper(self.String())), nil
}

func (s *State) stringDowncaseMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(strings.ToLower(self.String())), nil
}

func (s *State) rangeToAMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if data.high.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "cannot convert endless range to an array")}
	}
	if data.low.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't iterate")}
	}
	values := make([]Value, 0)
	if data.low.Kind() != KindInt {
		current := data.low
		for {
			cmp, err := s.compareValues(ctx, current, data.high, Nil)
			if err != nil {
				return Nil, err
			}
			if cmp > 0 || (data.exclusive && cmp == 0) {
				break
			}
			values = append(values, current)
			next, err := s.call(ctx, current, "succ", nil, Nil, Nil)
			if err != nil {
				return Nil, err
			}
			if valuesEqual(next, current) {
				break
			}
			current = next
		}
		return ArrayValue(values), nil
	}
	start := data.low.Int()
	switch data.high.Kind() {
	case KindInt:
		high := data.high.Int()
		for i := start; ; i++ {
			if data.exclusive && i >= high {
				break
			}
			if !data.exclusive && i > high {
				break
			}
			values = append(values, IntValue(i))
			if i == math.MaxInt64 {
				break
			}
		}
	case KindFloat:
		upper := data.high.Float()
		if math.IsInf(upper, 1) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "cannot convert endless range to an array")}
		}
		if math.IsInf(upper, -1) || math.IsNaN(upper) {
			return ArrayValue(values), nil
		}
		if upper >= 9223372036854775808.0 || upper < -9223372036854775808.0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "float domain error")}
		}
		if data.exclusive && upper <= -9223372036854775808.0 {
			return ArrayValue(values), nil
		}
		var maxInt int64
		if data.exclusive {
			maxInt = int64(math.Ceil(upper)) - 1
		} else {
			maxInt = int64(math.Floor(upper))
		}
		for i := start; i <= maxInt; i++ {
			values = append(values, IntValue(i))
			if i == math.MaxInt64 {
				break
			}
		}
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't iterate")}
	}
	return ArrayValue(values), nil
}

// LoadBytecode parses a RITE 04.00 binary.
func (s *State) LoadBytecode(data []byte) (*Irep, error) {
	return DecodeBytecode(data)
}

// Run loads and executes a RITE 04.00 binary.
func (s *State) Run(ctx context.Context, data []byte) (Value, error) {
	irep, err := s.LoadBytecode(data)
	if err != nil {
		return Nil, err
	}
	result, err := s.Execute(ctx, irep)
	if s.output != nil && s.output.exceeded {
		return result, errOutputLimit
	}
	return result, err
}

// Execute runs a top-level irep.
// Allowed reports whether the embedding's policy permits capability.
func (s *State) Allowed(capability sandbox.Capability) bool {
	return s.policy.Allowed(capability)
}

func (s *State) Execute(ctx context.Context, irep *Irep) (Value, error) {
	if s.initErr != nil {
		return Nil, s.initErr
	}
	if irep == nil {
		return Nil, fmt.Errorf("mrbvm: nil irep")
	}
	return s.executeFrame(ctx, irep, s.TopSelf, s.ObjectClass, nil, Nil, Nil, nil, "", nil, false)
}

const (
	hashInitialSize = 8
	fnvOffset       = 14695981039346656037
	fnvPrime        = 1099511628211
)

// indexableHashKey returns a table key for primitive hash keys. Other kinds
// fall back to a linear scan so that Ruby-level eql?/hash can be honoured.
func indexableHashKey(value Value) (hashKey, bool) {
	key := hashKey{kind: value.Kind()}
	switch value.Kind() {
	case KindNil, KindTrue, KindFalse:
	case KindInt:
		key.i = value.Int()
	case KindFloat:
		// mruby treats 0.0 and -0.0 as the same hash key.
		if value.Float() == 0 {
			key.bits = 0
		} else {
			key.bits = value.bits
		}
	case KindString, KindSymbol:
		key.str = value.String()
	default:
		return hashKey{}, false
	}
	return key, true
}

func (k hashKey) hash() uint64 {
	h := uint64(k.kind) * 0x9e3779b97f4a7c15
	switch k.kind {
	case KindString, KindSymbol:
		for i := 0; i < len(k.str); i++ {
			h ^= uint64(k.str[i])
			h *= fnvPrime
		}
	case KindFloat:
		h ^= k.bits
	default:
		h ^= uint64(k.i)
	}
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	return h
}

func ensureHashIndex(data *hashData) {
	if data.index != nil && data.indexBuilt == len(data.entries) {
		return
	}
	size := hashInitialSize
	for size < len(data.entries)*2 {
		size <<= 1
	}
	table := make([]int32, size)
	for i := range table {
		table[i] = -1
	}
	mask := size - 1
	for i := range data.entries {
		key, ok := indexableHashKey(data.entries[i].key)
		if !ok {
			continue
		}
		pos := int(key.hash()) & mask
		for table[pos] != -1 {
			pos = (pos + 1) & mask
		}
		table[pos] = int32(i)
	}
	data.index = table
	data.indexBuilt = len(data.entries)
}

func addHashIndex(data *hashData, index int) {
	if data.index == nil || len(data.entries)*2 >= len(data.index) {
		// Rebuild once the table would exceed a 50% load factor.
		ensureHashIndex(data)
		return
	}
	if key, ok := indexableHashKey(data.entries[index].key); ok {
		mask := len(data.index) - 1
		pos := int(key.hash()) & mask
		for data.index[pos] != -1 {
			pos = (pos + 1) & mask
		}
		data.index[pos] = int32(index)
	}
	data.indexBuilt = len(data.entries)
}

// appendHashEntry grows the entry slice geometrically. Go's append switches to
// a ~1.25x growth factor for large element sizes, which makes the cumulative
// allocation much larger than the final slice.
func appendHashEntry(entries []hashEntry, entry hashEntry) []hashEntry {
	if len(entries) == cap(entries) {
		capacity := cap(entries) * 2
		if capacity < 8 {
			capacity = 8
		}
		grown := make([]hashEntry, len(entries), capacity)
		copy(grown, entries)
		entries = grown
	}
	return append(entries, entry)
}

func hashLookup(data *hashData, key Value) (int, bool) {
	ensureHashIndex(data)
	if tableKey, ok := indexableHashKey(key); ok {
		mask := len(data.index) - 1
		pos := int(tableKey.hash()) & mask
		for {
			index := data.index[pos]
			if index < 0 {
				return -1, false
			}
			if valuesEqual(data.entries[index].key, key) {
				return int(index), true
			}
			pos = (pos + 1) & mask
		}
	}
	for i := range data.entries {
		if valuesEqual(data.entries[i].key, key) {
			return i, true
		}
	}
	return -1, false
}

func identityEqual(left, right Value) bool {
	if left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case KindObject, KindString, KindBigInt:
		return left.obj == right.obj
	case KindSymbol:
		return left.String() == right.String()
	default:
		return valuesEqual(left, right)
	}
}

func (s *State) hashCode(ctx context.Context, value Value) (int64, error) {
	result, err := s.call(ctx, value, "hash", nil, Nil, Nil)
	if err != nil {
		return 0, err
	}
	if result.Kind() != KindInt {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "hash must return an Integer")}
	}
	return result.Int(), nil
}

func (s *State) hashCompare(ctx context.Context, left, right Value, strict bool) (bool, error) {
	leftData, leftOK := hashDataOf(left)
	rightData, rightOK := hashDataOf(right)
	if !leftOK || !rightOK || len(leftData.entries) != len(rightData.entries) {
		return false, nil
	}
	for _, entry := range leftData.entries {
		index, found, err := s.hashLookupKey(ctx, rightData, entry.key)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		other := rightData.entries[index].value
		if strict {
			if !eqlValues(entry.value, other) {
				return false, nil
			}
		} else if !valuesEqual(entry.value, other) {
			return false, nil
		}
	}
	return true, nil
}

func (s *State) hashLookupObject(ctx context.Context, data *hashData, key Value) (int, bool, error) {
	keyHash, err := s.hashCode(ctx, key)
	if err != nil {
		return -1, false, err
	}
	limit := len(data.entries)
	for i := 0; i < limit && i < len(data.entries); i++ {
		stored := data.entries[i].key
		if stored.Kind() != KindObject && stored.Kind() != KindBigInt {
			continue
		}
		storedHash, err := s.hashCode(ctx, stored)
		if err != nil {
			return -1, false, err
		}
		if storedHash != keyHash {
			continue
		}
		equal, err := s.call(ctx, stored, "eql?", []Value{key}, Nil, Nil)
		if err != nil {
			return -1, false, err
		}
		if equal.IsTruthy() {
			if i >= len(data.entries) {
				return -1, false, nil
			}
			return i, true, nil
		}
	}
	return -1, false, nil
}

// hashLookupKey resolves a hash key. Primitive keys use the open-addressed
// index; object keys call Ruby hash/eql? so user-defined key semantics work.
func (s *State) hashLookupKey(ctx context.Context, data *hashData, key Value) (int, bool, error) {
	if data.identity {
		for i := range data.entries {
			if identityEqual(data.entries[i].key, key) {
				return i, true, nil
			}
		}
		return -1, false, nil
	}
	if tableKey, ok := indexableHashKey(key); ok {
		ensureHashIndex(data)
		mask := len(data.index) - 1
		position := int(tableKey.hash()) & mask
		for {
			index := data.index[position]
			if index < 0 {
				return -1, false, nil
			}
			if valuesEqual(data.entries[index].key, key) {
				return int(index), true, nil
			}
			position = (position + 1) & mask
		}
	}
	return s.hashLookupObject(ctx, data, key)
}

func (s *State) setIndex(ctx context.Context, recv, index, value Value) (Value, error) {
	if recv.kind == KindObject && recv.obj != nil {
		switch data := recv.obj.data.(type) {
		case *arrayData:
			if err := s.ensureMutable(recv); err != nil {
				return Nil, err
			}
			if index.Kind() == KindObject && index.obj != nil {
				if rangeIndex, ok := index.obj.data.(*rangeData); ok {
					start, length := rangeBounds(rangeIndex, len(data.values))
					return s.arrayAssign(data, start, length, value)
				}
			}
			pos, err := s.indexInteger(index)
			if err != nil {
				return Nil, err
			}
			if pos < 0 {
				pos += len(data.values)
			}
			if pos < 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, fmt.Sprintf("index %d too small for array", pos))}
			}
			for len(data.values) <= pos {
				data.values = append(data.values, Nil)
			}
			data.values[pos] = value
			return value, nil
		case *hashData:
			if i, ok, err := s.hashLookupKey(ctx, data, index); err != nil {
				return Nil, err
			} else if ok {
				data.entries[i].value = value
				return value, nil
			}
			if err := s.checkHashSize(len(data.entries) + 1); err != nil {
				return Nil, err
			}
			if index.Kind() == KindString {
				copied := StringValue(index.String())
				copied.obj.frozen = true
				index = copied
			}
			ensureHashIndex(data)
			data.entries = appendHashEntry(data.entries, hashEntry{key: index, value: value})
			addHashIndex(data, len(data.entries)-1)
			return value, nil
		}
	}
	return Nil, fmt.Errorf("mrbvm: %s does not support index assignment", recv.Inspect())
}

func (s *State) kernelP(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, arg := range args {
		fmt.Fprintln(s.stdout, arg.Inspect())
	}
	switch len(args) {
	case 0:
		return Nil, nil
	case 1:
		return args[0], nil
	default:
		// args may be a pooled buffer owned by the caller.
		return ArrayValue(append([]Value(nil), args...)), nil
	}
}

func (s *State) validateRangeBound(low, high Value) error {
	if low.IsNil() || high.IsNil() {
		return nil
	}
	if (low.Kind() == KindFloat && math.IsNaN(low.Float())) ||
		(high.Kind() == KindFloat && math.IsNaN(high.Float())) {
		return &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "bad value for range")}
	}
	return nil
}

func (s *State) rangeEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	data, ok := self.obj.data.(*rangeData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not a range")
	}
	if data.low.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't iterate")}
	}
	yield := func(value Value) (bool, error) {
		if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}
	if data.low.Kind() == KindInt && (data.high.IsNil() || data.high.Kind() == KindInt) {
		high := int64(0)
		if !data.high.IsNil() {
			high = data.high.Int()
		}
		for i := data.low.Int(); ; i++ {
			if !data.high.IsNil() {
				if data.exclusive && i >= high {
					break
				}
				if !data.exclusive && i > high {
					break
				}
			}
			more, err := yield(IntValue(i))
			if err != nil {
				return Nil, err
			}
			if !more {
				break
			}
			if i == math.MaxInt64 {
				break
			}
		}
		return self, nil
	}
	current := data.low
	for {
		if !data.high.IsNil() {
			cmp, err := s.compareValues(ctx, current, data.high, Nil)
			if err != nil {
				return Nil, err
			}
			if cmp > 0 || (data.exclusive && cmp == 0) {
				break
			}
		}
		more, err := yield(current)
		if err != nil {
			return Nil, err
		}
		if !more {
			break
		}
		next, err := s.call(ctx, current, "succ", nil, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		if valuesEqual(next, current) {
			break
		}
		current = next
	}
	return self, nil
}

func (s *State) arrayPushMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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
	data.values = append(data.values, args...)
	return self, nil
}

func (s *State) arraySetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := arrayDataOf(self)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	switch len(args) {
	case 0:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments (given 0, expected 2..3)")}
	case 2:
		if rangeData, ok := rangeDataOf(args[0]); ok {
			start, length := rangeBounds(rangeData, len(data.values))
			return s.arrayAssign(data, start, length, args[1])
		}
		return s.setIndex(ctx, self, args[0], args[1])
	case 3:
		start, err := s.indexInteger(args[0])
		if err != nil {
			return Nil, err
		}
		length, err := s.indexInteger(args[1])
		if err != nil {
			return Nil, err
		}
		return s.arrayAssign(data, start, length, args[2])
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected 2..3)", len(args)))}
	}
}

func arrayDataOf(value Value) (*arrayData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*arrayData)
	return data, ok
}

func rangeBounds(data *rangeData, length int) (int, int) {
	start := 0
	end := length - 1
	if !data.low.IsNil() && data.low.Kind() == KindInt {
		start = int(data.low.Int())
		if start < 0 {
			start += length
		}
	}
	if !data.high.IsNil() && data.high.Kind() == KindInt {
		end = int(data.high.Int())
		if end < 0 {
			end += length
		}
		if data.exclusive {
			end--
		}
	}
	if end >= length {
		end = length - 1
	}
	sliceLength := end - start + 1
	if sliceLength < 0 {
		sliceLength = 0
	}
	return start, sliceLength
}

func (s *State) arrayAssign(data *arrayData, start, length int, value Value) (Value, error) {
	if length < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "negative length")}
	}
	if start < 0 {
		start += len(data.values)
		if start < 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index too small for array")}
		}
	}
	var replacement []Value
	if replacementData, ok := arrayDataOf(value); ok {
		replacement = append([]Value(nil), replacementData.values...)
	} else {
		replacement = []Value{value}
	}
	if start > len(data.values) {
		if start-len(data.values) > 1<<20 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index too big for array")}
		}
		for len(data.values) < start {
			data.values = append(data.values, Nil)
		}
	}
	end := len(data.values)
	if start <= len(data.values) && length <= len(data.values)-start {
		end = start + length
	}
	if end < start {
		end = start
	}
	if err := s.checkArraySize(len(data.values) - (end - start) + len(replacement)); err != nil {
		return Nil, err
	}
	result := make([]Value, 0, len(data.values)-(end-start)+len(replacement))
	result = append(result, data.values[:start]...)
	result = append(result, replacement...)
	result = append(result, data.values[end:]...)
	data.values = result
	return value, nil
}

func (s *State) hashSetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 2 {
		return Nil, fmt.Errorf("mrbvm: Hash#[]= expects two arguments")
	}
	return s.setIndex(ctx, self, args[0], args[1])
}

func copyObjectIvars(obj *Object) map[string]Value {
	if len(obj.ivars) == 0 {
		return nil
	}
	ivars := make(map[string]Value, len(obj.ivars))
	for name, value := range obj.ivars {
		ivars[name] = value
	}
	return ivars
}

func (s *State) objectDupMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return self, nil
	}
	var duplicate Value
	switch data := self.obj.data.(type) {
	case *stringData:
		duplicate = StringValue(data.str())
	case *arrayData:
		duplicate = ArrayValue(append([]Value(nil), data.values...))
	case *hashData:
		duplicate = HashValue(append([]hashEntry(nil), data.entries...))
		if copyData, ok := duplicate.obj.data.(*hashData); ok {
			copyData.defaultValue = data.defaultValue
			copyData.defaultProc = data.defaultProc
			copyData.identity = data.identity
		}
	case *rangeData:
		duplicate = RangeValue(data.low, data.high, data.exclusive)
	case *exceptionData:
		copyData := *data
		copyData.backtrace = append([]string(nil), data.backtrace...)
		duplicate = ObjectValue(&Object{class: self.obj.class, data: &copyData})
	case *Class:
		return self, nil
	case *blockData:
		copyData := *data
		duplicate = ObjectValue(&Object{data: &copyData})
	default:
		duplicate = ObjectValue(&Object{class: self.obj.class})
	}
	if duplicate.obj != nil {
		duplicate.obj.class = self.obj.class
		duplicate.obj.ivars = copyObjectIvars(self.obj)
	}
	return duplicate, nil
}

func (s *State) objectRespondToMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	name := args[0].String()
	if class, ok := s.asClass(self); ok {
		if _, ok := class.lookupClassMethod(name); ok {
			return True, nil
		}
	}
	if _, ok := s.classOf(self).lookup(name); ok {
		return True, nil
	}
	return False, nil
}

func (s *State) kernelProcMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block given")}
	}
	return block, nil
}

func (s *State) newMethodError(name string, recv Value, args []Value) Value {
	message := fmt.Sprintf("undefined method '%s' for %s", name, s.classOf(recv).name)
	exception := newException(s.NoMethodErrorClass, message)
	if data, ok := exception.obj.data.(*exceptionData); ok {
		data.name = SymbolValue(name)
		data.receiver = recv
		data.receiverSet = true
		data.args = ArrayValue(append([]Value(nil), args...))
	}
	s.attachImplicitCause(exception)
	return exception
}

func (s *State) newConstError(name string, recv Value) Value {
	exception := newException(s.NameErrorClass, "uninitialized constant "+name)
	if data, ok := exception.obj.data.(*exceptionData); ok {
		data.name = SymbolValue(name)
		data.receiver = recv
		data.receiverSet = true
	}
	return exception
}

func (s *State) newNameError(name string, recv Value) Value {
	exception := newException(s.NameErrorClass, "undefined method '"+name+"'")
	if data, ok := exception.obj.data.(*exceptionData); ok {
		data.name = SymbolValue(name)
		data.receiver = recv
		data.receiverSet = true
	}
	s.attachImplicitCause(exception)
	return exception
}

func (s *State) attachImplicitCause(exception Value) {
	if s.currentException.IsNil() || s.currentException.obj == exception.obj {
		return
	}
	if data, ok := exception.obj.data.(*exceptionData); ok {
		if data.cause.IsNil() && !causeChainContains(s.currentException, exception) {
			data.cause = s.currentException
		}
	}
}

func (s *State) kernelRaise(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	causeGiven := false
	cause := Nil
	otherKeywords := false
	if kw.Kind() == KindObject && kw.obj != nil {
		if entries, ok := kw.obj.data.(*hashData); ok {
			for _, entry := range entries.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "cause" {
					cause = entry.value
					causeGiven = true
				} else {
					otherKeywords = true
				}
			}
		}
	}
	if len(args) == 0 {
		if !causeGiven && !s.currentException.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: s.currentException}
		}
		if causeGiven && s.currentException.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "only cause is given with no arguments")}
		}
		if !s.currentException.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: s.currentException}
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "")}
	}

	var class *Class
	var message string
	var exception Value
	switch {
	case len(args) == 1:
		if argClass, ok := s.asClass(args[0]); ok {
			class = argClass
			message = argClass.name
			if otherKeywords {
				message = kw.ToS()
			}
		} else if args[0].Kind() == KindObject && args[0].obj != nil {
			if _, ok := args[0].obj.data.(*exceptionData); ok {
				exception = args[0]
			} else {
				class = s.RuntimeErrorClass
				message = args[0].ToS()
			}
		} else {
			class = s.RuntimeErrorClass
			message = args[0].ToS()
		}
	default:
		argClass, ok := s.asClass(args[0])
		if !ok {
			return Nil, fmt.Errorf("mrbvm: exception class or instance expected")
		}
		class = argClass
		message = args[1].ToS()
	}
	if exception.IsNil() {
		exception = newException(class, message)
	}
	data, ok := exception.obj.data.(*exceptionData)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: exception}
	}
	if exception.obj.frozen {
		return Nil, &unwindSignal{kind: unwindException, value: exception}
	}
	if causeGiven && !cause.IsNil() {
		if cause.obj == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "exception object expected")}
		}
		if _, ok := cause.obj.data.(*exceptionData); !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "exception object expected")}
		}
		if cause.obj == exception.obj {
			cause = Nil
		} else {
			if causeChainContains(cause, exception) {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "circular causes")}
			}
			data.cause = cause
		}
	} else if !causeGiven {
		if data.cause.IsNil() && !s.currentException.IsNil() && s.currentException.obj != exception.obj &&
			!causeChainContains(s.currentException, exception) {
			data.cause = s.currentException
		}
	}
	return Nil, &unwindSignal{kind: unwindException, value: exception}
}

func causeChainContains(cause, exception Value) bool {
	seen := make(map[*Object]bool)
	for current := cause; !current.IsNil(); {
		if current.obj == nil {
			break
		}
		if current.obj == exception.obj {
			return true
		}
		if seen[current.obj] {
			break
		}
		seen[current.obj] = true
		data, ok := current.obj.data.(*exceptionData)
		if !ok {
			break
		}
		current = data.cause
	}
	return false
}

func (s *State) exceptionMessageMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(exceptionMessage(self)), nil
}

func (s *State) exceptionInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	class := s.classOf(self)
	message := exceptionMessage(self)
	if message == "" {
		return StringValue(class.name), nil
	}
	return StringValue("#<" + class.name + ": " + message + ">"), nil
}

func (s *State) captureBacktrace() []string {
	if len(s.frames) == 0 {
		return nil
	}
	lines := make([]string, 0, len(s.frames))
	for i := len(s.frames) - 1; i >= 0; i-- {
		// Frames usually raise from the same line repeatedly, so cache the
		// formatted file:line instead of building it on every exception.
		frame := &s.frames[i]
		if frame.lineText == "" || frame.lineTextFor != frame.line {
			file := frame.file
			if file == "" {
				file = "unknown"
			}
			frame.lineText = file + ":" + strconv.Itoa(frame.line)
			frame.lineTextFor = frame.line
		}
		lines = append(lines, frame.lineText)
	}
	return lines
}

func (s *State) ensureBacktrace(value Value) {
	if value.obj == nil {
		return
	}
	if data, ok := value.obj.data.(*exceptionData); ok && len(data.backtrace) == 0 {
		data.backtrace = s.captureBacktrace()
	}
}

func (s *State) arraySortMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	values := make([]Value, len(data.values))
	copy(values, data.values)
	copyValue := ArrayValue(values)
	return s.arraySortBangMethod(ctx, copyValue, args, kw, block)
}

func (s *State) arraySortBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
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
	for i := 1; i < len(data.values); i++ {
		for j := i; j > 0; j-- {
			cmp, err := s.compareValues(ctx, data.values[j-1], data.values[j], block)
			if err != nil {
				return Nil, err
			}
			if cmp <= 0 {
				break
			}
			data.values[j-1], data.values[j] = data.values[j], data.values[j-1]
		}
	}
	return self, nil
}

func (s *State) compareValues(ctx context.Context, left, right Value, block Value) (int, error) {
	if !block.IsNil() {
		result, err := s.callBlock2(ctx, block, left, right, Nil)
		if err != nil {
			return 0, err
		}
		return s.orderingResult(ctx, left, right, result)
	}
	result, err := s.call(ctx, left, "<=>", []Value{right}, Nil, Nil)
	if err != nil {
		return 0, err
	}
	return s.orderingResult(ctx, left, right, result)
}

func (s *State) orderingResult(ctx context.Context, left, right, result Value) (int, error) {
	switch result.Kind() {
	case KindInt:
		return compareSign(result.Int()), nil
	case KindBigInt:
		value, _ := result.BigInt()
		return value.Sign(), nil
	case KindFloat:
		value := result.Float()
		if math.IsNaN(value) {
			return 0, nil
		}
		switch {
		case value < 0:
			return -1, nil
		case value > 0:
			return 1, nil
		default:
			return 0, nil
		}
	case KindNil:
		return 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass,
			"comparison of "+s.classOf(left).name+" with "+s.classOf(right).name+" failed")}
	case KindObject:
		greater, err := s.call(ctx, result, ">", []Value{IntValue(0)}, Nil, Nil)
		if err != nil {
			return 0, err
		}
		if greater.IsTruthy() {
			return 1, nil
		}
		less, err := s.call(ctx, result, "<", []Value{IntValue(0)}, Nil, Nil)
		if err != nil {
			return 0, err
		}
		if less.IsTruthy() {
			return -1, nil
		}
		return 0, nil
	default:
		return 0, nil
	}
}

func compareSign(value int64) int {
	switch {
	case value < 0:
		return -1
	case value > 0:
		return 1
	default:
		return 0
	}
}

func (s *State) arraySValueMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return self, nil
	}
	switch len(data.values) {
	case 0:
		return Nil, nil
	case 1:
		return data.values[0], nil
	default:
		return self, nil
	}
}

func (s *State) arraySValueEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	value, err := s.arraySValueMethod(ctx, self, nil, Nil, Nil)
	if err != nil {
		return Nil, err
	}
	return BoolValue(valuesEqual(value, args[0])), nil
}

func (s *State) arrayCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || self.obj == nil {
		return False, nil
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return False, nil
	}
	for _, value := range data.values {
		if class, ok := s.asClass(value); ok && s.isA(args[0], class) {
			return True, nil
		}
		if valuesEqual(value, args[0]) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) arrayMapMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "map", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	values := make([]Value, 0, len(data.values))
	for _, value := range data.values {
		result, err := s.callBlock1(ctx, block, value, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
		values = append(values, result)
	}
	return ArrayValue(values), nil
}

func (s *State) arrayEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each", nil), nil
	}
	if self.obj == nil {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	data, ok := self.obj.data.(*arrayData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: not an array")
	}
	for _, value := range data.values {
		_, err := s.callBlock1(ctx, block, value, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
	}
	return self, nil
}

func (s *State) procCallMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.callBlock(ctx, self, args, kw)
}

func (s *State) callBlockWithSelf(ctx context.Context, block Value, self Value, args []Value, kw Value) (Value, error) {
	if block.Kind() == KindObject && block.obj != nil {
		if data, ok := block.obj.data.(*blockData); ok {
			copyData := *data
			copyData.self = self
			if target, ok := s.asClass(self); ok {
				copyData.targetClass = target
			} else if self.obj != nil {
				if self.obj.singleton == nil {
					original := s.classOf(self)
					self.obj.singleton = newClass("#<Class>", original)
					s.ensureClassObject(self.obj.singleton)
				}
				copyData.targetClass = self.obj.singleton
			}
			return s.executeFrame(ctx, copyData.irep, self, copyData.targetClass, args, kw, Nil, copyData.env, copyData.mid, copyData.owner, copyData.strict)
		}
	}
	return s.callBlock(ctx, block, args, kw)
}

// callBlock1 invokes a block with one argument without heap-allocating an
// argument slice for the common case.
// callHashPairBlock invokes a block for a Hash entry. Hash iteration yields
// a single [key, value] pair; a block with |k, v| destructures it.
func (s *State) callHashPairBlock(ctx context.Context, block Value, entry hashEntry, kw Value) (Value, error) {
	var buffer [1]Value
	buffer[0] = ArrayValue([]Value{entry.key, entry.value})
	return s.callBlock(ctx, block, buffer[:], kw)
}

func (s *State) callBlock1(ctx context.Context, block Value, arg Value, kw Value) (Value, error) {
	var buffer [1]Value
	buffer[0] = arg
	return s.callBlock(ctx, block, buffer[:], kw)
}

// callBlock2 invokes a block with two arguments without heap-allocating an
// argument slice for the common case.
func (s *State) callBlock2(ctx context.Context, block Value, first, second Value, kw Value) (Value, error) {
	var buffer [2]Value
	buffer[0] = first
	buffer[1] = second
	return s.callBlock(ctx, block, buffer[:], kw)
}

func (s *State) callBlock(ctx context.Context, block Value, args []Value, kw Value) (Value, error) {
	if block.Kind() == KindSymbol {
		proc, err := s.call(ctx, block, "to_proc", nil, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		return s.callBlock(ctx, proc, args, kw)
	}
	if block.Kind() != KindObject || block.obj == nil {
		return Nil, fmt.Errorf("mrbvm: no block given")
	}
	if data, ok := block.obj.data.(*nativeBlockData); ok {
		// Native blocks may retain the slice; callers such as callBlock1
		// pass stack-backed buffers.
		return data.fn(ctx, append([]Value(nil), args...))
	}
	data, ok := block.obj.data.(*blockData)
	if !ok {
		return Nil, fmt.Errorf("mrbvm: value is not a block")
	}
	return s.executeFrame(ctx, data.irep, data.self, data.targetClass, args, kw, Nil, data.env, data.mid, data.owner, data.strict)
}

func (s *State) kernelPrint(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	for _, arg := range args {
		fmt.Fprint(s.stdout, arg.ToS())
		if s.output != nil && s.output.exceeded {
			return Nil, errOutputLimit
		}
	}
	return Nil, nil
}

func (s *State) kernelPuts(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		fmt.Fprintln(s.stdout)
		return Nil, nil
	}
	for _, arg := range args {
		if arg.Kind() == KindObject && arg.obj != nil {
			if value, ok := arg.obj.data.(*arrayData); ok {
				for _, item := range value.values {
					fmt.Fprintln(s.stdout, item.ToS())
				}
				continue
			}
		}
		fmt.Fprintln(s.stdout, arg.ToS())
	}
	return Nil, nil
}

func (s *State) newVisibilityError(kind, name string, recv Value) Value {
	exception := newException(s.NoMethodErrorClass, fmt.Sprintf("%s method '%s' called for %s", kind, name, s.classOf(recv).name))
	if data, ok := exception.obj.data.(*exceptionData); ok {
		data.name = SymbolValue(name)
		data.receiver = recv
		data.receiverSet = true
	}
	return exception
}

func (s *State) privateCallError(recv, self Value, name string) error {
	if s.privateMethodCount == 0 && s.protectedCount == 0 {
		return nil
	}
	if self.obj != nil && recv.obj == self.obj {
		return nil
	}
	callerClass := s.classOf(self)
	for current := s.classOf(recv); current != nil; current = current.super {
		if current.privateMethods[name] {
			return &unwindSignal{kind: unwindException, value: s.newVisibilityError("private", name, recv)}
		}
		if current.protectedMethods[name] {
			if callerClass == current || s.classIsSubclass(callerClass, current) || s.classIsSubclass(current, callerClass) {
				return nil
			}
			return &unwindSignal{kind: unwindException, value: s.newVisibilityError("protected", name, recv)}
		}
	}
	return nil
}

func (s *State) ensureMutable(value Value) error {
	if value.obj != nil && value.obj.frozen {
		return &unwindSignal{kind: unwindException, value: newException(s.FrozenErrorClass, "can't modify frozen object")}
	}
	return nil
}

func (s *State) usesBuiltinMethod(recv Value, name string, builtin *method) bool {
	if builtin == nil {
		return false
	}
	m, ok := s.classOf(recv).lookup(name)
	return ok && m == builtin
}

func (s *State) indexOp(ctx context.Context, recv, index Value) (Value, error) {
	if s.usesBuiltinMethod(recv, "[]", s.arrayIndexBuiltin) {
		return s.index(ctx, recv, index)
	}
	return s.call(ctx, recv, "[]", []Value{index}, Nil, Nil)
}

func (s *State) setIndexOp(ctx context.Context, recv, index, value Value) (Value, error) {
	if s.usesBuiltinMethod(recv, "[]=", s.arraySetBuiltin) {
		return s.setIndex(ctx, recv, index, value)
	}
	return s.call(ctx, recv, "[]=", []Value{index, value}, Nil, Nil)
}
