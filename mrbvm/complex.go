package mrbvm

import (
	"context"
	"math"
	"math/big"
	"math/cmplx"
	"strings"
)

type complexData struct {
	real Value
	imag Value
}

func complexOf(value Value) (*complexData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*complexData)
	return data, ok
}

func isComplexObject(value Value) bool {
	_, ok := complexOf(value)
	return ok
}

func (s *State) complexValue(real, imag Value) Value {
	value := ObjectValue(&Object{data: &complexData{real: real, imag: imag}})
	value.obj.class = s.ComplexClass
	value.obj.frozen = true
	return value
}

func (s *State) installComplexLibrary() {
	class := s.ComplexClass
	class.classMethods["new"] = &method{owner: class, name: "new", native: s.kernelComplexMethod}
	class.classMethods["polar"] = &method{owner: class, name: "polar", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 2 {
			return Nil, argumentCountError(s, len(args), "2")
		}
		radius, ok := numericFloat(args[0])
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Float")}
		}
		theta, ok := numericFloat(args[1])
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't convert into Float")}
		}
		return s.complexValue(FloatValue(radius*math.Cos(theta)), FloatValue(radius*math.Sin(theta))), nil
	}}
	class.classMethods["rectangular"] = &method{owner: class, name: "rectangular", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 || len(args) > 2 {
			return Nil, argumentCountError(s, len(args), "1..2")
		}
		imag := Value(IntValue(0))
		if len(args) == 2 {
			imag = args[1]
		}
		return s.complexValue(args[0], imag), nil
	}}
	class.define("real", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := complexOf(self); ok {
			return data.real, nil
		}
		return Nil, nil
	})
	class.define("imaginary", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := complexOf(self); ok {
			return data.imag, nil
		}
		return Nil, nil
	})
	class.define("rectangular", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := complexOf(self); ok {
			return ArrayValue([]Value{data.real, data.imag}), nil
		}
		return ArrayValue(nil), nil
	})
	class.define("polar", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		real, _ := numericFloat(data.real)
		imag, _ := numericFloat(data.imag)
		return ArrayValue([]Value{FloatValue(math.Hypot(real, imag)), FloatValue(math.Atan2(imag, real))}), nil
	})
	class.define("conjugate", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		return s.complexValue(data.real, s.complexNegatePart(data.imag)), nil
	})
	class.define("conj", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		return s.complexValue(data.real, s.complexNegatePart(data.imag)), nil
	})
	class.define("abs", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		real, _ := numericFloat(data.real)
		imag, _ := numericFloat(data.imag)
		return FloatValue(math.Hypot(real, imag)), nil
	})
	class.define("abs2", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		realSquare, err := s.complexPartOp(data.real, "*", data.real)
		if err != nil {
			return Nil, err
		}
		imagSquare, err := s.complexPartOp(data.imag, "*", data.imag)
		if err != nil {
			return Nil, err
		}
		return s.complexPartOp(realSquare, "+", imagSquare)
	})
	class.define("arg", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		real, _ := numericFloat(data.real)
		imag, _ := numericFloat(data.imag)
		return FloatValue(math.Atan2(imag, real)), nil
	})
	class.define("real?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return False, nil
	})
	class.define("to_c", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return self, nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok || !s.complexImagZero(data.imag) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "can't convert into Float")}
		}
		f, _ := numericFloat(data.real)
		return FloatValue(f), nil
	})
	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok || !s.complexImagZero(data.imag) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "can't convert into Integer")}
		}
		return s.call(ctx, data.real, "to_i", nil, Nil, Nil)
	})
	class.define("fdiv", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		divisor, ok := numericFloat(args[0])
		if !ok || divisor == 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
		real, _ := numericFloat(data.real)
		imag, _ := numericFloat(data.imag)
		return s.complexValue(FloatValue(real/divisor), FloatValue(imag/divisor)), nil
	})
	class.define("==", s.complexEqualMethod)
	class.define("eql?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		left, leftOK := complexOf(self)
		right, rightOK := complexOf(args[0])
		if !leftOK || !rightOK {
			return False, nil
		}
		return BoolValue(eqlValues(left.real, right.real) && eqlValues(left.imag, right.imag)), nil
	})
	class.define("hash", s.complexHashMethod)
	class.define("+", s.complexArithmeticMethod("+"))
	class.define("-", s.complexArithmeticMethod("-"))
	class.define("*", s.complexArithmeticMethod("*"))
	class.define("/", s.complexArithmeticMethod("/"))
	class.define("-@", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := complexOf(self)
		if !ok {
			return Nil, nil
		}
		return s.complexValue(s.complexNegatePart(data.real), s.complexNegatePart(data.imag)), nil
	})
	class.define("**", s.complexPowMethod)
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(s.complexString(self, false)), nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return StringValue(s.complexString(self, true)), nil
	})
	class.define("frozen?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return True, nil
	})

	s.ObjectClass.define("Complex", s.kernelComplexMethod)
	s.KernelClass.classMethods["Complex"] = &method{owner: s.KernelClass, name: "Complex", native: s.kernelComplexMethod}
	s.NilClass.define("to_c", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.complexValue(IntValue(0), IntValue(0)), nil
	})
	for _, class := range []*Class{s.IntegerClass, s.FloatClass, s.RationalClass} {
		class.define("to_c", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
			return s.complexValue(self, IntValue(0)), nil
		})
	}
	s.IntegerClass.define("i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.complexValue(IntValue(0), self), nil
	})
}

func numericFloat(value Value) (float64, bool) {
	switch value.Kind() {
	case KindInt:
		return float64(value.Int()), true
	case KindFloat:
		return value.Float(), true
	case KindBigInt:
		integer, _ := value.BigInt()
		f, _ := new(big.Float).SetInt(integer).Float64()
		return f, true
	}
	if rat, ok := rationalOf(value); ok {
		f, _ := rat.Float64()
		return f, true
	}
	return 0, false
}

func (s *State) kernelComplexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if len(args) == 1 {
		if data, ok := complexOf(args[0]); ok {
			return s.complexValue(data.real, data.imag), nil
		}
		return s.complexValue(args[0], IntValue(0)), nil
	}
	return s.complexValue(args[0], args[1]), nil
}

func (s *State) complexNegatePart(value Value) Value {
	if rat, ok := rationalOf(value); ok {
		return s.rationalValue(new(big.Rat).Neg(rat))
	}
	switch value.Kind() {
	case KindInt:
		if value.Int() == math.MinInt64 {
			return BigIntValue(new(big.Int).Neg(big.NewInt(value.Int())))
		}
		return IntValue(-value.Int())
	case KindBigInt:
		integer, _ := value.BigInt()
		return BigIntValue(new(big.Int).Neg(integer))
	case KindFloat:
		return FloatValue(-value.Float())
	}
	return value
}

func (s *State) complexImagZero(value Value) bool {
	return complexValueImagZero(value)
}

func complexValueImagZero(value Value) bool {
	if value.Kind() == KindInt {
		return value.Int() == 0
	}
	if value.Kind() == KindFloat {
		return value.Float() == 0
	}
	if rat, ok := rationalOf(value); ok {
		return rat.Sign() == 0
	}
	return false
}

func (s *State) complexEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	left, ok := complexOf(self)
	if !ok {
		return False, nil
	}
	if right, rightOK := complexOf(args[0]); rightOK {
		return BoolValue(valuesEqual(left.real, right.real) && valuesEqual(left.imag, right.imag)), nil
	}
	if !s.complexImagZero(left.imag) {
		return False, nil
	}
	return BoolValue(valuesEqual(left.real, args[0])), nil
}

func (s *State) complexPartOp(left Value, name string, right Value) (Value, error) {
	return s.numericBinaryOperator(left, name, []Value{right})
}

func (s *State) complexArithmeticValues(leftReal, leftImag, rightReal, rightImag Value, op string) (Value, Value, error) {
	switch op {
	case "+":
		real, err := s.complexPartOp(leftReal, "+", rightReal)
		if err != nil {
			return Nil, Nil, err
		}
		imag, err := s.complexPartOp(leftImag, "+", rightImag)
		return real, imag, err
	case "-":
		real, err := s.complexPartOp(leftReal, "-", rightReal)
		if err != nil {
			return Nil, Nil, err
		}
		imag, err := s.complexPartOp(leftImag, "-", rightImag)
		return real, imag, err
	case "*":
		ac, err := s.complexPartOp(leftReal, "*", rightReal)
		if err != nil {
			return Nil, Nil, err
		}
		bd, err := s.complexPartOp(leftImag, "*", rightImag)
		if err != nil {
			return Nil, Nil, err
		}
		ad, err := s.complexPartOp(leftReal, "*", rightImag)
		if err != nil {
			return Nil, Nil, err
		}
		bc, err := s.complexPartOp(leftImag, "*", rightReal)
		if err != nil {
			return Nil, Nil, err
		}
		real, err := s.complexPartOp(ac, "-", bd)
		if err != nil {
			return Nil, Nil, err
		}
		imag, err := s.complexPartOp(ad, "+", bc)
		return real, imag, err
	}
	return Nil, Nil, nil
}

func (s *State) complexArithmetic(left Value, op string, right Value) (Value, error) {
	leftReal, leftImag := left, Value(IntValue(0))
	if data, ok := complexOf(left); ok {
		leftReal, leftImag = data.real, data.imag
	}
	rightReal, rightImag := right, Value(IntValue(0))
	if data, ok := complexOf(right); ok {
		rightReal, rightImag = data.real, data.imag
	}
	if op == "/" {
		ar, _ := numericFloat(leftReal)
		ai, _ := numericFloat(leftImag)
		br, _ := numericFloat(rightReal)
		bi, _ := numericFloat(rightImag)
		if br == 0 && bi == 0 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
		}
		// Scale by the larger component so a quotient whose parts overflow
		// the square of the denominator still comes out finite.
		if math.Abs(br) >= math.Abs(bi) {
			ratio := bi / br
			denominator := br + ratio*bi
			return s.complexValue(FloatValue((ar+ai*ratio)/denominator), FloatValue((ai-ar*ratio)/denominator)), nil
		}
		ratio := br / bi
		denominator := bi + ratio*br
		return s.complexValue(FloatValue((ar*ratio+ai)/denominator), FloatValue((ai*ratio-ar)/denominator)), nil
	}
	real, imag, err := s.complexArithmeticValues(leftReal, leftImag, rightReal, rightImag, op)
	if err != nil {
		return Nil, err
	}
	return s.complexValue(real, imag), nil
}

func (s *State) complexArithmeticMethod(op string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		return s.complexArithmetic(self, op, args[0])
	}
}

func (s *State) complexBinaryOperator(left Value, name string, right Value) (Value, bool, error) {
	if !isComplexObject(left) && !isComplexObject(right) {
		return Nil, false, nil
	}
	switch name {
	case "+", "-", "*", "/":
		if isComplexObject(left) && !isComplexObject(right) {
			if _, ok := numericFloat(right); !ok {
				if _, exact := exactRat(right); !exact {
					return Nil, false, nil
				}
			}
		}
		result, err := s.complexArithmetic(left, name, right)
		return result, true, err
	}
	return Nil, false, nil
}

func cpow(base, exponent complex128) complex128 {
	return cmplx.Pow(base, exponent)
}

func (s *State) complexPowMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	data, ok := complexOf(self)
	if !ok {
		return Nil, nil
	}
	exponent := args[0]
	if exponent.Kind() == KindInt && exponent.Int() >= 0 {
		result := s.complexValue(IntValue(1), IntValue(0))
		var err error
		for i := int64(0); i < exponent.Int(); i++ {
			result, err = s.complexArithmetic(result, "*", self)
			if err != nil {
				return Nil, err
			}
		}
		return result, nil
	}
	if exp, ok := complexOf(exponent); ok {
		if s.complexImagZero(exp.imag) {
			if exp.real.Kind() == KindInt {
				if exp.real.Int() >= 0 {
					return s.complexPowMethod(ctx, self, []Value{exp.real}, kw, block)
				}
			}
		}
	}
	if exponent.Kind() == KindInt && exponent.Int() < 0 {
		pos, err := s.complexPowMethod(ctx, self, []Value{IntValue(-exponent.Int())}, kw, block)
		if err != nil {
			return Nil, err
		}
		return s.complexArithmetic(s.complexValue(IntValue(1), IntValue(0)), "/", pos)
	}
	ar, _ := numericFloat(data.real)
	ai, _ := numericFloat(data.imag)
	br, bi := 0.0, 0.0
	if exp, ok := complexOf(exponent); ok {
		br, _ = numericFloat(exp.real)
		bi, _ = numericFloat(exp.imag)
	} else {
		br, _ = numericFloat(exponent)
	}
	result := cmplx.Pow(complex(ar, ai), complex(br, bi))
	return s.complexValue(FloatValue(complexReal(result)), FloatValue(complexImag(result))), nil
}

func complexAbs(value complex128) float64 {
	return cmplx.Abs(value)
}

func complexReal(value complex128) float64 { return real(value) }
func complexImag(value complex128) float64 { return imag(value) }

func (s *State) complexString(self Value, inspect bool) string {
	data, ok := complexOf(self)
	if !ok {
		return "(0+0i)"
	}
	realText := data.real.ToS()
	imagText := data.imag.ToS()
	negative := strings.HasPrefix(imagText, "-")
	if negative {
		imagText = imagText[1:]
	}
	if inspect {
		realText = data.real.Inspect()
		imagText = data.imag.Inspect()
		if negative {
			if strings.HasPrefix(imagText, "-") {
				imagText = imagText[1:]
			} else if strings.HasPrefix(imagText, "(-") {
				imagText = "(" + imagText[2:]
			}
		}
	}
	sep := "+"
	if negative {
		sep = "-"
	}
	suffix := ""
	if len(imagText) > 0 {
		last := imagText[len(imagText)-1]
		if last < '0' || last > '9' {
			suffix = "*"
		}
	}
	text := realText + sep + imagText + suffix + "i"
	if inspect {
		return "(" + text + ")"
	}
	return text
}

func complexPartHash(value Value) int64 {
	switch value.Kind() {
	case KindInt:
		return hashValue(value, map[*Object]bool{})*31 + 1
	case KindBigInt:
		return hashValue(value, map[*Object]bool{})*31 + 2
	case KindFloat:
		f := value.Float()
		if f == 0 {
			f = 0
		}
		return int64(math.Float64bits(f))*31 + 3
	}
	if rat, ok := rationalOf(value); ok {
		return int64(hashValue(StringValue("rational:"+rat.Num().String()+"/"+rat.Denom().String()), map[*Object]bool{}))*31 + 4
	}
	return hashValue(value, map[*Object]bool{})*31 + 5
}

func (s *State) complexHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := complexOf(self)
	if !ok {
		return IntValue(0), nil
	}
	h := complexPartHash(data.real)*31 + complexPartHash(data.imag)
	return IntValue(h), nil
}
