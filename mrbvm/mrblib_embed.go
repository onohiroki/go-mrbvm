package mrbvm

import _ "embed"

// mrblibBytecode is a precompiled subset of mruby 4.0.0's mrblib:
// 10error.rb, compar.rb, enum.rb, array.rb, hash.rb, string.rb, and
// range.rb. Regenerate it with go-mrbc when the source subset changes.
// The sources are MIT-licensed; see NOTICE.
//
//go:embed mrblib.mrb
var mrblibBytecode []byte
