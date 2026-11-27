package mrbvm

import (
	"context"
	"fmt"
)

type fiberStatus uint8

const (
	fiberCreated fiberStatus = iota
	fiberRunning
	fiberResumed
	fiberSuspended
	fiberTransferred
	fiberTerminated
)

type fiberVMState struct {
	frames           []callFrame
	callDepth        int
	strictDepth      int
	currentException Value
	definedKind      string
	definedSet       bool
	definedNil       bool
	evalScopes       []*evalScope
}

type fiberEvent struct {
	values []Value
	err    error
}

type fiberRequestKind uint8

const (
	fiberRequestResume fiberRequestKind = iota
	fiberRequestTransfer
	fiberRequestYield
	fiberRequestRaise
	fiberRequestTerminate
)

type fiberSwitchRequest struct {
	kind   fiberRequestKind
	from   *fiberData
	target *fiberData
	values []Value
	err    error
}

type fiberData struct {
	block   Value
	self    Value
	ctx     context.Context
	ch      chan fiberEvent
	status  fiberStatus
	prev    *fiberData
	started bool
	// shutdownLocked reports that this fiber holds State.shutdownMu while
	// unwinding on Close.
	shutdownLocked bool
	counted        bool
	storage        Value
	vm             fiberVMState
}

type fiberContextKey struct{}

func (s *State) installFiberClass() {
	class := s.FiberClass
	class.classMethods["new"] = &method{owner: class, name: "new", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Fiber.new requires a block")}
		}
		fiberClass := class
		if requested, ok := s.asClass(self); ok {
			fiberClass = requested
		}
		storage, err := s.fiberInitialStorage(ctx, kw)
		if err != nil {
			return Nil, err
		}
		if err := s.reserveFiber(); err != nil {
			return Nil, err
		}
		data := &fiberData{block: block, ctx: ctx, ch: make(chan fiberEvent), status: fiberCreated, storage: storage, counted: true}
		value := ObjectValue(&Object{class: fiberClass, data: data})
		data.self = value
		return value, nil
	}}
	class.classMethods["yield"] = &method{owner: class, name: "yield", native: s.fiberYieldMethod}
	class.classMethods["current"] = &method{owner: class, name: "current", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data := fiberFromContext(ctx); data != nil {
			return data.self, nil
		}
		return s.RootFiber, nil
	}}
	class.define("resume", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.fiberSwitch(ctx, self, args, fiberRequestResume, Nil)
	})
	class.define("transfer", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.fiberSwitch(ctx, self, args, fiberRequestTransfer, Nil)
	})
	class.define("raise", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		exception, err := s.fiberExceptionArg(ctx, args)
		if err != nil {
			return Nil, err
		}
		return s.fiberSwitch(ctx, self, nil, fiberRequestRaise, exception)
	})
	class.define("alive?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		return BoolValue(ok && data.status != fiberTerminated), nil
	})
	class.define("dead?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		return BoolValue(!ok || data.status == fiberTerminated), nil
	})
	class.define("==", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		return BoolValue(identicalValue(self, args[0])), nil
	})
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		return BoolValue(identicalValue(self, args[0])), nil
	})
	class.classMethods["[]"] = &method{owner: class, name: "[]", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		key, err := fiberStorageKey(s, args[0])
		if err != nil {
			return Nil, err
		}
		current := s.fiberCurrent(ctx)
		if current == nil || current.storage.IsNil() {
			return Nil, nil
		}
		data, _ := hashDataOf(current.storage)
		index, found, err := s.hashLookupKey(ctx, data, SymbolValue(key))
		if err != nil {
			return Nil, err
		}
		if !found {
			return Nil, nil
		}
		return data.entries[index].value, nil
	}}
	class.classMethods["[]="] = &method{owner: class, name: "[]=", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return Nil, argumentCountError(s, len(args), "2")
		}
		key, err := fiberStorageKey(s, args[0])
		if err != nil {
			return Nil, err
		}
		current := s.fiberCurrent(ctx)
		if current.storage.IsNil() {
			current.storage = HashValue(nil)
		}
		data, _ := hashDataOf(current.storage)
		if args[1].IsNil() {
			for i, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == key {
					data.entries = append(data.entries[:i], data.entries[i+1:]...)
					break
				}
			}
			return Nil, nil
		}
		_, err = s.setIndex(ctx, current.storage, SymbolValue(key), args[1])
		if err != nil {
			return Nil, err
		}
		return args[1], nil
	}}
	class.define("storage", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a fiber")}
		}
		if s.fiberCurrent(ctx) != data {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "storage is only accessible from the fiber itself")}
		}
		return fiberStorageCopy(data.storage), nil
	})
	class.define("storage=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		if !ok || len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		if s.fiberCurrent(ctx) != data {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "storage is only accessible from the fiber itself")}
		}
		copyValue, err := fiberStorageFromValue(s, args[0])
		if err != nil {
			return Nil, err
		}
		data.storage = copyValue
		return args[0], nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		status := "terminated"
		if ok {
			switch data.status {
			case fiberCreated:
				status = "created"
			case fiberSuspended, fiberTransferred:
				status = "suspended"
			default:
				status = "resumed"
			}
		}
		return StringValue(fmt.Sprintf("#<Fiber:0x%x (%s)>", objectIdentityID(self), status)), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.call(ctx, self, "inspect", nil, Nil, Nil)
	})
}

func fiberOf(value Value) (*fiberData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*fiberData)
	return data, ok
}

func fiberFromContext(ctx context.Context) *fiberData {
	if data, ok := ctx.Value(fiberContextKey{}).(*fiberData); ok {
		return data
	}
	return nil
}

func (s *State) fiberCurrent(ctx context.Context) *fiberData {
	if data := fiberFromContext(ctx); data != nil {
		return data
	}
	return s.rootFiber
}

func (s *State) saveFiberVM(data *fiberData) {
	data.vm.frames = s.frames
	data.vm.callDepth = s.callDepth
	data.vm.strictDepth = s.strictDepth
	data.vm.currentException = s.currentException
	data.vm.definedKind = s.definedKind
	data.vm.definedSet = s.definedSet
	data.vm.definedNil = s.definedNil
	data.vm.evalScopes = s.evalScopes
}

func (s *State) restoreFiberVM(data *fiberData) {
	s.frames = data.vm.frames
	s.callDepth = data.vm.callDepth
	s.strictDepth = data.vm.strictDepth
	s.currentException = data.vm.currentException
	s.definedKind = data.vm.definedKind
	s.definedSet = data.vm.definedSet
	s.definedNil = data.vm.definedNil
	s.evalScopes = data.vm.evalScopes
}

func fiberResult(values []Value) Value {
	switch len(values) {
	case 0:
		return Nil
	case 1:
		return values[0]
	default:
		return ArrayValue(append([]Value(nil), values...))
	}
}

func (s *State) lockFiberShutdown(data *fiberData) {
	if data == nil || data == s.rootFiber || data.shutdownLocked {
		return
	}
	s.shutdownMu.Lock()
	data.shutdownLocked = true
}

func (s *State) fiberSwitch(ctx context.Context, self Value, args []Value, kind fiberRequestKind, exception Value) (Value, error) {
	if s.isClosed() {
		return Nil, ErrStateClosed
	}
	target, ok := fiberOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a fiber")}
	}
	current := s.fiberCurrent(ctx)
	if kind == fiberRequestTransfer && s.fiberCBoundary > 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "can't cross C function boundary")}
	}
	if kind == fiberRequestTransfer && target == current {
		return fiberResult(args), nil
	}
	if kind == fiberRequestResume && target == current {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "attempt to resume the current fiber")}
	}
	if kind == fiberRequestTransfer && target == current {
		return fiberResult(args), nil
	}
	s.saveFiberVM(current)
	request := fiberSwitchRequest{kind: kind, from: current, target: target, values: append([]Value(nil), args...)}
	if kind == fiberRequestRaise {
		request.err = &unwindSignal{kind: unwindException, value: exception}
	}
	select {
	case s.fiberSwitchCh <- request:
	case <-s.fiberStop:
		s.lockFiberShutdown(current)
		s.restoreFiberVM(current)
		return Nil, ErrStateClosed
	}
	select {
	case event := <-current.ch:
		s.restoreFiberVM(current)
		if event.err != nil {
			return Nil, event.err
		}
		return fiberResult(event.values), nil
	case <-s.closed:
		s.lockFiberShutdown(current)
		s.restoreFiberVM(current)
		return Nil, ErrStateClosed
	}
}

func (s *State) fiberYieldMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if s.isClosed() {
		return Nil, ErrStateClosed
	}
	current := fiberFromContext(ctx)
	if current == nil || current == s.rootFiber {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "can't yield from root fiber")}
	}
	if current.prev == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "attempt to yield on a not resumed fiber")}
	}
	if current.prev.status == fiberTransferred {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "attempt to yield on a not resumed fiber")}
	}
	s.saveFiberVM(current)
	select {
	case s.fiberSwitchCh <- fiberSwitchRequest{kind: fiberRequestYield, from: current, target: current.prev, values: append([]Value(nil), args...)}:
	case <-s.fiberStop:
		s.lockFiberShutdown(current)
		s.restoreFiberVM(current)
		return Nil, ErrStateClosed
	}
	select {
	case event := <-current.ch:
		s.restoreFiberVM(current)
		if event.err != nil {
			return Nil, event.err
		}
		return fiberResult(event.values), nil
	case <-s.closed:
		s.lockFiberShutdown(current)
		s.restoreFiberVM(current)
		return Nil, ErrStateClosed
	}
}

func (s *State) fiberExceptionArg(ctx context.Context, args []Value) (Value, error) {
	if len(args) == 0 {
		return newException(s.RuntimeErrorClass, ""), nil
	}
	if class, ok := s.asClass(args[0]); ok {
		message := class.name
		if len(args) > 1 {
			message = args[1].ToS()
		}
		return newException(class, message), nil
	}
	if args[0].Kind() == KindObject && args[0].obj != nil {
		if _, ok := args[0].obj.data.(*exceptionData); ok {
			return args[0], nil
		}
	}
	return newException(s.RuntimeErrorClass, args[0].ToS()), nil
}

func (s *State) fiberScheduler() {
	current := s.rootFiber
	for {
		var request fiberSwitchRequest
		select {
		case <-s.fiberStop:
			return
		case request = <-s.fiberSwitchCh:
		}
		target := request.target
		from := request.from
		switch request.kind {
		case fiberRequestYield:
			from.status = fiberSuspended
			from.prev = nil
			target.status = fiberRunning
			current = target
			if !s.sendFiberEvent(target.ch, fiberEvent{values: request.values}) {
				return
			}
		case fiberRequestResume, fiberRequestRaise:
			status := target.status
			var failure string
			if target == from {
				failure = "attempt to resume the current fiber"
			} else {
				switch status {
				case fiberTransferred:
					failure = "resuming transferred fiber"
				case fiberRunning, fiberResumed:
					failure = "double resume"
				case fiberTerminated:
					failure = "resuming dead fiber"
				}
			}
			if request.kind == fiberRequestRaise && status == fiberCreated {
				failure = "cannot raise into an unstarted fiber"
			}
			if failure != "" {
				if !s.sendFiberEvent(from.ch, fiberEvent{err: &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, failure)}}) {
					return
				}
				continue
			}
			from.status = fiberResumed
			target.prev = from
			target.status = fiberRunning
			current = target
			if !target.started {
				target.started = true
				s.startFiber(target, request.values)
			} else {
				if !s.sendFiberEvent(target.ch, fiberEvent{values: request.values, err: request.err}) {
					return
				}
			}
		case fiberRequestTransfer:
			status := target.status
			if target != s.rootFiber {
				var failure string
				switch status {
				case fiberResumed:
					failure = "attempt to transfer to a resuming fiber"
				case fiberRunning:
					failure = "double resume"
				case fiberTerminated:
					failure = "resuming dead fiber"
				}
				if failure != "" {
					if !s.sendFiberEvent(from.ch, fiberEvent{err: &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, failure)}}) {
						return
					}
					continue
				}
				target.prev = nil
			}
			from.status = fiberTransferred
			target.status = fiberRunning
			current = target
			if target != s.rootFiber && !target.started {
				target.started = true
				s.startFiber(target, request.values)
			} else {
				if !s.sendFiberEvent(target.ch, fiberEvent{values: request.values}) {
					return
				}
			}
		case fiberRequestTerminate:
			if target == nil {
				target = s.rootFiber
			}
			target.status = fiberRunning
			current = target
			if !s.sendFiberEvent(target.ch, fiberEvent{values: request.values, err: request.err}) {
				return
			}
		}
		_ = current
	}
}

func (s *State) sendFiberEvent(ch chan fiberEvent, event fiberEvent) bool {
	select {
	case ch <- event:
		return true
	case <-s.fiberStop:
		return false
	}
}

func (s *State) startFiber(data *fiberData, args []Value) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if data.counted {
				data.counted = false
				s.releaseFiber()
			}
		}()
		s.fiberRun(data, args, nil)
	}()
}

func (s *State) fiberRun(data *fiberData, args []Value, _ *fiberEvent) {
	defer func() {
		if data.shutdownLocked {
			data.shutdownLocked = false
			s.shutdownMu.Unlock()
		}
	}()
	defer func() {
		if panicValue := recover(); panicValue != nil {
			s.fiberPanic(data, panicValue)
		}
	}()
	if s.isClosed() {
		return
	}
	s.restoreFiberVM(data)
	ctx := context.WithValue(data.ctx, fiberContextKey{}, data)
	result, err := s.callBlock(ctx, data.block, args, Nil)
	if signal, ok := err.(*unwindSignal); ok && (signal.kind == unwindBreak || signal.kind == unwindReturn) {
		result = signal.value
		err = nil
	}
	s.saveFiberVM(data)
	data.status = fiberTerminated
	if data.counted {
		data.counted = false
		s.releaseFiber()
	}
	target := data.prev
	if target == nil {
		target = s.rootFiber
	}
	data.prev = nil
	select {
	case s.fiberSwitchCh <- fiberSwitchRequest{kind: fiberRequestTerminate, from: data, target: target, values: []Value{result}, err: err}:
	case <-s.fiberStop:
	}
}

func (s *State) fiberPanic(data *fiberData, panicValue any) {
	err := s.recordPanic(panicValue)
	data.status = fiberTerminated
	if data.counted {
		data.counted = false
		s.releaseFiber()
	}
	target := data.prev
	if target == nil {
		target = s.rootFiber
	}
	data.prev = nil
	select {
	case s.fiberSwitchCh <- fiberSwitchRequest{kind: fiberRequestTerminate, from: data, target: target, values: []Value{Nil}, err: err}:
	case <-s.fiberStop:
	}
}

func fiberStorageKey(s *State, value Value) (string, error) {
	if value.Kind() != KindSymbol && value.Kind() != KindString {
		return "", &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "storage key must be a Symbol or String")}
	}
	return value.String(), nil
}

func fiberStorageFromValue(s *State, value Value) (Value, error) {
	if value.IsNil() {
		return Nil, nil
	}
	data, ok := hashDataOf(value)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "storage must be a Hash")}
	}
	entries := make([]hashEntry, len(data.entries))
	for i, entry := range data.entries {
		if entry.key.Kind() != KindSymbol {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "storage keys must be Symbols")}
		}
		entries[i] = entry
	}
	return HashValue(entries), nil
}

func fiberStorageCopy(storage Value) Value {
	if storage.IsNil() {
		return Nil
	}
	if data, ok := hashDataOf(storage); ok {
		return HashValue(append([]hashEntry(nil), data.entries...))
	}
	return Nil
}

func (s *State) fiberInitialStorage(ctx context.Context, kw Value) (Value, error) {
	if !kw.IsNil() {
		if data, ok := hashDataOf(kw); ok {
			for _, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "storage" {
					return fiberStorageFromValue(s, entry.value)
				}
				if entry.key.Kind() != KindSymbol {
					return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "keywords must be Symbols")}
				}
			}
		}
	}
	current := s.fiberCurrent(ctx)
	if current == nil || current.storage.IsNil() {
		return HashValue(nil), nil
	}
	return fiberStorageCopy(current.storage), nil
}

func (s *State) installFiberStorage() {
	class := s.ProcClass
	class.classMethods["c_tunnel"] = &method{owner: class, name: "c_tunnel", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.LocalJumpErrorClass, "no block given")}
		}
		s.fiberCBoundary++
		defer func() { s.fiberCBoundary-- }()
		return s.callBlock(ctx, block, nil, Nil)
	}}
}
