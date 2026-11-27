package mrbvm

import (
	"context"
	"strings"
	"unicode/utf8"
)

// installStringRegexpLibrary adds the regexp-aware String and Symbol methods
// the mruby-regexp gem defines. It runs after the core String and Symbol
// libraries so that it can take the names over and delegate the non-regexp
// argument forms back to the captured implementations.
func (s *State) installStringRegexpLibrary() {
	class := s.StringClass
	class.define("[]", s.stringRegexpIndexMethod)
	class.define("slice", s.stringRegexpIndexMethod)
	class.define("slice!", s.stringRegexpSliceBangMethod)
	class.define("[]=", s.stringRegexpSetIndexMethod)
	class.define("match", s.stringRegexpMatchMethod)
	class.define("match?", s.stringRegexpMatchPredicateMethod)
	class.define("=~", s.stringRegexpMatchOperatorMethod)
	class.define("index", s.stringRegexpIndexSearchMethod(false, false))
	class.define("rindex", s.stringRegexpIndexSearchMethod(true, false))
	class.define("byteindex", s.stringRegexpIndexSearchMethod(false, true))
	class.define("byterindex", s.stringRegexpIndexSearchMethod(true, true))
	class.define("partition", s.stringRegexpPartitionMethod(false))
	class.define("rpartition", s.stringRegexpPartitionMethod(true))
	class.define("split", s.stringRegexpSplitMethod)
	s.installStringRegexpSubstitutionMethods()
	class.define("scan", s.stringRegexpScanMethod)
	class.define("start_with?", s.stringRegexpStartWithMethod)

	s.SymbolClass.define("match", s.symbolRegexpDelegate("match"))
	s.SymbolClass.define("match?", s.symbolRegexpDelegate("match?"))
	s.SymbolClass.define("=~", s.symbolRegexpDelegate("=~"))
}

// installStringRegexpSubstitutionMethods installs the substitution methods
// that accept both String and Regexp patterns. mruby's string.rb defines the
// same names for String patterns only, so this is also called after loading
// mrblib to keep the regexp-aware implementations authoritative.
func (s *State) installStringRegexpSubstitutionMethods() {
	class := s.StringClass
	class.define("sub", s.stringRegexpSubMethod)
	class.define("sub!", s.stringRegexpSubBangMethod)
	class.define("gsub", s.stringRegexpGsubMethod)
	class.define("gsub!", s.stringRegexpGsubBangMethod)
}

func (s *State) symbolRegexpDelegate(name string) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.call(ctx, StringValue(self.String()), name, args, kw, block)
	}
}

func regexpArgumentTypeName(s *State, value Value) string {
	switch value.Kind() {
	case KindNil:
		return "nil"
	case KindTrue:
		return "true"
	case KindFalse:
		return "false"
	case KindSymbol:
		return "Symbol"
	}
	return s.classOf(value).name
}

func regexpArgumentTypeError(s *State, value Value) error {
	return &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
		"wrong argument type "+regexpArgumentTypeName(s, value)+" (expected Regexp)")}
}

// checkRegexpPattern accepts a real Regexp or a real String; everything else
// is refused with the message CRuby uses, named by value for the three
// immediates and by class otherwise.
func isStringLike(value Value) bool {
	if value.Kind() == KindSymbol {
		return false
	}
	_, ok := stringPayload(value)
	return ok
}

func (s *State) checkRegexpPattern(value Value) error {
	if _, ok := regexpOf(value); ok {
		return nil
	}
	if isStringLike(value) {
		return nil
	}
	return regexpArgumentTypeError(s, value)
}

// compileRegexpString compiles a String argument as pattern text, which is
// what String#match and String#match? do with one.
func (s *State) compileRegexpStringPattern(value Value) (Value, error) {
	compiled, err := s.compileRegexpValue(value.String(), 0, s.RegexpClass)
	if err != nil {
		return Nil, err
	}
	return compiled, nil
}

// regexpSubjectBinary validates the subject the way every regexp search
// does: a subject read as characters must spell valid UTF-8. A binary
// subject is exempt, and `unread` exempts a literal String pattern search.
func (s *State) regexpSubjectBinary(self Value, unread bool) (bool, error) {
	text := self.String()
	binary := stringBinaryOf(self)
	if !unread && !binary && !utf8.ValidString(text) {
		return binary, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")}
	}
	return binary, nil
}

func regexpCharLen(text string, pos int, binary bool) int {
	if pos >= len(text) {
		return 1
	}
	if binary {
		return 1
	}
	_, size := utf8.DecodeRuneInString(text[pos:])
	if size <= 0 {
		size = 1
	}
	return size
}

func (s *State) regexpSearchValue(pattern Value, input string, start int) (*matchData, error) {
	data, err := s.regexpCompiled(pattern)
	if err != nil {
		return nil, err
	}
	return s.regexpSearch(pattern, data, input, start)
}

func (s *State) stringRegexpSearch(subject Value, pattern Value, input string, start int) (*matchData, error) {
	data, err := s.regexpCompiled(pattern)
	if err != nil {
		return nil, err
	}
	return s.regexpSearchMode(pattern, data, input, start, stringBinaryOf(subject))
}

func (s *State) stringRegexpSearchData(subject Value, pattern Value, data *regexpData, input string, start int) (*matchData, error) {
	return s.regexpSearchMode(pattern, data, input, start, stringBinaryOf(subject))
}

func regexpLiteralSearch(text, pattern string, pos int) int {
	if pos > len(text) {
		return -1
	}
	if pattern == "" {
		return pos
	}
	if pos < 0 {
		return -1
	}
	index := strings.Index(text[pos:], pattern)
	if index < 0 {
		return -1
	}
	return pos + index
}

// literalMatchData publishes the single group a literal String pattern
// match has. It leaves the Regexp unset: MatchData#regexp builds the quoted
// one the first time anything asks for it.
func (s *State) literalMatchData(input string, begin, end int) *matchData {
	return &matchData{
		regexp:  Nil,
		groups:  []string{input[begin:end]},
		matched: []bool{true},
		indices: []int{begin, end},
		names:   []string{""},
		input:   input,
		start:   begin,
		end:     end,
		literal: true,
	}
}

func regexpGroupNameIndex(data *matchData, name string) int {
	fallback := -1
	for i := len(data.names) - 1; i >= 1; i-- {
		if data.names[i] != name {
			continue
		}
		if fallback < 0 {
			fallback = i
		}
		if data.matched[i] {
			return i
		}
	}
	return fallback
}

// expandRegexpReplacement applies the replacement escapes of sub and gsub.
// named reports whether the pattern has named groups, which turns \1..\9 off
// as CRuby does.
func (s *State) expandRegexpReplacement(replacement string, data *matchData, named bool) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(replacement); {
		if replacement[i] != '\\' || i+1 >= len(replacement) {
			builder.WriteByte(replacement[i])
			i++
			continue
		}
		c := replacement[i+1]
		group := -1
		ref := true
		next := i + 2
		switch {
		case c >= '0' && c <= '9':
			group = int(c - '0')
			if group > 0 && named {
				group = -1
			}
		case c == '&':
			group = 0
		case c == 'k' && i+2 < len(replacement) && replacement[i+2] == '<':
			close := strings.IndexByte(replacement[i+3:], '>')
			if close < 0 {
				return "", &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "invalid group name reference format")}
			}
			name := replacement[i+3 : i+3+close]
			group = regexpGroupNameIndex(data, name)
			if group < 0 {
				return "", &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "undefined group name reference: "+name)}
			}
			next = i + 3 + close + 1
		case c == '+':
			for j := len(data.groups) - 1; j >= 1; j-- {
				if data.matched[j] {
					group = j
					break
				}
			}
		default:
			ref = false
		}
		if ref {
			if group >= 0 && group < len(data.groups) && data.matched[group] {
				builder.WriteString(data.groups[group])
			}
		} else {
			switch c {
			case '`':
				builder.WriteString(data.input[:data.start])
			case '\'':
				builder.WriteString(data.input[data.end:])
			case '\\':
				builder.WriteByte('\\')
			default:
				builder.WriteByte('\\')
				builder.WriteByte(c)
			}
		}
		i = next
	}
	return builder.String(), nil
}

func regexpPatternHasNames(data *matchData) bool {
	for i := 1; i < len(data.names); i++ {
		if data.names[i] != "" {
			return true
		}
	}
	return false
}

func (s *State) stringRegexpIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if _, ok := regexpOf(args[0]); !ok {
		return s.stringExtSliceMethod(ctx, self, args, kw, block)
	}
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	match, err := s.stringRegexpSearch(self, args[0], self.String(), 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, nil
	}
	s.regexpSetGlobals(match)
	if len(args) == 1 {
		return s.matchDataGroupByIndex(match, 0, true)
	}
	return s.stringRegexpPatternGroup(match, args[1])
}

func (s *State) stringRegexpPatternGroup(match *matchData, argument Value) (Value, error) {
	if _, ok := rangeDataOf(argument); ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	index, err := s.matchDataGroupNumber(match, argument)
	if err != nil {
		return Nil, err
	}
	return s.matchDataGroupByIndex(match, index, true)
}

func (s *State) stringRegexpSliceBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 0 {
		return s.stringExtSliceBangMethod(ctx, self, args, kw, block)
	}
	if _, ok := regexpOf(args[0]); !ok {
		return s.stringExtSliceBangMethod(ctx, self, args, kw, block)
	}
	if len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if err := s.ensureMutableString(self); err != nil {
		return Nil, err
	}
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	text := self.String()
	match, err := s.stringRegexpSearch(self, args[0], text, 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, nil
	}
	s.regexpSetGlobals(match)
	group := Value(IntValue(0))
	if len(args) == 2 {
		group = args[1]
	}
	index, err := s.regexpCaptureIndex(match, group, false)
	if err != nil {
		return Nil, err
	}
	if index < 0 {
		return Nil, nil
	}
	begin := match.indices[index*2]
	end := match.indices[index*2+1]
	if begin < 0 {
		return StringValue(""), nil
	}
	removed := match.input[begin:end]
	data, _ := stringDataOf(self)
	data.set(text[:begin] + text[end:])
	return StringValue(removed), nil
}

// regexpCaptureIndex resolves the capture argument of [] and slice!. When
// strict is set an integer outside the groups is an error, as [] and []=
// make it, and an out-of-range name is always an error. It answers -1 for a
// group that took no part when strict is clear.
func (s *State) regexpCaptureIndex(match *matchData, argument Value, strict bool) (int, error) {
	switch argument.Kind() {
	case KindInt:
		index := int(argument.Int())
		size := len(match.groups)
		if strict && (index >= size || index <= -size) {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index "+argument.ToS()+" out of regexp")}
		}
		negative := index < 0
		if negative {
			index += size
		}
		if !strict && (index < 0 || index >= size || (negative && index == 0)) {
			return -1, nil
		}
		return index, nil
	default:
		return s.matchDataGroupNumber(match, argument)
	}
}

func (s *State) stringRegexpSetIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 {
		return s.stringSetIndexMethod(ctx, self, args, kw, block)
	}
	if _, ok := regexpOf(args[0]); !ok {
		if len(args) >= 3 && !args[2].IsNil() && !isStringLike(args[2]) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
		}
		return s.stringSetIndexMethod(ctx, self, args, kw, block)
	}
	if len(args) != 2 && len(args) != 3 {
		return Nil, argumentCountError(s, len(args), "2..3")
	}
	replacement := args[len(args)-1]
	group := Value(IntValue(0))
	if len(args) == 3 {
		group = args[1]
	}
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	text := self.String()
	match, err := s.stringRegexpSearch(self, args[0], text, 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "regexp not matched")}
	}
	s.regexpSetGlobals(match)
	index, err := s.regexpCaptureIndex(match, group, true)
	if err != nil {
		return Nil, err
	}
	if index < 0 || index >= len(match.groups) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "regexp group "+group.ToS()+" not matched")}
	}
	if !match.matched[index] || match.indices[index*2] < 0 {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "regexp group "+group.ToS()+" not matched")}
	}
	if replacement.IsNil() || !isStringLike(replacement) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
	}
	begin := match.indices[index*2]
	end := match.indices[index*2+1]
	if err := s.ensureMutableString(self); err != nil {
		return Nil, err
	}
	current, _ := stringDataOf(self)
	current.set(text[:begin] + replacement.String() + text[end:])
	return replacement, nil
}

func (s *State) stringRegexpMatchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	pattern := args[0]
	if isStringLike(pattern) {
		compiled, err := s.compileRegexpStringPattern(pattern)
		if err != nil {
			return Nil, err
		}
		pattern = compiled
	}
	callArgs := []Value{self}
	if len(args) == 2 {
		callArgs = append(callArgs, args[1])
	}
	return s.call(ctx, pattern, "match", callArgs, kw, block)
}

func (s *State) stringRegexpMatchPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	pattern := args[0]
	if isStringLike(pattern) {
		compiled, err := s.compileRegexpStringPattern(pattern)
		if err != nil {
			return Nil, err
		}
		pattern = compiled
	}
	position := 0
	if len(args) == 2 {
		if args[1].Kind() != KindInt && args[1].Kind() != KindBigInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		position = int(args[1].Int())
	}
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	text := self.String()
	byteStart, ok := regexpPositionByte(text, position, true)
	if !ok {
		return False, nil
	}
	match, err := s.stringRegexpSearch(self, pattern, text, byteStart)
	if err != nil {
		return Nil, err
	}
	return BoolValue(match != nil), nil
}

func (s *State) stringRegexpMatchOperatorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	if _, ok := regexpOf(args[0]); !ok {
		if isStringLike(args[0]) {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "type mismatch: String given")}
		}
		if args[0].IsNil() {
			s.regexpClearGlobals()
			return Nil, nil
		}
		return s.call(ctx, args[0], "=~", []Value{self}, Nil, Nil)
	}
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	text := self.String()
	match, err := s.stringRegexpSearch(self, args[0], text, 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, nil
	}
	s.regexpSetGlobals(match)
	return IntValue(int64(charOffsetOfByte(text, match.start))), nil
}

func (s *State) stringRegexpNotMatchOperatorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result, err := s.stringRegexpMatchOperatorMethod(ctx, self, args, kw, block)
	if err != nil {
		return Nil, err
	}
	return BoolValue(!result.IsTruthy()), nil
}

func regexpPositionByte(text string, position int, charSpace bool) (int, bool) {
	if charSpace {
		count := stringCharCount(text)
		if position < 0 {
			position += count
		}
		if position < 0 || position > count {
			return 0, false
		}
		byteOffset, _ := stringCharIndexToByte(text, position)
		return byteOffset, true
	}
	if position < 0 {
		position += len(text)
	}
	if position < 0 {
		return 0, false
	}
	if position > len(text) {
		position = len(text)
	}
	return position, true
}

func (s *State) stringRegexpIndexSearchMethod(reverse, byteSpace bool) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) < 1 || len(args) > 2 {
			return Nil, argumentCountError(s, len(args), "1..2")
		}
		if _, ok := regexpOf(args[0]); !ok {
			if reverse {
				if byteSpace {
					return s.stringExtByterindexMethod(ctx, self, args, kw, block)
				}
				return s.stringExtRindexMethod(ctx, self, args, kw, block)
			}
			if byteSpace {
				return s.stringExtByteindexMethod(ctx, self, args, kw, block)
			}
			return s.stringExtIndexMethod(ctx, self, args, kw, block)
		}
		if _, err := s.regexpSubjectBinary(self, false); err != nil {
			return Nil, err
		}
		text := self.String()
		position := 0
		if reverse {
			if byteSpace {
				position = len(text)
			} else {
				position = stringCharCount(text)
			}
		}
		if len(args) == 2 {
			if args[1].Kind() != KindInt && args[1].Kind() != KindBigInt {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
			}
			position = int(args[1].Int())
		}
		if byteSpace {
			if position < 0 {
				position += len(text)
				if position < 0 {
					s.regexpClearGlobals()
					return Nil, nil
				}
			} else if position > len(text) {
				if !reverse {
					s.regexpClearGlobals()
					return Nil, nil
				}
				position = len(text)
			}
			if !stringBinaryOf(self) && !stringIsCharBoundary(text, position) {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index is not a character boundary")}
			}
		} else {
			count := stringCharCount(text)
			if position < 0 {
				position += count
			} else if reverse && position > count {
				position = count
			}
			if position < 0 || position > count {
				s.regexpClearGlobals()
				return Nil, nil
			}
			position, _ = stringCharIndexToByte(text, position)
		}
		if !reverse {
			match, err := s.stringRegexpSearch(self, args[0], text, position)
			if err != nil {
				return Nil, err
			}
			if match == nil {
				s.regexpClearGlobals()
				return Nil, nil
			}
			s.regexpSetGlobals(match)
			if byteSpace {
				return IntValue(int64(match.start)), nil
			}
			return IntValue(int64(charOffsetOfByte(text, match.start))), nil
		}
		match, err := s.regexpLastMatchAt(self, args[0], text, position)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			s.regexpClearGlobals()
			return Nil, nil
		}
		s.regexpSetGlobals(match)
		if byteSpace {
			return IntValue(int64(match.start)), nil
		}
		return IntValue(int64(charOffsetOfByte(text, match.start))), nil
	}
}

func (s *State) regexpLastMatchAt(subject Value, pattern Value, text string, limitByte int) (*matchData, error) {
	data, err := s.regexpCompiled(pattern)
	if err != nil {
		return nil, err
	}
	var last *matchData
	position := 0
	for position <= len(text) {
		match, err := s.regexpSearchMode(pattern, data, text, position, stringBinaryOf(subject))
		if err != nil {
			return nil, err
		}
		if match == nil {
			break
		}
		if match.start > limitByte {
			break
		}
		last = match
		if match.start >= len(text) {
			break
		}
		size := 1
		if !stringBinaryOf(subject) {
			_, size = utf8.DecodeRuneInString(text[match.start:])
			if size <= 0 {
				size = 1
			}
		}
		position = match.start + size
		if position > limitByte {
			break
		}
	}
	return last, nil
}

func (s *State) stringRegexpPartitionMethod(reverse bool) NativeFunc {
	return func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		if _, ok := regexpOf(args[0]); !ok {
			if reverse {
				return s.stringExtRpartitionMethod(ctx, self, args, kw, block)
			}
			return s.stringExtPartitionMethod(ctx, self, args, kw, block)
		}
		if _, err := s.regexpSubjectBinary(self, false); err != nil {
			return Nil, err
		}
		text := self.String()
		var match *matchData
		var err error
		if reverse {
			match, err = s.regexpLastMatchAt(self, args[0], text, len(text))
		} else {
			match, err = s.stringRegexpSearch(self, args[0], text, 0)
		}
		if err != nil {
			return Nil, err
		}
		if match == nil {
			s.regexpClearGlobals()
			if reverse {
				return ArrayValue([]Value{StringValue(""), StringValue(""), StringValue(text)}), nil
			}
			return ArrayValue([]Value{StringValue(text), StringValue(""), StringValue("")}), nil
		}
		s.regexpSetGlobals(match)
		return ArrayValue([]Value{
			StringValue(text[:match.start]),
			StringValue(text[match.start:match.end]),
			StringValue(text[match.end:]),
		}), nil
	}
}

// stringRegexpStartWithMethod checks each argument left to right. A Regexp
// is searched for and answers only when the match starts at the beginning;
// a match further along is refused and its globals cleared.
func (s *State) stringRegexpStartWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	anyRegexp := false
	for _, arg := range args {
		if _, ok := regexpOf(arg); ok {
			anyRegexp = true
			break
		}
	}
	if !anyRegexp {
		return s.stringExtStartWithMethod(ctx, self, args, kw, block)
	}
	for _, arg := range args {
		if _, ok := regexpOf(arg); ok {
			if _, err := s.regexpSubjectBinary(self, false); err != nil {
				return Nil, err
			}
			match, err := s.stringRegexpSearch(self, arg, self.String(), 0)
			if err != nil {
				return Nil, err
			}
			if match != nil {
				if match.start == 0 {
					s.regexpSetGlobals(match)
					return True, nil
				}
			}
			s.regexpClearGlobals()
			continue
		}
		value, err := s.stringExtStartWithMethod(ctx, self, []Value{arg}, Nil, Nil)
		if err != nil {
			return Nil, err
		}
		if value.IsTruthy() {
			return True, nil
		}
	}
	return False, nil
}

// --- substitution ---

func (s *State) replacementHash(value Value) (*hashData, bool) {
	if data, ok := hashDataOf(value); ok {
		return data, true
	}
	return nil, false
}

func (s *State) hashReplacementValue(ctx context.Context, hash Value, data *hashData, key string) (Value, error) {
	idx, found, err := s.hashLookupKey(ctx, data, StringValue(key))
	if err != nil {
		return Nil, err
	}
	if found {
		return data.entries[idx].value, nil
	}
	if !data.defaultProc.IsNil() {
		return s.callBlock(ctx, data.defaultProc, []Value{hash, StringValue(key)}, Nil)
	}
	return data.defaultValue, nil
}

func (s *State) replacementPiece(ctx context.Context, block, hash Value, hashData *hashData, matched string) (string, error) {
	if block.IsNil() {
		value, err := s.hashReplacementValue(ctx, hash, hashData, matched)
		if err != nil {
			return "", err
		}
		return value.ToS(), nil
	}
	value, err := s.callBlock(ctx, block, []Value{StringValue(matched)}, Nil)
	if err != nil {
		return "", err
	}
	return value.ToS(), nil
}

func (s *State) subArgCount(hasBlock bool, argc int) error {
	if hasBlock {
		if argc != 1 && argc != 2 {
			return argumentCountError(s, argc, "1..2")
		}
		return nil
	}
	if argc != 2 {
		return argumentCountError(s, argc, "2")
	}
	return nil
}

func substitutionArgs(args []Value, kw Value) []Value {
	if data, ok := hashDataOf(kw); ok && len(data.entries) > 0 {
		return append(args, kw)
	}
	return args
}

func (s *State) stringRegexpSubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	args = substitutionArgs(args, kw)
	if err := s.subArgCount(!block.IsNil(), len(args)); err != nil {
		return Nil, err
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	literal := isStringLike(args[0])
	if _, err := s.regexpSubjectBinary(self, literal); err != nil {
		return Nil, err
	}
	text := self.String()
	replacement := Value(Nil)
	hashData := (*hashData)(nil)
	useHash := false
	if len(args) == 2 {
		if data, ok := s.replacementHash(args[1]); ok {
			hashData = data
			useHash = true
			replacement = args[1]
		} else {
			replacement = StringValue(args[1].ToS())
		}
	}
	_ = replacement
	var match *matchData
	var err error
	var literalPattern string
	if literal {
		literalPattern = args[0].String()
		begin := regexpLiteralSearch(text, literalPattern, 0)
		if begin < 0 {
			s.regexpClearGlobals()
			return StringValue(text), nil
		}
		match = s.literalMatchData(text, begin, begin+len(literalPattern))
	} else {
		match, err = s.stringRegexpSearch(self, args[0], text, 0)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			s.regexpClearGlobals()
			return StringValue(text), nil
		}
		if !block.IsNil() || useHash {
			s.regexpSetGlobals(match)
		}
	}
	if len(args) == 2 && !useHash {
		expanded, err := s.expandRegexpReplacement(args[1].ToS(), match, regexpPatternHasNames(match))
		if err != nil {
			return Nil, err
		}
		s.regexpSetGlobals(match)
		return s.checkedStringValue(text[:match.start] + expanded + text[match.end:])
	}
	if len(args) == 2 && useHash {
		piece, err := s.replacementPiece(ctx, Nil, replacement, hashData, text[match.start:match.end])
		if err != nil {
			return Nil, err
		}
		s.regexpSetGlobals(match)
		return s.checkedStringValue(text[:match.start] + piece + text[match.end:])
	}
	s.regexpSetGlobals(match)
	piece, err := s.replacementPiece(ctx, block, Nil, nil, text[match.start:match.end])
	if err != nil {
		return Nil, err
	}
	return s.checkedStringValue(text[:match.start] + piece + text[match.end:])
}

func (s *State) stringRegexpSubBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	args = substitutionArgs(args, kw)
	if err := s.subArgCount(!block.IsNil(), len(args)); err != nil {
		return Nil, err
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	literal := isStringLike(args[0])
	if err := s.ensureMutableString(self); err != nil {
		return Nil, err
	}
	if _, err := s.regexpSubjectBinary(self, literal); err != nil {
		return Nil, err
	}
	text := self.String()
	var match *matchData
	var err error
	var literalPattern string
	if literal {
		literalPattern = args[0].String()
		begin := regexpLiteralSearch(text, literalPattern, 0)
		if begin < 0 {
			s.regexpClearGlobals()
			return Nil, nil
		}
		match = s.literalMatchData(text, begin, begin+len(literalPattern))
	} else {
		match, err = s.stringRegexpSearch(self, args[0], text, 0)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			s.regexpClearGlobals()
			return Nil, nil
		}
	}
	s.regexpSetGlobals(match)
	var replacement Value
	hashData := (*hashData)(nil)
	if len(args) == 2 {
		if data, ok := s.replacementHash(args[1]); ok {
			hashData = data
			replacement = args[1]
		} else {
			replacement = StringValue(args[1].ToS())
		}
	}
	if len(args) == 2 && hashData == nil {
		expanded, err := s.expandRegexpReplacement(replacement.String(), match, regexpPatternHasNames(match))
		if err != nil {
			return Nil, err
		}
		result := text[:match.start] + expanded + text[match.end:]
		if err := s.checkStringSize(len(result)); err != nil {
			return Nil, err
		}
		if err := s.assignString(self, result); err != nil {
			return Nil, err
		}
		return self, nil
	}
	if len(args) == 2 && hashData != nil {
		piece, err := s.replacementPiece(ctx, Nil, replacement, hashData, text[match.start:match.end])
		if err != nil {
			return Nil, err
		}
		length := len(text)
		if len(self.String()) != length {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
		}
		current := self.String()
		result := current[:match.start] + piece + current[match.end:]
		if err := s.checkStringSize(len(result)); err != nil {
			return Nil, err
		}
		if err := s.assignString(self, result); err != nil {
			return Nil, err
		}
		return self, nil
	}
	piece, err := s.replacementPiece(ctx, block, Nil, nil, text[match.start:match.end])
	if err != nil {
		return Nil, err
	}
	length := len(text)
	if len(self.String()) != length {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
	}
	current := self.String()
	result := current[:match.start] + piece + current[match.end:]
	if err := s.checkStringSize(len(result)); err != nil {
		return Nil, err
	}
	if err := s.assignString(self, result); err != nil {
		return Nil, err
	}
	return self, nil
}

func (s *State) ensureMutableString(value Value) error {
	if value.obj != nil && value.obj.frozen {
		return &unwindSignal{kind: unwindException, value: newException(s.FrozenErrorClass, "can't modify frozen String")}
	}
	return nil
}

func (s *State) assignString(self Value, text string) error {
	if err := s.ensureMutableString(self); err != nil {
		return err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return nil
	}
	data.set(text)
	return nil
}

// regexpGsubWalk is the block and Hash form of gsub. It re-reads the
// receiver after every block call, so a block that rewrites it in place is
// answered on the bytes it left, and a length change is refused.
func (s *State) regexpGsubWalk(ctx context.Context, pattern Value, literalPattern string, literal bool, self Value, block, hash Value, hashData *hashData) (string, error) {
	text := self.String()
	initialLen := len(text)
	binary, err := s.regexpSubjectBinary(self, literal)
	if err != nil {
		return "", err
	}
	var data *regexpData
	if !literal {
		data, err = s.regexpCompiled(pattern)
		if err != nil {
			return "", err
		}
	}
	var builder strings.Builder
	pos := 0
	last := 0
	var lastMatch *matchData
	for pos <= initialLen {
		var match *matchData
		if literal {
			begin := regexpLiteralSearch(text, literalPattern, pos)
			if begin < 0 {
				break
			}
			match = s.literalMatchData(text, begin, begin+len(literalPattern))
		} else {
			match, err = s.stringRegexpSearchData(self, pattern, data, text, pos)
			if err != nil {
				return "", err
			}
			if match == nil {
				break
			}
		}
		begin, end := match.start, match.end
		lastMatch = match
		last = pos
		s.regexpSetGlobals(match)
		piece, err := s.replacementPiece(ctx, block, hash, hashData, text[begin:end])
		if err != nil {
			return "", err
		}
		if len(self.String()) != initialLen {
			return "", &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
		}
		text = self.String()
		binary = stringBinaryOf(self)
		if begin > pos {
			builder.WriteString(text[pos:begin])
		}
		builder.WriteString(piece)
		if begin == end {
			if end < initialLen {
				size := regexpCharLen(text, end, binary)
				builder.WriteString(text[end : end+size])
				pos = end + size
			} else {
				pos = end + 1
			}
		} else {
			pos = end
		}
	}
	if pos < initialLen {
		builder.WriteString(text[pos:])
	}
	if lastMatch == nil {
		s.regexpClearGlobals()
	} else if self.String() == lastMatch.input {
		s.regexpSetGlobals(lastMatch)
	} else {
		if len(self.String()) != initialLen {
			return "", &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
		}
		if literal {
			text = self.String()
			next := regexpLiteralSearch(text, literalPattern, last)
			if next < 0 {
				s.regexpClearGlobals()
			} else {
				m := s.literalMatchData(text, next, next+len(literalPattern))
				s.regexpSetGlobals(m)
			}
		} else if _, err := s.stringRegexpSearch(self, pattern, self.String(), last); err != nil {
			return "", err
		} else if m, _ := s.stringRegexpSearch(self, pattern, self.String(), last); m != nil {
			s.regexpSetGlobals(m)
		} else {
			s.regexpClearGlobals()
		}
	}
	return builder.String(), nil
}

func (s *State) regexpGsubReplacement(ctx context.Context, self Value, pattern Value, literalPattern string, literal bool, replacement string, hash Value, hashData *hashData) (string, error) {
	text := self.String()
	if _, err := s.regexpSubjectBinary(self, literal); err != nil {
		return "", err
	}
	var data *regexpData
	var err error
	if !literal {
		data, err = s.regexpCompiled(pattern)
		if err != nil {
			return "", err
		}
	}
	var builder strings.Builder
	pos := 0
	var lastMatch *matchData
	for pos <= len(text) {
		var match *matchData
		if literal {
			begin := regexpLiteralSearch(text, literalPattern, pos)
			if begin < 0 {
				break
			}
			match = s.literalMatchData(text, begin, begin+len(literalPattern))
		} else {
			match, err = s.stringRegexpSearchData(self, pattern, data, text, pos)
			if err != nil {
				return "", err
			}
			if match == nil {
				break
			}
		}
		lastMatch = match
		if match.start > pos {
			builder.WriteString(text[pos:match.start])
		}
		expanded, err := s.expandRegexpReplacement(replacement, match, regexpPatternHasNames(match))
		if err != nil {
			return "", err
		}
		builder.WriteString(expanded)
		if match.start == match.end {
			if match.end < len(text) {
				size := regexpCharLen(text, match.end, stringBinaryOf(self))
				builder.WriteString(text[match.end : match.end+size])
				pos = match.end + size
			} else {
				pos = match.end + 1
			}
		} else {
			pos = match.end
		}
	}
	if pos < len(text) {
		builder.WriteString(text[pos:])
	}
	if lastMatch != nil {
		s.regexpSetGlobals(lastMatch)
	} else {
		s.regexpClearGlobals()
	}
	return builder.String(), nil
}

func (s *State) stringRegexpGsubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	args = substitutionArgs(args, kw)
	if len(args) != 1 && len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if len(args) == 1 && block.IsNil() {
		return s.toEnum(self, "gsub", []Value{args[0]}), nil
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	literal := isStringLike(args[0])
	literalPattern := ""
	pattern := args[0]
	if literal {
		literalPattern = args[0].String()
	}
	if len(args) == 2 {
		if data, ok := s.replacementHash(args[1]); ok {
			result, err := s.regexpGsubWalk(ctx, pattern, literalPattern, literal, self, Nil, args[1], data)
			if err != nil {
				return Nil, err
			}
			return s.checkedStringValue(result)
		}
		replacement := args[1].ToS()
		result, err := s.regexpGsubReplacement(ctx, self, pattern, literalPattern, literal, replacement, Nil, nil)
		if err != nil {
			return Nil, err
		}
		return s.checkedStringValue(result)
	}
	result, err := s.regexpGsubWalk(ctx, pattern, literalPattern, literal, self, block, Nil, nil)
	if err != nil {
		return Nil, err
	}
	return s.checkedStringValue(result)
}

func (s *State) stringRegexpGsubBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	args = substitutionArgs(args, kw)
	if err := s.ensureMutableString(self); err != nil {
		return Nil, err
	}
	if len(args) != 1 && len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if len(args) == 1 && block.IsNil() {
		return s.toEnum(self, "gsub!", []Value{args[0]}), nil
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	literal := isStringLike(args[0])
	literalPattern := ""
	pattern := args[0]
	if literal {
		literalPattern = args[0].String()
	}
	text := self.String()
	var found bool
	if literal {
		found = regexpLiteralSearch(text, literalPattern, 0) >= 0
	} else {
		if _, err := s.regexpSubjectBinary(self, false); err != nil {
			return Nil, err
		}
		match, err := s.stringRegexpSearch(self, pattern, text, 0)
		if err != nil {
			return Nil, err
		}
		found = match != nil
		if match != nil {
			s.regexpSetGlobals(match)
		}
	}
	if !found {
		s.regexpClearGlobals()
		return Nil, nil
	}
	var result string
	var err error
	if len(args) == 2 {
		if data, ok := s.replacementHash(args[1]); ok {
			result, err = s.regexpGsubWalk(ctx, pattern, literalPattern, literal, self, Nil, args[1], data)
		} else {
			result, err = s.regexpGsubReplacement(ctx, self, pattern, literalPattern, literal, args[1].ToS(), Nil, nil)
		}
	} else {
		result, err = s.regexpGsubWalk(ctx, pattern, literalPattern, literal, self, block, Nil, nil)
	}
	if err != nil {
		return Nil, err
	}
	if err := s.checkStringSize(len(result)); err != nil {
		return Nil, err
	}
	if err := s.assignString(self, result); err != nil {
		return Nil, err
	}
	return self, nil
}

// --- scan ---

func (s *State) scanMatchValue(match *matchData) Value {
	if len(match.groups) <= 1 {
		if match.matched[0] {
			return StringValue(match.groups[0])
		}
		return Nil
	}
	values := make([]Value, len(match.groups)-1)
	for i := 1; i < len(match.groups); i++ {
		if match.matched[i] {
			values[i-1] = StringValue(match.groups[i])
		} else {
			values[i-1] = Nil
		}
	}
	return ArrayValue(values)
}

func (s *State) stringRegexpScanMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if err := s.checkRegexpPattern(args[0]); err != nil {
		return Nil, err
	}
	literal := isStringLike(args[0])
	if _, err := s.regexpSubjectBinary(self, false); err != nil {
		return Nil, err
	}
	pattern := args[0]
	literalPattern := ""
	var data *regexpData
	var err error
	if literal {
		literalPattern = args[0].String()
	} else {
		data, err = s.regexpCompiled(pattern)
		if err != nil {
			return Nil, err
		}
	}
	if block.IsNil() {
		if literal {
			return s.scanLiteralAry(self, literalPattern)
		}
		return s.scanAry(self, pattern, data)
	}
	initialLen := len(self.String())
	text := self.String()
	pos := 0
	last := 0
	var lastMatch *matchData
	for pos <= initialLen {
		if len(self.String()) != initialLen {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
		}
		text = self.String()
		var match *matchData
		if literal {
			begin := regexpLiteralSearch(text, literalPattern, pos)
			if begin < 0 {
				break
			}
			match = s.literalMatchData(text, begin, begin+len(literalPattern))
		} else {
			match, err = s.stringRegexpSearchData(self, pattern, data, text, pos)
			if err != nil {
				return Nil, err
			}
			if match == nil {
				s.regexpClearGlobals()
				break
			}
		}
		last = pos
		lastMatch = match
		s.regexpSetGlobals(match)
		if _, err := s.callBlock(ctx, block, []Value{s.scanMatchValue(match)}, Nil); err != nil {
			return Nil, err
		}
		begin, end := match.start, match.end
		if begin == end {
			pos = end + 1
		} else {
			pos = end
		}
	}
	if lastMatch != nil {
		if self.String() == lastMatch.input {
			s.regexpSetGlobals(lastMatch)
		} else {
			if len(self.String()) != initialLen {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.RuntimeErrorClass, "string modified")}
			}
			if literal {
				text = self.String()
				next := regexpLiteralSearch(text, literalPattern, last)
				if next < 0 {
					s.regexpClearGlobals()
				} else {
					s.regexpSetGlobals(s.literalMatchData(text, next, next+len(literalPattern)))
				}
			} else if match, _ := s.stringRegexpSearch(self, pattern, self.String(), last); match != nil {
				s.regexpSetGlobals(match)
			} else {
				s.regexpClearGlobals()
			}
		}
	}
	return self, nil
}

func (s *State) scanLiteralAry(self Value, literalPattern string) (Value, error) {
	text := self.String()
	values := []Value{}
	pos := 0
	var last *matchData
	for pos <= len(text) {
		begin := regexpLiteralSearch(text, literalPattern, pos)
		if begin < 0 {
			break
		}
		match := s.literalMatchData(text, begin, begin+len(literalPattern))
		last = match
		values = append(values, StringValue(literalPattern))
		if literalPattern == "" {
			pos = begin + regexpCharLen(text, begin, stringBinaryOf(self))
		} else {
			pos = begin + len(literalPattern)
		}
	}
	if last != nil {
		s.regexpSetGlobals(last)
	} else {
		s.regexpClearGlobals()
	}
	return ArrayValue(values), nil
}

func (s *State) scanAry(self Value, pattern Value, data *regexpData) (Value, error) {
	text := self.String()
	values := []Value{}
	pos := 0
	var last *matchData
	for pos <= len(text) {
		match, err := s.stringRegexpSearchData(self, pattern, data, text, pos)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			break
		}
		last = match
		values = append(values, s.scanMatchValue(match))
		if match.start == match.end {
			pos = match.end + 1
		} else {
			pos = match.end
		}
	}
	if last != nil {
		s.regexpSetGlobals(last)
	} else {
		s.regexpClearGlobals()
	}
	return ArrayValue(values), nil
}

// --- split ---

func (s *State) regexpSplitLimit(value Value) (int, error) {
	switch value.Kind() {
	case KindInt:
		return int(value.Int()), nil
	case KindBigInt:
		integer, _ := value.BigInt()
		if !integer.IsInt64() {
			return 0, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, "bignum too big to convert into `long'")}
		}
		return int(integer.Int64()), nil
	case KindFloat:
		return int(value.Float()), nil
	}
	return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+regexpArgumentTypeName(s, value)+" into Integer")}
}

func (s *State) stringRegexpSplitMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "0..2")
	}
	pattern := Value(Nil)
	if len(args) > 0 {
		pattern = args[0]
	}
	limit := 0
	limitGiven := len(args) > 1
	if limitGiven {
		var err error
		limit, err = s.regexpSplitLimit(args[1])
		if err != nil {
			return Nil, err
		}
	}
	if pattern.IsNil() || isStringLike(pattern) {
		if limit != 1 {
			if _, err := s.regexpSubjectBinary(self, false); err != nil {
				return Nil, err
			}
		}
		callArgs := []Value{}
		if len(args) > 0 {
			callArgs = append(callArgs, args[0])
		}
		if limitGiven {
			callArgs = append(callArgs, IntValue(int64(limit)))
		}
		return s.stringSplitMethod(ctx, self, callArgs, Nil, Nil)
	}
	text := self.String()
	if limit == 1 {
		if len(text) == 0 {
			return ArrayValue(nil), nil
		}
		return ArrayValue([]Value{StringValue(text)}), nil
	}
	if err := s.checkRegexpPattern(pattern); err != nil {
		return Nil, err
	}
	if _, ok := regexpOf(pattern); !ok {
		return s.stringSplitMethod(ctx, self, args, kw, block)
	}
	binary, err := s.regexpSubjectBinary(self, false)
	if err != nil {
		return Nil, err
	}
	data, err := s.regexpCompiled(pattern)
	if err != nil {
		return Nil, err
	}
	result := []Value{}
	fieldStart := 0
	searchPos := 0
	length := len(text)
	count := 0
	for searchPos <= length {
		if limit > 0 && count >= limit-1 {
			result = append(result, StringValue(text[fieldStart:]))
			return ArrayValue(result), nil
		}
		match, err := s.stringRegexpSearchData(self, pattern, data, text, searchPos)
		if err != nil {
			return Nil, err
		}
		if match == nil {
			break
		}
		ms, me := match.start, match.end
		if ms == me {
			if binary {
				searchPos = me + 1
			} else if me < length {
				searchPos = me + regexpCharLen(text, me, false)
			} else {
				searchPos = me + 1
			}
			if ms == fieldStart {
				continue
			}
		}
		result = append(result, StringValue(text[fieldStart:ms]))
		count++
		fieldStart = me
		if ms != me {
			searchPos = me
		}
		for i := 1; i < len(match.groups); i++ {
			if match.indices[i*2] >= 0 {
				result = append(result, StringValue(match.groups[i]))
			}
		}
	}
	if length > 0 && fieldStart <= length && (fieldStart < length || limit != 0) {
		result = append(result, StringValue(text[fieldStart:]))
	}
	if limit == 0 {
		for len(result) > 0 {
			last, ok := stringDataOf(result[len(result)-1])
			if !ok || last.str() != "" {
				break
			}
			result = result[:len(result)-1]
		}
	}
	return ArrayValue(result), nil
}
