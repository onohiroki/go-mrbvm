package mrbvm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type timeData struct {
	value time.Time
}

func (s *State) installTimeClass() {
	class := s.TimeClass
	class.classMethods["now"] = &method{owner: class, name: "now", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.newTimeValue(time.Now()), nil
	}}
	class.classMethods["at"] = &method{owner: class, name: "at", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Time.at expects one argument")}
		}
		if args[0].Kind() == KindFloat {
			seconds := int64(args[0].Float())
			nanos := int64((args[0].Float() - float64(seconds)) * 1e9)
			return s.newTimeValue(time.Unix(seconds, nanos)), nil
		}
		return s.newTimeValue(time.Unix(args[0].Int(), 0)), nil
	}}

	class.define("to_i", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			return IntValue(data.value.Unix()), nil
		}
		return Nil, nil
	})
	class.define("to_f", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			return FloatValue(float64(data.value.UnixNano()) / 1e9), nil
		}
		return Nil, nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			return StringValue(data.value.Format("2006-01-02 15:04:05 -0700")), nil
		}
		return Nil, nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			return StringValue(data.value.Format("2006-01-02 15:04:05 -0700")), nil
		}
		return Nil, nil
	})
	class.define("year", timeComponent(func(t time.Time) int64 { return int64(t.Year()) }))
	class.define("month", timeComponent(func(t time.Time) int64 { return int64(t.Month()) }))
	class.define("mon", timeComponent(func(t time.Time) int64 { return int64(t.Month()) }))
	class.define("day", timeComponent(func(t time.Time) int64 { return int64(t.Day()) }))
	class.define("hour", timeComponent(func(t time.Time) int64 { return int64(t.Hour()) }))
	class.define("min", timeComponent(func(t time.Time) int64 { return int64(t.Minute()) }))
	class.define("sec", timeComponent(func(t time.Time) int64 { return int64(t.Second()) }))
	class.define("usec", timeComponent(func(t time.Time) int64 { return int64(t.Nanosecond() / 1000) }))
	class.define("wday", timeComponent(func(t time.Time) int64 { return int64(t.Weekday()) }))
	class.define("yday", timeComponent(func(t time.Time) int64 { return int64(t.YearDay()) }))
	class.define("utc?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			name, offset := data.value.Zone()
			return BoolValue(name == "UTC" && offset == 0), nil
		}
		return False, nil
	})
	class.define("strftime", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := timeOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		return StringValue(strftime(data.value, args[0].ToS())), nil
	})
	class.define("+", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := timeOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		seconds := args[0].Float()
		if args[0].Kind() == KindInt {
			seconds = float64(args[0].Int())
		}
		return s.newTimeValue(data.value.Add(time.Duration(seconds * float64(time.Second)))), nil
	})
	class.define("-", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := timeOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		if other, ok := timeOf(args[0]); ok {
			return FloatValue(data.value.Sub(other.value).Seconds()), nil
		}
		seconds := args[0].Float()
		if args[0].Kind() == KindInt {
			seconds = float64(args[0].Int())
		}
		return s.newTimeValue(data.value.Add(-time.Duration(seconds * float64(time.Second)))), nil
	})
	class.define("<=>", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := timeOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		other, ok := timeOf(args[0])
		if !ok {
			return Nil, nil
		}
		switch {
		case data.value.Before(other.value):
			return IntValue(-1), nil
		case data.value.After(other.value):
			return IntValue(1), nil
		default:
			return IntValue(0), nil
		}
	})
}

func (s *State) newTimeValue(value time.Time) Value {
	return ObjectValue(&Object{class: s.TimeClass, data: &timeData{value: value}})
}

func timeOf(value Value) (*timeData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*timeData)
	return data, ok
}

func timeComponent(fn func(time.Time) int64) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := timeOf(self); ok {
			return IntValue(fn(data.value)), nil
		}
		return Nil, nil
	}
}

func strftime(value time.Time, format string) string {
	var builder strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			builder.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			builder.WriteString(fmt.Sprintf("%04d", value.Year()))
		case 'y':
			builder.WriteString(fmt.Sprintf("%02d", value.Year()%100))
		case 'm':
			builder.WriteString(fmt.Sprintf("%02d", int(value.Month())))
		case 'd':
			builder.WriteString(fmt.Sprintf("%02d", value.Day()))
		case 'e':
			builder.WriteString(fmt.Sprintf("%2d", value.Day()))
		case 'H':
			builder.WriteString(fmt.Sprintf("%02d", value.Hour()))
		case 'M':
			builder.WriteString(fmt.Sprintf("%02d", value.Minute()))
		case 'S':
			builder.WriteString(fmt.Sprintf("%02d", value.Second()))
		case 'L':
			builder.WriteString(fmt.Sprintf("%03d", value.Nanosecond()/1e6))
		case 'N':
			builder.WriteString(fmt.Sprintf("%09d", value.Nanosecond()))
		case 'z':
			builder.WriteString(value.Format("-0700"))
		case 'Z':
			name, _ := value.Zone()
			builder.WriteString(name)
		case 'a':
			builder.WriteString(value.Format("Mon"))
		case 'A':
			builder.WriteString(value.Format("Monday"))
		case 'b':
			builder.WriteString(value.Format("Jan"))
		case 'B':
			builder.WriteString(value.Format("January"))
		case 'j':
			builder.WriteString(fmt.Sprintf("%03d", value.YearDay()))
		case 'p':
			if value.Hour() < 12 {
				builder.WriteString("AM")
			} else {
				builder.WriteString("PM")
			}
		case '%':
			builder.WriteByte('%')
		default:
			builder.WriteByte('%')
			builder.WriteByte(format[i])
		}
	}
	return builder.String()
}
