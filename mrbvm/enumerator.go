package mrbvm

import "context"

// installEnumeratorLibrary provides Enumerator and its external iteration
// protocol. External iteration runs the source in a goroutine that suspends at
// each yield, so next/peek work for any Ruby method, not just materialised
// collections.
func (s *State) installEnumeratorLibrary() {
	class := s.EnumeratorClass
	class.define("each", s.enumeratorEachMethod)
	class.define("next", s.enumeratorNextMethod)
	class.define("next_values", s.enumeratorNextValuesMethod)
	class.define("peek", s.enumeratorPeekMethod)
	class.define("peek_values", s.enumeratorPeekValuesMethod)
	class.define("feed", s.enumeratorFeedMethod)
	class.define("rewind", s.enumeratorRewindMethod)
	class.define("size", s.enumeratorSizeMethod)
	class.define("with_index", s.enumeratorWithIndexMethod)
	class.define("each_with_index", s.enumeratorWithIndexMethod)
	class.define("with_object", s.enumeratorWithObjectMethod)
	class.define("inspect", s.enumeratorInspectMethod)
	class.define("to_s", s.enumeratorInspectMethod)
	class.define("dup", s.enumeratorDupMethod)
	class.define("initialize_copy", s.enumeratorDupMethod)

	class.classMethods["new"] = &method{owner: class, name: "new", native: s.enumeratorNewMethod}
	class.classMethods["produce"] = &method{owner: class, name: "produce", native: s.enumeratorProduceMethod}

	yielder := newClass("Enumerator::Yielder", s.ObjectClass)
	s.ensureClassObject(yielder)
	s.EnumeratorClass.constants["Yielder"] = s.classValue(yielder)
	yielder.classMethods["new"] = &method{owner: yielder, name: "new", native: s.yielderNewMethod}
	yielder.define("yield", s.yielderYieldMethod)
	yielder.define("<<", s.yielderPushMethod)
	yielder.define("call", s.yielderYieldMethod)

	generator := newClass("Enumerator::Generator", s.ObjectClass)
	s.ensureClassObject(generator)
	s.EnumeratorClass.constants["Generator"] = s.classValue(generator)
	generator.classMethods["new"] = &method{owner: generator, name: "new", native: s.generatorNewMethod}
	generator.define("each", s.generatorEachMethod)
	generator.define("dup", s.generatorDupMethod)
	generator.define("initialize_copy", s.generatorDupMethod)

	s.ObjectClass.define("to_enum", s.objectToEnumMethod)
	s.ObjectClass.define("enum_for", s.objectToEnumMethod)
}

func (s *State) newEnumerator(collect func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error)) Value {
	return ObjectValue(&Object{class: s.EnumeratorClass, data: &enumeratorData{collect: collect}})
}

func (s *State) toEnum(receiver Value, method string, args []Value) Value {
	return ObjectValue(&Object{class: s.EnumeratorClass, data: &enumeratorData{
		receiver: receiver,
		method:   method,
		args:     append([]Value(nil), args...),
	}})
}

func enumeratorOf(value Value) (*enumeratorData, bool) {
	if value.Kind() == KindObject && value.obj != nil {
		data, ok := value.obj.data.(*enumeratorData)
		return data, ok
	}
	return nil, false
}

func yielderOf(value Value) (*yielderData, bool) {
	if value.Kind() == KindObject && value.obj != nil {
		data, ok := value.obj.data.(*yielderData)
		return data, ok
	}
	return nil, false
}

func generatorOf(value Value) (*generatorData, bool) {
	if value.Kind() == KindObject && value.obj != nil {
		data, ok := value.obj.data.(*generatorData)
		return data, ok
	}
	return nil, false
}

func (s *State) isStopIteration(err error) bool {
	sig, ok := err.(*unwindSignal)
	if !ok || sig.kind != unwindException || sig.value.obj == nil {
		return false
	}
	class := s.classOf(sig.value)
	return class == s.StopIterationClass || s.classIsSubclass(class, s.StopIterationClass)
}

func (s *State) enumeratorRun(ctx context.Context, data *enumeratorData, yield func([]Value) (Value, error)) (Value, error) {
	switch {
	case data.collect != nil:
		return data.collect(ctx, yield)
	case data.produce != nil:
		return data.produce(ctx, yield)
	case !data.generator.IsNil():
		yielder := s.newYielder(Nil, func(ctx context.Context, args []Value) (Value, error) {
			return yield(args)
		})
		result, err := s.callBlock(ctx, data.generator, []Value{yielder}, Nil)
		data.result = result
		return result, err
	case data.method != "":
		block := nativeBlockValue(s.ProcClass, func(ctx context.Context, raw []Value) (Value, error) {
			return yield(raw)
		})
		result, err := s.call(ctx, data.receiver, data.method, data.args, Nil, block)
		data.result = result
		return result, err
	}
	return Nil, nil
}

func (s *State) newYielder(block Value, fn func(ctx context.Context, args []Value) (Value, error)) Value {
	return ObjectValue(&Object{class: s.yielderClass(), data: &yielderData{block: block, fn: fn}})
}

func (s *State) yielderClass() *Class {
	value := s.EnumeratorClass.constants["Yielder"]
	if class, ok := s.asClass(value); ok {
		return class
	}
	return s.ObjectClass
}

func (s *State) generatorClass() *Class {
	value := s.EnumeratorClass.constants["Generator"]
	if class, ok := s.asClass(value); ok {
		return class
	}
	return s.ObjectClass
}

func (s *State) objectToEnumMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	method := "each"
	rest := args
	if len(args) > 0 {
		method = args[0].String()
		rest = args[1:]
	}
	return s.toEnum(self, method, rest), nil
}

func (s *State) enumeratorNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Enumerator.new requires a block")}
	}
	class, _ := s.asClass(self)
	if class == nil {
		class = s.EnumeratorClass
	}
	return ObjectValue(&Object{class: class, data: &enumeratorData{generator: block}}), nil
}

func (s *State) enumeratorProduceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Enumerator.produce requires a block")}
	}
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	hasInitial := len(args) == 1
	initial := Nil
	if hasInitial {
		initial = args[0]
	}
	produce := func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error) {
		current := initial
		if !hasInitial {
			var err error
			value, err := s.callBlock(ctx, block, []Value{current}, Nil)
			if err != nil {
				if s.isStopIteration(err) {
					return Nil, nil
				}
				return Nil, err
			}
			current = value
		}
		for {
			if _, err := yield([]Value{current}); err != nil {
				return Nil, err
			}
			next, err := s.callBlock(ctx, block, []Value{current}, Nil)
			if err != nil {
				if s.isStopIteration(err) {
					return Nil, nil
				}
				return Nil, err
			}
			current = next
		}
	}
	return ObjectValue(&Object{class: s.EnumeratorClass, data: &enumeratorData{produce: produce}}), nil
}

func (s *State) enumeratorEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if block.IsNil() {
		if len(args) > 0 && data.method != "" && data.collect == nil && data.produce == nil && data.generator.IsNil() {
			combined := append(append([]Value(nil), data.args...), args...)
			return s.toEnum(data.receiver, data.method, combined), nil
		}
		return self, nil
	}
	callArgs := args
	if data.method != "" && data.collect == nil && data.produce == nil && data.generator.IsNil() && len(args) > 0 {
		original := data.args
		data.args = append(append([]Value(nil), data.args...), args...)
		defer func() { data.args = original }()
		callArgs = nil
	}
	_ = callArgs
	result, err := s.enumeratorRun(ctx, data, func(raw []Value) (Value, error) {
		return s.callBlock(ctx, block, raw, Nil)
	})
	if err != nil {
		if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
			return br.value, nil
		}
		if s.isStopIteration(err) {
			return Nil, nil
		}
		return Nil, err
	}
	return result, nil
}

func (s *State) enumeratorEnsureLazy(ctx context.Context, data *enumeratorData) *enumeratorLazy {
	if data.lazy != nil {
		return data.lazy
	}
	lazy := &enumeratorLazy{
		resume: make(chan enumeratorResume),
		out:    make(chan enumeratorOut),
		parked: make(chan struct{}),
		exited: make(chan struct{}),
	}
	data.lazy = lazy
	return lazy
}

func (s *State) enumeratorStartLazy(ctx context.Context, data *enumeratorData, lazy *enumeratorLazy) {
	lazy.started = true
	s.wg.Add(1)
	go func() {
		holdingShutdown := false
		counted := true
		defer func() {
			if counted {
				s.releaseEnumerator()
			}
		}()
		defer func() {
			if holdingShutdown {
				s.shutdownMu.Unlock()
			}
		}()
		defer s.wg.Done()
		defer s.releaseEnumerator()
		defer close(lazy.exited)
		sentDone := false
		lockShutdown := func() {
			if !holdingShutdown {
				s.shutdownMu.Lock()
				holdingShutdown = true
			}
		}
		defer func() {
			if panicValue := recover(); panicValue != nil {
				err := s.recordPanic(panicValue)
				if !sentDone {
					select {
					case lazy.out <- enumeratorOut{done: true, err: err}:
					case <-s.closed:
					}
				}
			}
		}()
		yield := func(values []Value) (Value, error) {
			select {
			case lazy.out <- enumeratorOut{values: append([]Value(nil), values...)}:
			case <-s.closed:
				lockShutdown()
				return Nil, ErrStateClosed
			}
			// Tell the consumer that the producer is parked. The consumer
			// must not touch the State until this handshake completes: the
			// producer may still be unwinding the interpreter frame that
			// called yield.
			select {
			case lazy.parked <- struct{}{}:
			case <-s.closed:
				lockShutdown()
				return Nil, ErrStateClosed
			}
			select {
			case reply := <-lazy.resume:
				if reply.cancel {
					return Nil, errStopEnumeration
				}
				if len(reply.values) == 0 {
					return Nil, nil
				}
				return reply.values[0], nil
			case <-s.closed:
				lockShutdown()
				return Nil, ErrStateClosed
			}
		}
		result, err := s.enumeratorRun(ctx, data, yield)
		data.result = result
		if counted {
			counted = false
			s.releaseEnumerator()
		}
		select {
		case lazy.out <- enumeratorOut{done: true, err: err}:
			sentDone = true
		case <-s.closed:
		}
	}()
}

func (s *State) enumeratorNextValueList(ctx context.Context, self Value, data *enumeratorData) ([]Value, error) {
	if s.isClosed() {
		return nil, ErrStateClosed
	}
	lazy := s.enumeratorEnsureLazy(ctx, data)
	if lazy.hasPending {
		values := lazy.pending
		lazy.pending = nil
		lazy.hasPending = false
		return values, nil
	}
	if lazy.done {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.StopIterationClass, "iteration reached an end")}
	}
	if !lazy.started {
		if err := s.reserveEnumerator(); err != nil {
			return nil, err
		}
		s.enumeratorStartLazy(ctx, data, lazy)
	} else {
		message := enumeratorResume{}
		if lazy.feedSet {
			message.values = []Value{lazy.feed}
			lazy.feedSet = false
			lazy.feed = Nil
		}
		select {
		case lazy.resume <- message:
		case <-s.closed:
			return nil, ErrStateClosed
		}
	}
	var out enumeratorOut
	select {
	case out = <-lazy.out:
	case <-s.closed:
		return nil, ErrStateClosed
	}
	if out.done {
		lazy.done = true
		select {
		case <-lazy.exited:
		case <-s.closed:
			return nil, ErrStateClosed
		}
		if out.err != nil {
			if out.err == errStopEnumeration || s.isStopIteration(out.err) {
				return nil, &unwindSignal{kind: unwindException, value: newException(s.StopIterationClass, "iteration reached an end")}
			}
			return nil, out.err
		}
		return nil, &unwindSignal{kind: unwindException, value: newException(s.StopIterationClass, "iteration reached an end")}
	}
	// Wait until the producer is parked before returning. Otherwise the
	// caller could start another enumerator while this producer is still
	// unwinding its interpreter frame.
	select {
	case <-lazy.parked:
	case <-s.closed:
		return nil, ErrStateClosed
	}
	return out.values, nil
}

func (s *State) enumeratorNextMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	values, err := s.enumeratorNextValueList(ctx, self, data)
	if err != nil {
		return Nil, err
	}
	return svalueOf(values), nil
}

func (s *State) enumeratorNextValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	values, err := s.enumeratorNextValueList(ctx, self, data)
	if err != nil {
		return Nil, err
	}
	return ArrayValue(values), nil
}

func (s *State) enumeratorPeekMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	lazy := s.enumeratorEnsureLazy(ctx, data)
	if !lazy.hasPending {
		values, err := s.enumeratorNextValueList(ctx, self, data)
		if err != nil {
			return Nil, err
		}
		lazy.pending = values
		lazy.hasPending = true
	}
	return svalueOf(lazy.pending), nil
}

func (s *State) enumeratorPeekValuesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	lazy := s.enumeratorEnsureLazy(ctx, data)
	if !lazy.hasPending {
		values, err := s.enumeratorNextValueList(ctx, self, data)
		if err != nil {
			return Nil, err
		}
		lazy.pending = values
		lazy.hasPending = true
	}
	return ArrayValue(append([]Value(nil), lazy.pending...)), nil
}

func (s *State) enumeratorFeedMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	lazy := s.enumeratorEnsureLazy(ctx, data)
	if lazy.feedSet {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "feed value already set")}
	}
	value := Nil
	if len(args) > 0 {
		value = args[0]
	}
	lazy.feedSet = true
	lazy.feed = value
	return Nil, nil
}

func (s *State) enumeratorRewindMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	s.enumeratorStopLazy(data.lazy)
	data.lazy = nil
	data.result = Nil
	return self, nil
}

// enumeratorStopLazy cancels a suspended producer and waits until its
// goroutine has stopped touching the State. The producer is parked on
// lazy.resume after each yield, so sending cancel and draining the final done
// event is a synchronous handoff.
func (s *State) enumeratorStopLazy(lazy *enumeratorLazy) {
	if lazy == nil || !lazy.started || lazy.done {
		return
	}
	select {
	case lazy.resume <- enumeratorResume{cancel: true}:
		for {
			select {
			case out := <-lazy.out:
				if out.done {
					goto wait
				}
			case <-s.closed:
				return
			}
		}
	case <-lazy.exited:
		// The producer already finished; its done event may still be queued.
	}
wait:
	select {
	case <-lazy.exited:
	case <-s.closed:
		return
	}
	lazy.done = true
}

func (s *State) enumeratorSizeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if data.collect == nil && data.produce == nil && data.generator.IsNil() && data.method != "" && len(data.args) == 0 {
		if _, found := s.classOf(data.receiver).lookup("size"); found {
			return s.call(ctx, data.receiver, "size", nil, Nil, Nil)
		}
	}
	return Nil, nil
}

func (s *State) enumeratorWithIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	offset := 0
	if len(args) > 0 {
		if args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
				"no implicit conversion into Integer")}
		}
		offset = int(args[0].Int())
	}
	if block.IsNil() {
		return s.newEnumerator(func(ctx context.Context, yield func([]Value) (Value, error)) (Value, error) {
			index := offset
			return s.enumeratorRun(ctx, data, func(raw []Value) (Value, error) {
				combined := []Value{svalueOf(raw), IntValue(int64(index))}
				index++
				return yield(combined)
			})
		}), nil
	}
	index := offset
	result, err := s.enumeratorRun(ctx, data, func(raw []Value) (Value, error) {
		combined := []Value{svalueOf(raw), IntValue(int64(index))}
		index++
		return s.callBlock(ctx, block, combined, Nil)
	})
	if err != nil {
		if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
			return br.value, nil
		}
		if s.isStopIteration(err) {
			return Nil, nil
		}
		return Nil, err
	}
	return result, nil
}

func (s *State) enumeratorWithObjectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	object := args[0]
	if block.IsNil() {
		return s.toEnum(self, "with_object", args), nil
	}
	_, err := s.enumeratorRun(ctx, data, func(raw []Value) (Value, error) {
		pair := ArrayValue([]Value{svalueOf(raw), object})
		_, err := s.callBlock1(ctx, block, pair, Nil)
		return Nil, err
	})
	if err != nil {
		if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
			return br.value, nil
		}
		if s.isStopIteration(err) {
			return object, nil
		}
		return Nil, err
	}
	return object, nil
}

func (s *State) enumeratorInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok || data.method == "" {
		return StringValue("#<Enumerator>"), nil
	}
	text := "#<Enumerator: " + data.receiver.Inspect() + ":" + data.method
	if len(data.args) > 0 {
		parts := make([]string, len(data.args))
		for i, arg := range data.args {
			parts[i] = arg.Inspect()
		}
		text += "(" + joinStrings(parts, ", ") + ")"
	}
	return StringValue(text + ">"), nil
}

func joinStrings(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += separator + part
	}
	return result
}

func (s *State) enumeratorDupMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := enumeratorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an enumerator")}
	}
	if data.lazy != nil && data.lazy.started {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't copy already started enumerator")}
	}
	copyData := *data
	copyData.lazy = nil
	copyData.result = Nil
	return ObjectValue(&Object{class: s.classOf(self), data: &copyData}), nil
}

func (s *State) yielderNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.LocalJumpErrorClass, "no block given")}
	}
	class, _ := s.asClass(self)
	if class == nil {
		class = s.yielderClass()
	}
	return ObjectValue(&Object{class: class, data: &yielderData{block: block}}), nil
}

func (s *State) yielderCall(ctx context.Context, self Value, args []Value) (Value, error) {
	data, ok := yielderOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a yielder")}
	}
	if data.fn != nil {
		return data.fn(ctx, args)
	}
	if data.block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.LocalJumpErrorClass, "no block given")}
	}
	return s.callBlock(ctx, data.block, args, Nil)
}

func (s *State) yielderYieldMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.yielderCall(ctx, self, args)
}

func (s *State) yielderPushMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, err := s.yielderCall(ctx, self, args); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) generatorNewMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.LocalJumpErrorClass, "no block given")}
	}
	class, _ := s.asClass(self)
	if class == nil {
		class = s.generatorClass()
	}
	return ObjectValue(&Object{class: class, data: &generatorData{block: block}}), nil
}

func (s *State) generatorEachMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := generatorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a generator")}
	}
	if block.IsNil() {
		return self, nil
	}
	yielder := s.newYielder(block, nil)
	callArgs := append([]Value{yielder}, args...)
	return s.callBlock(ctx, data.block, callArgs, Nil)
}

func (s *State) generatorDupMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := generatorOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a generator")}
	}
	copyData := *data
	return ObjectValue(&Object{class: s.classOf(self), data: &copyData}), nil
}
