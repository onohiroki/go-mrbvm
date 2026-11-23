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
	regexp  *regexp.Regexp
	groups  []string
	indices []int
	input   string
	start   int
	end     int
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
	class.define("options", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := regexpOf(self); ok {
			options := int64(0)
			if strings.Contains(data.flags, "i") {
				options |= 1
			}
			if strings.Contains(data.flags, "x") {
				options |= 2
			}
			if strings.Contains(data.flags, "m") {
				options |= 4
			}
			return IntValue(options), nil
		}
		return IntValue(0), nil
	})
	class.define("match?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 || len(args) > 2 {
			return False, nil
		}
		data, ok := regexpOf(self)
		if !ok {
			return False, nil
		}
		input := args[0].ToS()
		byteStart := 0
		if len(args) == 2 && args[1].Kind() == KindInt {
			position := int(args[1].Int())
			count := stringCharCount(input)
			if position < 0 {
				position += count
			}
			if position < 0 || position > count {
				return False, nil
			}
			byteStart, _ = stringCharIndexToByte(input, position)
		}
		return BoolValue(data.regexp.MatchString(input[byteStart:])), nil
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
		if len(args) < 1 || len(args) > 2 {
			return Nil, nil
		}
		data, ok := regexpOf(self)
		if !ok {
			return Nil, nil
		}
		input := args[0].ToS()
		byteStart := 0
		if len(args) == 2 {
			if args[1].Kind() != KindInt {
				return Nil, nil
			}
			position := int(args[1].Int())
			count := stringCharCount(input)
			if position < 0 {
				position += count
			}
			if position < 0 || position > count {
				return Nil, nil
			}
			byteStart, _ = stringCharIndexToByte(input, position)
		}
		result := s.newMatchDataAt(data.regexp, input, byteStart)
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
	if strings.Contains(flags, "x") {
		pattern = extendedPattern(pattern)
	}
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

func extendedPattern(pattern string) string {
	var builder strings.Builder
	inClass := false
	escaped := false
	comment := false
	for _, char := range pattern {
		if comment {
			if char == '\n' {
				comment = false
			}
			continue
		}
		if escaped {
			builder.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			builder.WriteRune(char)
			escaped = true
			continue
		}
		if char == '[' {
			inClass = true
		} else if char == ']' {
			inClass = false
		}
		if !inClass {
			if char == '#' {
				comment = true
				continue
			}
			if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
				continue
			}
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func regexpOf(value Value) (*regexpData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*regexpData)
	return data, ok
}

func (s *State) newMatchData(compiled *regexp.Regexp, input string) Value {
	return s.newMatchDataAt(compiled, input, 0)
}

func (s *State) newMatchDataAt(compiled *regexp.Regexp, input string, byteStart int) Value {
	if byteStart < 0 || byteStart > len(input) {
		return Nil
	}
	location := compiled.FindStringIndex(input[byteStart:])
	if location == nil {
		return Nil
	}
	groups := compiled.FindStringSubmatch(input[byteStart:])
	indices := compiled.FindStringSubmatchIndex(input[byteStart:])
	for i := range indices {
		if indices[i] >= 0 {
			indices[i] += byteStart
		}
	}
	return ObjectValue(&Object{class: s.MatchDataClass, data: &matchData{
		regexp:  compiled,
		groups:  groups,
		indices: indices,
		input:   input,
		start:   location[0] + byteStart,
		end:     location[1] + byteStart,
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
	class.define("names", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		var values []Value
		for _, name := range data.regexp.SubexpNames() {
			if name != "" {
				values = append(values, SymbolValue(name))
			}
		}
		return ArrayValue(values), nil
	})
	class.define("named_captures", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return HashValue(nil), nil
		}
		var entries []hashEntry
		for index, name := range data.regexp.SubexpNames() {
			if name == "" || index >= len(data.groups) {
				continue
			}
			entries = append(entries, hashEntry{key: StringValue(name), value: StringValue(data.groups[index])})
		}
		return HashValue(entries), nil
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
	class.define("begin", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		group := 0
		if len(args) > 0 && args[0].Kind() == KindInt {
			group = int(args[0].Int())
		}
		if group < 0 {
			group += len(data.groups)
		}
		if group < 0 || group*2+1 >= len(data.indices) || data.indices[group*2] < 0 {
			return Nil, nil
		}
		return IntValue(int64(stringByteIndexToChar(data.input, data.indices[group*2]))), nil
	})
	class.define("end", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		group := 0
		if len(args) > 0 && args[0].Kind() == KindInt {
			group = int(args[0].Int())
		}
		if group < 0 {
			group += len(data.groups)
		}
		if group < 0 || group*2+1 >= len(data.indices) || data.indices[group*2+1] < 0 {
			return Nil, nil
		}
		return IntValue(int64(stringByteIndexToChar(data.input, data.indices[group*2+1]))), nil
	})
	class.define("offset", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		group := 0
		if len(args) > 0 && args[0].Kind() == KindInt {
			group = int(args[0].Int())
		}
		if group < 0 {
			group += len(data.groups)
		}
		if group < 0 || group*2+1 >= len(data.indices) || data.indices[group*2] < 0 {
			return ArrayValue([]Value{IntValue(-1), IntValue(-1)}), nil
		}
		beginIndex := stringByteIndexToChar(data.input, data.indices[group*2])
		endIndex := stringByteIndexToChar(data.input, data.indices[group*2+1])
		return ArrayValue([]Value{IntValue(int64(beginIndex)), IntValue(int64(endIndex))}), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok || len(data.groups) == 0 {
			return StringValue(""), nil
		}
		return StringValue(data.groups[0]), nil
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
