package mrbvm

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

const (
	regexpIgnoreCase = 1
	regexpExtended   = 2
	regexpMultiline  = 4
)

type regexpData struct {
	compiled       *regexp2.Regexp
	source         string
	flags          int64
	compileFailed  bool
	compileMessage string
}

type matchData struct {
	regexp  Value
	groups  []string
	matched []bool
	indices []int
	names   []string
	input   string
	start   int
	end     int
}

func (s *State) installRegexpClass() {
	class := s.RegexpClass
	class.constants["IGNORECASE"] = IntValue(regexpIgnoreCase)
	class.constants["EXTENDED"] = IntValue(regexpExtended)
	class.constants["MULTILINE"] = IntValue(regexpMultiline)
	class.constants["FIXEDENCODING"] = IntValue(16)
	class.constants["NOENCODING"] = IntValue(32)

	class.classMethods["compile"] = &method{owner: class, name: "compile", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.classNewMethod(ctx, self, args, kw, block)
	}}
	class.classMethods["escape"] = &method{owner: class, name: "escape", native: s.regexpEscapeMethod}
	class.classMethods["quote"] = &method{owner: class, name: "quote", native: s.regexpEscapeMethod}
	class.classMethods["union"] = &method{owner: class, name: "union", native: s.regexpUnionMethod}
	class.classMethods["last_match"] = &method{owner: class, name: "last_match", native: s.regexpLastMatchMethod}

	class.define("initialize", s.regexpInitializeMethod)
	class.define("initialize_copy", s.regexpInitializeCopyMethod)
	class.define("dup", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.regexpCopy(ctx, self, false)
	})
	class.define("clone", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.regexpCopy(ctx, self, true)
	})
	class.define("source", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		source, _, err := s.regexpSourceAndFlags(self)
		if err != nil {
			return Nil, err
		}
		return StringValue(source), nil
	})
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return s.regexpToSMethod(self)
	})
	class.define("inspect", s.regexpInspectMethod)
	class.define("options", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		_, flags, err := s.regexpSourceAndFlags(self)
		if err != nil {
			return Nil, err
		}
		return IntValue(flags & (regexpIgnoreCase | regexpExtended | regexpMultiline)), nil
	})
	class.define("casefold?", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		_, flags, err := s.regexpSourceAndFlags(self)
		if err != nil {
			return Nil, err
		}
		return BoolValue(flags&regexpIgnoreCase != 0), nil
	})
	class.define("names", s.regexpNamesMethod)
	class.define("named_captures", s.regexpNamedCapturesMethod)
	class.define("match?", s.regexpMatchPredicateMethod)
	class.define("===", s.regexpCaseEqMethod)
	class.define("=~", s.regexpMatchOperatorMethod)
	class.define("match", s.regexpMatchMethod)
	class.define("==", s.regexpEqualMethod)
	class.define("eql?", s.regexpEqualMethod)
	class.define("hash", s.regexpHashMethod)
}

func (s *State) regexpInitializeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	if _, exists := self.obj.data.(*regexpData); exists {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "already initialized regexp")}
	}
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	var source string
	var flags int64
	switch args[0].Kind() {
	case KindString:
		source = args[0].String()
	case KindObject:
		if other, ok := regexpOf(args[0]); ok {
			if other == nil {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
			}
			var err error
			source, flags, err = s.regexpSourceAndFlags(args[0])
			if err != nil {
				return Nil, err
			}
		} else {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type "+s.classOf(args[0]).name+" (expected String or Regexp)")}
		}
	default:
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "wrong argument type "+s.classOf(args[0]).name+" (expected String or Regexp)")}
	}
	if len(args) == 2 {
		parsed, err := s.regexpFlagsArg(args[1])
		if err != nil {
			return Nil, err
		}
		flags = parsed
	}
	return s.regexpInitializeData(self, source, flags)
}

func (s *State) regexpFlagsArg(value Value) (int64, error) {
	switch value.Kind() {
	case KindInt:
		return value.Int() & (regexpIgnoreCase | regexpExtended | regexpMultiline), nil
	case KindNil:
		return 0, nil
	default:
		if value.Kind() == KindString || value.Kind() == KindSymbol {
			var flags int64
			for _, char := range value.String() {
				switch char {
				case 'i':
					flags |= regexpIgnoreCase
				case 'x':
					flags |= regexpExtended
				case 'm':
					flags |= regexpMultiline
				case 'n', 'o', 'e', 's', 'u':
					// Encoding options are accepted but not represented.
				default:
					return 0, &unwindSignal{kind: unwindException, value: newException(s.RegexpErrorClass, "unknown regexp option - "+string(char))}
				}
			}
			return flags, nil
		}
		return 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(value).name+" into Integer")}
	}
}

func (s *State) regexpInitializeData(self Value, source string, flags int64) (Value, error) {
	data := &regexpData{source: source, flags: flags}
	self.obj.data = data
	self.obj.setIvar("@source", StringValue(source))
	self.obj.setIvar("@flags", IntValue(flags))
	compiled, err := compileRegexpPattern(source, flags)
	if err != nil {
		data.compileFailed = true
		data.compileMessage = err.Error()
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RegexpErrorClass, err.Error())}
	}
	data.compiled = compiled
	return self, nil
}

func compileRegexpPattern(source string, flags int64) (*regexp2.Regexp, error) {
	options := regexp2.RegexOptions(regexp2.Multiline) // Ruby anchors match line boundaries by default.
	if flags&regexpIgnoreCase != 0 {
		options |= regexp2.IgnoreCase
	}
	if flags&regexpMultiline != 0 {
		options |= regexp2.Singleline
	}
	if flags&regexpExtended != 0 {
		options |= regexp2.IgnorePatternWhitespace
	}
	compiled, err := regexp2.Compile(translateRegexpInlineOptions(source), options)
	if err != nil {
		return nil, err
	}
	compiled.MatchTimeout = time.Second
	return compiled, nil
}

func (s *State) regexpInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if identicalValue(self, args[0]) {
		return self, nil
	}
	if data, ok := regexpOf(self); ok && data != nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize_copy should be called only once")}
	}
	source, flags, err := s.regexpSourceAndFlags(args[0])
	if err != nil {
		return Nil, err
	}
	return s.regexpInitializeData(self, source, flags)
}

func (s *State) regexpCopy(ctx context.Context, self Value, clone bool) (Value, error) {
	if self.obj == nil {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	copyValue := ObjectValue(&Object{class: self.obj.class})
	copyValue.obj.ivars = copyObjectIvars(self.obj)
	if _, err := s.call(ctx, copyValue, "initialize_copy", []Value{self}, Nil, Nil); err != nil {
		return Nil, err
	}
	if clone {
		copyValue.obj.frozen = self.obj.frozen
	}
	return copyValue, nil
}

func (s *State) regexpSourceAndFlags(self Value) (string, int64, error) {
	data, ok := regexpOf(self)
	if !ok || data == nil {
		return "", 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	if self.obj == nil {
		return "", 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	source, found := self.obj.ivars["@source"]
	if !found || source.Kind() != KindString {
		return "", 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	flags, found := self.obj.ivars["@flags"]
	if !found || flags.Kind() != KindInt {
		return "", 0, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	return source.String(), flags.Int(), nil
}

func regexpOf(value Value) (*regexpData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*regexpData)
	return data, ok
}

func (s *State) regexpCompiled(self Value) (*regexpData, error) {
	data, ok := regexpOf(self)
	if !ok || data == nil {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized Regexp")}
	}
	if _, _, err := s.regexpSourceAndFlags(self); err != nil {
		return nil, err
	}
	if data.compileFailed || data.compiled == nil {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "regexp is not compiled")}
	}
	return data, nil
}

func regexpDisplayFlags(flags int64, on bool) string {
	var builder strings.Builder
	letters := []struct {
		bit    int64
		letter byte
	}{
		{regexpMultiline, 'm'},
		{regexpIgnoreCase, 'i'},
		{regexpExtended, 'x'},
	}
	for _, entry := range letters {
		if (flags&entry.bit != 0) == on {
			builder.WriteByte(entry.letter)
		}
	}
	return builder.String()
}

func (s *State) regexpToSMethod(self Value) (Value, error) {
	source, flags, err := s.regexpSourceAndFlags(self)
	if err != nil {
		return Nil, err
	}
	body, folded := regexpFoldLeadingGroup(source, flags)
	text := "(?"
	on := regexpDisplayFlags(folded, true)
	off := regexpDisplayFlags(folded, false)
	text += on
	if off != "" {
		text += "-" + off
	}
	text += ":" + body + ")"
	return StringValue(text), nil
}

func regexpOptionLetterBits(char byte) int64 {
	switch char {
	case 'i':
		return regexpIgnoreCase
	case 'm':
		return regexpMultiline
	case 'x':
		return regexpExtended
	}
	return 0
}

// regexpFoldLeadingGroup implements the folding Regexp#to_s does for a source
// that begins with an option group, matching mruby's re_fold_leading_group.
func regexpFoldLeadingGroup(source string, flags int64) (string, int64) {
	ptr := source
	current := flags
	originalPtr := source
	originalFlags := flags
	for len(ptr) >= 4 && strings.HasPrefix(ptr, "(?") {
		p := 2
		on := current
		consumed := 0
		for p < len(ptr) {
			bits := regexpOptionLetterBits(ptr[p])
			if bits == 0 {
				break
			}
			on |= bits
			p++
			consumed++
		}
		if p+1 < len(ptr) && ptr[p] == '-' {
			p++
			consumed++
			for p < len(ptr) {
				bits := regexpOptionLetterBits(ptr[p])
				if bits == 0 {
					break
				}
				on &^= bits
				p++
				consumed++
			}
		}
		if p < len(ptr) && ptr[p] == ')' {
			current = on
			ptr = ptr[p+1:]
			continue
		}
		if p+1 < len(ptr) && ptr[p] == ':' && ptr[len(ptr)-1] == ')' {
			body := ptr[p+1 : len(ptr)-1]
			if _, err := compileRegexpPattern(body, on); err == nil {
				current = on
				ptr = body
				break
			}
		}
		// The group did not fold. Any toggles peeled before it go back as
		// written, so only a source that began with them prints them.
		return originalPtr, originalFlags
	}
	return ptr, current
}

func (s *State) regexpInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := regexpOf(self)
	if !ok || data == nil || self.obj == nil {
		return StringValue(regexpDefaultInspect(self)), nil
	}
	source, found := self.obj.ivars["@source"]
	if !found || source.Kind() != KindString {
		return StringValue(regexpDefaultInspect(self)), nil
	}
	flags, found := self.obj.ivars["@flags"]
	if !found || flags.Kind() != KindInt {
		return StringValue(regexpDefaultInspect(self)), nil
	}
	return StringValue("/" + source.String() + "/" + regexpDisplayFlags(flags.Int(), true)), nil
}

func regexpDefaultInspect(value Value) string {
	if value.obj == nil {
		return "#<Regexp>"
	}
	return fmt.Sprintf("#<Regexp:0x%x>", objectIdentityID(value))
}

func (s *State) regexpEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	if identicalValue(self, args[0]) {
		return True, nil
	}
	if !s.isA(args[0], s.RegexpClass) {
		return False, nil
	}
	leftSource, leftFlags, err := s.regexpSourceAndFlags(self)
	if err != nil {
		return Nil, err
	}
	rightSource, rightFlags, err := s.regexpSourceAndFlags(args[0])
	if err != nil {
		return Nil, err
	}
	return BoolValue(leftSource == rightSource && leftFlags == rightFlags), nil
}

func (s *State) regexpHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	source, flags, err := s.regexpSourceAndFlags(self)
	if err != nil {
		return Nil, err
	}
	h := hashValue(StringValue(source), map[*Object]bool{})
	return IntValue(h ^ (flags * 0x9e3779b9)), nil
}

func regexpNamedGroupTable(data *regexpData) ([]string, map[string][]int) {
	var names []string
	table := make(map[string][]int)
	if data == nil || data.compiled == nil {
		return names, table
	}
	for index, name := range data.compiled.GetGroupNames() {
		if name == "" || isDecimalString(name) {
			continue
		}
		if _, seen := table[name]; !seen {
			names = append(names, name)
		}
		table[name] = append(table[name], index)
	}
	return names, table
}

func isDecimalString(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

func (s *State) regexpNamesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, _, err := s.regexpSourceAndFlags(self); err != nil {
		return Nil, err
	}
	data, _ := regexpOf(self)
	names, _ := regexpNamedGroupTable(data)
	values := make([]Value, len(names))
	for i, name := range names {
		values[i] = StringValue(name)
	}
	return ArrayValue(values), nil
}

func (s *State) regexpNamedCapturesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if _, _, err := s.regexpSourceAndFlags(self); err != nil {
		return Nil, err
	}
	data, _ := regexpOf(self)
	names, table := regexpNamedGroupTable(data)
	var entries []hashEntry
	for _, name := range names {
		groups := table[name]
		values := make([]Value, len(groups))
		for i, group := range groups {
			values[i] = IntValue(int64(group))
		}
		entries = append(entries, hashEntry{key: StringValue(name), value: ArrayValue(values)})
	}
	return HashValue(entries), nil
}

func regexpInputString(value Value) (string, bool) {
	switch value.Kind() {
	case KindString, KindSymbol:
		return value.String(), true
	}
	return "", false
}

func regexpTypeError(s *State, value Value) error {
	return &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(value).name+" into String")}
}

func byteOffsetOfRune(text string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	i := 0
	r := 0
	for i < len(text) && r < runeIndex {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
		}
		i += size
		r++
	}
	return i
}

func charOffsetOfByte(text string, byteIndex int) int {
	if byteIndex <= 0 {
		return 0
	}
	if byteIndex > len(text) {
		byteIndex = len(text)
	}
	count := 0
	for i := 0; i < byteIndex; {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
		}
		i += size
		count++
	}
	return count
}

func (s *State) regexpSearch(self Value, data *regexpData, input string, startByte int) (*matchData, error) {
	if startByte < 0 || startByte > len(input) {
		return nil, nil
	}
	match, err := data.compiled.FindStringMatchStartingAt(input, startByte)
	if err != nil {
		return nil, &unwindSignal{kind: unwindException, value: newException(s.RegexpErrorClass, err.Error())}
	}
	if match == nil {
		return nil, nil
	}
	groups := match.Groups()
	result := &matchData{
		regexp:  self,
		groups:  make([]string, len(groups)),
		matched: make([]bool, len(groups)),
		indices: make([]int, len(groups)*2),
		names:   make([]string, len(groups)),
		input:   input,
	}
	for i, group := range groups {
		result.names[i] = group.Name
		if isDecimalString(group.Name) {
			result.names[i] = ""
		}
		if len(group.Captures) == 0 {
			result.indices[i*2] = -1
			result.indices[i*2+1] = -1
			continue
		}
		result.matched[i] = true
		result.groups[i] = group.String()
		result.indices[i*2] = byteOffsetOfRune(input, group.Index)
		result.indices[i*2+1] = byteOffsetOfRune(input, group.Index+group.Length)
	}
	result.start = result.indices[0]
	result.end = result.indices[1]
	return result, nil
}

func (s *State) regexpSetGlobals(match *matchData) {
	if match == nil {
		s.regexpClearGlobals()
		return
	}
	value := s.matchDataValue(match)
	s.globals["$~"] = value
	groups := match.groups
	matched := match.matched
	last := -1
	for i := 1; i < len(groups); i++ {
		if matched[i] {
			last = i
		}
	}
	set := func(name string, index int) {
		if index >= 0 && index < len(groups) && matched[index] {
			s.globals[name] = StringValue(groups[index])
		} else {
			s.globals[name] = Nil
		}
	}
	set("$&", 0)
	for i := 1; i <= 9; i++ {
		set("$"+fmt.Sprint(i), i)
	}
	if last >= 0 {
		s.globals["$+"] = StringValue(groups[last])
	} else {
		s.globals["$+"] = Nil
	}
	s.globals["$`"] = StringValue(match.input[:match.start])
	s.globals["$'"] = StringValue(match.input[match.end:])
}

func (s *State) regexpClearGlobals() {
	s.globals["$~"] = Nil
	for i := 1; i <= 9; i++ {
		s.globals["$"+fmt.Sprint(i)] = Nil
	}
	s.globals["$&"] = Nil
	s.globals["$`"] = Nil
	s.globals["$'"] = Nil
	s.globals["$+"] = Nil
}

func (s *State) regexpMatchPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	if args[0].IsNil() {
		return False, nil
	}
	input, ok := regexpInputString(args[0])
	if !ok {
		return Nil, regexpTypeError(s, args[0])
	}
	startByte := 0
	if len(args) == 2 && !args[1].IsNil() {
		if args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		position := int(args[1].Int())
		count := stringCharCount(input)
		if position < 0 {
			position += count
		}
		if position < 0 || position > count {
			return False, nil
		}
		startByte, _ = stringCharIndexToByte(input, position)
	}
	match, err := s.regexpSearch(self, data, input, startByte)
	if err != nil {
		return Nil, err
	}
	return BoolValue(match != nil), nil
}

func (s *State) regexpMatchOperatorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	if args[0].IsNil() {
		s.regexpClearGlobals()
		return Nil, nil
	}
	input, ok := regexpInputString(args[0])
	if !ok {
		return Nil, regexpTypeError(s, args[0])
	}
	match, err := s.regexpSearch(self, data, input, 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, nil
	}
	s.regexpSetGlobals(match)
	return IntValue(int64(charOffsetOfByte(input, match.start))), nil
}

func (s *State) regexpMatchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	if args[0].IsNil() {
		s.regexpClearGlobals()
		return Nil, nil
	}
	input, ok := regexpInputString(args[0])
	if !ok {
		return Nil, regexpTypeError(s, args[0])
	}
	startByte := 0
	if len(args) == 2 && !args[1].IsNil() {
		if args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		position := int(args[1].Int())
		count := stringCharCount(input)
		if position < 0 {
			position += count
		}
		if position < 0 || position > count {
			s.regexpClearGlobals()
			return Nil, nil
		}
		startByte, _ = stringCharIndexToByte(input, position)
	}
	match, err := s.regexpSearch(self, data, input, startByte)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return Nil, nil
	}
	s.regexpSetGlobals(match)
	result := s.matchDataValue(match)
	if !block.IsNil() {
		value, err := s.callBlock(ctx, block, []Value{result}, Nil)
		if err != nil {
			if br, ok := err.(*unwindSignal); ok && br.kind == unwindBreak {
				return br.value, nil
			}
			return Nil, err
		}
		return value, nil
	}
	return result, nil
}

func (s *State) regexpCaseEqMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	input, ok := regexpInputString(args[0])
	if !ok {
		return False, nil
	}
	match, err := s.regexpSearch(self, data, input, 0)
	if err != nil {
		return Nil, err
	}
	if match == nil {
		s.regexpClearGlobals()
		return False, nil
	}
	s.regexpSetGlobals(match)
	return True, nil
}

func (s *State) regexpLastMatchMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	last := s.globals["$~"]
	if last.IsNil() {
		return Nil, nil
	}
	index := 0
	if len(args) == 1 {
		if args[0].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		index = int(args[0].Int())
	}
	return s.matchDataGroup(last, index, true)
}

func (s *State) regexpEscapeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindString {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into String")}
	}
	return StringValue(regexpEscape(args[0].String())), nil
}

func regexpEscape(text string) string {
	var builder strings.Builder
	builder.Grow(len(text) * 2)
	for i := 0; i < len(text); i++ {
		char := text[i]
		switch char {
		case '\n':
			builder.WriteString(`\n`)
		case '\t':
			builder.WriteString(`\t`)
		case '\r':
			builder.WriteString(`\r`)
		case '\f':
			builder.WriteString(`\f`)
		case '\v':
			builder.WriteString(`\v`)
		case '\\', '.', '*', '+', '?', '|', '(', ')', '[', ']', '{', '}', '^', '$', '#', '-', ' ':
			builder.WriteByte('\\')
			builder.WriteByte(char)
		default:
			builder.WriteByte(char)
		}
	}
	return builder.String()
}

func (s *State) regexpUnionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) == 1 {
		if array, ok := arrayDataOf(args[0]); ok {
			args = array.values
		}
	}
	if len(args) == 0 {
		return s.compileRegexpValue("(?!)", 0, s.RegexpClass)
	}
	if len(args) == 1 {
		if _, ok := regexpOf(args[0]); ok {
			return args[0], nil
		}
	}
	var builder strings.Builder
	for i, arg := range args {
		if i > 0 {
			builder.WriteByte('|')
		}
		if _, ok := regexpOf(arg); ok {
			data, err := s.regexpCompiled(arg)
			if err != nil {
				return Nil, err
			}
			_ = data
			text, err := s.regexpToSMethod(arg)
			if err != nil {
				return Nil, err
			}
			builder.WriteString(text.String())
			continue
		}
		if arg.Kind() != KindString {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion of "+s.classOf(arg).name+" into String")}
		}
		builder.WriteString(regexpEscape(arg.String()))
	}
	return s.compileRegexpValue(builder.String(), 0, s.RegexpClass)
}

func (s *State) compileRegexpValue(source string, flags int64, class *Class) (Value, error) {
	if class == nil {
		class = s.RegexpClass
	}
	value := ObjectValue(&Object{class: class})
	if _, err := s.regexpInitializeData(value, source, flags); err != nil {
		return Nil, err
	}
	return value, nil
}

func matchDataOf(value Value) (*matchData, bool) {
	if value.obj == nil {
		return nil, false
	}
	data, ok := value.obj.data.(*matchData)
	return data, ok
}

func (s *State) matchDataValue(data *matchData) Value {
	return ObjectValue(&Object{class: s.MatchDataClass, data: data})
}

// newMatchDataAt keeps the older helper shape used by the String methods.
func (s *State) newMatchDataAt(regexp Value, data *regexpData, input string, byteStart int) Value {
	match, err := s.regexpSearch(regexp, data, input, byteStart)
	if err != nil || match == nil {
		return Nil
	}
	return s.matchDataValue(match)
}

// regexpIndex returns the character offset of the first match, or -1.
func regexpIndex(regexp Value, data *regexpData, input string) int {
	if data == nil || data.compiled == nil {
		return -1
	}
	match, err := data.compiled.FindStringMatchStartingAt(input, 0)
	if err != nil || match == nil {
		return -1
	}
	return charOffsetOfByte(input, byteOffsetOfRune(input, match.Index))
}

func (s *State) matchDataGroupNumber(data *matchData, argument Value) (int, error) {
	switch argument.Kind() {
	case KindInt:
		index := int(argument.Int())
		if index < 0 {
			index += len(data.groups)
		}
		return index, nil
	case KindFloat:
		return int(argument.Float()), nil
	case KindString, KindSymbol:
		name := argument.String()
		best := -1
		for i := 1; i < len(data.names); i++ {
			if data.names[i] != name {
				continue
			}
			best = i
			if data.matched[i] {
				return i, nil
			}
		}
		if best >= 0 {
			return best, nil
		}
		return -1, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "undefined group name reference: "+name)}
	}
	return -1, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
}

func (s *State) matchDataGroup(self Value, index int, strict bool) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "not a match data")}
	}
	return s.matchDataGroupByIndex(data, index, strict)
}

func (s *State) matchDataGroupByIndex(data *matchData, index int, strict bool) (Value, error) {
	if index < 0 || index >= len(data.groups) {
		if strict {
			return Nil, nil
		}
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index out of range")}
	}
	if !data.matched[index] {
		return Nil, nil
	}
	return StringValue(data.groups[index]), nil
}

func (s *State) matchDataBeginEndOffset(data *matchData, index int) (int, int, bool) {
	if index < 0 || index >= len(data.groups) || !data.matched[index] {
		return 0, 0, false
	}
	return charOffsetOfByte(data.input, data.indices[index*2]), charOffsetOfByte(data.input, data.indices[index*2+1]), true
}

func (s *State) installMatchDataClass() {
	class := s.MatchDataClass
	class.define("[]", s.matchDataIndexMethod)
	class.define("to_a", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		values := make([]Value, len(data.groups))
		for i := range data.groups {
			if data.matched[i] {
				values[i] = StringValue(data.groups[i])
			} else {
				values[i] = Nil
			}
		}
		return ArrayValue(values), nil
	})
	class.define("captures", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		values := make([]Value, len(data.groups)-1)
		for i := 1; i < len(data.groups); i++ {
			if data.matched[i] {
				values[i-1] = StringValue(data.groups[i])
			} else {
				values[i-1] = Nil
			}
		}
		return ArrayValue(values), nil
	})
	class.define("names", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return ArrayValue(nil), nil
		}
		var names []Value
		seen := make(map[string]bool)
		for i := 1; i < len(data.names); i++ {
			name := data.names[i]
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, StringValue(name))
		}
		return ArrayValue(names), nil
	})
	class.define("named_captures", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return HashValue(nil), nil
		}
		var entries []hashEntry
		seen := make(map[string]bool)
		for i := 1; i < len(data.names); i++ {
			name := data.names[i]
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			index, err := s.matchDataGroupNumber(data, SymbolValue(name))
			if err != nil {
				return Nil, err
			}
			if index >= 0 && index < len(data.groups) && data.matched[index] {
				entries = append(entries, hashEntry{key: StringValue(name), value: StringValue(data.groups[index])})
			} else {
				entries = append(entries, hashEntry{key: StringValue(name), value: Nil})
			}
		}
		return HashValue(entries), nil
	})
	class.define("regexp", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return data.regexp, nil
		}
		return Nil, nil
	})
	class.define("string", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		result := StringValue(data.input)
		if result.obj != nil {
			result.obj.frozen = true
		}
		return result, nil
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
	class.define("to_s", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok || !data.matched[0] {
			return StringValue(""), nil
		}
		return StringValue(data.groups[0]), nil
	})
	class.define("begin", s.matchDataBeginMethod)
	class.define("end", s.matchDataEndMethod)
	class.define("offset", s.matchDataOffsetMethod)
	class.define("length", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return IntValue(int64(len(data.groups))), nil
		}
		return IntValue(0), nil
	})
	class.define("size", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return IntValue(int64(len(data.groups))), nil
		}
		return IntValue(0), nil
	})
	class.define("inspect", s.matchDataInspectMethod)
	class.define("__group", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, nil
		}
		return s.matchDataGroup(self, int(args[0].Int()), true)
	})
}

func (s *State) matchDataIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, nil
	}
	if len(args) == 1 {
		if rangeValue, ok := rangeDataOf(args[0]); ok {
			start, length := rangeBounds(rangeValue, len(data.groups))
			return s.matchDataSlice(data, start, length)
		}
		index, err := s.matchDataGroupNumber(data, args[0])
		if err != nil {
			return Nil, err
		}
		return s.matchDataGroupByIndex(data, index, true)
	}
	if len(args) == 2 {
		if args[0].Kind() != KindInt || args[1].Kind() != KindInt {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		return s.matchDataSlice(data, int(args[0].Int()), int(args[1].Int()))
	}
	return Nil, argumentCountError(s, len(args), "1..2")
}

func (s *State) matchDataSlice(data *matchData, start, length int) (Value, error) {
	if start < 0 {
		start += len(data.groups)
	}
	if start < 0 || start > len(data.groups) {
		return Nil, nil
	}
	if length < 0 {
		return Nil, nil
	}
	if start+length > len(data.groups) {
		length = len(data.groups) - start
	}
	values := make([]Value, length)
	for i := 0; i < length; i++ {
		index := start + i
		if data.matched[index] {
			values[i] = StringValue(data.groups[index])
		} else {
			values[i] = Nil
		}
	}
	return ArrayValue(values), nil
}

func (s *State) matchDataGroupArg(data *matchData, args []Value) (int, error) {
	index := 0
	if len(args) > 0 {
		value, err := s.matchDataGroupNumber(data, args[0])
		if err != nil {
			return 0, err
		}
		index = value
	}
	if index < 0 || index >= len(data.groups) {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, "index out of range")}
	}
	return index, nil
}

func (s *State) matchDataBeginMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, nil
	}
	index, err := s.matchDataGroupArg(data, args)
	if err != nil {
		return Nil, err
	}
	begin, _, matched := s.matchDataBeginEndOffset(data, index)
	if !matched {
		return Nil, nil
	}
	return IntValue(int64(begin)), nil
}

func (s *State) matchDataEndMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, nil
	}
	index, err := s.matchDataGroupArg(data, args)
	if err != nil {
		return Nil, err
	}
	_, end, matched := s.matchDataBeginEndOffset(data, index)
	if !matched {
		return Nil, nil
	}
	return IntValue(int64(end)), nil
}

func (s *State) matchDataOffsetMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, nil
	}
	index, err := s.matchDataGroupArg(data, args)
	if err != nil {
		return Nil, err
	}
	begin, end, matched := s.matchDataBeginEndOffset(data, index)
	if !matched {
		return ArrayValue([]Value{Nil, Nil}), nil
	}
	return ArrayValue([]Value{IntValue(int64(begin)), IntValue(int64(end))}), nil
}

func (s *State) matchDataInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return StringValue("#<MatchData>"), nil
	}
	parts := []string{"#<MatchData"}
	if data.matched[0] {
		parts = append(parts, inspectUTF8String(data.groups[0]))
	}
	for i := 1; i < len(data.groups); i++ {
		label := data.names[i]
		if label == "" {
			label = fmt.Sprint(i)
		}
		if data.matched[i] {
			parts = append(parts, label+":"+inspectUTF8String(data.groups[i]))
		} else {
			parts = append(parts, label+":nil")
		}
	}
	return StringValue(strings.Join(parts, " ") + ">"), nil
}

func (s *State) regexpAllMatches(self Value, data *regexpData, input string) ([]*matchData, error) {
	var results []*matchData
	position := 0
	for position <= len(input) {
		match, err := s.regexpSearch(self, data, input, position)
		if err != nil {
			return nil, err
		}
		if match == nil {
			break
		}
		results = append(results, match)
		next := match.end
		if next == match.start {
			if next >= len(input) {
				break
			}
			_, size := utf8.DecodeRuneInString(input[next:])
			if size <= 0 {
				size = 1
			}
			next += size
		}
		position = next
	}
	return results, nil
}

func expandRegexpReplacement(replacement string, match *matchData) string {
	var builder strings.Builder
	for i := 0; i < len(replacement); i++ {
		if replacement[i] != '\\' || i+1 >= len(replacement) {
			builder.WriteByte(replacement[i])
			continue
		}
		i++
		switch replacement[i] {
		case '0', '&':
			if match.matched[0] {
				builder.WriteString(match.groups[0])
			}
		case '`':
			builder.WriteString(match.input[:match.start])
		case '\'':
			builder.WriteString(match.input[match.end:])
		case '+':
			found := false
			for index := len(match.groups) - 1; index >= 1; index-- {
				if match.matched[index] {
					builder.WriteString(match.groups[index])
					found = true
					break
				}
			}
			_ = found
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			index := int(replacement[i] - '0')
			if index < len(match.groups) && match.matched[index] {
				builder.WriteString(match.groups[index])
			}
		case '\\':
			builder.WriteByte('\\')
		default:
			builder.WriteByte(replacement[i])
		}
	}
	return builder.String()
}

// translateRegexpInlineOptions maps Ruby's inline `m` (dot matches newline)
// onto regexp2's `s`. Anchors are multiline for every pattern already, so
// regexp2's own `m` would mean the wrong thing here.
func translateRegexpInlineOptions(source string) string {
	var builder strings.Builder
	for i := 0; i < len(source); i++ {
		if source[i] != '(' || i+1 >= len(source) || source[i+1] != '?' {
			builder.WriteByte(source[i])
			continue
		}
		j := i + 2
		if j < len(source) && (source[j] == '<' || source[j] == '=' || source[j] == '!' ||
			source[j] == ':' || source[j] == '#' || source[j] == '>' || source[j] == 'P' || source[j] == '&') {
			builder.WriteByte(source[i])
			continue
		}
		k := j
		seenLetter := false
		for k < len(source) && ((source[k] >= 'a' && source[k] <= 'z') || source[k] == '-') {
			if source[k] != '-' {
				seenLetter = true
			}
			k++
		}
		if !seenLetter || k >= len(source) || (source[k] != ':' && source[k] != ')') {
			builder.WriteByte(source[i])
			continue
		}
		builder.WriteString(source[i:j])
		for ; j < k; j++ {
			if source[j] == 'm' {
				builder.WriteByte('s')
			} else {
				builder.WriteByte(source[j])
			}
		}
		i = k - 1
	}
	return builder.String()
}
