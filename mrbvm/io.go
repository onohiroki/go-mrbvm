package mrbvm

import (
	"bufio"
	"context"
	"io"
)

type ioData struct {
	reader *bufio.Reader
	writer io.Writer
	name   string
}

func (s *State) installIOLibrary() {
	stdout := ObjectValue(&Object{class: s.IOClass, data: &ioData{writer: s.stdout, name: "STDOUT"}})
	stderr := ObjectValue(&Object{class: s.IOClass, data: &ioData{writer: s.stderr, name: "STDERR"}})
	stdin := ObjectValue(&Object{class: s.IOClass, data: &ioData{reader: bufio.NewReader(s.stdin), name: "STDIN"}})
	for name, value := range map[string]Value{
		"STDOUT": stdout,
		"STDERR": stderr,
		"STDIN":  stdin,
	} {
		s.constants[name] = value
		s.ObjectClass.constants[name] = value
	}
	s.globals["$stdout"] = stdout
	s.globals["$stderr"] = stderr
	s.globals["$stdin"] = stdin

	class := s.IOClass
	class.define("write", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.writer == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "not writable")}
		}
		total := 0
		for _, arg := range args {
			n, err := io.WriteString(data.writer, arg.ToS())
			if err != nil {
				return Nil, s.ioError(err)
			}
			total += n
		}
		if s.output != nil && s.output.exceeded {
			return Nil, errOutputLimit
		}
		return IntValue(int64(total)), nil
	})
	class.define("print", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.writer == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "not writable")}
		}
		for _, arg := range args {
			if _, err := io.WriteString(data.writer, arg.ToS()); err != nil {
				return Nil, s.ioError(err)
			}
		}
		if s.output != nil && s.output.exceeded {
			return Nil, errOutputLimit
		}
		return Nil, nil
	})
	class.define("puts", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.writer == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "not writable")}
		}
		if len(args) == 0 {
			if _, err := io.WriteString(data.writer, "\n"); err != nil {
				return Nil, s.ioError(err)
			}
		}
		for _, arg := range args {
			if _, err := io.WriteString(data.writer, arg.ToS()+"\n"); err != nil {
				return Nil, s.ioError(err)
			}
		}
		if s.output != nil && s.output.exceeded {
			return Nil, errOutputLimit
		}
		return Nil, nil
	})
	class.define("flush", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("gets", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.reader == nil {
			return Nil, nil
		}
		line, err := data.reader.ReadString('\n')
		if err != nil && line == "" {
			return Nil, nil
		}
		return StringValue(line), nil
	})
	class.define("readline", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.reader == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "not readable")}
		}
		line, err := data.reader.ReadString('\n')
		if err != nil && line == "" {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "end of file reached")}
		}
		return StringValue(line), nil
	})
	class.define("read", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.reader == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "not readable")}
		}
		if len(args) == 0 {
			content, err := io.ReadAll(data.reader)
			if err != nil {
				return Nil, s.ioError(err)
			}
			return StringValue(string(content)), nil
		}
		buffer := make([]byte, args[0].Int())
		n, err := data.reader.Read(buffer)
		if err != nil && n == 0 {
			return Nil, nil
		}
		return StringValue(string(buffer[:n])), nil
	})
	class.define("eof?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := ioOf(self)
		if !ok || data.reader == nil {
			return True, nil
		}
		_, err := data.reader.Peek(1)
		return BoolValue(err != nil), nil
	})
	class.define("close", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return Nil, nil
	})
	class.define("closed?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return False, nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := ioOf(self); ok {
			return StringValue("#<IO:" + data.name + ">"), nil
		}
		return StringValue("#<IO>"), nil
	})
}

func ioOf(value Value) (*ioData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*ioData)
	return data, ok
}

func (s *State) newIOValue(reader io.Reader, writer io.Writer, name string) Value {
	data := &ioData{writer: writer, name: name}
	if reader != nil {
		data.reader = bufio.NewReader(reader)
	}
	return ObjectValue(&Object{class: s.IOClass, data: data})
}
