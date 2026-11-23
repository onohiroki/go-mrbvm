package mrbvm

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
)

type rationalData struct {
	rat *big.Rat
}

func rationalOf(value Value) (*big.Rat, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*rationalData)
	if !ok {
		return nil, false
	}
	return data.rat, true
}

func newRationalValue(r *big.Rat) Value {
	copyRat := new(big.Rat).Set(r)
	value := ObjectValue(&Object{data: &rationalData{rat: copyRat}})
	value.obj.frozen = true
	return value
}

func (s *State) rationalValue(r *big.Rat) Value {
	value := newRationalValue(r)
	value.obj.class = s.RationalClass
	return value
}

func exactRat(value Value) (*big.Rat, bool) {
	switch value.Kind() {
	case KindInt:
		return new(big.Rat).SetInt64(value.Int()), true
	case KindBigInt:
		integer, _ := value.BigInt()
		return new(big.Rat).SetInt(integer), true
	}
	if rat, ok := rationalOf(value); ok {
		return new(big.Rat).Set(rat), true
	}
	return nil, false
}

func numericRat(value Value) (*big.Rat, bool) {
	if rat, ok := exactRat(value); ok {
		return rat, true
	}
	if value.Kind() == KindFloat {
		f := value.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		rat := new(big.Rat)
		if rat.SetFloat64(f) == nil {
			return nil, false
		}
		return rat, true
	}
	return nil, false
}

func isRationalObject(value Value) bool {
	_, ok := rationalOf(value)
	return ok
}

func intValueFromBig(integer *big.Int) Value {
	if integer.IsInt64() {
		return IntValue(integer.Int64())
	}
	return BigIntValue(new(big.Int).Set(integer))
}

func (s *State) installRationalLibrary() {
	class := s.RationalClass
	includeModule(class, s.ComparableClass)
	class.classMethods["new"] = &method{owner: class, name: "new", native: s.kernelRationalMethod}
	class.define("numerator", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		return intValueFromBig(new(big.Int).Set(rat.Num())), nil
	})
	class.define("denominator", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		return intValueFromBig(new(big.Int).Set(rat.Denom())), nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		value, _ := rat.Float64()
		return FloatValue(value), nil
	})
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		return intValueFromBig(new(big.Int).Quo(rat.Num(), rat.Denom())), nil
	})
	class.define("negative?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		return BoolValue(ok && rat.Sign() < 0), nil
	})
	class.define("frozen?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return True, nil
	})
	class.define("hash", s.rationalHashMethod)
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		left, leftOK := rationalOf(self)
		right, rightOK := rationalOf(args[0])
		return BoolValue(leftOK && rightOK && left.Cmp(right) == 0), nil
	})
	class.define("==", s.rationalCompareEqMethod)
	class.define("!=", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		equal, err := s.rationalCompareEqMethod(ctx, self, args, kw, block)
		if err != nil {
			return Nil, err
		}
		return BoolValue(!equal.IsTruthy()), nil
	})
	class.define("<=>", s.rationalSpaceshipMethod)
	class.define("+", s.rationalArithmeticMethod("+"))
	class.define("-", s.rationalArithmeticMethod("-"))
	class.define("*", s.rationalArithmeticMethod("*"))
	class.define("/", s.rationalArithmeticMethod("/"))
	class.define("-@", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		return s.rationalValue(new(big.Rat).Neg(rat)), nil
	})
	class.define("**", s.rationalPowMethod)
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return StringValue("()"), nil
		}
		return StringValue("(" + rat.Num().String() + "/" + rat.Denom().String() + ")"), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat, ok := rationalOf(self)
		if !ok {
			return StringValue(""), nil
		}
		return StringValue(rat.Num().String() + "/" + rat.Denom().String()), nil
	})
	class.define("floor", s.rationalRoundingMethod("floor"))
	class.define("ceil", s.rationalRoundingMethod("ceil"))
	class.define("truncate", s.rationalRoundingMethod("truncate"))
	class.define("round", s.rationalRoundMethod)

	s.ObjectClass.define("Rational", s.kernelRationalMethod)
	s.KernelClass.classMethods["Rational"] = &method{owner: s.KernelClass, name: "Rational", native: s.kernelRationalMethod}
	s.FloatClass.define("to_r", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		rat := new(big.Rat)
		if rat.SetFloat64(self.Float()) == nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.FloatDomainErrorClass, "float domain error")}
		}
		return s.rationalValue(rat), nil
	})
	s.IntegerClass.define("quo", s.integerQuoMethod)
}

func (s *State) kernelRationalMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if len(args) == 2 && args[1].Kind() == KindFloat && args[1].Float() == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
	}
	numerator, ok := numericRat(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Rational")}
	}
	if len(args) == 1 {
		return s.rationalValue(numerator), nil
	}
	denominator, ok := numericRat(args[1])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Rational")}
	}
	if denominator.Sign() == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
	}
	return s.rationalValue(new(big.Rat).Quo(numerator, denominator)), nil
}

func (s *State) integerQuoMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	left, ok := exactRat(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not an integer")}
	}
	right, ok := numericRat(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Numeric")}
	}
	if right.Sign() == 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
	}
	return s.rationalValue(new(big.Rat).Quo(left, right)), nil
}

func (s *State) rationalHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	rat, ok := rationalOf(self)
	if !ok {
		return IntValue(0), nil
	}
	if rat.IsInt() {
		return IntValue(hashValue(intValueFromBig(rat.Num()), map[*Object]bool{})), nil
	}
	return IntValue(hashValue(StringValue("rational:"+rat.Num().String()+"/"+rat.Denom().String()), map[*Object]bool{})), nil
}

func (s *State) rationalCompareEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	left, ok := rationalOf(self)
	if !ok {
		return False, nil
	}
	right, ok := numericRat(args[0])
	if !ok {
		return False, nil
	}
	return BoolValue(left.Cmp(right) == 0), nil
}

func (s *State) rationalSpaceshipMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	left, ok := rationalOf(self)
	if !ok {
		return Nil, nil
	}
	right, ok := numericRat(args[0])
	if !ok {
		return Nil, nil
	}
	return IntValue(int64(left.Cmp(right))), nil
}

func (s *State) rationalArithmeticMethod(op string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		left, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		right := args[0]
		if op == "/" {
			if rational, ok := numericRat(right); ok && rational.Sign() == 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
			}
		}
		if right.Kind() == KindFloat {
			l, _ := left.Float64()
			r := right.Float()
			switch op {
			case "+":
				return FloatValue(l + r), nil
			case "-":
				return FloatValue(l - r), nil
			case "*":
				return FloatValue(l * r), nil
			case "/":
				return FloatValue(l / r), nil
			}
		}
		other, ok := numericRat(right)
		if !ok {
			return Nil, s.operatorTypeError(right)
		}
		result := new(big.Rat)
		switch op {
		case "+":
			result.Add(left, other)
		case "-":
			result.Sub(left, other)
		case "*":
			result.Mul(left, other)
		case "/":
			if other.Sign() == 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
			}
			result.Quo(left, other)
		}
		return s.rationalValue(result), nil
	}
}

func ratIntegerExponent(rat *big.Rat, exponent *big.Int) (*big.Rat, bool) {
	if exponent.Sign() == 0 {
		return new(big.Rat).SetInt64(1), true
	}
	negative := exponent.Sign() < 0
	exp := new(big.Int).Abs(exponent)
	if !exp.IsInt64() {
		return nil, false
	}
	num := new(big.Int).Exp(rat.Num(), exp, nil)
	den := new(big.Int).Exp(rat.Denom(), exp, nil)
	result := new(big.Rat).SetFrac(num, den)
	if negative {
		if result.Sign() == 0 {
			return nil, false
		}
		result.Inv(result)
	}
	return result, true
}

func (s *State) rationalPowMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	base, ok := rationalOf(self)
	if !ok {
		return Nil, nil
	}
	exponent := args[0]
	if integer, ok := exactRat(exponent); ok && integer.IsInt() {
		result, ok := ratIntegerExponent(base, integer.Num())
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
		return s.rationalValue(result), nil
	}
	if rat, ok := rationalOf(exponent); ok {
		if rat.IsInt() {
			result, ok := ratIntegerExponent(base, rat.Num())
			if !ok {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
			}
			return s.rationalValue(result), nil
		}
	}
	l, _ := base.Float64()
	r := 0.0
	if exponent.Kind() == KindFloat {
		r = exponent.Float()
	} else if rat, ok := numericRat(exponent); ok {
		r, _ = rat.Float64()
	} else if integer, ok := exactRat(exponent); ok {
		r, _ = integer.Float64()
	} else {
		return Nil, s.operatorTypeError(exponent)
	}
	return FloatValue(math.Pow(l, r)), nil
}

func scaleRat(rat *big.Rat, digits int) *big.Rat {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(digits))), nil)
	if digits >= 0 {
		return new(big.Rat).Mul(rat, new(big.Rat).SetInt(scale))
	}
	return new(big.Rat).Quo(rat, new(big.Rat).SetInt(scale))
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func integerFromRat(rat *big.Rat, mode string) *big.Int {
	num := rat.Num()
	den := rat.Denom()
	quo := new(big.Int)
	rem := new(big.Int)
	quo.QuoRem(num, den, rem)
	if rem.Sign() == 0 {
		return quo
	}
	negative := rat.Sign() < 0
	switch mode {
	case "floor":
		if negative {
			quo.Sub(quo, big.NewInt(1))
		}
	case "ceil":
		if !negative {
			quo.Add(quo, big.NewInt(1))
		}
	case "truncate":
	}
	return quo
}

func (s *State) rationalRoundingMethod(mode string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) > 1 {
			return Nil, argumentCountError(s, len(args), "0..1")
		}
		rat, ok := rationalOf(self)
		if !ok {
			return Nil, nil
		}
		if len(args) == 0 {
			return intValueFromBig(integerFromRat(rat, mode)), nil
		}
		if args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		digits := int(args[0].Int())
		scaled := scaleRat(rat, digits)
		integer := integerFromRat(scaled, mode)
		if digits > 0 {
			denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
			return s.rationalValue(new(big.Rat).SetFrac(integer, denominator)), nil
		}
		if digits < 0 {
			integer.Mul(integer, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-digits)), nil))
		}
		return intValueFromBig(integer), nil
	}
}

func (s *State) rationalRoundMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	half := "up"
	if !kw.IsNil() {
		if data, ok := hashDataOf(kw); ok {
			for _, entry := range data.entries {
				if entry.key.Kind() == KindSymbol && entry.key.String() == "half" {
					if entry.value.IsNil() {
						half = "up"
					} else {
						half = entry.value.ToS()
					}
					if half != "up" && half != "even" && half != "down" {
						return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid rounding mode")}
					}
				}
			}
		}
	}
	rat, ok := rationalOf(self)
	if !ok {
		return Nil, nil
	}
	roundScaled := func(r *big.Rat) *big.Int {
		num := new(big.Int).Set(r.Num())
		den := new(big.Int).Set(r.Denom())
		quo := new(big.Int)
		rem := new(big.Int)
		quo.QuoRem(num, den, rem)
		if rem.Sign() == 0 {
			return quo
		}
		negative := r.Sign() < 0
		rem.Abs(rem)
		twice := new(big.Int).Mul(rem, big.NewInt(2))
		compare := twice.Cmp(den)
		up := compare > 0
		if compare == 0 {
			switch half {
			case "down":
				up = false
			case "even":
				up = quo.Bit(0) != 0
			default:
				up = true
			}
		}
		if up {
			if negative {
				quo.Sub(quo, big.NewInt(1))
			} else {
				quo.Add(quo, big.NewInt(1))
			}
		}
		return quo
	}
	if len(args) == 0 {
		return intValueFromBig(roundScaled(rat)), nil
	}
	if args[0].Kind() != KindInt {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	digits := int(args[0].Int())
	scaled := scaleRat(rat, digits)
	integer := roundScaled(scaled)
	if digits > 0 {
		denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
		return s.rationalValue(new(big.Rat).SetFrac(integer, denominator)), nil
	}
	if digits < 0 {
		integer.Mul(integer, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-digits)), nil))
	}
	return intValueFromBig(integer), nil
}

// rationalBinaryOperator handles arithmetic when either side is a Rational.
func (s *State) rationalBinaryOperator(left Value, name string, right Value) (Value, bool, error) {
	leftRat, leftOK := rationalOf(left)
	rightRat, rightOK := rationalOf(right)
	if !leftOK && !rightOK {
		return Nil, false, nil
	}
	if left.Kind() == KindFloat || right.Kind() == KindFloat {
		lf, lok := numericRat(left)
		rf, rok := numericRat(right)
		if !lok || !rok {
			return Nil, true, s.operatorTypeError(right)
		}
		l, _ := lf.Float64()
		r, _ := rf.Float64()
		switch name {
		case "+":
			return FloatValue(l + r), true, nil
		case "-":
			return FloatValue(l - r), true, nil
		case "*":
			return FloatValue(l * r), true, nil
		case "/":
			if r == 0 {
				return Nil, true, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
			}
			return FloatValue(l / r), true, nil
		}
		return Nil, false, nil
	}
	if !leftOK {
		leftRat, leftOK = exactRat(left)
	}
	if !rightOK {
		rightRat, rightOK = exactRat(right)
	}
	if !leftOK || !rightOK {
		return Nil, false, nil
	}
	result := new(big.Rat)
	switch name {
	case "+":
		result.Add(leftRat, rightRat)
	case "-":
		result.Sub(leftRat, rightRat)
	case "*":
		result.Mul(leftRat, rightRat)
	case "/":
		if rightRat.Sign() == 0 {
			return Nil, true, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
		result.Quo(leftRat, rightRat)
	default:
		return Nil, false, nil
	}
	return s.rationalValue(result), true, nil
}

func rationalComponentString(value Value) string {
	if rat, ok := rationalOf(value); ok {
		return rat.Num().String() + "/" + rat.Denom().String()
	}
	return strings.TrimSpace(value.ToS())
}

var _ = fmt.Sprint
