package mrbvm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

func (s *State) installJSONModule() {
	module := s.JSONClass
	module.classMethods["parse"] = &method{owner: module, name: "parse", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "JSON.parse expects a string")}
		}
		decoder := json.NewDecoder(bytes.NewBufferString(args[0].ToS()))
		decoder.UseNumber()
		var decoded any
		if err := decoder.Decode(&decoded); err != nil {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "JSON parse error: "+err.Error())}
		}
		return s.jsonToValue(decoded), nil
	}}
	module.classMethods["generate"] = &method{owner: module, name: "generate", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.jsonGenerate(args, false)
	}}
	module.classMethods["dump"] = &method{owner: module, name: "dump", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.jsonGenerate(args, false)
	}}
	module.classMethods["pretty_generate"] = &method{owner: module, name: "pretty_generate", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.jsonGenerate(args, true)
	}}
}

func (s *State) jsonGenerate(args []Value, pretty bool) (Value, error) {
	if len(args) != 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "JSON.generate expects one argument")}
	}
	native, err := s.valueToJSON(args[0])
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, err.Error())}
	}
	var data []byte
	if pretty {
		data, err = json.MarshalIndent(native, "", "  ")
	} else {
		data, err = json.Marshal(native)
	}
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, err.Error())}
	}
	return StringValue(string(data)), nil
}

func (s *State) jsonToValue(value any) Value {
	switch data := value.(type) {
	case nil:
		return Nil
	case bool:
		return BoolValue(data)
	case string:
		return StringValue(data)
	case json.Number:
		if integer, err := data.Int64(); err == nil {
			return IntValue(integer)
		}
		if number, err := data.Float64(); err == nil {
			return FloatValue(number)
		}
		return StringValue(data.String())
	case float64:
		return FloatValue(data)
	case []any:
		values := make([]Value, len(data))
		for i, item := range data {
			values[i] = s.jsonToValue(item)
		}
		return ArrayValue(values)
	case map[string]any:
		entries := make([]hashEntry, 0, len(data))
		for key, item := range data {
			entries = append(entries, hashEntry{key: StringValue(key), value: s.jsonToValue(item)})
		}
		return HashValue(entries)
	default:
		return StringValue(fmt.Sprint(data))
	}
}

func (s *State) valueToJSON(value Value) (any, error) {
	switch value.Kind() {
	case KindNil:
		return nil, nil
	case KindTrue:
		return true, nil
	case KindFalse:
		return false, nil
	case KindInt:
		return value.Int(), nil
	case KindBigInt:
		return json.Number(value.String()), nil
	case KindFloat:
		return value.Float(), nil
	case KindString:
		return value.String(), nil
	case KindSymbol:
		return value.String(), nil
	case KindObject:
		if value.obj == nil {
			return nil, nil
		}
		switch data := value.obj.data.(type) {
		case *arrayData:
			values := make([]any, len(data.values))
			for i, item := range data.values {
				converted, err := s.valueToJSON(item)
				if err != nil {
					return nil, err
				}
				values[i] = converted
			}
			return values, nil
		case *hashData:
			result := make(map[string]any, len(data.entries))
			for _, entry := range data.entries {
				key := entry.key.String()
				converted, err := s.valueToJSON(entry.value)
				if err != nil {
					return nil, err
				}
				result[key] = converted
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("JSON.generate cannot serialize %s", value.Inspect())
}
