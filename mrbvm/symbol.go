package mrbvm

import "context"

func (s *State) installSymbolLibrary() {
	class := s.SymbolClass
	class.define("size", s.symbolLengthMethod)
	class.define("length", s.symbolLengthMethod)
	class.define("empty?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(self.String() == ""), nil
	})
	class.define("intern", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("capitalize", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		converted, err := convertCase(self.String(), unicodeUpper, true)
		if err != nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, err.Error())}
		}
		return SymbolValue(converted), nil
	})
	class.define("upcase", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		converted, err := convertCase(self.String(), unicodeUpper, false)
		if err != nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, err.Error())}
		}
		return SymbolValue(converted), nil
	})
	class.define("downcase", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		converted, err := convertCase(self.String(), unicodeLower, false)
		if err != nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, err.Error())}
		}
		return SymbolValue(converted), nil
	})
	class.define("casecmp", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindSymbol {
			return Nil, nil
		}
		left := self.String()
		right := args[0].String()
		cmp := 0
		for i := 0; i < len(left) || i < len(right); i++ {
			var l, r byte
			if i < len(left) {
				l = lowerASCII(left[i])
			}
			if i < len(right) {
				r = lowerASCII(right[i])
			}
			if l < r {
				cmp = -1
				break
			}
			if l > r {
				cmp = 1
				break
			}
		}
		return IntValue(int64(cmp)), nil
	})
	class.define("succ", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return SymbolValue(succString(self.String())), nil
	})
	class.define("next", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return SymbolValue(succString(self.String())), nil
	})
	class.define("slice", s.symbolSliceMethod)
	class.define("[]", s.symbolSliceMethod)
	class.define("start_with?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := self.String()
		for _, arg := range args {
			if arg.Kind() != KindString {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
			}
			if len(arg.String()) <= len(text) && text[:len(arg.String())] == arg.String() {
				return True, nil
			}
		}
		return False, nil
	})
	class.define("end_with?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		text := self.String()
		for _, arg := range args {
			if arg.Kind() != KindString {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
			}
			if len(arg.String()) <= len(text) && text[len(text)-len(arg.String()):] == arg.String() {
				return True, nil
			}
		}
		return False, nil
	})
}

func (s *State) symbolLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return IntValue(int64(stringCharCount(self.String()))), nil
}

func (s *State) symbolSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	return s.call(ctx, StringValue(self.String()), "[]", args, kw, block)
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
