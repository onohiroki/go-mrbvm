package mrbvm

import (
	"fmt"
	"math"
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

// FloatValue returns a floating-point value.
func FloatValue(v float64) Value {
	return Value{kind: KindFloat, bits: math.Float64bits(v)}
}

// SymbolValue returns a symbol value.
func SymbolValue(name string) Value {
	return Value{kind: KindSymbol, obj: &Object{data: &stringData{value: name}}}
}

// StringValue returns a string value.
func StringValue(v string) Value {
	return Value{kind: KindString, obj: &Object{data: &stringData{value: v}}}
}

// ObjectValue returns an object value.
func ObjectValue(v *Object) Value {
	return Value{kind: KindObject, obj: v}
}

// ArrayValue returns an array value.
func ArrayValue(values []Value) Value {
	return Value{kind: KindObject, obj: &Object{data: &arrayData{values: values}}}
}

// HashValue returns a hash value.
func HashValue(entries []hashEntry) Value {
	return Value{kind: KindObject, obj: &Object{data: &hashData{entries: entries}}}
}

// RangeValue returns a range value.
func RangeValue(low, high Value, exclusive bool) Value {
	return Value{kind: KindObject, obj: &Object{data: &rangeData{low: low, high: high, exclusive: exclusive}}}
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
			return s.value
		}
	}
	return v.Inspect()
}

// Inspect returns a debug representation.
func (v Value) Inspect() string {
	switch v.kind {
	case KindNil:
		return "nil"
	case KindFalse:
		return "false"
	case KindTrue:
		return "true"
	case KindInt:
		return strconv.FormatInt(v.Int(), 10)
	case KindFloat:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
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
				parts := make([]string, len(data.values))
				for i, item := range data.values {
					parts[i] = item.Inspect()
				}
				return "[" + strings.Join(parts, ", ") + "]"
			case *hashData:
				parts := make([]string, len(data.entries))
				for i, entry := range data.entries {
					if entry.key.Kind() == KindSymbol {
						parts[i] = entry.key.String() + ": " + entry.value.Inspect()
					} else {
						parts[i] = entry.key.Inspect() + " => " + entry.value.Inspect()
					}
				}
				return "{" + strings.Join(parts, ", ") + "}"
			case *rangeData:
				op := ".."
				if data.exclusive {
					op = "..."
				}
				return data.low.Inspect() + op + data.high.Inspect()
			case *blockData:
				return "#<Proc>"
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
