package mrbvm

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Kind identifies the runtime type of a Value.
type Kind uint8

const (
	KindNil Kind = iota
	KindFalse
	KindTrue
	KindInt
	KindBigInt
	KindFloat
	KindSymbol
	KindString
	KindObject
)

// Value is an mruby value.
//
// The initial implementation uses a GC-friendly tagged struct. Floating-point
// values are stored as their IEEE-754 bits. Heap objects are stored in obj so
// the Go garbage collector can trace them.
type Value struct {
	kind Kind
	bits uint64
	obj  *Object
}

// Nil is the Ruby nil value.
var Nil = Value{kind: KindNil}

// True is the Ruby true value.
var True = Value{kind: KindTrue}

// False is the Ruby false value.
var False = Value{kind: KindFalse}

// IntValue returns an integer value.
func IntValue(v int64) Value {
	return Value{kind: KindInt, bits: uint64(v)}
}

// BoolValue returns true or false.
func BoolValue(v bool) Value {
	if v {
		return True
	}
	return False
}

// BigIntValue returns an arbitrary-precision integer value.
func BigIntValue(v *big.Int) Value {
	return Value{kind: KindBigInt, obj: &Object{data: &bigIntData{value: v}}}
}

// BigInt returns the arbitrary-precision payload.
func (v Value) BigInt() (*big.Int, bool) {
	if v.kind == KindBigInt && v.obj != nil {
		if data, ok := v.obj.data.(*bigIntData); ok {
			return data.value, true
		}
	}
	return nil, false
}

// FloatValue returns a floating-point value.
func FloatValue(v float64) Value {
	return Value{kind: KindFloat, bits: math.Float64bits(v)}
}

// inlineStringObject keeps an Object and its immediate string payload in a
// single allocation. Object.data points back into the same allocation.
type inlineStringObject struct {
	obj  Object
	data stringData
}

func newStringObject(kind Kind, value string) Value {
	holder := &inlineStringObject{data: stringData{raw: value}}
	holder.obj.data = &holder.data
	return Value{kind: kind, obj: &holder.obj}
}

// SymbolValue returns a symbol value.
func SymbolValue(name string) Value {
	return newStringObject(KindSymbol, name)
}

// StringValue returns a string value.
func StringValue(v string) Value {
	return newStringObject(KindString, v)
}

// ObjectValue returns an object value.
func ObjectValue(v *Object) Value {
	return Value{kind: KindObject, obj: v}
}

type inlineArrayObject struct {
	obj  Object
	data arrayData
}

// ArrayValue returns an array value.
func ArrayValue(values []Value) Value {
	holder := &inlineArrayObject{data: arrayData{values: values}}
	holder.obj.data = &holder.data
	return Value{kind: KindObject, obj: &holder.obj}
}

type inlineHashObject struct {
	obj  Object
	data hashData
}

// HashValue returns a hash value.
func HashValue(entries []hashEntry) Value {
	holder := &inlineHashObject{data: hashData{entries: entries}}
	holder.obj.data = &holder.data
	return Value{kind: KindObject, obj: &holder.obj}
}

type inlineBlockObject struct {
	obj  Object
	data blockData
}

// BlockValue returns a block value in a single allocation.
func BlockValue(block blockData) Value {
	holder := &inlineBlockObject{data: block}
	holder.obj.data = &holder.data
	return Value{kind: KindObject, obj: &holder.obj}
}

type inlineRangeObject struct {
	obj  Object
	data rangeData
}

// RangeValue returns a range value.
func RangeValue(low, high Value, exclusive bool) Value {
	holder := &inlineRangeObject{data: rangeData{low: low, high: high, exclusive: exclusive}}
	holder.obj.data = &holder.data
	return Value{kind: KindObject, obj: &holder.obj}
}

// Kind returns the value's runtime kind.
func (v Value) Kind() Kind { return v.kind }

// IsNil reports whether v is nil.
func (v Value) IsNil() bool { return v.kind == KindNil }

// IsTruthy reports whether v is truthy in Ruby (everything except nil/false).
func (v Value) IsTruthy() bool { return v.kind != KindNil && v.kind != KindFalse }

// Int returns the integer payload.
func (v Value) Int() int64 { return int64(v.bits) }

// Float returns the float payload.
func (v Value) Float() float64 { return math.Float64frombits(v.bits) }

// formatFloat renders a float the way mruby does: 15 significant digits,
// "Infinity"/"-Infinity"/"NaN", and always with a visible decimal point.
func formatFloat(value float64) string {
	if math.IsInf(value, 1) {
		return "Infinity"
	}
	if math.IsInf(value, -1) {
		return "-Infinity"
	}
	if math.IsNaN(value) {
		return "NaN"
	}
	text := strconv.FormatFloat(value, 'g', 15, 64)
	for i := 0; i < len(text); i++ {
		if text[i] == '.' {
			return text
		}
		if text[i] == 'e' {
			return text[:i] + ".0" + text[i:]
		}
	}
	return text + ".0"
}

// ToS returns Ruby's to_s representation.
func (v Value) ToS() string {
	switch v.kind {
	case KindNil:
		return ""
	case KindSymbol:
		return v.String()
	case KindString:
		return v.String()
	default:
		return v.Inspect()
	}
}

// String returns the string or symbol payload.
func (v Value) String() string {
	if (v.kind == KindString || v.kind == KindSymbol) && v.obj != nil {
		if s, ok := v.obj.data.(*stringData); ok {
			return s.str()
		}
	}
	return v.Inspect()
}

// Inspect returns a debug representation.
func (v Value) Inspect() string {
	return inspectValue(v, make(map[*Object]bool))
}

func inspectValue(v Value, seen map[*Object]bool) string {
	switch v.kind {
	case KindNil:
		return "nil"
	case KindFalse:
		return "false"
	case KindTrue:
		return "true"
	case KindInt:
		return strconv.FormatInt(v.Int(), 10)
	case KindBigInt:
		if value, ok := v.BigInt(); ok {
			return value.String()
		}
		return "0"
	case KindFloat:
		return formatFloat(v.Float())
	case KindSymbol:
		return ":" + v.String()
	case KindString:
		return strconv.Quote(v.String())
	case KindObject:
		if v.obj != nil {
			if c, ok := v.obj.data.(*Class); ok {
				return c.name
			}
			switch data := v.obj.data.(type) {
			case *arrayData:
				if seen[v.obj] {
					return "[...]"
				}
				seen[v.obj] = true
				parts := make([]string, len(data.values))
				for i, item := range data.values {
					parts[i] = inspectValue(item, seen)
				}
				delete(seen, v.obj)
				return "[" + strings.Join(parts, ", ") + "]"
			case *hashData:
				if seen[v.obj] {
					return "{...}"
				}
				seen[v.obj] = true
				parts := make([]string, len(data.entries))
				for i, entry := range data.entries {
					if entry.key.Kind() == KindSymbol {
						parts[i] = entry.key.String() + ": " + inspectValue(entry.value, seen)
					} else {
						parts[i] = inspectValue(entry.key, seen) + " => " + inspectValue(entry.value, seen)
					}
				}
				delete(seen, v.obj)
				return "{" + strings.Join(parts, ", ") + "}"
			case *rangeData:
				op := ".."
				if data.exclusive {
					op = "..."
				}
				return inspectValue(data.low, seen) + op + inspectValue(data.high, seen)
			case *blockData:
				return "#<Proc>"
			case *enumeratorData:
				return "#<Enumerator>"
			case *timeData:
				return data.value.Format("2006-01-02 15:04:05 -0700")
			}
			if v.obj.class != nil {
				return fmt.Sprintf("#<%s>", v.obj.class.name)
			}
		}
		return "#<Object>"
	default:
		return "<unknown>"
	}
}
