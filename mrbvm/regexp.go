package mrbvm

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"
)

type regexpData struct {
	regexp *regexp.Regexp
	source string
	flags  string
}

type matchData struct {
	regexp *regexp.Regexp
	groups []string
	input  string
	start  int
	end    int
}

func (s *State) installRegexpClass() {
	class := s.RegexpClass
	class.classMethods["compile"] = &method{owner: class, name: "compile", native: s.regexpCompileMethod}
	class.classMethods["new"] = &method{owner: class, name: "new", native: s.regexpCompileMethod}
	class.classMethods["escape"] = &method{owner: class, name: "escape", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Regexp.escape expects a string")}
		}
		return StringValue(regexp.QuoteMeta(args[0].ToS())), nil
	}}
	class.define("source", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := regexpOf(self); ok {
			return StringValue(data.source), nil
		}
		return Nil, nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := regexpOf(self); ok {
			return StringValue(data.source), nil
		}
		return Nil, nil
	})
	class.define("inspect", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := regexpOf(self); ok {
			return StringValue("/" + data.source + "/"), nil
		}
		return Nil, nil
	})
	class.define("match?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		data, ok := regexpOf(self)
		return BoolValue(ok && data.regexp.MatchString(args[0].ToS())), nil
	})
	class.define("===", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return False, nil
		}
		data, ok := regexpOf(self)
		return BoolValue(ok && data.regexp.MatchString(args[0].ToS())), nil
	})
	class.define("=~", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, nil
		}
		data, ok := regexpOf(self)
		if !ok {
			return Nil, nil
		}
		return IntValue(int64(regexpIndex(data.regexp, args[0].ToS()))), nil
	})
	class.define("match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, nil
		}
		data, ok := regexpOf(self)
		if !ok {
			return Nil, nil
		}
		result := s.newMatchData(data.regexp, args[0].ToS())
		if !block.IsNil() && !result.IsNil() {
			return s.callBlock(ctx, block, []Value{result}, Nil)
		}
		return result, nil
	})
}

func (s *State) regexpCompileMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "Regexp.compile expects a pattern")}
	}
	source := args[0].ToS()
	flags := ""
	if len(args) > 1 {
		flags = args[1].ToS()
	}
	pattern := source
	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	if strings.Contains(flags, "m") {
		pattern = "(?m)" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RegexpErrorClass, err.Error())}
	}
	return ObjectValue(&Object{class: s.RegexpClass, data: &regexpData{regexp: compiled, source: source, flags: flags}}), nil
}

func regexpOf(value Value) (*regexpData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*regexpData)
	return data, ok
}

func (s *State) newMatchData(compiled *regexp.Regexp, input string) Value {
	location := compiled.FindStringIndex(input)
	if location == nil {
		return Nil
	}
	groups := compiled.FindStringSubmatch(input)
	return ObjectValue(&Object{class: s.MatchDataClass, data: &matchData{
		regexp: compiled,
		groups: groups,
		input:  input,
		start:  location[0],
		end:    location[1],
	}})
}

func regexpIndex(compiled *regexp.Regexp, input string) int {
	location := compiled.FindStringIndex(input)
	if location == nil {
		return -1
	}
	return utf8.RuneCountInString(input[:location[0]])
}

func matchDataOf(value Value) (*matchData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*matchData)
	return data, ok
}

func (s *State) installMatchDataClass() {
	class := s.MatchDataClass
	class.define("[]", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok || len(args) != 1 {
			return Nil, nil
		}
		if args[0].Kind() == KindInt {
			index := int(args[0].Int())
			if index < 0 {
				index += len(data.groups)
			}
			if index < 0 || index >= len(data.groups) {
				return Nil, nil
			}
			return StringValue(data.groups[index]), nil
		}
		index := data.regexp.SubexpIndex(args[0].ToS())
		if index < 0 || index >= len(data.groups) {
			return Nil, nil
		}
		return StringValue(data.groups[index]), nil
	})
	class.define("to_a", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		values := make([]Value, len(data.groups))
		for i, group := range data.groups {
			values[i] = StringValue(group)
		}
		return ArrayValue(values), nil
	})
	class.define("captures", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		var values []Value
		for _, group := range data.groups[1:] {
			values = append(values, StringValue(group))
		}
		return ArrayValue(values), nil
	})
	class.define("pre_match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return StringValue(data.input[:data.start]), nil
		}
		return Nil, nil
	})
	class.define("post_match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return StringValue(data.input[data.end:]), nil
		}
		return Nil, nil
	})
}
