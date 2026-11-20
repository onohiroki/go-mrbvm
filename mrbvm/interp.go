package mrbvm

import (
	"context"
	"fmt"
	"math"

	"github.com/onohiroki/go-mrbc/bytecode"
)

func (s *State) executeIrep(ctx context.Context, irep *Irep, self Value) (Value, error) {
	instructions, err := bytecode.Decode(irep.Code)
	if err != nil {
		return Nil, err
	}
	nregs := int(irep.Nregs)
	if nlocals := int(irep.Nlocals); nlocals > nregs {
		nregs = nlocals
	}
	if nregs < 1 {
		nregs = 1
	}
	regs := make([]Value, nregs)
	regs[0] = self

	for pc := 0; pc < len(instructions); pc++ {
		if err := ctx.Err(); err != nil {
			return Nil, err
		}
		in := instructions[pc]
		switch in.Op {
		case bytecode.OpNOP:
			// no-op

		case bytecode.OpMOVE:
			regs[in.Operands[0]] = regs[in.Operands[1]]

		case bytecode.OpLOADNIL:
			regs[in.Operands[0]] = Nil
		case bytecode.OpLOADTRUE:
			regs[in.Operands[0]] = True
		case bytecode.OpLOADFALSE:
			regs[in.Operands[0]] = False
		case bytecode.OpLOADSELF:
			regs[in.Operands[0]] = self

		case bytecode.OpLOADI_0, bytecode.OpLOADI_1, bytecode.OpLOADI_2,
			bytecode.OpLOADI_3, bytecode.OpLOADI_4, bytecode.OpLOADI_5,
			bytecode.OpLOADI_6, bytecode.OpLOADI_7:
			regs[in.Operands[0]] = IntValue(int64(in.Op - bytecode.OpLOADI_0))
		case bytecode.OpLOADI__1:
			regs[in.Operands[0]] = IntValue(-1)
		case bytecode.OpLOADI8:
			regs[in.Operands[0]] = IntValue(int64(int8(in.Operands[1])))
		case bytecode.OpLOADINEG:
			regs[in.Operands[0]] = IntValue(-int64(in.Operands[1]))
		case bytecode.OpLOADI16:
			regs[in.Operands[0]] = IntValue(int64(int16(in.Operands[1])))
		case bytecode.OpLOADI32:
			raw := uint32(in.Operands[1])<<16 | uint32(in.Operands[2])
			regs[in.Operands[0]] = IntValue(int64(int32(raw)))

		case bytecode.OpLOADL:
			value, err := s.poolValue(irep, int(in.Operands[1]))
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = value

		case bytecode.OpSTRING:
			value, err := s.poolValue(irep, int(in.Operands[1]))
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = value

		case bytecode.OpSEND0, bytecode.OpSSEND0:
			recv := self
			if in.Op == bytecode.OpSEND0 {
				recv = regs[in.Operands[0]]
			}
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], nil)
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = result

		case bytecode.OpSEND, bytecode.OpSSEND:
			recv := self
			if in.Op == bytecode.OpSEND {
				recv = regs[in.Operands[0]]
			}
			argInfo := in.Operands[2]
			if argInfo>>4 != 0 {
				return Nil, fmt.Errorf("mrbvm: keyword arguments are not supported yet")
			}
			n := int(argInfo & 0x0f)
			if n == 15 {
				return Nil, fmt.Errorf("mrbvm: packed argument arrays are not supported yet")
			}
			args := make([]Value, n)
			copy(args, regs[in.Operands[0]+1:in.Operands[0]+1+uint32(n)])
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], args)
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = result

		case bytecode.OpRETURN:
			return regs[in.Operands[0]], nil
		case bytecode.OpRETNIL:
			return Nil, nil
		case bytecode.OpRETSELF:
			return self, nil
		case bytecode.OpRETTRUE:
			return True, nil
		case bytecode.OpRETFALSE:
			return False, nil

		case bytecode.OpSTOP:
			return Nil, nil

		default:
			return Nil, fmt.Errorf("mrbvm: opcode %s is not implemented yet", in.Op)
		}
	}
	return Nil, nil
}

func (s *State) call(ctx context.Context, recv Value, name string, args []Value) (Value, error) {
	fn, ok := s.classOf(recv).lookup(name)
	if !ok {
		return Nil, fmt.Errorf("undefined method %q for %s", name, recv.Inspect())
	}
	return fn(ctx, recv, args)
}

func (s *State) poolValue(irep *Irep, index int) (Value, error) {
	if index < 0 || index >= len(irep.Pool) {
		return Nil, fmt.Errorf("mrbvm: pool index %d out of range", index)
	}
	entry := irep.Pool[index]
	switch entry.Kind {
	case bytecode.PoolString:
		return StringValue(entry.String), nil
	case bytecode.PoolInt32, bytecode.PoolInt64:
		return IntValue(entry.Int), nil
	case bytecode.PoolFloat:
		return Value{kind: KindFloat, bits: math.Float64bits(entry.Float)}, nil
	default:
		return Nil, fmt.Errorf("mrbvm: unsupported pool entry kind %d", entry.Kind)
	}
}
