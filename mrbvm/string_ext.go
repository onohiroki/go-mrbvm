package mrbvm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// installStringExtensionLibrary adds the methods mruby implements in
// mruby-string-ext. Many of them are also used by the core string tests.
func (s *State) installStringExtensionLibrary() {
	class := s.StringClass
	class.define("length", s.stringExtLengthMethod)
	class.define("size", s.stringExtLengthMethod)
	class.define("inspect", s.stringExtInspectMethod)
	class.define("dump", s.stringExtDumpMethod)
	class.define("b", s.stringExtBinaryMethod)
	class.define("chars", s.stringExtCharsMethod)
	class.define("each_char", s.stringExtEachCharMethod)
	class.define("codepoints", s.stringExtCodepointsMethod)
	class.define("each_codepoint", s.stringExtEachCodepointMethod)
	class.define("ord", s.stringExtOrdMethod)
	class.define("chr", s.stringExtChrMethod)
	class.define("clear", s.stringExtClearMethod)
	class.define("concat", s.stringExtConcatMethod)
	class.define("<<", s.stringExtConcatMethod)
	class.define("append_as_bytes", s.stringExtAppendAsBytesMethod)
	class.define("prepend", s.stringExtPrependMethod)
	class.define("insert", s.stringExtInsertMethod)
	class.define("start_with?", s.stringExtStartWithMethod)
	class.define("end_with?", s.stringExtEndWithMethod)
	class.define("partition", s.stringExtPartitionMethod)
	class.define("rpartition", s.stringExtRpartitionMethod)
	class.define("hex", s.stringExtHexMethod)
	class.define("oct", s.stringExtOctMethod)
	class.define("slice!", s.stringExtSliceBangMethod)
	class.define("slice", s.stringExtSliceMethod)
	class.define("[]", s.stringExtSliceMethod)
	class.define("index", s.stringExtIndexMethod)
	class.define("rindex", s.stringExtRindexMethod)
	class.define("byteindex", s.stringExtByteindexMethod)
	class.define("byterindex", s.stringExtByterindexMethod)
	class.define("include?", s.stringExtIncludeMethod)
	class.define("swapcase", s.stringExtSwapcaseMethod)
	class.define("swapcase!", s.stringExtSwapcaseBangMethod)
	class.define("casecmp?", s.stringExtCasecmpPredicateMethod)
	class.define("casecmp", s.stringExtCasecmpMethod)
	class.define("chop", s.stringExtChopMethod)
	class.define("chop!", s.stringExtChopBangMethod)
	class.define("strip", s.stringExtStripMethod)
	class.define("lstrip", s.stringExtLstripMethod)
	class.define("rstrip", s.stringExtRstripMethod)
	class.define("strip!", s.stringExtStripBangMethod)
	class.define("lstrip!", s.stringExtLstripBangMethod)
	class.define("rstrip!", s.stringExtRstripBangMethod)
	class.define("ljust", s.stringExtLjustMethod)
	class.define("rjust", s.stringExtRjustMethod)
	class.define("center", s.stringExtCenterMethod)
	class.define("delete_prefix", s.stringExtDeletePrefixMethod)
	class.define("delete_suffix", s.stringExtDeleteSuffixMethod)
	class.define("delete_prefix!", s.stringExtDeletePrefixBangMethod)
	class.define("delete_suffix!", s.stringExtDeleteSuffixBangMethod)
	class.define("+@", s.stringExtUPlusMethod)
	class.define("-@", s.stringExtUMinusMethod)
	class.define("dedup", s.stringExtUMinusMethod)
	class.define("succ", s.stringExtSuccMethod)
	class.define("next", s.stringExtSuccMethod)
	class.define("succ!", s.stringExtSuccBangMethod)
	class.define("next!", s.stringExtSuccBangMethod)
	class.define("upto", s.stringExtUptoMethod)
	class.define("scrub", s.stringExtScrubMethod)
	class.define("scrub!", s.stringExtScrubBangMethod)
	class.define("count", s.stringExtCountMethod)
	class.define("delete", s.stringExtDeleteMethod)
	class.define("delete!", s.stringExtDeleteBangMethod)
	class.define("squeeze", s.stringExtSqueezeMethod)
	class.define("squeeze!", s.stringExtSqueezeBangMethod)
	class.define("tr", s.stringExtTrMethod)
	class.define("tr!", s.stringExtTrBangMethod)
	class.define("tr_s", s.stringExtTrSMethod)
	class.define("tr_s!", s.stringExtTrSBangMethod)
}

func (s *State) stringExtError(class *Class, message string) error {
	return &unwindSignal{kind: unwindException, value: newException(class, message)}
}

type stringUnit struct {
	value int
	start int
	end   int
}

func stringUnitsOf(text string, binary bool) ([]stringUnit, error) {
	if binary {
		units := make([]stringUnit, len(text))
		for i := 0; i < len(text); i++ {
			units[i] = stringUnit{value: int(text[i]), start: i, end: i + 1}
		}
		return units, nil
	}
	units := make([]stringUnit, 0, len(text))
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, errInvalidCaseInput
		}
		units = append(units, stringUnit{value: int(r), start: i, end: i + size})
		i += size
	}
	return units, nil
}

func stringUnitsText(text string, units []stringUnit) string {
	var builder strings.Builder
	for _, unit := range units {
		builder.WriteString(text[unit.start:unit.end])
	}
	return builder.String()
}

func (s *State) stringExtLengthMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if stringBinaryOf(self) {
		return IntValue(int64(len(self.String()))), nil
	}
	return IntValue(int64(stringCharCount(self.String()))), nil
}

func (s *State) stringExtInspectMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	if stringBinaryOf(self) {
		return StringValue(inspectBinaryString(text)), nil
	}
	return StringValue(inspectUTF8String(text)), nil
}

func inspectBinaryString(text string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for i := 0; i < len(text); i++ {
		b := text[i]
		switch b {
		case '\a':
			builder.WriteString(`\a`)
		case '\b':
			builder.WriteString(`\b`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\v':
			builder.WriteString(`\v`)
		case '\f':
			builder.WriteString(`\f`)
		case '\r':
			builder.WriteString(`\r`)
		case 0x1b:
			builder.WriteString(`\e`)
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		default:
			if b < 0x20 || b >= 0x7f {
				fmt.Fprintf(&builder, "\\x%02X", b)
			} else {
				builder.WriteByte(b)
			}
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func (s *State) stringExtDumpMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return StringValue(inspectBinaryString(self.String())), nil
}

func (s *State) stringExtBinaryMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	result := BinaryStringValue(self.String())
	if self.obj != nil && result.obj != nil {
		result.obj.class = self.obj.class
	}
	return result, nil
}

func (s *State) stringExtCharsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	var units []stringUnit
	if stringBinaryOf(self) {
		units, _ = stringUnitsOf(text, true)
	} else {
		for _, span := range stringCharSpans(text) {
			units = append(units, stringUnit{start: span.start, end: span.end})
		}
	}
	if !block.IsNil() {
		for _, unit := range units {
			if _, err := s.callBlock1(ctx, block, stringValueLike(self, text[unit.start:unit.end]), Nil); err != nil {
				return Nil, err
			}
		}
		return self, nil
	}
	values := make([]Value, len(units))
	for i, unit := range units {
		values[i] = stringValueLike(self, text[unit.start:unit.end])
	}
	return ArrayValue(values), nil
}

func (s *State) stringExtEachCharMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_char", nil), nil
	}
	return s.stringExtCharsMethod(ctx, self, args, kw, block)
}

func (s *State) stringExtCodepointsMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	binary := stringBinaryOf(self)
	units, err := stringUnitsOf(text, binary)
	if err != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	if !block.IsNil() {
		for _, unit := range units {
			if _, err := s.callBlock1(ctx, block, IntValue(int64(unit.value)), Nil); err != nil {
				return Nil, err
			}
		}
		return self, nil
	}
	values := make([]Value, len(units))
	for i, unit := range units {
		values[i] = IntValue(int64(unit.value))
	}
	return ArrayValue(values), nil
}

func (s *State) stringExtEachCodepointMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if block.IsNil() {
		return s.toEnum(self, "each_codepoint", nil), nil
	}
	return s.stringExtCodepointsMethod(ctx, self, args, kw, block)
}

func (s *State) stringExtOrdMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	if text == "" {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "empty string")
	}
	if stringBinaryOf(self) {
		return IntValue(int64(text[0])), nil
	}
	r, size := utf8.DecodeRuneInString(text)
	if r == utf8.RuneError && size == 1 {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	return IntValue(int64(r)), nil
}

func (s *State) stringExtChrMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	if text == "" {
		return stringValueLike(self, ""), nil
	}
	if stringBinaryOf(self) {
		return stringValueLike(self, text[:1]), nil
	}
	_, size := utf8.DecodeRuneInString(text)
	return stringValueLike(self, text[:size]), nil
}

func (s *State) stringExtClearMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	data.set("")
	return self, nil
}

func (s *State) stringExtConcatMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	binary := data.binary
	for _, arg := range args {
		var addition string
		switch arg.Kind() {
		case KindString:
			addition = arg.String()
			if !binary && stringBinaryOf(arg) {
				add := []byte(addition)
				for _, b := range add {
					if b >= 0x80 {
						binary = true
						break
					}
				}
			}
		case KindInt:
			if binary {
				if arg.Int() < 0 || arg.Int() > 0xff {
					return Nil, s.stringExtError(s.RangeErrorClass, "out of char range")
				}
				addition = string([]byte{byte(arg.Int())})
			} else {
				addition = string(rune(arg.Int()))
			}
		case KindFloat:
			addition = string(rune(int64(arg.Float())))
		default:
			return Nil, s.stringExtError(s.TypeErrorClass, fmt.Sprintf("no implicit conversion of %s into String", s.classOf(arg).name))
		}
		if err := s.checkStringSize(data.len() + len(addition)); err != nil {
			return Nil, err
		}
		data.appendString(addition)
	}
	if binary {
		data.binary = true
	}
	return self, nil
}

func (s *State) stringExtAppendAsBytesMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	for _, arg := range args {
		switch arg.Kind() {
		case KindString:
			data.appendString(arg.String())
		case KindInt:
			data.appendString(string([]byte{byte(arg.Int())}))
		default:
			return Nil, s.stringExtError(s.TypeErrorClass, fmt.Sprintf("no implicit conversion of %s into String", s.classOf(arg).name))
		}
	}
	data.binary = true
	return self, nil
}

func (s *State) stringExtPrependMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	original := data.str()
	var prefix strings.Builder
	for _, arg := range args {
		if arg.Kind() != KindString {
			return Nil, s.stringExtError(s.TypeErrorClass, fmt.Sprintf("no implicit conversion of %s into String", s.classOf(arg).name))
		}
		prefix.WriteString(arg.String())
	}
	data.set(prefix.String() + original)
	return self, nil
}

func (s *State) stringExtInsertMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	if len(args) != 2 || args[0].Kind() != KindInt || args[1].Kind() != KindString {
		return Nil, s.stringExtError(s.TypeErrorClass, "no implicit conversion")
	}
	original := data.str()
	count := stringCharCount(original)
	if stringBinaryOf(self) {
		count = len(original)
	}
	index := int(args[0].Int())
	if index < 0 {
		index += count + 1
	}
	if index < 0 || index > count {
		return Nil, s.stringExtError(s.IndexErrorClass, "index out of string")
	}
	inserted := args[1].String()
	if stringBinaryOf(self) {
		data.set(original[:index] + inserted + original[index:])
		return self, nil
	}
	byteIndex, _ := stringCharIndexToByte(original, index)
	data.set(original[:byteIndex] + inserted + original[byteIndex:])
	return self, nil
}

func (s *State) stringExtStringArg(arg Value) (string, error) {
	if arg.Kind() != KindString {
		return "", s.stringExtError(s.TypeErrorClass, fmt.Sprintf("no implicit conversion of %s into String", s.classOf(arg).name))
	}
	return arg.String(), nil
}

func (s *State) stringExtStartWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	for _, arg := range args {
		needle, err := s.stringExtStringArg(arg)
		if err != nil {
			return Nil, err
		}
		if !stringBinaryOf(self) && !stringBinaryOf(arg) && !utf8.ValidString(needle) {
			continue
		}
		if strings.HasPrefix(text, needle) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) stringExtEndWithMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	for _, arg := range args {
		needle, err := s.stringExtStringArg(arg)
		if err != nil {
			return Nil, err
		}
		if !stringBinaryOf(self) && !stringBinaryOf(arg) && !utf8.ValidString(needle) {
			continue
		}
		if strings.HasSuffix(text, needle) {
			return True, nil
		}
	}
	return False, nil
}

func (s *State) stringExtPartitionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	separator, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if separator == "" {
		return ArrayValue([]Value{StringValue(""), StringValue(""), stringValueLike(self, text)}), nil
	}
	if !stringBinaryOf(self) && !stringBinaryOf(firstArg(args)) && !utf8.ValidString(separator) {
		return ArrayValue([]Value{stringValueLike(self, text), StringValue(""), StringValue("")}), nil
	}
	index := strings.Index(text, separator)
	if index < 0 {
		return ArrayValue([]Value{stringValueLike(self, text), StringValue(""), StringValue("")}), nil
	}
	return ArrayValue([]Value{
		stringValueLike(self, text[:index]),
		stringValueLike(self, separator),
		stringValueLike(self, text[index+len(separator):]),
	}), nil
}

func (s *State) stringExtRpartitionMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	separator, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if separator == "" {
		return ArrayValue([]Value{stringValueLike(self, text), StringValue(""), StringValue("")}), nil
	}
	if !stringBinaryOf(self) && !stringBinaryOf(firstArg(args)) && !utf8.ValidString(separator) {
		return ArrayValue([]Value{StringValue(""), StringValue(""), stringValueLike(self, text)}), nil
	}
	index := strings.LastIndex(text, separator)
	if index < 0 {
		return ArrayValue([]Value{StringValue(""), StringValue(""), stringValueLike(self, text)}), nil
	}
	return ArrayValue([]Value{
		stringValueLike(self, text[:index]),
		stringValueLike(self, separator),
		stringValueLike(self, text[index+len(separator):]),
	}), nil
}

func firstArg(args []Value) Value {
	if len(args) == 0 {
		return Nil
	}
	return args[0]
}

func (s *State) stringExtParseInteger(text string, base int, hexPrefix bool) Value {
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	sign := int64(1)
	if i < len(text) && (text[i] == '+' || text[i] == '-') {
		if text[i] == '-' {
			sign = -1
		}
		i++
	}
	if hexPrefix && i+1 < len(text) && text[i] == '0' && (text[i+1] == 'x' || text[i+1] == 'X') {
		base = 16
		i += 2
	}
	value := int64(0)
	digits := 0
	for ; i < len(text); i++ {
		c := text[i]
		if c == '_' {
			continue
		}
		var digit int
		switch {
		case c >= '0' && c <= '9':
			digit = int(c - '0')
		case c >= 'a' && c <= 'f':
			digit = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			digit = int(c-'A') + 10
		default:
			return IntValue(sign * value)
		}
		if digit >= base {
			return IntValue(sign * value)
		}
		value = value*int64(base) + int64(digit)
		digits++
	}
	return IntValue(sign * value)
}

func (s *State) stringExtHexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.stringExtParseInteger(self.String(), 16, true), nil
}

func (s *State) stringExtOctMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	i := 0
	for i < len(text) && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n' || text[i] == '\r') {
		i++
	}
	if i < len(text) && (text[i] == '+' || text[i] == '-') {
		i++
	}
	base := 8
	if i+1 < len(text) && text[i] == '0' {
		switch text[i+1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O', 'd', 'D':
			base = 8
			if text[i+1] == 'd' || text[i+1] == 'D' {
				base = 10
			}
		}
		if base != 8 || text[i+1] == 'o' || text[i+1] == 'O' {
			return s.stringExtParseInteger(text[:i]+text[i+2:], base, false), nil
		}
	}
	return s.stringExtParseInteger(text, base, false), nil
}

func (s *State) stringExtSliceMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text := self.String()
	binary := stringBinaryOf(self)
	count := stringCharCount(text)
	if binary {
		count = len(text)
	}
	if len(args) != 1 && len(args) != 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	if len(args) == 1 {
		switch {
		case args[0].Kind() == KindInt || args[0].Kind() == KindFloat:
			argumentFloat := args[0].Kind() == KindFloat
			index := int(args[0].Int())
			if argumentFloat {
				index = int(args[0].Float())
			}
			if index < 0 {
				index += count
			}
			if index < 0 || index >= count {
				return Nil, nil
			}
			if binary {
				return BinaryStringValue(text[index : index+1]), nil
			}
			unit, _ := stringCharAt(text, index)
			return stringValueLike(self, text[unit.start:unit.end]), nil
		case rangeDataOfValue(args[0]) != nil:
			rangeValue, _ := rangeDataOf(args[0])
			start, length := rangeBounds(rangeValue, count)
			if start < 0 || start > count || length < 0 {
				return Nil, nil
			}
			if start+length > count {
				length = count - start
			}
			return s.stringSubstring(self, text, start, length), nil
		case args[0].Kind() == KindString:
			pattern := args[0].String()
			if !binary && !utf8.ValidString(pattern) {
				return Nil, nil
			}
			index := strings.Index(text, pattern)
			if index < 0 {
				return Nil, nil
			}
			return stringValueLike(self, pattern), nil
		default:
			return Nil, s.stringExtError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
	}
	if (args[0].Kind() != KindInt && args[0].Kind() != KindFloat) || (args[1].Kind() != KindInt && args[1].Kind() != KindFloat) {
		return Nil, s.stringExtError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	start := int(args[0].Int())
	length := int(args[1].Int())
	if args[0].Kind() == KindFloat {
		start = int(args[0].Float())
	}
	if args[1].Kind() == KindFloat {
		length = int(args[1].Float())
	}
	if start < 0 {
		start += count
	}
	if start < 0 || start > count || length < 0 {
		return Nil, nil
	}
	if length > count-start {
		length = count - start
	}
	return s.stringSubstring(self, text, start, length), nil
}

func (s *State) stringSubstring(self Value, text string, start, length int) Value {
	if stringBinaryOf(self) {
		return BinaryStringValue(text[start : start+length])
	}
	if length == 0 {
		return StringValue("")
	}
	spans := stringCharSpans(text)
	if start >= len(spans) {
		return StringValue("")
	}
	end := start + length
	if end > len(spans) {
		end = len(spans)
	}
	return StringValue(text[spans[start].start:spans[end-1].end])
}

func rangeDataOfValue(value Value) *rangeData {
	if value.obj == nil {
		return nil
	}
	if data, ok := value.obj.data.(*rangeData); ok {
		return data
	}
	return nil
}

func (s *State) stringExtSliceBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	if len(args) == 0 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	text := data.str()
	binary := data.binary
	count := stringCharCount(text)
	if binary {
		count = len(text)
	}
	start, length := 0, 0
	if len(args) == 1 {
		switch {
		case args[0].Kind() == KindInt:
			index := int(args[0].Int())
			if index < 0 {
				index += count
			}
			if index < 0 || index >= count {
				return Nil, nil
			}
			var removed string
			var rest string
			if binary {
				removed = text[index : index+1]
				rest = text[:index] + text[index+1:]
			} else {
				unit, _ := stringCharAt(text, index)
				removed = text[unit.start:unit.end]
				rest = text[:unit.start] + text[unit.end:]
			}
			data.set(rest)
			return stringValueLike(self, removed), nil
		case rangeDataOfValue(args[0]) != nil:
			rangeValue := rangeDataOfValue(args[0])
			start, length = rangeBounds(rangeValue, count)
			if start < -count || start > count {
				return Nil, nil
			}
			if start < 0 {
				return Nil, nil
			}
			if length < 0 {
				length = 0
			}
			if start+length > count {
				length = count - start
			}
			removed := s.stringSubstring(self, text, start, length)
			if binary {
				data.set(text[:start] + text[start+length:])
			} else {
				data.set(s.stringRemoveSubstring(text, start, length))
			}
			return removed, nil
		case args[0].Kind() == KindString:
			pattern := args[0].String()
			if !binary && !utf8.ValidString(pattern) {
				return Nil, nil
			}
			index := strings.Index(text, pattern)
			if index < 0 {
				return Nil, nil
			}
			data.set(text[:index] + text[index+len(pattern):])
			return stringValueLike(self, pattern), nil
		default:
			return Nil, s.stringExtError(s.TypeErrorClass, "no implicit conversion into Integer")
		}
	}
	if args[0].Kind() != KindInt || args[1].Kind() != KindInt {
		return Nil, s.stringExtError(s.TypeErrorClass, "no implicit conversion into Integer")
	}
	start = int(args[0].Int())
	length = int(args[1].Int())
	if start < 0 {
		start += count
	}
	if start < 0 || start > count || length < 0 {
		return Nil, nil
	}
	if length > count-start {
		length = count - start
	}
	removed := s.stringSubstring(self, text, start, length)
	if binary {
		data.set(text[:start] + text[start+length:])
	} else {
		data.set(s.stringRemoveSubstring(text, start, length))
	}
	return removed, nil
}

func (s *State) stringRemoveSubstring(text string, start, length int) string {
	spans := stringCharSpans(text)
	if start >= len(spans) || length <= 0 {
		return text
	}
	end := start + length
	if end > len(spans) {
		end = len(spans)
	}
	return text[:spans[start].start] + text[spans[end-1].end:]
}

func (s *State) stringExtIndexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	needle, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	text := self.String()
	binary := stringBinaryOf(self)
	if !binary && !utf8.ValidString(needle) {
		return Nil, nil
	}
	count := stringCharCount(text)
	if binary {
		count = len(text)
	}
	offset := 0
	if len(args) > 1 {
		if args[1].Kind() != KindInt {
			return Nil, s.stringExtError(s.TypeErrorClass, "offset must be an integer")
		}
		offset = int(args[1].Int())
		if offset < 0 {
			offset += count
		}
	}
	if offset < 0 || offset > count {
		return Nil, nil
	}
	if needle == "" {
		return IntValue(int64(offset)), nil
	}
	if binary {
		index := strings.Index(text[offset:], needle)
		if index < 0 {
			return Nil, nil
		}
		return IntValue(int64(offset + index)), nil
	}
	byteOffset, _ := stringCharIndexToByte(text, offset)
	if index, ok := stringCharIndex(text, needle, byteOffset); ok {
		return IntValue(int64(index)), nil
	}
	return Nil, nil
}

func (s *State) stringExtRindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	needle, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	text := self.String()
	binary := stringBinaryOf(self)
	if !binary && !utf8.ValidString(needle) {
		return Nil, nil
	}
	count := stringCharCount(text)
	if binary {
		count = len(text)
	}
	position := count
	if len(args) > 1 {
		if args[1].Kind() != KindInt {
			return Nil, s.stringExtError(s.TypeErrorClass, "position must be an integer")
		}
		position = int(args[1].Int())
		if position < 0 {
			position += count
		}
	}
	if position < 0 {
		return Nil, nil
	}
	if position > count {
		position = count
	}
	if needle == "" {
		return IntValue(int64(position)), nil
	}
	if binary {
		end := position + len(needle)
		if end > len(text) {
			end = len(text)
		}
		index := strings.LastIndex(text[:end], needle)
		if index < 0 || index > position {
			return Nil, nil
		}
		return IntValue(int64(index)), nil
	}
	byteLimit, _ := stringCharIndexToByte(text, position)
	if index, ok := stringCharRindex(text, needle, byteLimit); ok {
		return IntValue(int64(index)), nil
	}
	return Nil, nil
}

func (s *State) stringExtByteindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	needle, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	text := self.String()
	binary := stringBinaryOf(self)
	if !binary && !utf8.ValidString(needle) {
		return Nil, nil
	}
	offset := 0
	if len(args) > 1 && args[1].Kind() == KindInt {
		offset = int(args[1].Int())
		if offset < 0 {
			offset += len(text)
		}
	}
	if offset < 0 || offset > len(text) {
		return Nil, nil
	}
	if !binary && !stringIsCharBoundary(text, offset) {
		return Nil, s.stringExtError(s.IndexErrorClass, "offset is not a character boundary")
	}
	index := strings.Index(text[offset:], needle)
	if index < 0 {
		return Nil, nil
	}
	return IntValue(int64(offset + index)), nil
}

func (s *State) stringExtByterindexMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	needle, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	text := self.String()
	binary := stringBinaryOf(self)
	if !binary && !utf8.ValidString(needle) {
		return Nil, nil
	}
	offset := len(text)
	if len(args) > 1 && args[1].Kind() == KindInt {
		offset = int(args[1].Int())
		if offset < 0 {
			offset += len(text)
		}
	}
	if offset < 0 {
		return Nil, nil
	}
	if offset > len(text) {
		offset = len(text)
	}
	if !binary && !stringIsCharBoundary(text, offset) {
		return Nil, s.stringExtError(s.IndexErrorClass, "offset is not a character boundary")
	}
	end := offset + len(needle)
	if end > len(text) {
		end = len(text)
	}
	index := strings.LastIndex(text[:end], needle)
	if index < 0 || index > offset {
		return Nil, nil
	}
	return IntValue(int64(index)), nil
}

func (s *State) stringExtIncludeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return False, nil
	}
	needle, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	if !stringBinaryOf(self) && !stringBinaryOf(args[0]) && !utf8.ValidString(needle) {
		return False, nil
	}
	return BoolValue(strings.Contains(self.String(), needle)), nil
}

func stringTrimSet(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r', 0:
		return true
	}
	return false
}

func stringTrim(text string, left, right, binary bool) string {
	if binary {
		trimByte := func(b byte) bool {
			return b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r' || b == 0
		}
		if left {
			for len(text) > 0 && trimByte(text[0]) {
				text = text[1:]
			}
		}
		if right {
			for len(text) > 0 && trimByte(text[len(text)-1]) {
				text = text[:len(text)-1]
			}
		}
		return text
	}
	if left {
		text = strings.TrimLeftFunc(text, stringTrimSet)
	}
	if right {
		text = strings.TrimRightFunc(text, stringTrimSet)
	}
	return text
}

func (s *State) stringExtStripMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return stringValueLike(self, stringTrim(self.String(), true, true, stringBinaryOf(self))), nil
}

func (s *State) stringExtLstripMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return stringValueLike(self, stringTrim(self.String(), true, false, stringBinaryOf(self))), nil
}

func (s *State) stringExtRstripMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return stringValueLike(self, stringTrim(self.String(), false, true, stringBinaryOf(self))), nil
}

func (s *State) stringExtStripBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.stringTrimBang(ctx, self, true, true)
}

func (s *State) stringExtLstripBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.stringTrimBang(ctx, self, true, false)
}

func (s *State) stringExtRstripBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return s.stringTrimBang(ctx, self, false, true)
}

func (s *State) stringTrimBang(ctx context.Context, self Value, left, right bool) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	trimmed := stringTrim(data.str(), left, right, data.binary)
	if trimmed == data.str() {
		return Nil, nil
	}
	data.set(trimmed)
	return self, nil
}

func (s *State) stringExtLjustMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	if pad == "" {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "zero width padding")
	}
	return stringValueLike(self, s.stringPad(self.String(), width, pad, false, true)), nil
}

func (s *State) stringExtRjustMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	if pad == "" {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "zero width padding")
	}
	return stringValueLike(self, s.stringPad(self.String(), width, pad, true, false)), nil
}

func (s *State) stringExtCenterMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	width := intArg(args, 0, 0)
	pad := " "
	if len(args) > 1 {
		pad = args[1].String()
	}
	if pad == "" {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "zero width padding")
	}
	return stringValueLike(self, s.stringPad(self.String(), width, pad, true, true)), nil
}

func (s *State) stringExtDeletePrefixMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	prefix, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if !strings.HasPrefix(self.String(), prefix) {
		return stringValueLike(self, self.String()), nil
	}
	return stringValueLike(self, self.String()[len(prefix):]), nil
}

func (s *State) stringExtDeleteSuffixMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	suffix, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if !strings.HasSuffix(self.String(), suffix) {
		return stringValueLike(self, self.String()), nil
	}
	return stringValueLike(self, self.String()[:len(self.String())-len(suffix)]), nil
}

func (s *State) stringExtDeletePrefixBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	prefix, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if !strings.HasPrefix(data.str(), prefix) {
		return Nil, nil
	}
	data.set(data.str()[len(prefix):])
	return self, nil
}

func (s *State) stringExtDeleteSuffixBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	suffix, err := s.stringExtStringArg(firstArg(args))
	if err != nil {
		return Nil, err
	}
	if !strings.HasSuffix(data.str(), suffix) {
		return Nil, nil
	}
	data.set(data.str()[:len(data.str())-len(suffix)])
	return self, nil
}

func (s *State) stringExtCopy(self Value, frozen bool) Value {
	class := self.obj.class
	if class == nil {
		class = s.classOf(self)
	}
	class = attachedClass(class)
	data := &stringData{raw: self.String(), binary: stringBinaryOf(self)}
	result := ObjectValue(&Object{class: class, data: data})
	result.obj.frozen = frozen
	return result
}

func (s *State) stringExtUPlusMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj == nil || !self.obj.frozen {
		return self, nil
	}
	return s.stringExtCopy(self, false), nil
}

func (s *State) stringExtUMinusMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if self.obj != nil && self.obj.frozen {
		return self, nil
	}
	return s.stringExtCopy(self, true), nil
}

func stringSwapRune(r rune) (string, bool) {
	if r == 0x01C5 {
		return "dŽ", true
	}
	if unicode.IsLower(r) {
		if mapped, ok := unicodeUpper[r]; ok {
			return mapped, true
		}
		return string(unicode.ToUpper(r)), true
	}
	if unicode.IsUpper(r) {
		if mapped, ok := unicodeLower[r]; ok {
			return mapped, true
		}
		return string(unicode.ToLower(r)), true
	}
	return string(r), false
}

func stringSwapcase(text string, binary bool) (string, bool, error) {
	if binary {
		var builder strings.Builder
		builder.Grow(len(text))
		changed := false
		for i := 0; i < len(text); i++ {
			b := text[i]
			switch {
			case b >= 'a' && b <= 'z':
				builder.WriteByte(b - ('a' - 'A'))
				changed = true
			case b >= 'A' && b <= 'Z':
				builder.WriteByte(b + ('a' - 'A'))
				changed = true
			default:
				builder.WriteByte(b)
			}
		}
		return builder.String(), changed, nil
	}
	var builder strings.Builder
	builder.Grow(len(text))
	changed := false
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return "", false, errInvalidCaseInput
		}
		mapped, didChange := stringSwapRune(r)
		builder.WriteString(mapped)
		changed = changed || didChange
		i += size
	}
	return builder.String(), changed, nil
}

func (s *State) stringExtSwapcaseMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	converted, _, err := stringSwapcase(self.String(), stringBinaryOf(self))
	if err != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	return stringValueLike(self, converted), nil
}

func (s *State) stringExtSwapcaseBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	converted, changed, err := stringSwapcase(data.str(), data.binary)
	if err != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	if !changed {
		return Nil, nil
	}
	data.set(converted)
	return self, nil
}

func foldCaseString(text string, binary bool) (string, error) {
	if binary {
		var builder strings.Builder
		for i := 0; i < len(text); i++ {
			b := text[i]
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			builder.WriteByte(b)
		}
		return builder.String(), nil
	}
	var builder strings.Builder
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			return "", errInvalidCaseInput
		}
		switch r {
		case 'ß', 'ẞ':
			builder.WriteString("ss")
		case 'ﬀ':
			builder.WriteString("ff")
		case 'ﬁ':
			builder.WriteString("fi")
		case 'ﬂ':
			builder.WriteString("fl")
		case 'ﬃ':
			builder.WriteString("ffi")
		case 'ﬄ':
			builder.WriteString("ffl")
		case 'ﬅ', 'ﬆ':
			builder.WriteString("st")
		case 'K':
			builder.WriteString("k")
		case 'İ':
			builder.WriteString("i\u0307")
		default:
			builder.WriteRune(unicode.ToLower(r))
		}
		i += size
	}
	return builder.String(), nil
}

func (s *State) stringExtCasecmpPredicateMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 || args[0].Kind() != KindString {
		return Nil, nil
	}
	left, err := foldCaseString(self.String(), stringBinaryOf(self))
	if err != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	right, err := foldCaseString(args[0].String(), stringBinaryOf(self) || stringBinaryOf(args[0]))
	if err != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	return BoolValue(left == right), nil
}

type charPattern struct {
	negated bool
	ranges  [][2]int
	set     map[int]bool
}

func parseCharPattern(pattern string, binary, allowNegation bool) (*charPattern, error) {
	units, err := stringUnitsOf(pattern, binary)
	if err != nil {
		return nil, err
	}
	result := &charPattern{set: make(map[int]bool)}
	index := 0
	if allowNegation && len(units) > 1 && units[0].value == '^' {
		result.negated = true
		index = 1
	}
	nextEscaped := func() int {
		value := units[index].value
		index++
		if value == '\\' && index < len(units) {
			value = units[index].value
			index++
		}
		return value
	}
	for index < len(units) {
		if len(result.ranges) >= 65535 {
			return nil, errInvalidCaseInput
		}
		first := nextEscaped()
		last := first
		if len(units)-index > 1 && units[index].value == '-' {
			index++
			last = units[index].value
			index++
		}
		if first > last {
			return nil, errInvalidCaseInput
		}
		result.ranges = append(result.ranges, [2]int{first, last})
		for value := first; value <= last; value++ {
			result.set[value] = true
		}
	}
	return result, nil
}

func (p *charPattern) inSet(value int) bool {
	return p.set[value]
}

func (p *charPattern) matches(value int) bool {
	if p.negated {
		return !p.set[value]
	}
	return p.set[value]
}

// findIndex returns the offset of value in the pattern order, or -1 when the
// pattern names it not at all. A negated pattern names everything outside its
// ranges, at an index past the last range.
func (p *charPattern) findIndex(value int) int {
	if p.negated {
		if p.set[value] {
			return -1
		}
		return int(^uint(0) >> 1)
	}
	found := -1
	offset := 0
	for _, span := range p.ranges {
		if value >= span[0] && value <= span[1] {
			found = offset + (value - span[0])
		}
		offset += span[1] - span[0] + 1
	}
	return found
}

// charAt returns the character at index in the pattern order. An index past
// the end answers the last character, as mruby pads a short replacement.
func (p *charPattern) charAt(index int) int {
	last := -1
	for _, span := range p.ranges {
		size := span[1] - span[0] + 1
		if index < size {
			return span[0] + index
		}
		index -= size
		last = span[1]
	}
	return last
}

func stringPatternMode(self Value, args []Value) bool {
	if stringBinaryOf(self) {
		return true
	}
	for _, arg := range args {
		if stringBinaryOf(arg) {
			return true
		}
	}
	return false
}

func (s *State) stringPatternArg(args []Value, binary bool, allowNegation bool) (*charPattern, error) {
	if len(args) == 0 {
		return nil, nil
	}
	pattern, err := parseCharPattern(args[0].String(), binary, allowNegation)
	if err != nil {
		return nil, s.stringExtError(s.ArgumentErrorClass, "invalid character pattern")
	}
	return pattern, nil
}

func (s *State) stringExtCountMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	binary := stringPatternMode(self, args)
	pattern, err := s.stringPatternArg(args, binary, true)
	if err != nil {
		return Nil, err
	}
	units, unitErr := stringUnitsOf(self.String(), binary)
	if unitErr != nil {
		return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	count := int64(0)
	for _, unit := range units {
		if pattern.matches(unit.value) {
			count++
		}
	}
	return IntValue(count), nil
}

func (s *State) stringExtFilterPattern(self Value, pattern *charPattern, delete bool) (string, error) {
	text := self.String()
	binary := stringPatternMode(self, nil)
	units, err := stringUnitsOf(text, binary)
	if err != nil {
		return "", s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	var builder strings.Builder
	for _, unit := range units {
		matched := pattern.matches(unit.value)
		if delete {
			if !matched {
				builder.WriteString(text[unit.start:unit.end])
			}
		} else if matched {
			builder.WriteString(text[unit.start:unit.end])
		}
	}
	return builder.String(), nil
}

func (s *State) stringExtDeleteMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	binary := stringPatternMode(self, args)
	pattern, err := s.stringPatternArg(args, binary, true)
	if err != nil {
		return Nil, err
	}
	text, filterErr := s.stringExtFilterPattern(self, pattern, true)
	if filterErr != nil {
		return Nil, filterErr
	}
	return stringValueLike(self, text), nil
}

func (s *State) stringExtDeleteBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	binary := stringPatternMode(self, args)
	pattern, err := s.stringPatternArg(args, binary, true)
	if err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	text, filterErr := s.stringExtFilterPattern(self, pattern, true)
	if filterErr != nil {
		return Nil, filterErr
	}
	if text == data.str() {
		return Nil, nil
	}
	data.set(text)
	return self, nil
}

func (s *State) stringExtSqueezeMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text, err := s.stringExtSqueezeText(self, args)
	if err != nil {
		return Nil, err
	}
	return stringValueLike(self, text), nil
}

func (s *State) stringExtSqueezeBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	text, err := s.stringExtSqueezeText(self, args)
	if err != nil {
		return Nil, err
	}
	if text == data.str() {
		return Nil, nil
	}
	data.set(text)
	return self, nil
}

func (s *State) stringExtSqueezeText(self Value, args []Value) (string, error) {
	if len(args) > 1 {
		return "", argumentCountError(s, len(args), "0..1")
	}
	binary := stringPatternMode(self, args)
	var pattern *charPattern
	var err error
	if len(args) == 1 {
		pattern, err = s.stringPatternArg(args, binary, true)
		if err != nil {
			return "", err
		}
	}
	text := self.String()
	units, unitErr := stringUnitsOf(text, binary)
	if unitErr != nil {
		return "", s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	var builder strings.Builder
	previous := -1
	havePrevious := false
	for _, unit := range units {
		if pattern != nil && !pattern.matches(unit.value) {
			builder.WriteString(text[unit.start:unit.end])
			havePrevious = false
			continue
		}
		if havePrevious && previous == unit.value {
			continue
		}
		builder.WriteString(text[unit.start:unit.end])
		previous = unit.value
		havePrevious = true
	}
	return builder.String(), nil
}

func stringPatternUnitText(value int, binary bool) string {
	if binary {
		return string([]byte{byte(value)})
	}
	return string(rune(value))
}

func (s *State) stringExtTranslateText(self Value, args []Value, squeeze bool) (string, error) {
	if len(args) != 2 {
		return "", argumentCountError(s, len(args), "2")
	}
	binary := stringPatternMode(self, args)
	pattern, err := s.stringPatternArg(args[:1], binary, true)
	if err != nil {
		return "", err
	}
	replacement, err := s.stringPatternArg(args[1:], binary, false)
	if err != nil {
		return "", err
	}
	text := self.String()
	units, unitErr := stringUnitsOf(text, binary)
	if unitErr != nil {
		return "", s.stringExtError(s.ArgumentErrorClass, "invalid byte sequence in UTF-8")
	}
	replacementBinary := stringBinaryOf(args[1])
	var builder strings.Builder
	last := -1
	for _, unit := range units {
		index := pattern.findIndex(unit.value)
		if index < 0 {
			builder.WriteString(text[unit.start:unit.end])
			last = -1
			continue
		}
		target := replacement.charAt(index)
		if target < 0 || (squeeze && target == last) {
			continue
		}
		last = target
		if replacementBinary {
			builder.WriteByte(byte(target))
		} else if binary {
			builder.WriteByte(byte(target))
		} else {
			builder.WriteRune(rune(target))
		}
	}
	return builder.String(), nil
}

func (s *State) stringExtTrMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text, err := s.stringExtTranslateText(self, args, false)
	if err != nil {
		return Nil, err
	}
	return stringValueLike(self, text), nil
}

func (s *State) stringExtTrBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	text, err := s.stringExtTranslateText(self, args, false)
	if err != nil {
		return Nil, err
	}
	if text == data.str() {
		return Nil, nil
	}
	data.set(text)
	return self, nil
}

func (s *State) stringExtTrSMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text, err := s.stringExtTranslateText(self, args, true)
	if err != nil {
		return Nil, err
	}
	return stringValueLike(self, text), nil
}

func (s *State) stringExtTrSBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	text, err := s.stringExtTranslateText(self, args, true)
	if err != nil {
		return Nil, err
	}
	if text == data.str() {
		return Nil, nil
	}
	data.set(text)
	return self, nil
}

const (
	succNotFound = iota
	succFound
	succWrapped
)

func succPrevChar(b []byte, p int, chars bool) int {
	if !chars || p <= 0 {
		return p - 1
	}
	_, size := utf8.DecodeLastRune(b[:p])
	if size <= 0 {
		size = 1
	}
	return p - size
}

func succCharLen(b []byte, p int, chars bool) int {
	if !chars {
		return 1
	}
	r, size := utf8.DecodeRune(b[p:])
	if r == utf8.RuneError && size == 1 && b[p] >= 0x80 {
		return 0
	}
	return size
}

func succUTF8Write(b []byte, p, size int, cp rune) bool {
	if cp >= 0xD800 && cp <= 0xDFFF {
		return false
	}
	encoded := string(cp)
	if len(encoded) != size {
		return false
	}
	copy(b[p:p+size], encoded)
	return true
}

func succFirstOfLen(size int) rune {
	switch size {
	case 2:
		return 0x80
	case 3:
		return 0x800
	case 4:
		return 0x10000
	}
	return 0
}

func succNextCP(cp rune) rune {
	if cp+1 == 0xD800 {
		return 0xE000
	}
	return cp + 1
}

func unicodeAlnumKind(cp rune) int {
	if unicode.IsDigit(cp) {
		return 2
	}
	if unicode.IsLetter(cp) {
		return 1
	}
	return 0
}

func unicodeAlnumRun(cp rune) (int, rune) {
	kind := unicodeAlnumKind(cp)
	if kind == 0 {
		return 0, cp
	}
	start := cp
	for start > 0 {
		prev := start - 1
		if prev == 0xD800 {
			prev = 0xD7FF
		}
		if unicodeAlnumKind(prev) != kind {
			break
		}
		start = prev
	}
	return kind, start
}

func isASCIIDigitByte(b byte) bool { return b >= '0' && b <= '9' }
func isASCIIAlphaByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
func isASCIIAlnumByte(b byte) bool { return isASCIIDigitByte(b) || isASCIIAlphaByte(b) }

func succAlnumAt(b []byte, p, size int, chars bool) (int, []byte) {
	if size == 1 {
		c := b[p]
		if !isASCIIAlnumByte(c) {
			return succNotFound, nil
		}
		switch c {
		case '9':
			b[p] = '0'
			return succWrapped, []byte{'1'}
		case 'z':
			b[p] = 'a'
			return succWrapped, []byte{'a'}
		case 'Z':
			b[p] = 'A'
			return succWrapped, []byte{'A'}
		}
		b[p] = c + 1
		return succFound, nil
	}
	if !chars {
		return succNotFound, nil
	}
	cp, _ := utf8.DecodeRune(b[p : p+size])
	kind, start := unicodeAlnumRun(cp)
	if kind == 0 {
		return succNotFound, nil
	}
	next := cp
	for step := 0; step < 2; step++ {
		next = succNextCP(next)
		nextKind, _ := unicodeAlnumRun(next)
		if nextKind == kind && succUTF8Write(b, p, size, next) {
			return succFound, nil
		}
	}
	first := start
	if minimum := succFirstOfLen(size); first < minimum {
		first = minimum
	}
	if first == cp {
		return succNotFound, nil
	}
	succUTF8Write(b, p, size, first)
	carryCP := first
	if kind == 2 {
		carryCP = first + 1
	}
	return succWrapped, []byte(string(carryCP))
}

func succCharAt(b []byte, p, size int, chars bool) int {
	if chars {
		cp, _ := utf8.DecodeRune(b[p : p+size])
		if succUTF8Write(b, p, size, succNextCP(cp)) {
			return succFound
		}
		succUTF8Write(b, p, size, succFirstOfLen(size))
		return succWrapped
	}
	if b[p] == 0xFF {
		b[p] = 0
		return succWrapped
	}
	b[p]++
	return succFound
}

func succStringText(text string, binary bool) string {
	if text == "" {
		return text
	}
	b := []byte(text)
	chars := !binary
	carry := []byte{1}
	carryPos := 0
	lastAlnum := -1
	foundAlnum := false
	step := succFound
	p := len(b)
	for p > 0 {
		next := succPrevChar(b, p, chars)
		if step == succNotFound && lastAlnum >= 0 {
			last := b[lastAlnum]
			current := b[next]
			if (isASCIIAlphaByte(last) && isASCIIDigitByte(current)) ||
				(isASCIIDigitByte(last) && isASCIIAlphaByte(current)) {
				break
			}
		}
		p = next
		size := succCharLen(b, p, chars)
		if size == 0 {
			continue
		}
		state, nextCarry := succAlnumAt(b, p, size, chars)
		if state == succNotFound {
			step = succNotFound
			continue
		}
		if state == succFound {
			return string(b)
		}
		step = succWrapped
		lastAlnum = p
		foundAlnum = true
		carryPos = p
		carry = nextCarry
	}
	if !foundAlnum {
		p = len(b)
		for p > 0 {
			p = succPrevChar(b, p, chars)
			size := succCharLen(b, p, chars)
			if size == 0 {
				continue
			}
			if succCharAt(b, p, size, chars) == succFound {
				return string(b)
			}
			carryPos = p
		}
	}
	result := make([]byte, 0, len(b)+len(carry))
	result = append(result, b[:carryPos]...)
	result = append(result, carry...)
	result = append(result, b[carryPos:]...)
	return string(result)
}

func (s *State) stringExtSuccMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	return stringValueLike(self, succStringText(self.String(), stringBinaryOf(self))), nil
}

func (s *State) stringExtSuccBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	data.set(succStringText(data.str(), data.binary))
	return self, nil
}

func stringAllDigits(text string) (int64, bool) {
	if text == "" {
		return 0, false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func stringPadZero(value int64, width int) string {
	text := strconv.FormatInt(value, 10)
	for len(text) < width {
		text = "0" + text
	}
	return text
}

func (s *State) stringExtUptoMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return Nil, argumentCountError(s, len(args), "1..2")
	}
	max, err := s.stringExtStringArg(args[0])
	if err != nil {
		return Nil, err
	}
	if block.IsNil() {
		return s.toEnum(self, "upto", nil), nil
	}
	exclusive := len(args) > 1 && args[1].IsTruthy()
	text := self.String()
	binary := stringBinaryOf(self) || stringBinaryOf(args[0])
	length := stringCharCount(text)
	maxLength := stringCharCount(max)
	if binary {
		length = len(text)
		maxLength = len(max)
	}
	if length == 1 && maxLength == 1 {
		start := int64(text[0])
		end := int64(max[0])
		if !binary {
			startRune, _ := utf8.DecodeRuneInString(text)
			endRune, _ := utf8.DecodeRuneInString(max)
			start, end = int64(startRune), int64(endRune)
		}
		for c := start; c <= end; c++ {
			if exclusive && c == end {
				break
			}
			value := stringValueLike(self, string(rune(c)))
			if binary {
				value = stringValueLike(self, string([]byte{byte(c)}))
			}
			if _, err := s.callBlock1(ctx, block, value, Nil); err != nil {
				return Nil, err
			}
		}
		return self, nil
	}
	startNumber, startDigits := stringAllDigits(text)
	endNumber, endDigits := stringAllDigits(max)
	if startDigits && endDigits && startNumber >= 0 && endNumber >= 0 {
		for value := startNumber; value <= endNumber; value++ {
			if exclusive && value == endNumber {
				break
			}
			out := stringPadZero(value, length)
			if _, err := s.callBlock1(ctx, block, stringValueLike(self, out), Nil); err != nil {
				return Nil, err
			}
		}
		return self, nil
	}
	order := strings.Compare(text, max)
	if order > 0 || (exclusive && order == 0) {
		return self, nil
	}
	afterMax := succStringText(max, binary)
	current := text
	for current != afterMax {
		var next string
		haveNext := true
		if exclusive || current != max {
			next = succStringText(current, binary)
		} else {
			haveNext = false
		}
		if _, err := s.callBlock1(ctx, block, stringValueLike(self, current), Nil); err != nil {
			return Nil, err
		}
		if !haveNext {
			break
		}
		current = next
		if exclusive && current == max {
			break
		}
		size := stringCharCount(current)
		if binary {
			size = len(current)
		}
		if size > maxLength || size == 0 {
			break
		}
	}
	return self, nil
}

type scrubPart struct {
	start int
	end   int
	valid bool
}

func stringScrubParts(text string) []scrubPart {
	var parts []scrubPart
	for i := 0; i < len(text); {
		b := text[i]
		if b < 0x80 {
			parts = append(parts, scrubPart{start: i, end: i + 1, valid: true})
			i++
			continue
		}
		length := 0
		secondLow, secondHigh := byte(0x80), byte(0xBF)
		switch {
		case b >= 0xC2 && b <= 0xDF:
			length = 2
		case b == 0xE0:
			length = 3
			secondLow = 0xA0
		case b >= 0xE1 && b <= 0xEC:
			length = 3
		case b == 0xED:
			length = 3
			secondHigh = 0x9F
		case b >= 0xEE && b <= 0xEF:
			length = 3
		case b == 0xF0:
			length = 4
			secondLow = 0x90
		case b >= 0xF1 && b <= 0xF3:
			length = 4
		case b == 0xF4:
			length = 4
			secondHigh = 0x8F
		default:
			parts = append(parts, scrubPart{start: i, end: i + 1})
			i++
			continue
		}
		if i+1 >= len(text) {
			parts = append(parts, scrubPart{start: i, end: len(text)})
			i = len(text)
			continue
		}
		second := text[i+1]
		if second < secondLow || second > secondHigh {
			parts = append(parts, scrubPart{start: i, end: i + 1})
			i++
			continue
		}
		consumed := 2
		valid := true
		for consumed < length {
			if i+consumed >= len(text) {
				valid = false
				break
			}
			next := text[i+consumed]
			if next < 0x80 || next > 0xBF {
				valid = false
				break
			}
			consumed++
		}
		if valid && consumed == length {
			parts = append(parts, scrubPart{start: i, end: i + length, valid: true})
			i += length
			continue
		}
		parts = append(parts, scrubPart{start: i, end: i + consumed})
		i += consumed
	}
	return parts
}

func stringScrubText(text string) string {
	var builder strings.Builder
	for _, part := range stringScrubParts(text) {
		if part.valid {
			builder.WriteString(text[part.start:part.end])
		} else {
			builder.WriteRune('\uFFFD')
		}
	}
	return builder.String()
}

func (s *State) stringExtScrubMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if stringBinaryOf(self) {
		return StringValue(self.String()), nil
	}
	text := self.String()
	parts := stringScrubParts(text)
	replacement := "\uFFFD"
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	if len(args) == 1 {
		replacement = args[0].String()
		if !utf8.ValidString(replacement) {
			return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid replacement string")
		}
	}
	if !block.IsNil() {
		replacement = ""
		var builder strings.Builder
		for _, part := range parts {
			if part.valid {
				builder.WriteString(text[part.start:part.end])
				continue
			}
			value, err := s.callBlock1(ctx, block, BinaryStringValue(text[part.start:part.end]), Nil)
			if err != nil {
				return Nil, err
			}
			builder.WriteString(value.ToS())
		}
		return StringValue(builder.String()), nil
	}
	if len(args) == 0 && allScrubPartsValid(parts) {
		return StringValue(text), nil
	}
	var builder strings.Builder
	for _, part := range parts {
		if part.valid {
			builder.WriteString(text[part.start:part.end])
		} else {
			builder.WriteString(replacement)
		}
	}
	return StringValue(builder.String()), nil
}

func allScrubPartsValid(parts []scrubPart) bool {
	for _, part := range parts {
		if !part.valid {
			return false
		}
	}
	return true
}

func (s *State) stringExtScrubBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if stringBinaryOf(self) {
		return self, nil
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	parts := stringScrubParts(data.str())
	if allScrubPartsValid(parts) {
		return self, nil
	}
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	replacement := "\uFFFD"
	if len(args) > 1 {
		return Nil, argumentCountError(s, len(args), "0..1")
	}
	if len(args) == 1 {
		replacement = args[0].String()
		if !utf8.ValidString(replacement) {
			return Nil, s.stringExtError(s.ArgumentErrorClass, "invalid replacement string")
		}
	}
	text := data.str()
	var builder strings.Builder
	for _, part := range parts {
		if part.valid {
			builder.WriteString(text[part.start:part.end])
			continue
		}
		if !block.IsNil() {
			value, err := s.callBlock1(ctx, block, BinaryStringValue(text[part.start:part.end]), Nil)
			if err != nil {
				return Nil, err
			}
			builder.WriteString(value.ToS())
		} else {
			builder.WriteString(replacement)
		}
	}
	data.set(builder.String())
	return self, nil
}

func (s *State) stringExtCasecmpMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if len(args) != 1 {
		return Nil, argumentCountError(s, len(args), "1")
	}
	if args[0].Kind() != KindString {
		return Nil, nil
	}
	left, right := self.String(), args[0].String()
	for i := 0; i < len(left) || i < len(right); i++ {
		var l, r byte
		if i < len(left) {
			l = left[i]
			if l >= 'A' && l <= 'Z' {
				l += 'a' - 'A'
			}
		}
		if i < len(right) {
			r = right[i]
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
		}
		if l < r {
			return IntValue(-1), nil
		}
		if l > r {
			return IntValue(1), nil
		}
	}
	return IntValue(0), nil
}

func chopText(text string, binary bool) (string, bool) {
	if text == "" {
		return text, false
	}
	if strings.HasSuffix(text, "\r\n") {
		return text[:len(text)-2], true
	}
	if binary {
		return text[:len(text)-1], true
	}
	_, size := utf8.DecodeLastRuneInString(text)
	if size <= 0 {
		size = 1
	}
	return text[:len(text)-size], true
}

func (s *State) stringExtChopMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	text, _ := chopText(self.String(), stringBinaryOf(self))
	return stringValueLike(self, text), nil
}

func (s *State) stringExtChopBangMethod(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
	if err := s.ensureMutable(self); err != nil {
		return Nil, err
	}
	data, ok := stringDataOf(self)
	if !ok {
		return Nil, s.stringExtError(s.TypeErrorClass, "not a string")
	}
	text, changed := chopText(data.str(), data.binary)
	if !changed {
		return Nil, nil
	}
	data.set(text)
	return self, nil
}
