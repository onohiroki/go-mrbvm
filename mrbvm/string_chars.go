package mrbvm

import (
	"strings"
	"unicode/utf8"
)

// charSpan is one character position in a string. A valid UTF-8 sequence is
// one span; each byte of an invalid sequence is a span of its own, so the
// original bytes are preserved and every byte still has a position.
type charSpan struct {
	start int
	end   int
}

func stringCharSpans(text string) []charSpan {
	if text == "" {
		return nil
	}
	spans := make([]charSpan, 0, len(text))
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 || i+size > len(text) {
			size = 1
		}
		spans = append(spans, charSpan{start: i, end: i + size})
		i += size
	}
	return spans
}

func stringCharCount(text string) int {
	count := 0
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 || i+size > len(text) {
			size = 1
		}
		i += size
		count++
	}
	return count
}

func stringCharAt(text string, index int) (charSpan, bool) {
	spans := stringCharSpans(text)
	if index < 0 {
		index += len(spans)
	}
	if index < 0 || index >= len(spans) {
		return charSpan{}, false
	}
	return spans[index], true
}

func stringCharSlice(text string, start, length int) (string, bool) {
	spans := stringCharSpans(text)
	if start < 0 {
		start += len(spans)
	}
	if start < 0 || start > len(spans) || length < 0 {
		return "", false
	}
	end := start + length
	if end > len(spans) {
		end = len(spans)
	}
	if start == end {
		return "", true
	}
	return text[spans[start].start:spans[end-1].end], true
}

func stringCharIndexToByte(text string, index int) (int, bool) {
	spans := stringCharSpans(text)
	if index < 0 || index > len(spans) {
		return 0, false
	}
	if index == len(spans) {
		return len(text), true
	}
	return spans[index].start, true
}

func stringByteIndexToChar(text string, offset int) int {
	count := 0
	for i := 0; i < len(text) && i < offset; {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 || i+size > len(text) {
			size = 1
		}
		i += size
		count++
	}
	return count
}

func stringIsCharBoundary(text string, offset int) bool {
	if offset == 0 || offset == len(text) {
		return true
	}
	if offset < 0 || offset > len(text) {
		return false
	}
	for i := 0; i < len(text); {
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 || i+size > len(text) {
			size = 1
		}
		if i == offset {
			return true
		}
		if i+size > offset {
			return false
		}
		i += size
	}
	return false
}

func stringCharIndex(text, needle string, from int) (int, bool) {
	if needle == "" {
		return from, true
	}
	if from < 0 || from > len(text) {
		return 0, false
	}
	index := indexFrom(text, needle, from)
	if index < 0 {
		return 0, false
	}
	if !stringIsCharBoundary(text, index) || !stringIsCharBoundary(text, index+len(needle)) {
		return 0, false
	}
	return stringByteIndexToChar(text, index), true
}

func stringCharRindex(text, needle string, from int) (int, bool) {
	if needle == "" {
		return from, true
	}
	if from < 0 || from > len(text) {
		return 0, false
	}
	end := from + len(needle)
	if end > len(text) {
		end = len(text)
	}
	index := lastIndexFrom(text, needle, end)
	if index < 0 {
		return 0, false
	}
	if !stringIsCharBoundary(text, index) || !stringIsCharBoundary(text, index+len(needle)) {
		return 0, false
	}
	return stringByteIndexToChar(text, index), true
}

func indexFrom(text, needle string, from int) int {
	if from > len(text) {
		return -1
	}
	index := strings.Index(text[from:], needle)
	if index < 0 {
		return -1
	}
	return from + index
}

func lastIndexFrom(text, needle string, end int) int {
	if end > len(text) {
		end = len(text)
	}
	return strings.LastIndex(text[:end], needle)
}
