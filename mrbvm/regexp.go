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
	// externalNames maps an internal group name, made unique to keep the
	// numbering Ruby gives groups that share a name, back to the name the
	// pattern spelled.
	externalNames map[string]string
}

type matchData struct {
	regexp  Value
	literal bool
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
	// regexp2 bounds its work with a match timeout rather than the
	// backtracking stack and step limits the C engine exposes. The names are
	// kept for programs that read them; a limit of one makes the checks that
	// size a pattern from them take their skip branch.
	class.constants["STACK_LIMIT"] = IntValue(1)
	class.constants["STEP_LIMIT"] = IntValue(1)

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
	compiled, externalNames, err := compileRegexpPattern(source, flags)
	if err != nil {
		data.compileFailed = true
		data.compileMessage = err.Error()
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.RegexpErrorClass, err.Error())}
	}
	data.compiled = compiled
	compiled.MatchTimeout = s.regexpMatchTimeout()
	data.externalNames = externalNames
	return self, nil
}

func (s *State) regexpMatchTimeout() time.Duration {
	timeout := time.Second
	if s.maxTime > 0 {
		remaining := s.maxTime - time.Since(s.startTime)
		if remaining < timeout {
			timeout = remaining
		}
		if timeout <= 0 {
			timeout = time.Millisecond
		}
	}
	return timeout
}

func compileRegexpPattern(source string, flags int64) (*regexp2.Regexp, map[string]string, error) {
	if regexpLinebreakInLookbehind(source) {
		return nil, nil, fmt.Errorf("invalid pattern in look-behind: /%s/", source)
	}
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
	unescaped, err := translateRubyUnicodeEscapes(source)
	if err != nil {
		return nil, nil, err
	}
	translated, externalNames := dedupeRegexpGroupNames(translateUnnamedGroupsWhenNamed(translatePOSIXClasses(translateLinebreakEscape(translateRubyAnchors(translatePossessiveQuantifiers(translateRubyQuantifiers(translateRegexpInlineOptions(unescaped))))))))
	if regexpReversedClassRange(translated) {
		return nil, nil, fmt.Errorf("empty range in char class: /%s/", source)
	}
	compiled, err := regexp2.Compile(translated, options)
	if err != nil {
		return nil, externalNames, err
	}
	compiled.MatchTimeout = time.Second
	return compiled, externalNames, nil
}

func regexpExternalName(data *regexpData, name string) string {
	if data != nil && name != "" {
		if external, ok := data.externalNames[name]; ok {
			return external
		}
	}
	return name
}

// regexpLinebreakInLookbehind reports a \R inside a look-behind, which
// Onigmo refuses because the escape is not of one fixed width.
func regexpLinebreakInLookbehind(source string) bool {
	for i := 0; i+3 < len(source); i++ {
		if source[i] != '(' || source[i+1] != '?' || source[i+2] != '<' ||
			(source[i+3] != '=' && source[i+3] != '!') {
			continue
		}
		depth := 0
		for j := i; j < len(source); j++ {
			if source[j] == '\\' {
				if j+1 < len(source) && source[j+1] == 'R' {
					return true
				}
				j++
				continue
			}
			if source[j] == '(' {
				depth++
			} else if source[j] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
	}
	return false
}

// translateLinebreakEscape spells out Onigmo's \R outside a character class
// and leaves it as the letter R inside one, which is what it means there.
func translateLinebreakEscape(source string) string {
	var builder strings.Builder
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '[' {
			inClass = true
			builder.WriteByte(c)
			continue
		}
		if c == ']' {
			inClass = false
			builder.WriteByte(c)
			continue
		}
		if c == '\\' && i+1 < len(source) {
			if source[i+1] == 'R' {
				if inClass {
					builder.WriteByte('R')
				} else {
					builder.WriteString(`(?>\r\n|[\n\v\f\r\x{85}\u2028\u2029])`)
				}
				i++
				continue
			}
			if source[i+1] == 'h' {
				if inClass {
					builder.WriteString(`0-9A-Fa-f`)
				} else {
					builder.WriteString(`[0-9A-Fa-f]`)
				}
				i++
				continue
			}
			if source[i+1] == 'H' {
				if !inClass {
					builder.WriteString(`[^0-9A-Fa-f]`)
				} else {
					builder.WriteString(`\H`)
				}
				i++
				continue
			}
			builder.WriteByte(c)
			builder.WriteByte(source[i+1])
			i++
			continue
		}
		builder.WriteByte(c)
	}
	return builder.String()
}

var regexpPOSIXClasses = map[string]string{
	"alpha":  `\p{L}\p{Nl}\p{Mn}\p{Mc}`,
	"digit":  `\p{Nd}`,
	"alnum":  `\p{L}\p{Nl}\p{Nd}`,
	"upper":  `\p{Lu}`,
	"lower":  `\p{Ll}`,
	"space":  `\p{Z}\t\n\v\f\r`,
	"blank":  `\p{Zs}\t`,
	"word":   `\p{L}\p{Nl}\p{Nd}_\p{Mn}\p{Mc}\p{Pc}`,
	"cntrl":  `\p{Cc}`,
	"xdigit": `0-9A-Fa-f`,
	"ascii":  `\x00-\x7F`,
	"print":  `^\p{C}\p{Zl}\p{Zp}`,
	"graph":  `^\p{Z}\p{C}`,
	"punct":  `\p{P}`,
	"w":      `\p{L}\p{Nl}\p{Nd}_`,
	"W":      `^\p{L}\p{Nl}\p{Nd}_`,
}

func appendRegexpLiteral(builder *strings.Builder, char rune, inClass bool) {
	text := string(char)
	if inClass {
		switch char {
		case '\\', ']', '-', '^', '[':
			builder.WriteByte('\\')
		}
	} else {
		switch char {
		case '\\', '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|', '^', '$':
			builder.WriteByte('\\')
		}
	}
	builder.WriteString(text)
}

func validUnicodeEscape(cp int) bool {
	return cp >= 0 && cp <= 0x10FFFF && !(cp >= 0xD800 && cp <= 0xDFFF)
}

// translateRubyUnicodeEscapes spells \uXXXX and \u{...} out as the characters
// they name, which also makes a following quantifier bind to the whole
// character and a list in a class read as the members it names.
func translateRubyUnicodeEscapes(source string) (string, error) {
	var builder strings.Builder
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '[' {
			inClass = true
			builder.WriteByte(c)
			continue
		}
		if c == ']' {
			inClass = false
			builder.WriteByte(c)
			continue
		}
		if c != '\\' || i+1 >= len(source) {
			builder.WriteByte(c)
			continue
		}
		next := source[i+1]
		if next == 'x' && i+3 < len(source) {
			var bytes []byte
			j := i
			for j+3 < len(source) && source[j] == '\\' && source[j+1] == 'x' {
				high, low := hexDigit(source[j+2]), hexDigit(source[j+3])
				if high < 0 || low < 0 {
					break
				}
				bytes = append(bytes, byte(high*16+low))
				j += 4
			}
			if len(bytes) > 0 {
				text := string(bytes)
				if utf8.ValidString(text) {
					for _, char := range text {
						appendRegexpLiteral(&builder, char, inClass)
					}
					i = j - 1
					continue
				}
				builder.WriteString(strings.ReplaceAll(text, `\`, `\\`))
				i = j - 1
				continue
			}
		}
		if next >= 0x80 {
			char, size := utf8.DecodeRuneInString(source[i+1:])
			if char != utf8.RuneError && size > 1 {
				appendRegexpLiteral(&builder, char, inClass)
				i += size
				continue
			}
		}
		if next != 'u' {
			builder.WriteByte(c)
			builder.WriteByte(next)
			i++
			continue
		}
		if i+2 >= len(source) {
			return "", fmt.Errorf("too short escape sequence: /%s/", source)
		}
		if source[i+2] == '{' {
			close := strings.IndexByte(source[i+3:], '}')
			if close < 0 {
				return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
			}
			content := source[i+3 : i+3+close]
			fields := strings.Fields(content)
			if len(fields) == 0 {
				return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
			}
			for _, field := range fields {
				if len(field) > 6 {
					return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
				}
				value := 0
				for j := 0; j < len(field); j++ {
					digit := hexDigit(field[j])
					if digit < 0 {
						return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
					}
					value = value*16 + digit
				}
				if !validUnicodeEscape(value) {
					return "", fmt.Errorf("invalid Unicode range: /%s/", source)
				}
				appendRegexpLiteral(&builder, rune(value), inClass)
			}
			i = i + 3 + close
			continue
		}
		if i+2 >= len(source) {
			return "", fmt.Errorf("too short escape sequence: /%s/", source)
		}
		if i+5 >= len(source) {
			return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
		}
		value := 0
		for j := 0; j < 4; j++ {
			digit := hexDigit(source[i+2+j])
			if digit < 0 {
				return "", fmt.Errorf("invalid Unicode escape: /%s/", source)
			}
			value = value*16 + digit
		}
		if !validUnicodeEscape(value) {
			return "", fmt.Errorf("invalid Unicode range: /%s/", source)
		}
		appendRegexpLiteral(&builder, rune(value), inClass)
		i += 5
	}
	return builder.String(), nil
}

// regexpReversedClassRange reports a range in a character class whose low
// bound is above its high bound, which Onigmo refuses.
func regexpReversedClassRange(source string) bool {
	inClass := false
	var previous rune
	havePrevious := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			char, size := utf8.DecodeRuneInString(source[i+1:])
			if char == utf8.RuneError {
				char = rune(source[i+1])
				size = 1
			}
			previous, havePrevious = char, true
			i += size
			continue
		}
		if c == '[' {
			inClass, havePrevious = true, false
			if i+1 < len(source) && source[i+1] == '^' {
				i++
			}
			continue
		}
		if c == ']' {
			inClass, havePrevious = false, false
			continue
		}
		if !inClass {
			continue
		}
		if c == '-' && havePrevious && i+1 < len(source) && source[i+1] != ']' && source[i+1] != '-' {
			next, size := utf8.DecodeRuneInString(source[i+1:])
			if next == utf8.RuneError {
				next = rune(source[i+1])
				size = 1
			}
			if previous > next {
				return true
			}
			previous, havePrevious = next, true
			i += size - 1
			continue
		}
		char, size := utf8.DecodeRuneInString(source[i:])
		if char == utf8.RuneError {
			char = rune(c)
			size = 1
		}
		previous, havePrevious = char, true
		i += size - 1
	}
	return false
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// translatePOSIXClasses replaces [:name:] and [:^name:] with the members
// regexp2 can spell. The bracket text is left in place so that the outer class
// still delimits it.
func translatePOSIXClasses(source string) string {
	var builder strings.Builder
	for i := 0; i < len(source); i++ {
		if source[i] != '[' || i+2 >= len(source) || source[i+1] != ':' {
			builder.WriteByte(source[i])
			continue
		}
		negate := false
		start := i + 2
		if source[start] == '^' {
			negate = true
			start++
		}
		end := strings.Index(source[start:], ":]")
		if end < 0 {
			builder.WriteByte(source[i])
			continue
		}
		name := strings.ToLower(source[start : start+end])
		members, ok := regexpPOSIXClasses[name]
		if !ok {
			builder.WriteByte(source[i])
			continue
		}
		if negate {
			builder.WriteString(`^`)
		}
		builder.WriteString(members)
		i = start + end + 1
	}
	return builder.String()
}

// translateUnnamedGroupsWhenNamed turns plain capturing groups into
// non-capturing ones in a pattern that names any group, which is Onigmo's
// rule: a named group takes the numbering over, and a plain group beside it
// has no number left to reach.
func translateUnnamedGroupsWhenNamed(source string) string {
	named := false
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass && i+2 < len(source) && source[i+1] == '?' {
				if source[i+2] == '<' && i+3 < len(source) && source[i+3] != '=' && source[i+3] != '!' {
					named = true
				} else if source[i+2] == '\'' {
					named = true
				}
			}
		}
	}
	if !named {
		return source
	}
	var builder strings.Builder
	inClass = false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			builder.WriteByte(c)
			builder.WriteByte(source[i+1])
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass && (i+1 >= len(source) || source[i+1] != '?') {
				builder.WriteString("(?:")
				continue
			}
		}
		builder.WriteByte(c)
	}
	return builder.String()
}

// dedupeRegexpGroupNames gives every occurrence of a repeated capture name a
// unique internal name. Onigmo numbers each occurrence separately, where
// regexp2 merges groups that share a name; the external table puts the
// spelled name back on the match.
func dedupeRegexpGroupNames(source string) (string, map[string]string) {
	var builder strings.Builder
	counts := make(map[string]int)
	external := make(map[string]string)
	inClass := false
	for i := 0; i < len(source); {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			builder.WriteByte(c)
			builder.WriteByte(source[i+1])
			i += 2
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		}
		if !inClass && c == '(' && i+2 < len(source) && source[i+1] == '?' {
			start := -1
			close := byte(0)
			if source[i+2] == '<' && i+3 < len(source) && source[i+3] != '=' && source[i+3] != '!' {
				start = i + 3
				close = '>'
			} else if source[i+2] == '\'' {
				start = i + 3
				close = '\''
			}
			if start >= 0 {
				end := start
				for end < len(source) && source[end] != close {
					end++
				}
				if end < len(source) && end > start && validRegexpGroupName(source[start:end]) {
					name := source[start:end]
					counts[name]++
					internal := name
					if counts[name] > 1 {
						internal = fmt.Sprintf("mrbdup%d_%s", counts[name], name)
						external[internal] = name
					}
					builder.WriteString(source[i:start])
					builder.WriteString(internal)
					builder.WriteByte(close)
					i = end + 1
					continue
				}
			}
		}
		builder.WriteByte(c)
		i++
	}
	return builder.String(), external
}

func validRegexpGroupName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
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
			if _, _, err := compileRegexpPattern(body, on); err == nil {
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
		name = regexpExternalName(data, name)
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
		if regexpTimeoutError(err) {
			return nil, nil
		}
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
		result.names[i] = regexpExternalName(data, group.Name)
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
	binary := stringBinaryOf(args[0])
	if !binary && !utf8.ValidString(input) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")}
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	match, err := s.regexpSearchMode(self, data, input, startByte, binary)
	if err != nil {
		return Nil, err
	}
	return BoolValue(match != nil), nil
}

func (s *State) regexpMatchOperatorMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, nil
	}
	if args[0].IsNil() {
		s.regexpClearGlobals()
		return Nil, nil
	}
	input, ok := regexpInputString(args[0])
	if !ok {
		return Nil, regexpTypeError(s, args[0])
	}
	binary := stringBinaryOf(args[0])
	if !binary && !utf8.ValidString(input) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")}
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	match, err := s.regexpSearchMode(self, data, input, 0, binary)
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
	binary := stringBinaryOf(args[0])
	if !binary && !utf8.ValidString(input) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")}
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	match, err := s.regexpSearchMode(self, data, input, startByte, binary)
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
	input, ok := regexpInputString(args[0])
	if !ok {
		return False, nil
	}
	binary := stringBinaryOf(args[0])
	if !binary && !utf8.ValidString(input) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")}
	}
	data, err := s.regexpCompiled(self)
	if err != nil {
		return Nil, err
	}
	match, err := s.regexpSearchMode(self, data, input, 0, binary)
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
	if len(args) == 0 {
		return last, nil
	}
	if args[0].Kind() != KindInt {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	return s.matchDataGroup(last, int(args[0].Int()), true)
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

func (s *State) matchDataResolveGroup(data *matchData, argument Value) (int, error) {
	switch argument.Kind() {
	case KindInt:
		return int(argument.Int()), nil
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

func (s *State) matchDataGroupNumber(data *matchData, argument Value) (int, error) {
	index, err := s.matchDataResolveGroup(data, argument)
	if err != nil {
		return -1, err
	}
	if index < 0 {
		index += len(data.groups)
	}
	return index, nil
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
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		return s.matchDataRegexpValue(data)
	})
	class.define("values_at", s.matchDataValuesAtMethod)
	class.define("==", s.matchDataEqualMethod)
	class.define("eql?", s.matchDataEqualMethod)
	class.define("hash", s.matchDataHashMethod)
	class.define("initialize_copy", s.matchDataInitializeCopyMethod)
	class.define("__group", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 || args[0].Kind() != KindInt {
			return Nil, nil
		}
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		index := int(args[0].Int())
		if index < 0 || index >= len(data.groups) {
			return Nil, nil
		}
		return s.matchDataGroupByIndex(data, index, true)
	})
	class.define("__last_group", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		data, ok := matchDataOf(self)
		if !ok {
			return Nil, nil
		}
		for i := len(data.groups) - 1; i >= 1; i-- {
			if data.matched[i] {
				return StringValue(data.groups[i]), nil
			}
		}
		return Nil, nil
	})
	class.define("__pre_match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return StringValue(data.input[:data.start]), nil
		}
		return Nil, nil
	})
	class.define("__post_match", func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if data, ok := matchDataOf(self); ok {
			return StringValue(data.input[data.end:]), nil
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
}

func (s *State) matchDataIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return Nil, nil
	}
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	sliceForm := len(args) == 2 && !args[1].IsNil()
	if !sliceForm {
		if rangeValue, ok := rangeDataOf(args[0]); ok {
			if low := rangeValue.low; low.Kind() == KindInt && int(low.Int()) < -len(data.groups) {
				return Nil, nil
			}
			start, length := rangeBounds(rangeValue, len(data.groups))
			return s.matchDataSlice(data, start, length)
		}
		negative := (args[0].Kind() == KindInt && args[0].Int() < 0) || (args[0].Kind() == KindFloat && args[0].Float() < 0)
		index, err := s.matchDataGroupNumber(data, args[0])
		if err != nil {
			return Nil, err
		}
		if negative && index <= 0 {
			return Nil, nil
		}
		return s.matchDataGroupByIndex(data, index, true)
	}
	start, ok := regexpIntegerArg(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	length, ok := regexpIntegerArg(args[1])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
	}
	if start < 0 {
		start += len(data.groups)
		if start < 0 {
			return Nil, nil
		}
	} else if start > len(data.groups) {
		return Nil, nil
	}
	if length < 0 {
		return Nil, nil
	}
	if length > len(data.groups)-start {
		length = len(data.groups) - start
	}
	return s.matchDataSlice(data, start, length)
}

func regexpIntegerArg(value Value) (int, bool) {
	switch value.Kind() {
	case KindInt:
		return int(value.Int()), true
	case KindFloat:
		return int(value.Float()), true
	}
	return 0, false
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
		value, err := s.matchDataResolveGroup(data, args[0])
		if err != nil {
			return 0, err
		}
		index = value
	}
	if index < 0 || index >= len(data.groups) {
		return 0, &unwindSignal{kind: unwindException, value: newException(s.IndexErrorClass, fmt.Sprintf("index %d out of matches", index))}
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
	if data.literal && data.regexp.IsNil() {
		return StringValue("#<MatchData: " + data.groups[0] + ">"), nil
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

// translateRubyQuantifiers spells out Onigmo's `{,n}` repetition, which
// means `{0,n}` there and is not a repetition in regexp2. A quantifier inside
// a character class or escaped stands for itself.
func translateRubyQuantifiers(source string) string {
	var builder strings.Builder
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			if !inClass && source[i+1] == '0' && (i+2 >= len(source) || source[i+2] < '0' || source[i+2] > '9') {
				builder.WriteString(`(?:\\x00)`)
				i++
				continue
			}
			builder.WriteByte(c)
			builder.WriteByte(source[i+1])
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '{':
			if !inClass && i+1 < len(source) && source[i+1] == ',' {
				builder.WriteString("{0")
				continue
			}
		}
		builder.WriteByte(c)
	}
	return builder.String()
}

// translatePossessiveQuantifiers turns Onigmo's possessive quantifiers into
// atomic groups, which is the same match with the backtracking handed back at
// the same point and what regexp2 can express.
func translatePossessiveQuantifiers(source string) string {
	out := make([]byte, 0, len(source))
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			out = append(out, c, source[i+1])
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		}
		if !inClass && (c == '*' || c == '+' || c == '?') && i+1 < len(source) && source[i+1] == '+' {
			start := regexpAtomStart(out)
			if start >= 0 {
				atom := append([]byte(nil), out[start:]...)
				out = out[:start]
				out = append(out, "(?>"...)
				out = append(out, atom...)
				out = append(out, c, ')')
				i++
				continue
			}
		}
		if !inClass && c == '}' && i+1 < len(source) && source[i+1] == '+' {
			if open := bytesLastIndexByte(out, '{'); open >= 0 {
				start := regexpAtomStart(out[:open])
				if start >= 0 {
					atom := append([]byte(nil), out[start:]...)
					out = out[:start]
					out = append(out, "(?>"...)
					out = append(out, atom...)
					out = append(out, '}')
					out = append(out, ')')
					i++
					continue
				}
			}
		}
		out = append(out, c)
	}
	return string(out)
}

func bytesLastIndexByte(data []byte, c byte) int {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == c {
			return i
		}
	}
	return -1
}

// regexpAtomStart answers the start of the atom a quantifier in `out` would
// repeat: a group, a character class, an escape, or one character.
func regexpAtomStart(out []byte) int {
	if len(out) == 0 {
		return -1
	}
	end := len(out) - 1
	switch out[end] {
	case ')':
		depth := 0
		for i := end; i >= 0; i-- {
			if out[i] == '\\' {
				i--
				continue
			}
			if out[i] == ')' {
				depth++
			} else if out[i] == '(' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
		return -1
	case ']':
		for i := end; i >= 0; i-- {
			if out[i] == ']' {
				continue
			}
			if out[i] == '[' {
				return i
			}
		}
		return -1
	}
	if end >= 1 && out[end-1] == '\\' {
		return end - 1
	}
	_, size := utf8.DecodeLastRune(out)
	if size <= 0 {
		size = 1
	}
	return len(out) - size
}

// translateRubyAnchors rewrites Ruby's `^`, which matches the start of the
// string and the position after any newline but never the end of a string
// that ends with one. regexp2 always matches the trailing position, so the
// anchor is spelled out. `^` inside a character class and an escaped one
// stand for themselves.
func translateRubyAnchors(source string) string {
	var builder strings.Builder
	inClass := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\\' && i+1 < len(source) {
			builder.WriteByte(c)
			builder.WriteByte(source[i+1])
			i++
			continue
		}
		switch c {
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '^':
			if !inClass {
				builder.WriteString(`(?:\A|(?<=\n)(?!\z))`)
				continue
			}
		}
		builder.WriteByte(c)
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

// matchDataRegexpValue answers the Regexp a match reports itself as. A match
// against a literal String pattern compiled nothing, so the quoted pattern is
// built here on the first ask and kept in the one-entry memo CRuby's
// rb_reg_regcomp() uses.
func (s *State) matchDataRegexpValue(data *matchData) (Value, error) {
	if !data.regexp.IsNil() {
		return data.regexp, nil
	}
	literal := data.input[data.start:data.end]
	if s.regexpQuotedSet && s.regexpQuotedLiteral == literal {
		data.regexp = s.regexpQuotedRegexp
		return data.regexp, nil
	}
	source := regexpEscape(literal)
	regexp, err := s.compileRegexpValue(source, 0, s.RegexpClass)
	if err != nil {
		return Nil, err
	}
	s.regexpQuotedSet = true
	s.regexpQuotedLiteral = literal
	s.regexpQuotedRegexp = regexp
	data.regexp = regexp
	return regexp, nil
}

func regexpTimeoutError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "match timeout")
}

// binaryEngineText maps each subject byte to one rune so that the regexp
// engine, which counts in runes, sees one position per byte. The pattern side
// stays as compiled; ASCII patterns and ASCII bytes are unaffected.
func binaryEngineText(input string) (string, []int) {
	var builder strings.Builder
	builder.Grow(len(input) * 2)
	offsets := make([]int, len(input)+1)
	for i := 0; i < len(input); i++ {
		offsets[i] = builder.Len()
		builder.WriteRune(rune(input[i]))
	}
	offsets[len(input)] = builder.Len()
	return builder.String(), offsets
}

// regexpSearchBinary searches a subject read by byte. Group offsets are
// reported in byte space, which the one-rune-per-byte rendering preserves,
// and the group text is sliced from the original bytes.
func (s *State) regexpSearchBinary(self Value, data *regexpData, input string, startByte int) (*matchData, error) {
	if startByte < 0 || startByte > len(input) {
		return nil, nil
	}
	engine, offsets := binaryEngineText(input)
	match, err := data.compiled.FindStringMatchStartingAt(engine, offsets[startByte])
	if err != nil {
		if regexpTimeoutError(err) {
			return nil, nil
		}
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
		result.names[i] = regexpExternalName(data, group.Name)
		if isDecimalString(group.Name) {
			result.names[i] = ""
		}
		if len(group.Captures) == 0 {
			result.indices[i*2] = -1
			result.indices[i*2+1] = -1
			continue
		}
		begin := group.Index
		end := group.Index + group.Length
		if begin < 0 || end < begin || end > len(input) {
			result.indices[i*2] = -1
			result.indices[i*2+1] = -1
			continue
		}
		result.matched[i] = true
		result.groups[i] = input[begin:end]
		result.indices[i*2] = begin
		result.indices[i*2+1] = end
	}
	result.start = result.indices[0]
	result.end = result.indices[1]
	return result, nil
}

func (s *State) regexpSearchMode(self Value, data *regexpData, input string, startByte int, binary bool) (*matchData, error) {
	if binary {
		return s.regexpSearchBinary(self, data, input, startByte)
	}
	return s.regexpSearch(self, data, input, startByte)
}

func matchDataGroupValue(data *matchData, index int) Value {
	if index < 0 || index >= len(data.groups) || !data.matched[index] {
		return Nil
	}
	return StringValue(data.groups[index])
}

func (s *State) matchDataValuesAtMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return ArrayValue(nil), nil
	}
	values := []Value{}
	for _, arg := range args {
		if arg.Kind() == KindString || arg.Kind() == KindSymbol {
			index, err := s.matchDataResolveGroup(data, arg)
			if err != nil {
				return Nil, err
			}
			values = append(values, matchDataGroupValue(data, index))
			continue
		}
		if rangeValue, ok := rangeDataOf(arg); ok {
			low := rangeValue.low
			if low.Kind() == KindInt && int(low.Int()) < -len(data.groups) {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, arg.Inspect()+" out of range")}
			}
			start := 0
			end := len(data.groups) - 1
			if low.Kind() == KindInt {
				start = int(low.Int())
				if start < 0 {
					start += len(data.groups)
				}
			}
			high := rangeValue.high
			if high.Kind() == KindInt {
				end = int(high.Int())
				if end < 0 {
					end += len(data.groups)
				}
				if rangeValue.exclusive {
					end--
				}
			}
			if start < 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.RangeErrorClass, arg.Inspect()+" out of range")}
			}
			for i := start; i <= end; i++ {
				if i < 0 || i >= len(data.groups) {
					values = append(values, Nil)
				} else {
					values = append(values, matchDataGroupValue(data, i))
				}
			}
			continue
		}
		index, ok := regexpIntegerArg(arg)
		if !ok {
			return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "no implicit conversion into Integer")}
		}
		if index < 0 {
			index += len(data.groups)
			if index <= 0 {
				values = append(values, Nil)
				continue
			}
		}
		values = append(values, matchDataGroupValue(data, index))
	}
	return ArrayValue(values), nil
}

func (s *State) matchDataEqualMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	if identicalValue(self, args[0]) {
		return True, nil
	}
	left, ok := matchDataOf(self)
	if !ok {
		return False, nil
	}
	right, ok := matchDataOf(args[0])
	if !ok {
		return False, nil
	}
	if len(left.groups) != len(right.groups) || left.input != right.input {
		return False, nil
	}
	for i := range left.indices {
		if left.indices[i] != right.indices[i] {
			return False, nil
		}
	}
	for i := range left.matched {
		if left.matched[i] != right.matched[i] {
			return False, nil
		}
	}
	leftRegexp, err := s.matchDataRegexpValue(left)
	if err != nil {
		return False, nil
	}
	rightRegexp, err := s.matchDataRegexpValue(right)
	if err != nil {
		return False, nil
	}
	if _, ok := regexpOf(leftRegexp); !ok {
		return False, nil
	}
	if _, ok := regexpOf(rightRegexp); !ok {
		return False, nil
	}
	leftSource, leftFlags, err := s.regexpSourceAndFlags(leftRegexp)
	if err != nil {
		return False, nil
	}
	rightSource, rightFlags, err := s.regexpSourceAndFlags(rightRegexp)
	if err != nil {
		return False, nil
	}
	return BoolValue(leftSource == rightSource && leftFlags == rightFlags), nil
}

func matchDataHashString(text string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= 1099511628211
	}
	return h
}

func (s *State) matchDataHashMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	data, ok := matchDataOf(self)
	if !ok {
		return IntValue(0), nil
	}
	h := matchDataHashString(data.input)
	for _, index := range data.indices {
		h = h*31 + uint64(index)
	}
	if regexp, err := s.matchDataRegexpValue(data); err == nil {
		if source, flags, err := s.regexpSourceAndFlags(regexp); err == nil {
			h ^= matchDataHashString(source) ^ uint64(flags)*0x9e3779b9
		}
	}
	return IntValue(int64(h)), nil
}

func (s *State) matchDataInitializeCopyMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if identicalValue(self, args[0]) {
		return self, nil
	}
	if self.obj == nil || s.classOf(self) != s.classOf(args[0]) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "initialize_copy should take same class object")}
	}
	if _, holds := matchDataOf(self); holds {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "already initialized MatchData")}
	}
	source, ok := matchDataOf(args[0])
	if !ok {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "uninitialized MatchData")}
	}
	copyData := *source
	copyData.groups = append([]string(nil), source.groups...)
	copyData.matched = append([]bool(nil), source.matched...)
	copyData.indices = append([]int(nil), source.indices...)
	copyData.names = append([]string(nil), source.names...)
	self.obj.data = &copyData
	return self, nil
}
