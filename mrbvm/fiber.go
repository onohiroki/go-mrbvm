package mrbvm

import "context"

type fiberMessage struct {
	values []Value
	done   bool
	err    error
}

type fiberData struct {
	block    Value
	resumeCh chan fiberMessage
	outCh    chan fiberMessage
	self     Value
	started  bool
	dead     bool
}

type fiberContextKey struct{}

func (s *State) installFiberClass() {
	class := s.FiberClass
	class.classMethods["new"] = &method{owner: class, name: "new", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if block.IsNil() {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Fiber.new requires a block")}
		}
		data := &fiberData{block: block, resumeCh: make(chan fiberMessage), outCh: make(chan fiberMessage)}
		value := ObjectValue(&Object{class: class, data: data})
		data.self = value
		return value, nil
	}}
	class.classMethods["yield"] = &method{owner: class, name: "yield", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ctx.Value(fiberContextKey{}).(*fiberData)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "can't yield from root fiber")}
		}
		data.outCh <- fiberMessage{values: append([]Value(nil), args...)}
		next := <-data.resumeCh
		if next.err != nil {
			return Nil, next.err
		}
		if len(next.values) == 0 {
			return Nil, nil
		}
		return next.values[0], nil
	}}
	class.define("resume", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a fiber")}
		}
		if data.dead {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "dead fiber called")}
		}
		if !data.started {
			data.started = true
			go func() {
				fiberCtx := context.WithValue(ctx, fiberContextKey{}, data)
				result, err := s.callBlock(fiberCtx, data.block, args, Nil)
				data.outCh <- fiberMessage{values: []Value{result}, done: true, err: err}
			}()
		} else {
			data.resumeCh <- fiberMessage{values: append([]Value(nil), args...)}
		}
		message := <-data.outCh
		if message.done {
			data.dead = true
			if message.err != nil {
				return Nil, message.err
			}
			if len(message.values) == 0 {
				return Nil, nil
			}
			return message.values[0], nil
		}
		if len(message.values) == 0 {
			return Nil, nil
		}
		return message.values[0], nil
	})
	class.define("raise", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a fiber")}
		}
		if data.dead {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "dead fiber called")}
		}
		if !data.started {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FiberErrorClass, "cannot raise into an unstarted fiber")}
		}
		var exception Value
		if len(args) == 0 {
			exception = newException(s.RuntimeErrorClass, "")
		} else if class, ok := s.asClass(args[0]); ok {
			message := class.name
			if len(args) > 1 {
				message = args[1].ToS()
			}
			exception = newException(class, message)
		} else if args[0].Kind() == KindObject && args[0].obj != nil {
			if _, ok := args[0].obj.data.(*exceptionData); ok {
				exception = args[0]
			} else {
				exception = newException(s.RuntimeErrorClass, args[0].ToS())
			}
		} else {
			exception = newException(s.RuntimeErrorClass, args[0].ToS())
		}
		data.resumeCh <- fiberMessage{err: &unwindSignal{kind: unwindException, value: exception}}
		message := <-data.outCh
		if message.done {
			data.dead = true
			if message.err != nil {
				return Nil, message.err
			}
			if len(message.values) == 0 {
				return Nil, nil
			}
			return message.values[0], nil
		}
		if len(message.values) == 0 {
			return Nil, nil
		}
		return message.values[0], nil
	})
	class.define("alive?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		return BoolValue(ok && !data.dead), nil
	})
	class.classMethods["current"] = &method{owner: class, name: "current", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := ctx.Value(fiberContextKey{}).(*fiberData); ok {
			return data.self, nil
		}
		return s.RootFiber, nil
	}}
	class.define("dead?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fiberOf(self)
		return BoolValue(!ok || data.dead), nil
	})
}

func fiberOf(value Value) (*fiberData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*fiberData)
	return data, ok
}
