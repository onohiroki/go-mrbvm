package mrbvm

import "github.com/onohiroki/go-mrbc/bytecode"

// Irep is the decoded mruby instruction representation shared with go-mrbc.
type Irep = bytecode.Irep

// DecodeBytecode reads a RITE 04.00 binary and returns its top-level irep.
func DecodeBytecode(data []byte) (*Irep, error) {
	return bytecode.ReadBinary(data)
}
