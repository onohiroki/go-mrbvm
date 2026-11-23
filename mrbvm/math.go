package mrbvm

import (
	"context"
	"math"
)

// installMathLibrary provides the Math module used by the numeric gems and by
// Ruby code that asks for elementary functions. mruby exposes it as a module
// with module functions, so the functions live on the module's class methods.
func (s *State) installMathLibrary() {
	class := s.MathClass
	domainError := newClass("DomainError", s.StandardErrorClass)
	s.ensureClassObject(domainError)
	class.constants["DomainError"] = s.classValue(domainError)
	class.constants["PI"] = FloatValue(math.Pi)
	class.constants["E"] = FloatValue(math.E)

	mathError := func(name string) error {
		return &unwindSignal{kind: unwindException, value: newException(domainError, "Numerical argument is out of domain - "+name)}
	}
	floatArg := func(args []Value, index int) (float64, error) {
		if index >= len(args) {
			return 0, argumentCountError(s, len(args), "1")
		}
		value, ok := numericFloat(args[index])
		if !ok {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Float")}
		}
		return value, nil
	}
	unary := func(name string, fn func(float64) float64, check func(float64) bool) {
		class.classMethods[name] = &method{owner: class, name: name, native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			if len(args) != 1 {
				return Nil, argumentCountError(s, len(args), "1")
			}
			value, err := floatArg(args, 0)
			if err != nil {
				return Nil, err
			}
			if check != nil && !check(value) {
				return Nil, mathError(name)
			}
			return FloatValue(fn(value)), nil
		}}
	}
	unary("sin", math.Sin, nil)
	unary("cos", math.Cos, nil)
	unary("tan", math.Tan, nil)
	unary("asin", math.Asin, func(v float64) bool { return v >= -1 && v <= 1 })
	unary("acos", math.Acos, func(v float64) bool { return v >= -1 && v <= 1 })
	unary("atan", math.Atan, nil)
	unary("sinh", math.Sinh, nil)
	unary("cosh", math.Cosh, nil)
	unary("tanh", math.Tanh, nil)
	unary("asinh", math.Asinh, nil)
	unary("acosh", math.Acosh, func(v float64) bool { return v >= 1 })
	unary("atanh", math.Atanh, func(v float64) bool { return v > -1 && v < 1 })
	unary("exp", math.Exp, nil)
	unary("expm1", math.Expm1, nil)
	unary("log1p", math.Log1p, func(v float64) bool { return v > -1 })
	unary("log2", math.Log2, func(v float64) bool { return v > 0 })
	unary("log10", math.Log10, func(v float64) bool { return v > 0 })
	unary("sqrt", math.Sqrt, func(v float64) bool { return v >= 0 })
	unary("cbrt", math.Cbrt, nil)
	unary("erf", math.Erf, nil)
	unary("erfc", math.Erfc, nil)

	class.classMethods["atan2"] = &method{owner: class, name: "atan2", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return Nil, argumentCountError(s, len(args), "2")
		}
		y, err := floatArg(args, 0)
		if err != nil {
			return Nil, err
		}
		x, err := floatArg(args, 1)
		if err != nil {
			return Nil, err
		}
		return FloatValue(math.Atan2(y, x)), nil
	}}
	class.classMethods["hypot"] = &method{owner: class, name: "hypot", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return Nil, argumentCountError(s, len(args), "2")
		}
		x, err := floatArg(args, 0)
		if err != nil {
			return Nil, err
		}
		y, err := floatArg(args, 1)
		if err != nil {
			return Nil, err
		}
		return FloatValue(math.Hypot(x, y)), nil
	}}
	class.classMethods["log"] = &method{owner: class, name: "log", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 || len(args) > 2 {
			return Nil, argumentCountError(s, len(args), "1..2")
		}
		x, err := floatArg(args, 0)
		if err != nil {
			return Nil, err
		}
		if x < 0 {
			return Nil, mathError("log")
		}
		if len(args) == 2 {
			base, err := floatArg(args, 1)
			if err != nil {
				return Nil, err
			}
			if base <= 0 || base == 1 {
				return Nil, mathError("log")
			}
			return FloatValue(math.Log(x) / math.Log(base)), nil
		}
		return FloatValue(math.Log(x)), nil
	}}
	class.classMethods["frexp"] = &method{owner: class, name: "frexp", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		x, err := floatArg(args, 0)
		if err != nil {
			return Nil, err
		}
		frac, exp := math.Frexp(x)
		return ArrayValue([]Value{FloatValue(frac), IntValue(int64(exp))}), nil
	}}
	class.classMethods["ldexp"] = &method{owner: class, name: "ldexp", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return Nil, argumentCountError(s, len(args), "2")
		}
		x, err := floatArg(args, 0)
		if err != nil {
			return Nil, err
		}
		exponent, err := floatArg(args, 1)
		if err != nil {
			return Nil, err
		}
		return FloatValue(math.Ldexp(x, int(exponent))), nil
	}}
}
