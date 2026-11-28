package mrbvm

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/onohiroki/go-mrbvm/sandbox"
)

type fileData struct {
	file   *os.File
	path   string
	closed bool
}

func (s *State) installFileLibrary() {
	fileClass := s.FileClass
	fileClass.classMethods["read"] = &method{owner: fileClass, name: "read", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.read expects a path")}
		}
		path := args[0].ToS()
		if data, found := s.readAsset(path); found {
			return StringValue(string(data)), nil
		}
		if err := s.requireCapability(sandbox.FileRead); err != nil {
			return Nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Nil, s.ioError(err)
		}
		return StringValue(string(data)), nil
	}}
	fileClass.classMethods["write"] = &method{owner: fileClass, name: "write", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.requireCapability(sandbox.FileWrite); err != nil {
			return Nil, err
		}
		if len(args) < 2 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.write expects a path and data")}
		}
		data := []byte(args[1].ToS())
		if err := os.WriteFile(args[0].ToS(), data, 0o644); err != nil {
			return Nil, s.ioError(err)
		}
		return IntValue(int64(len(data))), nil
	}}
	fileClass.classMethods["exist?"] = &method{owner: fileClass, name: "exist?", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		path := args[0].ToS()
		if _, found := s.assetStat(path); found {
			return True, nil
		}
		if err := s.requireCapability(sandbox.FileRead); err != nil {
			return Nil, err
		}
		_, err := os.Stat(path)
		return BoolValue(err == nil), nil
	}}
	fileClass.classMethods["size"] = &method{owner: fileClass, name: "size", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.size expects a path")}
		}
		path := args[0].ToS()
		if info, found := s.assetStat(path); found {
			return IntValue(info.Size()), nil
		}
		if err := s.requireCapability(sandbox.FileRead); err != nil {
			return Nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return Nil, s.ioError(err)
		}
		return IntValue(info.Size()), nil
	}}
	fileClass.classMethods["delete"] = &method{owner: fileClass, name: "delete", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.requireCapability(sandbox.FileWrite); err != nil {
			return Nil, err
		}
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.delete expects a path")}
		}
		if err := os.Remove(args[0].ToS()); err != nil {
			return Nil, s.ioError(err)
		}
		return IntValue(0), nil
	}}
	fileClass.classMethods["basename"] = &method{owner: fileClass, name: "basename", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.basename expects a path")}
		}
		return StringValue(filepath.Base(args[0].ToS())), nil
	}}
	fileClass.classMethods["dirname"] = &method{owner: fileClass, name: "dirname", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.dirname expects a path")}
		}
		return StringValue(filepath.Dir(args[0].ToS())), nil
	}}
	fileClass.classMethods["extname"] = &method{owner: fileClass, name: "extname", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.extname expects a path")}
		}
		return StringValue(filepath.Ext(args[0].ToS())), nil
	}}
	fileClass.classMethods["open"] = &method{owner: fileClass, name: "open", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "File.open expects a path")}
		}
		path := args[0].ToS()
		mode := "r"
		if len(args) > 1 {
			mode = args[1].ToS()
		}
		if !validFileMode(mode) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid access mode "+mode)}
		}
		capability := sandbox.FileRead
		if strings.ContainsAny(mode, "wa+") {
			capability = sandbox.FileWrite
		}
		if err := s.requireCapability(capability); err != nil {
			return Nil, err
		}
		flags := os.O_RDONLY
		switch {
		case strings.HasPrefix(mode, "w"):
			flags = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
		case strings.HasPrefix(mode, "a"):
			flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
		case strings.Contains(mode, "+"):
			flags = os.O_CREATE | os.O_RDWR
		}
		file, err := os.OpenFile(path, flags, 0o644)
		if err != nil {
			return Nil, s.ioError(err)
		}
		value := s.newFileValue(file, path)
		if block.IsNil() {
			return value, nil
		}
		result, err := s.callBlockWithSelf(ctx, block, value, []Value{value}, Nil)
		data, _ := fileOf(value)
		if err != nil {
			s.untrackFile(file)
			if data == nil || !data.closed {
				_ = file.Close()
				if data != nil {
					data.closed = true
				}
			}
			return Nil, err
		}
		if data != nil && data.closed {
			return result, nil
		}
		s.untrackFile(file)
		closeErr := file.Close()
		if data != nil {
			data.closed = true
		}
		if closeErr != nil {
			return Nil, s.ioError(closeErr)
		}
		return result, nil
	}}

	fileClass.define("read", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.requireCapability(sandbox.FileRead); err != nil {
			return Nil, err
		}
		data, ok := fileOf(self)
		if !ok || data.closed {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "closed stream")}
		}
		if len(args) == 0 {
			content, err := os.ReadFile(data.path)
			if err != nil {
				return Nil, s.ioError(err)
			}
			return StringValue(string(content)), nil
		}
		buffer := make([]byte, args[0].Int())
		n, err := data.file.Read(buffer)
		if err != nil && n == 0 {
			return Nil, nil
		}
		return StringValue(string(buffer[:n])), nil
	})
	fileClass.define("write", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.requireCapability(sandbox.FileWrite); err != nil {
			return Nil, err
		}
		data, ok := fileOf(self)
		if !ok || data.closed {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "closed stream")}
		}
		total := 0
		for _, arg := range args {
			n, err := data.file.WriteString(arg.ToS())
			if err != nil {
				return Nil, s.ioError(err)
			}
			total += n
		}
		return IntValue(int64(total)), nil
	})
	fileClass.define("puts", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if err := s.requireCapability(sandbox.FileWrite); err != nil {
			return Nil, err
		}
		data, ok := fileOf(self)
		if !ok || data.closed {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, "closed stream")}
		}
		for _, arg := range args {
			if _, err := data.file.WriteString(arg.ToS() + "\n"); err != nil {
				return Nil, s.ioError(err)
			}
		}
		return Nil, nil
	})
	fileClass.define("close", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fileOf(self)
		if !ok || data.closed {
			return Nil, nil
		}
		if err := data.file.Close(); err != nil {
			return Nil, s.ioError(err)
		}
		data.closed = true
		s.untrackFile(data.file)
		return Nil, nil
	})
	fileClass.define("closed?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := fileOf(self)
		return BoolValue(!ok || data.closed), nil
	})
	fileClass.define("path", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := fileOf(self); ok {
			return StringValue(data.path), nil
		}
		return Nil, nil
	})
}

func (s *State) newFileValue(file *os.File, path string) Value {
	s.trackFile(file)
	return ObjectValue(&Object{class: s.FileClass, data: &fileData{file: file, path: path}})
}

func fileOf(value Value) (*fileData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*fileData)
	return data, ok
}

func (s *State) requireCapability(capability sandbox.Capability) error {
	if s.Allowed(capability) {
		return nil
	}
	return &unwindSignal{kind: unwindException, value: newException(s.SecurityErrorClass, "capability denied: "+string(capability))}
}

func validFileMode(mode string) bool {
	if mode == "" {
		return false
	}
	switch mode[0] {
	case 'r', 'w', 'a':
	default:
		return false
	}
	for _, char := range mode[1:] {
		if !strings.ContainsRune("b+t", char) {
			return false
		}
	}
	return true
}

func (s *State) ioError(err error) error {
	return &unwindSignal{kind: unwindException, value: newException(s.IOErrorClass, err.Error())}
}
