package mrbvm

import (
	"context"
	"fmt"
	"math"
	"math/big"

	"github.com/onohiroki/go-mrbc/bytecode"
)

func (s *State) executeIrep(ctx context.Context, irep *Irep, self Value) (Value, error) {
	return s.executeFrame(ctx, irep, self, s.ObjectClass, nil, Nil, nil)
}

func (s *State) executeFrame(ctx context.Context, irep *Irep, self Value, targetClass *Class, args []Value, block Value, capturedEnv *environment) (Value, error) {
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
	frameArgs := args
	frameBlock := block
	frameEnv := &environment{parent: capturedEnv, regs: regs, block: block}
	_ = frameEnv
	pcByOffset := make(map[int]int, len(instructions))
	for i, in := range instructions {
		pcByOffset[in.Offset] = i
	}

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

		case bytecode.OpENTER:
			aspec := in.Operands[0]
			req := int((aspec >> 18) & 0x1f)
			opt := int((aspec >> 13) & 0x1f)
			hasRest := (aspec>>12)&1 != 0
			post := int((aspec >> 7) & 0x1f)
			key := int((aspec >> 2) & 0x1f)
			kdict := (aspec>>1)&1 != 0
			hasBlock := aspec&1 != 0
			if key != 0 || kdict {
				return Nil, fmt.Errorf("mrbvm: keyword arguments are not supported yet")
			}
			provided := len(frameArgs)
			if provided < req+post {
				return Nil, fmt.Errorf("mrbvm: wrong number of arguments (given %d, expected %d)", provided, req)
			}
			optional := provided - req - post
			if optional < 0 {
				optional = 0
			}
			if optional > opt {
				optional = opt
			}
			copy(regs[1:1+req], frameArgs[:req])
			copy(regs[1+req:1+req+optional], frameArgs[req:req+optional])
			restCount := provided - req - optional - post
			if !hasRest && restCount > 0 {
				return Nil, fmt.Errorf("mrbvm: too many arguments")
			}
			restSlot := 1 + req + opt
			if hasRest {
				rest := make([]Value, restCount)
				copy(rest, frameArgs[req+optional:req+optional+restCount])
				regs[restSlot] = ArrayValue(rest)
			}
			postStart := restSlot
			if hasRest {
				postStart++
			}
			copy(regs[postStart:postStart+post], frameArgs[provided-post:provided])
			if hasBlock {
				blockSlot := postStart + post
				if blockSlot < len(regs) {
					regs[blockSlot] = frameBlock
				}
			}
			// The next opt+1 instructions are the optional-default jump table.
			pc += optional

		case bytecode.OpTDEF:
			a := int(in.Operands[0])
			name := irep.Syms[in.Operands[1]]
			childIndex := int(in.Operands[2])
			if childIndex < 0 || childIndex >= len(irep.Reps) {
				return Nil, fmt.Errorf("mrbvm: TDEF child irep %d out of range", childIndex)
			}
			targetClass.defineIrep(name, irep.Reps[childIndex])
			regs[a] = SymbolValue(name)

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
			regs[in.Operands[0]] = IntValue(int64(uint8(in.Operands[1])))
		case bytecode.OpLOADINEG:
			regs[in.Operands[0]] = IntValue(-int64(in.Operands[1]))
		case bytecode.OpLOADI16:
			regs[in.Operands[0]] = IntValue(int64(int16(in.Operands[1])))
		case bytecode.OpLOADI32:
			raw := uint32(in.Operands[1])<<16 | uint32(in.Operands[2])
			regs[in.Operands[0]] = IntValue(int64(int32(raw)))

		case bytecode.OpLOADSYM:
			regs[in.Operands[0]] = SymbolValue(irep.Syms[in.Operands[1]])
		case bytecode.OpSYMBOL:
			regs[in.Operands[0]] = SymbolValue(irep.Pool[in.Operands[1]].String)
		case bytecode.OpINTERN:
			regs[in.Operands[0]] = SymbolValue(regs[in.Operands[0]].String())
		case bytecode.OpSTRCAT:
			a := int(in.Operands[0])
			regs[a] = StringValue(regs[a].ToS() + regs[a+1].ToS())

		case bytecode.OpGETGV:
			regs[in.Operands[0]] = s.globals[irep.Syms[in.Operands[1]]]
		case bytecode.OpSETGV:
			s.globals[irep.Syms[in.Operands[1]]] = regs[in.Operands[0]]
		case bytecode.OpGETIV:
			regs[in.Operands[0]] = self.obj.ivar(irep.Syms[in.Operands[1]])
		case bytecode.OpSETIV:
			self.obj.setIvar(irep.Syms[in.Operands[1]], regs[in.Operands[0]])
		case bytecode.OpGETCONST:
			regs[in.Operands[0]] = s.constants[irep.Syms[in.Operands[1]]]
		case bytecode.OpSETCONST:
			s.constants[irep.Syms[in.Operands[1]]] = regs[in.Operands[0]]

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

		case bytecode.OpGETUPVAR:
			a := int(in.Operands[0])
			slot := int(in.Operands[1])
			level := int(in.Operands[2])
			env := frameEnv
			for i := 0; i <= level; i++ {
				if env == nil {
					return Nil, fmt.Errorf("mrbvm: broken upvar chain")
				}
				env = env.parent
			}
			if env == nil || slot < 0 || slot >= len(env.regs) {
				return Nil, fmt.Errorf("mrbvm: upvar slot %d out of range", slot)
			}
			regs[a] = env.regs[slot]
		case bytecode.OpSETUPVAR:
			a := int(in.Operands[0])
			slot := int(in.Operands[1])
			level := int(in.Operands[2])
			env := frameEnv
			for i := 0; i <= level; i++ {
				if env == nil {
					return Nil, fmt.Errorf("mrbvm: broken upvar chain")
				}
				env = env.parent
			}
			if env == nil || slot < 0 || slot >= len(env.regs) {
				return Nil, fmt.Errorf("mrbvm: upvar slot %d out of range", slot)
			}
			env.regs[slot] = regs[a]

		case bytecode.OpBLOCK, bytecode.OpLAMBDA:
			a := int(in.Operands[0])
			childIndex := int(in.Operands[1])
			if childIndex < 0 || childIndex >= len(irep.Reps) {
				return Nil, fmt.Errorf("mrbvm: block irep %d out of range", childIndex)
			}
			regs[a] = ObjectValue(&Object{data: &blockData{
				irep:        irep.Reps[childIndex],
				env:         frameEnv,
				self:        self,
				targetClass: targetClass,
			}})
		case bytecode.OpBLKPUSH:
			a := int(in.Operands[0])
			level := int(in.Operands[1] & 0x0f)
			env := frameEnv
			for i := 0; i < level; i++ {
				if env == nil {
					return Nil, fmt.Errorf("mrbvm: broken block environment")
				}
				env = env.parent
			}
			if env == nil {
				return Nil, fmt.Errorf("mrbvm: broken block environment")
			}
			regs[a] = env.block
		case bytecode.OpBLKCALL:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			args := make([]Value, n)
			copy(args, regs[a+1:a+1+n])
			result, err := s.callBlock(ctx, regs[a], args)
			if err != nil {
				return Nil, err
			}
			regs[a] = result
		case bytecode.OpBREAK:
			return Nil, &breakError{value: regs[in.Operands[0]]}
		case bytecode.OpRETURN_BLK:
			// TODO: non-local return from the enclosing method.
			return Nil, fmt.Errorf("mrbvm: return from a block is not supported yet")

		case bytecode.OpSENDB, bytecode.OpSSENDB:
			recv := self
			if in.Op == bytecode.OpSENDB {
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
			block := regs[in.Operands[0]+1+uint32(n)]
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], args, block)
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = result

		case bytecode.OpSEND0, bytecode.OpSSEND0:
			recv := self
			if in.Op == bytecode.OpSEND0 {
				recv = regs[in.Operands[0]]
			}
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], nil, Nil)
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
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], args, Nil)
			if err != nil {
				return Nil, err
			}
			regs[in.Operands[0]] = result

		case bytecode.OpJMP, bytecode.OpJMPUW:
			target, err := jumpTarget(in, pcByOffset)
			if err != nil {
				return Nil, err
			}
			pc = target - 1
		case bytecode.OpJMPIF:
			if regs[in.Operands[0]].IsTruthy() {
				target, err := jumpTarget(in, pcByOffset)
				if err != nil {
					return Nil, err
				}
				pc = target - 1
			}
		case bytecode.OpJMPNOT:
			if !regs[in.Operands[0]].IsTruthy() {
				target, err := jumpTarget(in, pcByOffset)
				if err != nil {
					return Nil, err
				}
				pc = target - 1
			}
		case bytecode.OpJMPNIL:
			if regs[in.Operands[0]].IsNil() {
				target, err := jumpTarget(in, pcByOffset)
				if err != nil {
					return Nil, err
				}
				pc = target - 1
			}

		case bytecode.OpADD, bytecode.OpSUB, bytecode.OpMUL, bytecode.OpDIV,
			bytecode.OpEQ, bytecode.OpLT, bytecode.OpLE, bytecode.OpGT, bytecode.OpGE:
			name := map[bytecode.Opcode]string{
				bytecode.OpADD: "+", bytecode.OpSUB: "-", bytecode.OpMUL: "*",
				bytecode.OpDIV: "/", bytecode.OpEQ: "==", bytecode.OpLT: "<",
				bytecode.OpLE: "<=", bytecode.OpGT: ">", bytecode.OpGE: ">=",
			}[in.Op]
			value, ok := binaryValue(regs[in.Operands[0]], name, regs[in.Operands[0]+1])
			if !ok {
				return Nil, fmt.Errorf("mrbvm: cannot apply %s to %s and %s", name, regs[in.Operands[0]].Inspect(), regs[in.Operands[0]+1].Inspect())
			}
			regs[in.Operands[0]] = value
		case bytecode.OpADDI, bytecode.OpSUBI:
			name := "+"
			if in.Op == bytecode.OpSUBI {
				name = "-"
			}
			value, ok := binaryValue(regs[in.Operands[0]], name, IntValue(int64(in.Operands[1])))
			if !ok {
				return Nil, fmt.Errorf("mrbvm: cannot apply %s to %s and %d", name, regs[in.Operands[0]].Inspect(), in.Operands[1])
			}
			regs[in.Operands[0]] = value
		case bytecode.OpADDILV, bytecode.OpSUBILV:
			name := "+"
			if in.Op == bytecode.OpSUBILV {
				name = "-"
			}
			value, ok := binaryValue(regs[in.Operands[0]], name, IntValue(int64(in.Operands[2])))
			if !ok {
				return Nil, fmt.Errorf("mrbvm: cannot apply %s to %s and %d", name, regs[in.Operands[0]].Inspect(), in.Operands[2])
			}
			regs[in.Operands[0]] = value

		case bytecode.OpARRAY:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			values := make([]Value, n)
			copy(values, regs[a:a+n])
			regs[a] = ArrayValue(values)
		case bytecode.OpARRAY2:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			n := int(in.Operands[2])
			values := make([]Value, n)
			copy(values, regs[b:b+n])
			regs[a] = ArrayValue(values)
		case bytecode.OpRANGE_INC, bytecode.OpRANGE_EXC:
			a := int(in.Operands[0])
			regs[a] = RangeValue(regs[a], regs[a+1], in.Op == bytecode.OpRANGE_EXC)
		case bytecode.OpHASH:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			entries := make([]hashEntry, n)
			for i := 0; i < n; i++ {
				entries[i] = hashEntry{key: regs[a+2*i], value: regs[a+2*i+1]}
			}
			regs[a] = HashValue(entries)
		case bytecode.OpGETIDX:
			a := int(in.Operands[0])
			value, err := s.index(ctx, regs[a], regs[a+1])
			if err != nil {
				return Nil, err
			}
			regs[a] = value
		case bytecode.OpARYPUSH:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			data, ok := regs[a].obj.data.(*arrayData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: ARYPUSH receiver is not an array")
			}
			for i := 0; i < n; i++ {
				data.values = append(data.values, regs[a+1+i])
			}
		case bytecode.OpARYCAT:
			a := int(in.Operands[0])
			dst, ok := regs[a].obj.data.(*arrayData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: ARYCAT receiver is not an array")
			}
			src, ok := regs[a+1].obj.data.(*arrayData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: ARYCAT source is not an array")
			}
			dst.values = append(dst.values, src.values...)
		case bytecode.OpARYSPLAT:
			a := int(in.Operands[0])
			if data, ok := regs[a].obj.data.(*arrayData); ok {
				values := make([]Value, len(data.values))
				copy(values, data.values)
				regs[a] = ArrayValue(values)
			} else if regs[a].IsNil() {
				regs[a] = ArrayValue(nil)
			} else {
				regs[a] = ArrayValue([]Value{regs[a]})
			}
		case bytecode.OpAREF:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			c := int64(in.Operands[2])
			isArray := false
			if regs[b].Kind() == KindObject && regs[b].obj != nil {
				_, isArray = regs[b].obj.data.(*arrayData)
			}
			if !isArray {
				if c == 0 {
					regs[a] = regs[b]
				} else {
					regs[a] = Nil
				}
				continue
			}
			value, err := s.index(ctx, regs[b], IntValue(c))
			if err != nil {
				return Nil, err
			}
			regs[a] = value
		case bytecode.OpASET:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			c := int64(in.Operands[2])
			if _, err := s.setIndex(ctx, regs[b], IntValue(c), regs[a]); err != nil {
				return Nil, err
			}
		case bytecode.OpAPOST:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			c := int(in.Operands[2])
			var values []Value
			if data, ok := regs[a].obj.data.(*arrayData); ok {
				values = data.values
			} else {
				values = []Value{regs[a]}
			}
			data := &arrayData{values: values}
			if b < 0 || b > len(data.values) {
				return Nil, fmt.Errorf("mrbvm: APOST index out of range")
			}
			postStart := len(data.values) - c
			if postStart < b {
				postStart = b
			}
			rest := append([]Value(nil), data.values[b:postStart]...)
			regs[a] = ArrayValue(rest)
			for i := 0; i < c; i++ {
				regs[a+1+i] = data.values[postStart+i]
			}
		case bytecode.OpHASHADD:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			data, ok := regs[a].obj.data.(*hashData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: HASHADD receiver is not a hash")
			}
			for i := 0; i < n; i++ {
				key := regs[a+1+2*i]
				value := regs[a+2+2*i]
				found := false
				for j := range data.entries {
					if valuesEqual(data.entries[j].key, key) {
						data.entries[j].value = value
						found = true
						break
					}
				}
				if !found {
					data.entries = append(data.entries, hashEntry{key: key, value: value})
				}
			}
		case bytecode.OpHASHCAT:
			a := int(in.Operands[0])
			dst, ok := regs[a].obj.data.(*hashData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: HASHCAT receiver is not a hash")
			}
			src, ok := regs[a+1].obj.data.(*hashData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: HASHCAT source is not a hash")
			}
			for _, entry := range src.entries {
				found := false
				for j := range dst.entries {
					if valuesEqual(dst.entries[j].key, entry.key) {
						dst.entries[j].value = entry.value
						found = true
						break
					}
				}
				if !found {
					dst.entries = append(dst.entries, entry)
				}
			}

		case bytecode.OpGETIDX0:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			value, err := s.index(ctx, regs[b], IntValue(0))
			if err != nil {
				return Nil, err
			}
			regs[a] = value

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

func (s *State) call(ctx context.Context, recv Value, name string, args []Value, block Value) (Value, error) {
	if len(args) == 0 {
		switch name {
		case "!":
			return BoolValue(!recv.IsTruthy()), nil
		case "-@":
			switch recv.Kind() {
			case KindInt:
				return IntValue(-recv.Int()), nil
			case KindFloat:
				return FloatValue(-recv.Float()), nil
			}
		}
	}
	if name == "===" && len(args) == 1 {
		if m, ok := s.classOf(recv).lookup(name); ok {
			return s.invoke(ctx, m, recv, args, block)
		}
		return BoolValue(valuesEqual(recv, args[0])), nil
	}
	if result, ok := binaryCall(recv, name, args); ok {
		return result, nil
	}
	m, ok := s.classOf(recv).lookup(name)
	if !ok {
		return Nil, fmt.Errorf("undefined method %q for %s", name, recv.Inspect())
	}
	return s.invoke(ctx, m, recv, args, block)
}

func (s *State) invoke(ctx context.Context, m *method, recv Value, args []Value, block Value) (Value, error) {
	if m.native != nil {
		return m.native(ctx, recv, args, block)
	}
	return s.executeFrame(ctx, m.irep, recv, m.owner, args, block, nil)
}

func binaryCall(recv Value, name string, args []Value) (Value, bool) {
	if len(args) != 1 {
		return Nil, false
	}
	return binaryValue(recv, name, args[0])
}

func binaryValue(left Value, name string, right Value) (Value, bool) {
	switch name {
	case "==":
		return BoolValue(valuesEqual(left, right)), true
	case "<", "<=", ">", ">=":
		var cmp int
		if li, ri, ok := bothInts(left, right); ok {
			switch {
			case li < ri:
				cmp = -1
			case li > ri:
				cmp = 1
			}
		} else if lf, rf, ok := bothFloats(left, right); ok {
			switch {
			case lf < rf:
				cmp = -1
			case lf > rf:
				cmp = 1
			}
		} else {
			return Nil, false
		}
		switch name {
		case "<":
			return BoolValue(cmp < 0), true
		case "<=":
			return BoolValue(cmp <= 0), true
		case ">":
			return BoolValue(cmp > 0), true
		case ">=":
			return BoolValue(cmp >= 0), true
		}
	case "+":
		if left.Kind() == KindString && right.Kind() == KindString {
			return StringValue(left.String() + right.String()), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li + ri), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf + rf), true
		}
	case "-":
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li - ri), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf - rf), true
		}
	case "*":
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li * ri), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf * rf), true
		}
	case "/":
		if li, ri, ok := bothInts(left, right); ok {
			if ri == 0 {
				return Nil, false
			}
			return IntValue(li / ri), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			if rf == 0 {
				return Nil, false
			}
			return FloatValue(lf / rf), true
		}
	}
	return Nil, false
}

func bothInts(left, right Value) (int64, int64, bool) {
	if left.Kind() == KindInt && right.Kind() == KindInt {
		return left.Int(), right.Int(), true
	}
	return 0, 0, false
}

func bothFloats(left, right Value) (float64, float64, bool) {
	li, lf, ok1 := numeric(left)
	ri, rf, ok2 := numeric(right)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	_ = li
	_ = ri
	return lf, rf, true
}

func numeric(v Value) (int64, float64, bool) {
	switch v.Kind() {
	case KindInt:
		return v.Int(), float64(v.Int()), true
	case KindFloat:
		return 0, v.Float(), true
	default:
		return 0, 0, false
	}
}

func valuesEqual(left, right Value) bool {
	if left.Kind() != right.Kind() {
		if li, lf, ok := numeric(left); ok {
			if ri, rf, ok := numeric(right); ok {
				_ = li
				_ = ri
				return lf == rf
			}
		}
		return false
	}
	switch left.Kind() {
	case KindNil:
		return true
	case KindTrue, KindFalse:
		return true
	case KindInt:
		return left.Int() == right.Int()
	case KindFloat:
		return left.Float() == right.Float()
	case KindString, KindSymbol:
		return left.String() == right.String()
	case KindObject:
		return left.obj == right.obj
	default:
		return false
	}
}

func jumpTarget(in bytecode.Instruction, pcByOffset map[int]int) (int, error) {
	index := 0
	if in.Op != bytecode.OpJMP && in.Op != bytecode.OpJMPUW {
		index = 1
	}
	if index >= len(in.Operands) {
		return 0, fmt.Errorf("mrbvm: %s has no jump operand", in.Op)
	}
	offset := int(int16(uint16(in.Operands[index])))
	targetOffset := in.Offset + in.Size + offset
	target, ok := pcByOffset[targetOffset]
	if !ok {
		return 0, fmt.Errorf("mrbvm: jump target %d is not an instruction boundary", targetOffset)
	}
	return target, nil
}

func parseBigInt(payload []byte) (*big.Int, error) {
	if len(payload) < 2 {
		return nil, fmt.Errorf("mrbvm: malformed bigint pool entry")
	}
	length := int(payload[0])
	if len(payload) != length+2 {
		return nil, fmt.Errorf("mrbvm: malformed bigint payload length %d", length)
	}
	sign := int8(payload[1])
	if sign != 10 && sign != -10 {
		return nil, fmt.Errorf("mrbvm: unsupported bigint sign byte %d", payload[1])
	}
	value, ok := new(big.Int).SetString(string(payload[2:]), 10)
	if !ok {
		return nil, fmt.Errorf("mrbvm: invalid bigint digits")
	}
	if sign < 0 {
		value.Neg(value)
	}
	return value, nil
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
	case bytecode.PoolBigInt:
		value, err := parseBigInt(entry.BigInt)
		if err != nil {
			return Nil, err
		}
		if !value.IsInt64() {
			return Nil, fmt.Errorf("mrbvm: big integer outside int64 range is not supported yet")
		}
		return IntValue(value.Int64()), nil
	default:
		return Nil, fmt.Errorf("mrbvm: unsupported pool entry kind %d", entry.Kind)
	}
}
