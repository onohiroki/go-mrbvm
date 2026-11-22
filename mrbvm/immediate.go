package mrbvm

import "context"

// installImmediateLibrary defines the classes and logical operators for nil,
// true, and false. They have no instance state, so the methods are shared.
func (s *State) installImmediateLibrary() {
	s.NilClass.define("&", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return False, nil
	})
	s.NilClass.define("|", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && args[0].IsTruthy()), nil
	})
	s.NilClass.define("^", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && args[0].IsTruthy()), nil
	})
	s.NilClass.define("=~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
		}
		return Nil, nil
	})
	s.NilClass.define("!~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
		}
		return True, nil
	})

	s.TrueClass.define("&", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && args[0].IsTruthy()), nil
	})
	s.TrueClass.define("|", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return True, nil
	})
	s.TrueClass.define("^", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) != 1 || !args[0].IsTruthy()), nil
	})

	s.FalseClass.define("&", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return False, nil
	})
	s.FalseClass.define("|", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && args[0].IsTruthy()), nil
	})
	s.FalseClass.define("^", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(len(args) == 1 && args[0].IsTruthy()), nil
	})
}
