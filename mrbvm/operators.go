package mrbvm

import (
	"context"
	"math"
	"math/big"
)

// builtinOperatorNames is the set of method names whose opcodes may take a
// native fast path. A Ruby-level redefinition of any of them disables that
// path for the rest of the program.
var builtinOperatorNames = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true, "**": true,
	"<<": true, ">>": true, "&": true, "|": true, "^": true,
	"~": true, "-@": true, "+@": true,
	"==": true, "!=": true, "===": true,
	"<": true, "<=": true, ">": true, ">=": true, "<=>": true,
}

func (s *State) noteOperatorDefinition(name string) {
	if builtinOperatorNames[name] {
		s.bopRedefined = true
	}
}

// installNumericOperatorMethods gives Integer and Float real methods for the
// arithmetic operators. mruby's compiler emits OP_ADD and friends, but the
// methods still have to exist: alias_method, method_defined?, respond_to?,
// send, and a later redefinition all go through the method table.
func (s *State) installNumericOperatorMethods() {
	integerOperators := []string{"+", "-", "*", "/", "%", "**", "<<", ">>", "&", "|", "^"}
	for _, name := range integerOperators {
		name := name
		s.IntegerClass.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return s.numericBinaryOperator(self, name, args)
		})
	}
	s.IntegerClass.define("~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
		}
		return s.integerBitNot(self)
	})

	floatOperators := []string{"+", "-", "*", "/", "%", "**"}
	for _, name := range floatOperators {
		name := name
		s.FloatClass.define(name, func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return s.numericBinaryOperator(self, name, args)
		})
	}

	for _, class := range []*Class{s.IntegerClass, s.FloatClass} {
		class.define("-@", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if len(args) != 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
			}
			return s.numericNegate(self)
		})
	}
}

// markBuiltinOperatorMethods records which operator methods are still the
// builtin implementations. A user redefinition installs a new *method and
// therefore does not carry the mark.
func (s *State) markBuiltinOperatorMethods() {
	classes := []*Class{
		s.BasicObjectClass, s.ObjectClass, s.ModuleClass, s.KernelClass,
		s.NumericClass, s.IntegerClass, s.FloatClass, s.StringClass,
		s.SymbolClass, s.ArrayClass, s.HashClass, s.RangeClass,
		s.ProcClass, s.EnumerableClass, s.ComparableClass,
		s.NilClass, s.TrueClass, s.FalseClass,
		s.TimeClass, s.RegexpClass, s.MatchDataClass,
	}
	for _, class := range classes {
		if class == nil {
			continue
		}
		for name := range builtinOperatorNames {
			if m, ok := class.methods[name]; ok {
				m.builtin = true
			}
		}
	}
}

func (s *State) numericBinaryOperator(self Value, name string, args []Value) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "wrong number of arguments")}
	}
	right := args[0]
	if value, handled, err := s.complexBinaryOperator(self, name, right); handled {
		return value, err
	}
	if value, handled, err := s.rationalBinaryOperator(self, name, right); handled {
		return value, err
	}
	switch name {
	case "/":
		if isIntegerValue(self) && isIntegerValue(right) && isNumericZero(right) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
	case "%":
		if isNumericZero(right) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
	case "<<", ">>":
		if shift, ok := integerBig(right); ok && shift.Sign() < 0 {
			opposite := ">>"
			if name == ">>" {
				opposite = "<<"
			}
			amount := normalizedBigInt(new(big.Int).Neg(shift))
			if value, ok := binaryValue(self, opposite, amount); ok {
				return value, nil
			}
		}
	}
	if value, ok := binaryValue(self, name, right); ok {
		return value, nil
	}
	return Nil, s.operatorTypeError(right)
}

func (s *State) numericNegate(self Value) (Value, error) {
	switch self.Kind() {
	case KindInt:
		value := self.Int()
		if value == math.MinInt64 {
			return normalizedBigInt(new(big.Int).Neg(big.NewInt(value))), nil
		}
		return IntValue(-value), nil
	case KindBigInt:
		value, _ := self.BigInt()
		return normalizedBigInt(new(big.Int).Neg(value)), nil
	case KindFloat:
		return FloatValue(-self.Float()), nil
	}
	return Nil, s.operatorTypeError(self)
}

func (s *State) integerBitNot(self Value) (Value, error) {
	switch self.Kind() {
	case KindInt:
		return IntValue(^self.Int()), nil
	case KindBigInt:
		value, _ := self.BigInt()
		return normalizedBigInt(new(big.Int).Not(value)), nil
	}
	return Nil, s.operatorTypeError(self)
}

func (s *State) operatorTypeError(other Value) error {
	name := "Object"
	if class := s.classOf(other); class != nil && class.name != "" {
		name = class.name
	}
	return &unwindSignal{
		kind:  unwindException,
		value: newException(s.TypeErrorClass, "no implicit conversion of "+name+" into Integer"),
	}
}

func isIntegerValue(value Value) bool {
	return value.Kind() == KindInt || value.Kind() == KindBigInt
}

func isNumericZero(value Value) bool {
	switch value.Kind() {
	case KindInt:
		return value.Int() == 0
	case KindFloat:
		return value.Float() == 0
	case KindBigInt:
		n, _ := value.BigInt()
		return n.Sign() == 0
	}
	return false
}
