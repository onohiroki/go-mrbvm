package mrbvm

import "context"

// installComparableLibrary implements Comparable on top of <=>. Including
// the module copies these methods into the receiver class.
func (s *State) installComparableLibrary() {
	class := s.ComparableClass
	class.define("<", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		cmp, ok, err := s.spaceship(ctx, self, args, "<")
		if err != nil {
			return Nil, err
		}
		return BoolValue(ok && cmp < 0), nil
	})
	class.define("<=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		cmp, ok, err := s.spaceship(ctx, self, args, "<=")
		if err != nil {
			return Nil, err
		}
		return BoolValue(ok && cmp <= 0), nil
	})
	class.define(">", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		cmp, ok, err := s.spaceship(ctx, self, args, ">")
		if err != nil {
			return Nil, err
		}
		return BoolValue(ok && cmp > 0), nil
	})
	class.define(">=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		cmp, ok, err := s.spaceship(ctx, self, args, ">=")
		if err != nil {
			return Nil, err
		}
		return BoolValue(ok && cmp >= 0), nil
	})
	class.define("==", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		result, err := s.call(ctx, self, "<=>", args, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		if result.IsNil() {
			return False, nil
		}
		return BoolValue(result.Kind() == KindInt && result.Int() == 0), nil
	})
	class.define("between?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return False, nil
		}
		lower, err := s.compareValues(ctx, self, args[0], Nil)
		if err != nil {
			return Nil, err
		}
		if lower < 0 {
			return False, nil
		}
		upper, err := s.compareValues(ctx, self, args[1], Nil)
		if err != nil {
			return Nil, err
		}
		return BoolValue(upper <= 0), nil
	})
	class.define("clamp", s.numericClampMethod)
}

func (s *State) spaceship(ctx context.Context, self Value, args []Value, operator string) (int, bool, error) {
	if len(args) != 1 {
		return 0, false, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	result, err := s.call(ctx, self, "<=>", args, Nil, Nil)
	if err != nil {
		return 0, false, err
	}
	if result.IsNil() {
		return 0, false, nil
	}
	if result.Kind() != KindInt {
		return 0, false, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "comparison failed")}
	}
	value := result.Int()
	switch {
	case value < 0:
		return -1, true, nil
	case value > 0:
		return 1, true, nil
	default:
		return 0, true, nil
	}
}
