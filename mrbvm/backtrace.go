package mrbvm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type locationData struct {
	path     string
	absolute string
	lineno   int
	label    string
	text     string
}

func (s *State) installBacktraceLibrary() {
	thread := s.constants["Thread"]
	threadClass, ok := s.asClass(thread)
	if !ok {
		threadClass = newClass("Thread", s.ObjectClass)
		threadClass.isModule = true
		s.ensureClassObject(threadClass)
		s.constants["Thread"] = s.classValue(threadClass)
		s.ObjectClass.constants["Thread"] = s.classValue(threadClass)
	}
	backtrace := newClass("Thread::Backtrace", s.ObjectClass)
	backtrace.isModule = true
	s.ensureClassObject(backtrace)
	threadClass.constants["Backtrace"] = s.classValue(backtrace)
	location := newClass("Thread::Backtrace::Location", s.ObjectClass)
	s.ensureClassObject(location)
	backtrace.constants["Location"] = s.classValue(location)
	location.classMethods["new"] = &method{owner: location, name: "new", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.NoMethodErrorClass, "undefined method 'new' for Class")}
	}}
	location.define("path", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(data.path), nil
		}
		return Nil, nil
	})
	location.define("absolute_path", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(data.absolute), nil
		}
		return Nil, nil
	})
	location.define("lineno", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return IntValue(int64(data.lineno)), nil
		}
		return Nil, nil
	})
	location.define("label", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(data.label), nil
		}
		return Nil, nil
	})
	location.define("base_label", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(data.label), nil
		}
		return Nil, nil
	})
	location.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(data.text), nil
		}
		return StringValue(""), nil
	})
	location.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := self.obj.data.(*locationData); ok {
			return StringValue(inspectUTF8String(data.text)), nil
		}
		return StringValue(`""`), nil
	})
	s.BacktraceLocationClass = location
	s.ObjectClass.define("caller", s.kernelCallerMethod)
	s.ObjectClass.define("caller_locations", s.kernelCallerLocationsMethod)
	s.KernelClass.classMethods["caller"] = &method{owner: s.KernelClass, name: "caller", native: s.kernelCallerMethod}
	s.KernelClass.classMethods["caller_locations"] = &method{owner: s.KernelClass, name: "caller_locations", native: s.kernelCallerLocationsMethod}
	s.ExceptionClass.define("backtrace_locations", s.exceptionBacktraceLocationsMethod)
}

func (s *State) newLocation(path string, lineno int, label, text string) Value {
	return ObjectValue(&Object{class: s.BacktraceLocationClass, data: &locationData{
		path: path, absolute: path, lineno: lineno, label: label, text: text,
	}})
}

func callerRangeArgs(s *State, args []Value) (int, int, error) {
	start := 1
	length := -1
	if len(args) == 0 {
		return start, length, nil
	}
	if len(args) > 2 {
		return 0, 0, argumentCountError(s, len(args), "0..2")
	}
	if rangeDataOfValue(args[0]) != nil {
		rangeValue := rangeDataOfValue(args[0])
		if rangeValue.low.IsNil() || rangeValue.low.Kind() != KindInt {
			return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		start = int(rangeValue.low.Int())
		if start < 0 {
			return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative level")}
		}
		if rangeValue.high.IsNil() {
			return start, -1, nil
		}
		if rangeValue.high.Kind() != KindInt {
			return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		end := int(rangeValue.high.Int())
		if end < start {
			return start, 0, nil
		}
		length = end - start + 1
		if rangeValue.exclusive {
			length--
		}
		return start, length, nil
	}
	if args[0].Kind() != KindInt {
		return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	start = int(args[0].Int())
	if start < 0 {
		return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative level")}
	}
	if len(args) == 2 {
		if args[1].IsNil() {
			return start, -1, nil
		}
		if args[1].Kind() != KindInt {
			return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		length = int(args[1].Int())
		if length < 0 {
			return 0, 0, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "negative length")}
		}
	}
	return start, length, nil
}

func (s *State) callerFrames(start, length int) []*callFrame {
	total := len(s.frames)
	if start >= total {
		return nil
	}
	end := total
	if length >= 0 && start+length < end {
		end = start + length
	}
	frames := make([]*callFrame, 0, end-start)
	for i := start; i < end; i++ {
		frames = append(frames, &s.frames[total-1-i])
	}
	return frames
}

func (s *State) kernelCallerMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	start, length, err := callerRangeArgs(s, args)
	if err != nil {
		return Nil, err
	}
	frames := s.callerFrames(start, length)
	if frames == nil {
		return Nil, nil
	}
	values := make([]Value, len(frames))
	for i, frame := range frames {
		file := frame.file
		if file == "" {
			file = "unknown"
		}
		text := file + ":" + strconv.Itoa(frame.line)
		if frame.method != "" {
			text += ":in " + frame.method
		}
		values[i] = StringValue(text)
	}
	return ArrayValue(values), nil
}

func (s *State) kernelCallerLocationsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	start, length, err := callerRangeArgs(s, args)
	if err != nil {
		return Nil, err
	}
	frames := s.callerFrames(start, length)
	if frames == nil {
		return Nil, nil
	}
	values := make([]Value, len(frames))
	for i, frame := range frames {
		file := frame.file
		if file == "" {
			file = "unknown"
		}
		text := file + ":" + strconv.Itoa(frame.line)
		if frame.method != "" {
			text += ":in " + frame.method
		}
		values[i] = s.newLocation(file, frame.line, frame.method, text)
	}
	return ArrayValue(values), nil
}

func (s *State) exceptionBacktraceLocationsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, nil
	}
	data, ok := self.obj.data.(*exceptionData)
	if !ok || len(data.backtrace) == 0 {
		return Nil, nil
	}
	values := make([]Value, len(data.backtrace))
	for i, line := range data.backtrace {
		path := line
		lineno := 0
		label := ""
		if index := strings.LastIndex(line, ":"); index >= 0 {
			path = line[:index]
			if parsed, err := strconv.Atoi(line[index+1:]); err == nil {
				lineno = parsed
			}
		}
		if index := strings.Index(path, ":"); index >= 0 {
			_ = index
		}
		values[i] = s.newLocation(path, lineno, label, line)
	}
	return ArrayValue(values), nil
}

func (s *State) formatFrameForDebug(frame *callFrame) string {
	return fmt.Sprintf("%s:%d", frame.file, frame.line)
}
