package mrbvm

import (
	"context"
	"math/big"
	"strings"
)

type stringData struct {
	// raw holds the value when no mutation buffer is active. buffer holds
	// pending appends so repeated String#<< grows amortised instead of
	// copying the whole string each time. Use str/set/appendString/len
	// rather than touching raw directly.
	raw    string
	buffer []byte
	// binary marks a string whose characters are bytes (String#b and
	// friends). The default UTF-8 reading decodes characters instead.
	binary bool
}

type arrayData struct {
	values []Value
}

type hashEntry struct {
	key   Value
	value Value
}

type hashData struct {
	entries      []hashEntry
	version      uint64
	defaultValue Value
	// index is an open-addressed table of entry indexes (-1 for empty),
	// used only for primitive keys. indexBuilt records how many entries it
	// describes so length-changing mutations can trigger a rebuild.
	index      []int32
	indexBuilt int
	// identity selects compare_by_identity semantics for object keys.
	identity    bool
	defaultProc Value
}

type hashKey struct {
	kind Kind
	i    int64
	bits uint64
	str  string
}

type rangeData struct {
	low       Value
	high      Value
	exclusive bool
}

// Object is the base representation of a heap-allocated Ruby object.
type Object struct {
	class     *Class
	singleton *Class
	data      any
	ivars     map[string]Value
	frozen    bool
}

func (o *Object) ivar(name string) Value {
	if o.ivars == nil {
		return Nil
	}
	return o.ivars[name]
}

func (o *Object) setIvar(name string, value Value) {
	if o.ivars == nil {
		o.ivars = make(map[string]Value)
	}
	o.ivars[name] = value
}

// Class is a minimal mruby class or module.
type Class struct {
	name                  string
	super                 *Class
	lexicalOuter          *Class
	methods               map[string]*method
	classMethods          map[string]*method
	constants             map[string]Value
	autoloads             map[string]string
	classVars             map[string]Value
	object                *Object
	singletonOf           *Class
	singletonOwner        Value
	singleton             *Class
	isModule              bool
	moduleFunction        bool
	defaultVisibility     string
	privateClassMethods   map[string]bool
	protectedClassMethods map[string]bool
	undefMethods          map[string]bool
	isSingleton           bool
	privateConstants      map[string]bool
	deprecatedConstants   map[string]bool
	included              []*Class
	prepended             []*Class
	subclasses            []*Class
	isStruct              bool
	isData                bool
	structMembers         []string
	structKeywordInit     Value
	structKeywordInitSet  bool
	temporaryName         bool
	permanentName         bool
	privateMethods        map[string]bool
	protectedMethods      map[string]bool
	state                 *State
}

type method struct {
	notImplemented bool
	body           *blockData
	originalName   string
	owner          *Class
	cref           *Class
	name           string
	native         NativeFunc
	irep           *Irep
	builtin        bool
}

// NativeFunc is a Go function exposed as a Ruby method.
//
// The host API will grow a more ergonomic wrapper later; this is the minimal
// internal calling convention used by the VM.
type NativeFunc func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error)

type environment struct {
	parent      *environment
	regs        []Value
	block       Value
	methodFrame bool
	// pinned marks an environment a Binding reaches. The frame pool must not
	// recycle it when its frame returns.
	pinned bool
	names  []string
}

type blockData struct {
	irep        *Irep
	env         *environment
	self        Value
	targetClass *Class
	strict      bool
	mid         string
	owner       *Class
	visibility  string
	cref        *Class
}

// exceptionData is the payload of a Ruby exception object.
type exceptionData struct {
	message        string
	backtrace      []string
	backtraceValue Value
	cause          Value
	name           Value
	receiver       Value
	receiverSet    bool
	args           Value
}

type bigIntData struct {
	value *big.Int
}

// unwindKind identifies a non-local control-flow signal.
type unwindKind int

const (
	unwindException unwindKind = iota
	unwindReturn
	unwindBreak
	unwindJump
)

// unwindSignal travels through Go error returns while a rescue or ensure
// handler catches it. Carrying the signal as a value lets OP_EXCEPT and
// OP_RAISEIF reproduce mruby's mrb->exc based unwinding.
type unwindSignal struct {
	kind       unwindKind
	value      Value
	target     *environment
	jumpTarget int
}

func (e *unwindSignal) Error() string {
	switch e.kind {
	case unwindException:
		text := "unhandled exception: " + e.value.Inspect()
		if e.value.obj != nil {
			if data, ok := e.value.obj.data.(*exceptionData); ok {
				for _, line := range data.backtrace {
					text += "\n\tfrom " + line
				}
			}
		}
		return text
	case unwindReturn:
		return "return"
	case unwindBreak:
		return "break"
	default:
		return "jump"
	}
}

// signalData wraps a control-flow signal so it can live in a register while
// an ensure body runs.
type signalData struct {
	signal *unwindSignal
}

type enumeratorData struct {
	receiver  Value
	method    string
	args      []Value
	generator Value
	produce   func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error)
	collect   func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error)
	lazy      *enumeratorLazy
	result    Value
}

type enumeratorLazy struct {
	resume     chan enumeratorResume
	out        chan enumeratorOut
	started    bool
	done       bool
	pending    []Value
	hasPending bool
	feedSet    bool
	feed       Value
}

type enumeratorResume struct {
	values []Value
	cancel bool
}

type enumeratorOut struct {
	values []Value
	done   bool
	err    error
}

type yielderData struct {
	block Value
	fn    func(ctx context.Context, args []Value) (Value, error)
}

type generatorData struct {
	block Value
}

// nativeBlockData lets Go code act as a Ruby block. Enumerable uses it to
// pass a collector block to an arbitrary object's each method.
type nativeBlockData struct {
	fn       func(ctx context.Context, args []Value) (Value, error)
	fnFull   func(ctx context.Context, args []Value, kw Value, block Value) (Value, error)
	strict   bool
	arity    int
	hasArity bool
}

type inlineExceptionObject struct {
	obj  Object
	data exceptionData
}

func newException(class *Class, message string) Value {
	holder := &inlineExceptionObject{data: exceptionData{message: message, cause: Nil}}
	holder.obj.class = class
	holder.obj.data = &holder.data
	return ObjectValue(&holder.obj)
}

func exceptionMessage(v Value) string {
	if v.Kind() == KindObject && v.obj != nil {
		if data, ok := v.obj.data.(*exceptionData); ok {
			return data.message
		}
	}
	return v.ToS()
}

func signalValue(sig *unwindSignal) Value {
	return ObjectValue(&Object{data: &signalData{signal: sig}})
}

func asSignal(v Value) (*unwindSignal, bool) {
	if v.Kind() == KindObject && v.obj != nil {
		if data, ok := v.obj.data.(*signalData); ok {
			return data.signal, true
		}
	}
	return nil, false
}

func newClass(name string, super *Class) *Class {
	return &Class{
		name:                  name,
		permanentName:         name != "" && !strings.HasPrefix(name, "#<"),
		super:                 super,
		methods:               make(map[string]*method),
		classMethods:          make(map[string]*method),
		constants:             make(map[string]Value),
		autoloads:             make(map[string]string),
		classVars:             make(map[string]Value),
		privateMethods:        make(map[string]bool),
		protectedMethods:      make(map[string]bool),
		privateClassMethods:   make(map[string]bool),
		protectedClassMethods: make(map[string]bool),
		undefMethods:          make(map[string]bool),
		privateConstants:      make(map[string]bool),
		deprecatedConstants:   make(map[string]bool),
	}
}

// classAncestors returns the method lookup order: prepended modules, the
// class itself, included modules (last included first), then the superclass.
func classAncestors(c *Class) []*Class {
	result := make([]*Class, 0, 8)
	path := make(map[*Class]bool)
	var visit func(*Class)
	visit = func(current *Class) {
		if current == nil || path[current] {
			return
		}
		path[current] = true
		defer delete(path, current)
		for i := len(current.prepended) - 1; i >= 0; i-- {
			visit(current.prepended[i])
		}
		result = append(result, current)
		for i := len(current.included) - 1; i >= 0; i-- {
			visit(current.included[i])
		}
		visit(current.super)
	}
	visit(c)
	return result
}

func (c *Class) noteDefinition(name string) {
	if c.state != nil {
		c.state.noteOperatorDefinition(name)
	}
}

func (c *Class) lookup(name string) (*method, bool) {
	for _, current := range classAncestors(c) {
		if current.undefMethods[name] {
			return nil, false
		}
		if m, ok := current.methods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

func (c *Class) applyVisibility(name string) {
	if c.moduleFunction {
		if !c.privateMethods[name] {
			c.privateMethods[name] = true
			if c.state != nil {
				c.state.privateMethodCount++
			}
		}
		if m, ok := c.methods[name]; ok {
			c.classMethods[name] = m
		}
	}
	if c.defaultVisibility == "" || c.defaultVisibility == "public" {
		return
	}
	if c.defaultVisibility == "private" {
		if !c.privateMethods[name] {
			c.privateMethods[name] = true
			if c.state != nil {
				c.state.privateMethodCount++
			}
		}
		return
	}
	if c.defaultVisibility == "protected" {
		if !c.protectedMethods[name] {
			c.protectedMethods[name] = true
			if c.state != nil {
				c.state.protectedCount++
			}
		}
	}
}

func (c *Class) define(name string, fn NativeFunc) {
	if c.singletonOf != nil {
		c.singletonOf.classMethods[name] = &method{owner: c, name: name, native: fn}
		if c.defaultVisibility == "private" {
			if !c.singletonOf.privateClassMethods[name] && c.state != nil {
				c.state.classPrivateCount++
			}
			c.singletonOf.privateClassMethods[name] = true
		} else if c.defaultVisibility == "protected" {
			c.singletonOf.protectedClassMethods[name] = true
		} else if c.defaultVisibility == "public" {
			delete(c.singletonOf.privateClassMethods, name)
			delete(c.singletonOf.protectedClassMethods, name)
		}
		c.noteDefinition(name)
		return
	}
	c.methods[name] = &method{owner: c, name: name, native: fn}
	delete(c.undefMethods, name)
	c.applyVisibility(name)
	c.noteDefinition(name)
}

func (c *Class) defineIrep(name string, irep *Irep) {
	if c.singletonOf != nil {
		c.singletonOf.classMethods[name] = &method{owner: c, name: name, irep: irep}
		if c.defaultVisibility == "private" {
			if !c.singletonOf.privateClassMethods[name] && c.state != nil {
				c.state.classPrivateCount++
			}
			c.singletonOf.privateClassMethods[name] = true
		} else if c.defaultVisibility == "protected" {
			c.singletonOf.protectedClassMethods[name] = true
		} else if c.defaultVisibility == "public" {
			delete(c.singletonOf.privateClassMethods, name)
			delete(c.singletonOf.protectedClassMethods, name)
		}
		c.noteDefinition(name)
		return
	}
	m := &method{owner: c, name: name, irep: irep}
	c.methods[name] = m
	c.applyVisibility(name)
	if c.moduleFunction {
		c.classMethods[name] = m
		if !c.privateMethods[name] {
			c.privateMethods[name] = true
			if c.state != nil {
				c.state.privateMethodCount++
			}
		}
	}
	c.noteDefinition(name)
}

func (c *Class) defineClassIrep(name string, irep *Irep) {
	c.classMethods[name] = &method{owner: c, name: name, irep: irep}
	c.noteDefinition(name)
}

func (c *Class) lookupClassMethod(name string) (*method, bool) {
	for current := c; current != nil; current = current.super {
		if m, ok := current.classMethods[name]; ok {
			return m, true
		}
	}
	return nil, false
}

func (s *State) ensureClassObject(c *Class) *Object {
	if c.state == nil {
		c.state = s
	}
	if c.object == nil {
		c.object = &Object{class: s.ClassClass, data: c}
	}
	return c.object
}

func (s *State) classValue(c *Class) Value {
	return ObjectValue(s.ensureClassObject(c))
}

func (s *State) asClass(v Value) (*Class, bool) {
	if v.Kind() == KindObject && v.obj != nil {
		c, ok := v.obj.data.(*Class)
		return c, ok
	}
	return nil, false
}

func (s *State) classOf(v Value) *Class {
	if v.obj != nil && v.obj.singleton != nil {
		return v.obj.singleton
	}
	if v.kind == KindObject && v.obj != nil {
		if c, ok := v.obj.data.(*Class); ok && c.isModule {
			return s.ModuleClass
		}
		if v.obj.class != nil {
			return v.obj.class
		}
		switch v.obj.data.(type) {
		case *arrayData:
			return s.ArrayClass
		case *hashData:
			return s.HashClass
		case *rangeData:
			return s.RangeClass
		case *Class:
			return s.ClassClass
		case *blockData:
			return s.ProcClass
		case *setData:
			return s.SetClass
		case *structData:
			return s.StructClass
		case *enumeratorData:
			return s.EnumeratorClass
		case *timeData:
			return s.TimeClass
		case *fileData:
			return s.FileClass
		case *regexpData:
			return s.RegexpClass
		case *matchData:
			return s.MatchDataClass
		case *fiberData:
			return s.FiberClass
		}
	}
	switch v.kind {
	case KindNil:
		if singleton := s.immediateSingletons[KindNil]; singleton != nil {
			return singleton
		}
		return s.NilClass
	case KindTrue:
		if singleton := s.immediateSingletons[KindTrue]; singleton != nil {
			return singleton
		}
		return s.TrueClass
	case KindFalse:
		if singleton := s.immediateSingletons[KindFalse]; singleton != nil {
			return singleton
		}
		return s.FalseClass
	case KindInt, KindBigInt:
		return s.IntegerClass
	case KindFloat:
		return s.FloatClass
	case KindString, KindSymbol:
		if v.obj != nil && v.obj.class != nil {
			return v.obj.class
		}
		if v.kind == KindSymbol {
			return s.SymbolClass
		}
		return s.StringClass
	default:
		return s.ObjectClass
	}
}

// str returns the string payload, materialising any pending append buffer.
func (d *stringData) str() string {
	if d.buffer != nil {
		d.raw = string(d.buffer)
		d.buffer = nil
	}
	return d.raw
}

// set replaces the string payload and drops any pending append buffer.
func (d *stringData) set(value string) {
	d.raw = value
	d.buffer = nil
}

// appendString appends to the string, growing a byte buffer instead of
// copying the whole payload on every call.
func (d *stringData) appendString(value string) {
	if value == "" {
		return
	}
	if d.buffer == nil {
		buffer := make([]byte, len(d.raw), len(d.raw)+len(value))
		copy(buffer, d.raw)
		d.raw = ""
		d.buffer = buffer
	}
	d.buffer = append(d.buffer, value...)
}

// len returns the current byte length without materialising the buffer.
func (d *stringData) len() int {
	if d.buffer != nil {
		return len(d.buffer)
	}
	return len(d.raw)
}
