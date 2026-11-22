package mrbvm

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/onohiroki/go-mrbc/bytecode"
)

func (s *State) executeIrep(ctx context.Context, irep *Irep, self Value) (Value, error) {
	return s.executeFrame(ctx, irep, self, s.ObjectClass, nil, Nil, Nil, nil, "", nil, s.ObjectClass, false)
}

type irepProgram struct {
	instructions []bytecode.Instruction
	pcByOffset   map[int]int
	lineAt       []int32
	jumpAt       []int32
	binaryOps    []binaryOp
	hasBlocks    bool
}

type callFrame struct {
	file        string
	line        int
	method      string
	lineText    string
	lineTextFor int
}

type exceptionScope struct {
	prev   Value
	exitPC int
	kind   uint8
}

func (s *State) program(irep *Irep) (*irepProgram, error) {
	if program, ok := s.programs[irep]; ok {
		return program, nil
	}
	instructions, err := bytecode.Decode(irep.Code)
	if err != nil {
		return nil, err
	}
	pcByOffset := make(map[int]int, len(instructions))
	for i, in := range instructions {
		pcByOffset[in.Offset] = i
	}
	lineAt := make([]int32, len(instructions))
	lineEntry := 0
	line := int32(0)
	for i, in := range instructions {
		for lineEntry < len(irep.Lines) && int(irep.Lines[lineEntry].Offset) <= in.Offset {
			line = int32(irep.Lines[lineEntry].Line)
			lineEntry++
		}
		lineAt[i] = line
	}
	hasBlocks := false
	for _, in := range instructions {
		if in.Op == bytecode.OpBLOCK || in.Op == bytecode.OpLAMBDA {
			hasBlocks = true
			break
		}
	}
	jumpAt := make([]int32, len(instructions))
	for i := range jumpAt {
		jumpAt[i] = -1
	}
	for i, in := range instructions {
		switch in.Op {
		case bytecode.OpJMP, bytecode.OpJMPUW, bytecode.OpJMPIF, bytecode.OpJMPNOT, bytecode.OpJMPNIL:
			if target, err := jumpTarget(in, pcByOffset); err == nil {
				jumpAt[i] = int32(target)
			}
		}
	}
	binaryOps := make([]binaryOp, len(instructions))
	for i, in := range instructions {
		if in.Op != bytecode.OpSEND && in.Op != bytecode.OpSSEND {
			continue
		}
		argInfo := in.Operands[2]
		if argInfo&0x0f != 1 || (argInfo>>4)&0x0f != 0 {
			continue
		}
		if sym := int(in.Operands[1]); sym < len(irep.Syms) {
			binaryOps[i] = binaryOpForName(irep.Syms[sym])
		}
	}
	program := &irepProgram{instructions: instructions, pcByOffset: pcByOffset, lineAt: lineAt, jumpAt: jumpAt, binaryOps: binaryOps, hasBlocks: hasBlocks}
	s.programs[irep] = program
	return program, nil
}

// leaveFrame undoes executeFrame's bookkeeping in one deferred call. Using a
// single defer keeps the per-frame defer overhead low.
func leaveFrame(s *State, entryException Value, regs []Value, env *environment, pooled bool) {
	s.frames = s.frames[:len(s.frames)-1]
	s.currentException = entryException
	if pooled {
		if env != nil {
			s.releaseEnvironment(env)
		}
		s.releaseSlice(&s.regFree, regs)
	}
	s.callDepth--
}

func (s *State) executeFrame(ctx context.Context, irep *Irep, self Value, targetClass *Class, args []Value, kw Value, block Value, capturedEnv *environment, mid string, owner *Class, cref *Class, strict bool) (Value, error) {
	s.callDepth++
	if s.maxDepth > 0 && s.callDepth > s.maxDepth {
		s.callDepth--
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.SystemStackErrorClass, "stack level too deep")}
	}
	program, err := s.program(irep)
	if err != nil {
		s.callDepth--
		return Nil, err
	}
	instructions := program.instructions
	nregs := int(irep.Nregs)
	if nlocals := int(irep.Nlocals); nlocals > nregs {
		nregs = nlocals
	}
	if nregs < 1 {
		nregs = 1
	}
	// Some mruby compiler outputs use a register just past nregs for
	// method/class definition results. Keep a small guard area.
	nregs += 2
	pooled := !program.hasBlocks
	var regs []Value
	if pooled {
		regs = s.acquireSlice(&s.regFree, nregs)
	} else {
		regs = make([]Value, nregs)
	}
	regs[0] = self
	frameArgs := args
	frameKw := kw
	frameBlock := block
	var frameEnv *environment
	if pooled {
		frameEnv = s.acquireEnvironment()
		frameEnv.parent = capturedEnv
		frameEnv.regs = regs
		frameEnv.block = block
		frameEnv.methodFrame = mid != ""
	} else {
		frameEnv = &environment{parent: capturedEnv, regs: regs, block: block, methodFrame: mid != ""}
	}
	_ = frameEnv
	keywordSlot := -1
	_ = keywordSlot
	pcByOffset := program.pcByOffset
	lineAt := program.lineAt
	jumpAt := program.jumpAt
	binaryOps := program.binaryOps
	frame := callFrame{file: irep.Filename, method: mid}
	entryException := s.currentException
	var exceptionScopes []exceptionScope
	s.frames = append(s.frames, frame)
	if strict {
		s.strictDepth++
	}
	defer func() {
		if strict {
			s.strictDepth--
		}
		leaveFrame(s, entryException, regs, frameEnv, pooled)
	}()

	// caught holds the signal that transferred control into an ensure or
	// rescue handler. OP_EXCEPT turns it back into a register value.
	var caught *unwindSignal

	handlerAt := func(offset int) *bytecode.CatchHandler {
		for i := len(irep.Catch) - 1; i >= 0; i-- {
			handler := &irep.Catch[i]
			if offset >= int(handler.Begin) && offset < int(handler.End) {
				return handler
			}
		}
		return nil
	}
	ensureAt := func(offset, target int) *bytecode.CatchHandler {
		for i := len(irep.Catch) - 1; i >= 0; i-- {
			handler := &irep.Catch[i]
			if handler.Type != bytecode.CatchEnsure {
				continue
			}
			if offset < int(handler.Begin) || offset >= int(handler.End) {
				continue
			}
			if target >= 0 && target >= int(handler.Begin) && target < int(handler.End) {
				return nil
			}
			return handler
		}
		return nil
	}
	// handle routes a signal through catch handlers. It returns either a new
	// pc, a value to return from this frame, or the signal to propagate.
	handlerExit := func(handler *bytecode.CatchHandler) int {
		start, ok := pcByOffset[int(handler.Target)]
		if !ok {
			return len(instructions)
		}
		if handler.Type == bytecode.CatchEnsure {
			for i := start; i < len(instructions); i++ {
				if instructions[i].Op == bytecode.OpRAISEIF {
					return i + 1
				}
			}
			return len(instructions)
		}
		i := start
		if i < len(instructions) && instructions[i].Op == bytecode.OpEXCEPT {
			i++
		}
		for i < len(instructions) {
			for i < len(instructions) && instructions[i].Op != bytecode.OpJMPIF {
				i++
			}
			if i >= len(instructions) {
				break
			}
			bodyStart, err := jumpTarget(instructions[i], pcByOffset)
			if err != nil {
				break
			}
			j := i + 1
			for j < len(instructions) && instructions[j].Op != bytecode.OpJMP {
				j++
			}
			if j >= len(instructions) {
				break
			}
			nextClause, err := jumpTarget(instructions[j], pcByOffset)
			if err != nil {
				break
			}
			exit := -1
			for k := bodyStart; k < nextClause && k < len(instructions); k++ {
				if instructions[k].Op == bytecode.OpJMP {
					if target, err := jumpTarget(instructions[k], pcByOffset); err == nil && target > exit {
						exit = target
					}
				}
			}
			if exit >= 0 {
				return exit
			}
			if nextClause >= len(instructions) {
				break
			}
			i = nextClause
		}
		return start
	}

	pruneExceptionScopes := func(target int) {
		for len(exceptionScopes) > 0 && target >= exceptionScopes[len(exceptionScopes)-1].exitPC {
			scope := exceptionScopes[len(exceptionScopes)-1]
			exceptionScopes = exceptionScopes[:len(exceptionScopes)-1]
			s.currentException = scope.prev
		}
	}
	pushExceptionScope := func(handler *bytecode.CatchHandler, target int) {
		pruneExceptionScopes(target)
		exceptionScopes = append(exceptionScopes, exceptionScope{prev: s.currentException, exitPC: handlerExit(handler), kind: handler.Type})
	}
	targetPC := func(handler *bytecode.CatchHandler) (int, error) {
		target, ok := pcByOffset[int(handler.Target)]
		if !ok {
			return 0, fmt.Errorf("mrbvm: catch target %d is not an instruction boundary", handler.Target)
		}
		return target, nil
	}
	handle := func(sig *unwindSignal, offset int) (int, Value, bool, error) {
		switch sig.kind {
		case unwindJump:
			for len(exceptionScopes) > 0 && exceptionScopes[len(exceptionScopes)-1].kind == bytecode.CatchEnsure && sig.jumpTarget >= exceptionScopes[len(exceptionScopes)-1].exitPC {
				scope := exceptionScopes[len(exceptionScopes)-1]
				exceptionScopes = exceptionScopes[:len(exceptionScopes)-1]
				s.currentException = scope.prev
			}
			jumpOffset := -1
			if sig.jumpTarget >= 0 && sig.jumpTarget < len(instructions) {
				jumpOffset = instructions[sig.jumpTarget].Offset
			}
			if handler := ensureAt(offset, jumpOffset); handler != nil {
				target, err := targetPC(handler)
				if err != nil {
					return 0, Nil, false, err
				}
				pushExceptionScope(handler, target)
				caught = sig
				return target, Nil, false, nil
			}
			return sig.jumpTarget, Nil, false, nil
		case unwindException:
			if pc, ok := pcByOffset[offset]; ok {
				if line := lineAt[pc]; line != 0 {
					s.frames[len(s.frames)-1].line = int(line)
				}
			}
			s.ensureBacktrace(sig.value)
			if handler := handlerAt(offset); handler != nil {
				target, err := targetPC(handler)
				if err != nil {
					return 0, Nil, false, err
				}
				pushExceptionScope(handler, target)
				caught = sig
				return target, Nil, false, nil
			}
			return 0, Nil, false, sig
		case unwindReturn:
			if handler := ensureAt(offset, -1); handler != nil {
				target, err := targetPC(handler)
				if err != nil {
					return 0, Nil, false, err
				}
				pushExceptionScope(handler, target)
				caught = sig
				return target, Nil, false, nil
			}
			if sig.target == frameEnv {
				return 0, sig.value, true, nil
			}
			return 0, Nil, false, sig
		case unwindBreak:
			if handler := ensureAt(offset, -1); handler != nil {
				target, err := targetPC(handler)
				if err != nil {
					return 0, Nil, false, err
				}
				pushExceptionScope(handler, target)
				caught = sig
				return target, Nil, false, nil
			}
			if sig.target == frameEnv {
				return 0, sig.value, true, nil
			}
			return 0, Nil, false, sig
		}
		return 0, Nil, false, sig
	}
	dispatch := func(err error, offset int) (int, Value, bool, error) {
		sig, ok := err.(*unwindSignal)
		if !ok {
			return 0, Nil, false, err
		}
		return handle(sig, offset)
	}
	ctxDone := ctx.Done()
	for pc := 0; pc < len(instructions); pc++ {
		if pc&1023 == 0 && ctxDone != nil {
			if err := ctx.Err(); err != nil {
				return Nil, err
			}
		}
		if s.limitsEnabled {
			if err := s.checkExecutionLimits(); err != nil {
				return Nil, err
			}
		}
		for len(exceptionScopes) > 0 && pc == exceptionScopes[len(exceptionScopes)-1].exitPC {
			scope := exceptionScopes[len(exceptionScopes)-1]
			exceptionScopes = exceptionScopes[:len(exceptionScopes)-1]
			s.currentException = scope.prev
		}
		in := instructions[pc]
		switch in.Op {
		case bytecode.OpNOP:
			// no-op

		case bytecode.OpMOVE:
			regs[in.Operands[0]] = regs[in.Operands[1]]

		case bytecode.OpKEY_P:
			a := int(in.Operands[0])
			if keywordSlot < 0 {
				regs[a] = False
				continue
			}
			regs[a] = BoolValue(hashHasKey(regs[keywordSlot], irep.Syms[in.Operands[1]]))
		case bytecode.OpKARG:
			a := int(in.Operands[0])
			name := irep.Syms[in.Operands[1]]
			value, ok := hashDeleteKey(regs[keywordSlot], name)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: missing keyword: %s", name)
			}
			regs[a] = value
		case bytecode.OpKEYEND:
			if keywordSlot >= 0 && regs[keywordSlot].Kind() == KindObject && regs[keywordSlot].obj != nil {
				if hash, ok := regs[keywordSlot].obj.data.(*hashData); ok && len(hash.entries) > 0 {
					return Nil, fmt.Errorf("mrbvm: unknown keyword: %s", hash.entries[0].key.Inspect())
				}
			}

		case bytecode.OpENTER:
			aspec := in.Operands[0]
			req := int((aspec >> 18) & 0x1f)
			opt := int((aspec >> 13) & 0x1f)
			hasRest := (aspec>>12)&1 != 0
			post := int((aspec >> 7) & 0x1f)
			key := int((aspec >> 2) & 0x1f)
			kdict := (aspec>>1)&1 != 0
			hasBlock := aspec&1 != 0
			noBlock := (aspec>>23)&1 != 0
			if noBlock && !frameBlock.IsNil() {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, "no block accepted")}
			}
			// A keyword dictionary passed to a method without keyword
			// parameters becomes the last positional argument.
			if key == 0 && !kdict && !frameKw.IsNil() {
				if hash, ok := frameKw.obj.data.(*hashData); ok && len(hash.entries) > 0 {
					frameArgs = append(append([]Value(nil), frameArgs...), frameKw)
				}
				frameKw = Nil
			}
			argCount := req + opt + post
			if hasRest {
				argCount++
			}
			if !strict && argCount > 1 && len(frameArgs) == 1 && frameArgs[0].obj != nil {
				if array, ok := frameArgs[0].obj.data.(*arrayData); ok {
					frameArgs = array.values
				}
			}
			provided := len(frameArgs)
			if strict && provided < req+post {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected %d)", provided, req+post))}
			}
			optional := provided - req - post
			if optional < 0 {
				optional = 0
			}
			if optional > opt {
				optional = opt
			}
			for i := 0; i < req; i++ {
				if i < provided {
					regs[1+i] = frameArgs[i]
				} else {
					regs[1+i] = Nil
				}
			}
			for i := 0; i < optional; i++ {
				regs[1+req+i] = frameArgs[req+i]
			}
			restCount := provided - req - optional - post
			if restCount < 0 {
				restCount = 0
			}
			if strict && !hasRest && restCount > 0 {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.ArgumentErrorClass, fmt.Sprintf("wrong number of arguments (given %d, expected %d)", provided, req+opt+post))}
			}
			restSlot := 1 + req + opt
			if hasRest {
				rest := make([]Value, restCount)
				if restCount > 0 {
					start := req + optional
					copy(rest, frameArgs[start:start+restCount])
				}
				regs[restSlot] = ArrayValue(rest)
			}
			postStart := restSlot
			if hasRest {
				postStart++
			}
			filledPost := provided - req
			if filledPost > post {
				filledPost = post
			}
			if filledPost < 0 {
				filledPost = 0
			}
			for i := 0; i < post; i++ {
				if i < post-filledPost {
					regs[postStart+i] = Nil
					continue
				}
				index := provided - (post - i)
				regs[postStart+i] = frameArgs[index]
			}
			positional := req + opt
			if hasRest {
				positional++
			}
			positional += post
			keywordSlot = -1
			blockSlot := 1 + positional
			if key > 0 || kdict {
				keywordSlot = 1 + positional
				if keywordSlot < len(regs) {
					if kdict && frameKw.IsNil() {
						regs[keywordSlot] = HashValue(nil)
					} else {
						regs[keywordSlot] = frameKw
					}
				}
				blockSlot = keywordSlot + 1
			}
			if hasBlock && blockSlot < len(regs) {
				regs[blockSlot] = frameBlock
			}
			// The next opt+1 instructions are the optional-default jump table.
			pc += optional

		case bytecode.OpALIAS:
			a := irep.Syms[in.Operands[0]]
			b := irep.Syms[in.Operands[1]]
			if m, ok := targetClass.lookup(b); ok {
				targetClass.methods[a] = m
			} else {
				sig := &unwindSignal{kind: unwindException, value: s.newNameError(b, s.classValue(targetClass))}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
		case bytecode.OpUNDEF:
			delete(targetClass.methods, irep.Syms[in.Operands[0]])

		case bytecode.OpSCLASS:
			a := int(in.Operands[0])
			if class, ok := s.asClass(regs[a]); ok {
				if class.singleton == nil {
					class.singleton = newClass(class.name, nil)
					class.singleton.singletonOf = class
					class.singleton.isSingleton = true
					class.singleton.lexicalOuter = class
					s.ensureClassObject(class.singleton)
				}
				regs[a] = s.classValue(class.singleton)
				break
			}
			if regs[a].obj == nil {
				kind := regs[a].Kind()
				if kind == KindNil || kind == KindTrue || kind == KindFalse {
					if s.immediateSingletons == nil {
						s.immediateSingletons = make(map[Kind]*Class)
					}
					singleton := s.immediateSingletons[kind]
					if singleton == nil {
						singleton = newClass("#<Class:"+s.classOf(regs[a]).name+">", s.classOf(regs[a]))
						singleton.isSingleton = true
						s.ensureClassObject(singleton)
						s.immediateSingletons[kind] = singleton
					}
					regs[a] = s.classValue(singleton)
					break
				}
				sig := &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass, "can't define singleton")}
				np, rv, done, fatal := dispatch(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			obj := regs[a].obj
			if obj.singleton == nil {
				original := s.classOf(regs[a])
				obj.singleton = newClass("#<Class:"+original.name+">", original)
				obj.singleton.isSingleton = true
				s.ensureClassObject(obj.singleton)
			}
			if obj.frozen && obj.singleton.object != nil {
				obj.singleton.object.frozen = true
			}
			regs[a] = s.classValue(obj.singleton)
		case bytecode.OpTCLASS:
			a := int(in.Operands[0])
			if targetClass == nil {
				return Nil, fmt.Errorf("mrbvm: TCLASS outside a class")
			}
			regs[a] = s.classValue(targetClass)

		case bytecode.OpTDEF:
			a := int(in.Operands[0])
			name := irep.Syms[in.Operands[1]]
			childIndex := int(in.Operands[2])
			if childIndex < 0 || childIndex >= len(irep.Reps) {
				return Nil, fmt.Errorf("mrbvm: TDEF child irep %d out of range", childIndex)
			}
			if err := s.ensureMutable(s.classValue(targetClass)); err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			savedModuleFunction := targetClass.moduleFunction
			if strict {
				targetClass.moduleFunction = false
			}
			targetClass.defineIrep(name, irep.Reps[childIndex])
			methodCref := cref
			if targetClass.singletonOf != nil {
				if m, ok := targetClass.singletonOf.classMethods[name]; ok {
					m.cref = targetClass
				}
			} else if m, ok := targetClass.methods[name]; ok {
				m.cref = methodCref
			}
			targetClass.moduleFunction = savedModuleFunction
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
			leftText := regs[a].ToS()
			right := regs[a+1]
			rightText := right.ToS()
			if right.Kind() == KindObject {
				converted, err := s.call(ctx, right, "to_s", nil, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				if converted.Kind() == KindString {
					rightText = converted.String()
				}
			}
			regs[a] = StringValue(leftText + rightText)

		case bytecode.OpGETGV:
			name := irep.Syms[in.Operands[1]]
			if name == "$!" {
				regs[in.Operands[0]] = s.currentException
			} else {
				regs[in.Operands[0]] = s.globals[name]
			}
			s.definedKind = "global-variable"
			s.definedSet = true
			s.definedNil = regs[in.Operands[0]].IsNil()
		case bytecode.OpSETGV:
			name := irep.Syms[in.Operands[1]]
			if name == "$!" {
				s.currentException = regs[in.Operands[0]]
			} else {
				s.globals[name] = regs[in.Operands[0]]
			}
		case bytecode.OpGETIV:
			regs[in.Operands[0]] = self.obj.ivar(irep.Syms[in.Operands[1]])
			s.definedKind = "instance-variable"
			s.definedSet = true
			s.definedNil = regs[in.Operands[0]].IsNil()
		case bytecode.OpSETIV:
			self.obj.setIvar(irep.Syms[in.Operands[1]], regs[in.Operands[0]])
		case bytecode.OpOCLASS:
			regs[in.Operands[0]] = s.classValue(s.ObjectClass)
		case bytecode.OpGETCV:
			varClass := cref
			if varClass == nil {
				varClass = targetClass
			}
			regs[in.Operands[0]] = lookupClassVar(classVarTarget(varClass), irep.Syms[in.Operands[1]])
			s.definedKind = "class variable"
			s.definedSet = true
			s.definedNil = regs[in.Operands[0]].IsNil()
		case bytecode.OpSETCV:
			varClass := cref
			if varClass == nil {
				varClass = targetClass
			}
			if err := s.setClassVar(classVarTarget(varClass), irep.Syms[in.Operands[1]], regs[in.Operands[0]]); err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
		case bytecode.OpGETCONST:
			name := irep.Syms[in.Operands[1]]
			value, found := lookupLexicalConstant(cref, name)
			if !found && cref != nil && cref.singletonOf != nil && !strict {
				for _, current := range classAncestors(cref.singletonOf) {
					if candidate, ok := current.constants[name]; ok {
						value, found = candidate, true
						break
					}
				}
			}
			if !found {
				if candidate, ok := s.ObjectClass.constants[name]; ok {
					value, found = candidate, true
				}
			}
			if !found {
				if builtin, ok := s.constants[name]; ok {
					value, found = builtin, true
				}
			}
			if found {
				regs[in.Operands[0]] = value
				s.definedKind, s.definedSet, s.definedNil = "constant", true, false
			} else if definedNext(instructions, pc, irep.Syms) {
				regs[in.Operands[0]] = Nil
				s.definedKind, s.definedSet, s.definedNil = "constant", true, true
			} else if constAssignedSoon(instructions, pc, in.Operands[1]) {
				regs[in.Operands[0]] = Nil
			} else if cref != nil {
				if hook, ok := cref.lookupClassMethod("const_missing"); ok {
					missing, err := s.invoke(ctx, hook, s.classValue(cref), []Value{SymbolValue(name)}, Nil, Nil)
					if err != nil {
						np, rv, done, fatal := dispatch(err, in.Offset)
						if fatal != nil {
							return Nil, fatal
						}
						if done {
							return rv, nil
						}
						pc = np - 1
						continue
					}
					regs[in.Operands[0]] = missing
				} else {
					sig := &unwindSignal{kind: unwindException, value: s.newConstError(name, s.classValue(cref))}
					np, rv, done, fatal := handle(sig, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
			} else {
				sig := &unwindSignal{kind: unwindException, value: s.newConstError(name, s.classValue(s.ObjectClass))}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}

		case bytecode.OpSETCONST:
			name := irep.Syms[in.Operands[1]]
			constTarget := cref
			if constTarget == nil {
				constTarget = targetClass
			}
			if constTarget == nil {
				constTarget = s.ObjectClass
			}
			s.nameClassConstant(regs[in.Operands[0]], constTarget, name)
			if constTarget == s.ObjectClass {
				s.constants[name] = regs[in.Operands[0]]
			}
			constTarget.constants[name] = regs[in.Operands[0]]
		case bytecode.OpGETMCNST:
			a := int(in.Operands[0])
			class, ok := s.asClass(regs[a])
			if !ok {
				return Nil, fmt.Errorf("mrbvm: GETMCNST receiver is not a class")
			}
			name := irep.Syms[in.Operands[1]]
			if owner := constantOwner(class, name); owner != nil && owner.privateConstants[name] {
				if definedNext(instructions, pc, irep.Syms) {
					regs[a] = Nil
					s.definedKind, s.definedSet, s.definedNil = "constant", true, true
					continue
				}
				if hook, found := class.lookupClassMethod("const_missing"); found {
					missing, err := s.invoke(ctx, hook, s.classValue(class), []Value{SymbolValue(name)}, Nil, Nil)
					if err != nil {
						np, rv, done, fatal := dispatch(err, in.Offset)
						if fatal != nil {
							return Nil, fatal
						}
						if done {
							return rv, nil
						}
						pc = np - 1
						continue
					}
					regs[a] = missing
					continue
				}
				sig := &unwindSignal{kind: unwindException, value: s.newPrivateConstError(name, s.classValue(owner))}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			value := s.lookupConstant(class, name)
			if value.IsNil() {
				if definedNext(instructions, pc, irep.Syms) {
					regs[a] = Nil
					s.definedKind, s.definedSet, s.definedNil = "constant", true, true
					continue
				}
				sig := &unwindSignal{kind: unwindException, value: s.newConstError(name, s.classValue(class))}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = value
			s.definedKind, s.definedSet, s.definedNil = "constant", true, false
		case bytecode.OpSETMCNST:
			a := int(in.Operands[0])
			class, ok := s.asClass(regs[a+1])
			if !ok {
				return Nil, fmt.Errorf("mrbvm: SETMCNST receiver is not a class")
			}
			name := irep.Syms[in.Operands[1]]
			s.nameClassConstant(regs[a], class, name)
			class.constants[name] = regs[a]

		case bytecode.OpMODULE, bytecode.OpCLASS:
			a := int(in.Operands[0])
			var super Value = Nil
			if in.Op == bytecode.OpCLASS && a+1 < len(regs) {
				super = regs[a+1]
			}
			outer := regs[a]
			if outer.IsNil() {
				if targetClass != nil && targetClass.singletonOf != nil {
					outer = s.classValue(targetClass)
				} else if cref != nil {
					outer = s.classValue(cref)
				} else if targetClass != nil {
					outer = s.classValue(targetClass)
				}
			}
			outerWasNil := regs[a].IsNil()
			value, err := s.classOp(ctx, outer, super, irep.Syms[in.Operands[1]], in.Op == bytecode.OpCLASS)
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			if outerWasNil && owner != nil {
				if created, ok := s.asClass(value); ok {
					outerCref := cref
					if outerCref != nil && outerCref.singletonOf != nil {
						outerCref = outerCref.singletonOf
					}
					created.lexicalOuter = outerCref
				}
			}
			regs[a] = value

		case bytecode.OpEXEC:
			a := int(in.Operands[0])
			childIndex := int(in.Operands[1])
			if childIndex < 0 || childIndex >= len(irep.Reps) {
				return Nil, fmt.Errorf("mrbvm: EXEC child irep %d out of range", childIndex)
			}
			class, ok := s.asClass(regs[a])
			if !ok {
				return Nil, fmt.Errorf("mrbvm: EXEC target is not a class")
			}
			savedVisibility := class.defaultVisibility
			result, err := s.executeFrame(ctx, irep.Reps[childIndex], regs[a], class, nil, Nil, Nil, nil, "", class, class, false)
			class.defaultVisibility = savedVisibility
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = result

		case bytecode.OpSDEF:
			a := int(in.Operands[0])
			name := irep.Syms[in.Operands[1]]
			childIndex := int(in.Operands[2])
			if childIndex < 0 || childIndex >= len(irep.Reps) {
				return Nil, fmt.Errorf("mrbvm: SDEF child irep %d out of range", childIndex)
			}
			if class, ok := s.asClass(regs[a]); ok {
				class.defineClassIrep(name, irep.Reps[childIndex])
				if m, ok := class.classMethods[name]; ok {
					m.cref = cref
				}
			} else if regs[a].obj != nil {
				obj := regs[a].obj
				if obj.singleton == nil {
					original := s.classOf(regs[a])
					obj.singleton = newClass("#<Class>", original)
					s.ensureClassObject(obj.singleton)
				}
				obj.singleton.defineIrep(name, irep.Reps[childIndex])
			} else {
				return Nil, fmt.Errorf("mrbvm: singleton method definition on an immediate value is not supported")
			}
			regs[a] = SymbolValue(name)

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
			adjust := strict && mid != "" && pc+1 < len(instructions) && instructions[pc+1].Op == bytecode.OpSUPER
			env := resolveUpvar(frameEnv, level, adjust)
			if env == nil || slot < 0 || slot >= len(env.regs) {
				return Nil, fmt.Errorf("mrbvm: upvar slot %d out of range", slot)
			}
			regs[a] = env.regs[slot]
		case bytecode.OpSETUPVAR:
			a := int(in.Operands[0])
			slot := int(in.Operands[1])
			level := int(in.Operands[2])
			env := resolveUpvar(frameEnv, level, false)
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
			visibility := ""
			if targetClass != nil {
				visibility = targetClass.defaultVisibility
			}
			regs[a] = BlockValue(blockData{
				irep:        irep.Reps[childIndex],
				env:         frameEnv,
				self:        self,
				targetClass: targetClass,
				strict:      in.Op == bytecode.OpLAMBDA,
				mid:         mid,
				owner:       owner,
				visibility:  visibility,
				cref:        cref,
			})
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
			args := s.acquireValues(n)
			copy(args, regs[a+1:a+1+n])
			if line := lineAt[pc]; line != 0 {
				s.frames[len(s.frames)-1].line = int(line)
			}
			result, err := s.callBlock(ctx, regs[a], args, Nil)
			s.releaseValues(args)
			if err != nil {
				if sig, ok := err.(*unwindSignal); ok && sig.kind == unwindBreak && sig.target == nil {
					copySig := *sig
					copySig.target = frameEnv
					err = &copySig
				}
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = result
		case bytecode.OpMATCHERR:
			if !regs[in.Operands[0]].IsTruthy() {
				sig := &unwindSignal{kind: unwindException, value: newException(s.NoMatchingPatternErrorClass, "pattern not matched")}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
			}

		case bytecode.OpEXCEPT:
			a := int(in.Operands[0])
			if caught != nil {
				if caught.kind == unwindException {
					regs[a] = caught.value
					s.currentException = caught.value
				} else {
					regs[a] = signalValue(caught)
				}
				caught = nil
			} else {
				regs[a] = Nil
			}
		case bytecode.OpRESCUE:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			if sig, ok := asSignal(regs[a]); ok && sig.kind != unwindException {
				regs[b] = False
				break
			}
			class, ok := s.asClass(regs[b])
			if !ok {
				return Nil, fmt.Errorf("mrbvm: class or module required for rescue clause")
			}
			regs[b] = BoolValue(s.isA(regs[a], class))
		case bytecode.OpRAISEIF:
			value := regs[in.Operands[0]]
			if value.IsNil() {
				break
			}
			sig, ok := asSignal(value)
			if !ok {
				sig = &unwindSignal{kind: unwindException, value: value}
			}
			if len(exceptionScopes) > 0 {
				scope := exceptionScopes[len(exceptionScopes)-1]
				pop := scope.exitPC == pc+1
				if scope.kind == bytecode.CatchRescue {
					pop = false
					for j := pc + 1; j < len(instructions); j++ {
						if instructions[j].Op == bytecode.OpJMP {
							if target, err := jumpTarget(instructions[j], pcByOffset); err == nil && target == scope.exitPC {
								pop = true
							}
							break
						}
					}
				}
				if pop {
					exceptionScopes = exceptionScopes[:len(exceptionScopes)-1]
					s.currentException = scope.prev
				}
			}
			np, rv, done, fatal := handle(sig, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1

		case bytecode.OpBREAK:
			value := regs[in.Operands[0]]
			sig := &unwindSignal{kind: unwindBreak, value: value}
			if strict {
				sig = &unwindSignal{kind: unwindReturn, value: value, target: frameEnv}
			}
			np, rv, done, fatal := handle(sig, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpRETURN_BLK:
			value := regs[in.Operands[0]]
			var target *environment
			if strict {
				target = frameEnv
			} else {
				target = nearestMethodEnv(capturedEnv)
			}
			if target == nil {
				sig := &unwindSignal{kind: unwindException, value: newException(s.LocalJumpErrorClass, "unexpected return")}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: value, target: target}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1

		case bytecode.OpSENDB, bytecode.OpSSENDB:
			recv := self
			if in.Op == bytecode.OpSENDB {
				recv = regs[in.Operands[0]]
			}
			args, kw, block, err := s.sendArguments(irep, regs, int(in.Operands[0]), in.Operands[2], true)
			if err != nil {
				return Nil, err
			}
			if s.privateMethodCount > 0 || s.protectedCount > 0 || s.classPrivateCount > 0 {
				if err := s.privateCallError(recv, self, irep.Syms[in.Operands[1]]); err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
			}
			if line := lineAt[pc]; line != 0 {
				s.frames[len(s.frames)-1].line = int(line)
			}
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], args, kw, block)
			s.releaseValues(args)
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[in.Operands[0]] = result

		case bytecode.OpARGARY:
			a := int(in.Operands[0])
			spec := in.Operands[1]
			m1 := int((spec >> 11) & 0x3f)
			hasRest := (spec>>10)&1 != 0
			m2 := int((spec >> 5) & 0x1f)
			hasKeyword := (spec>>4)&1 != 0
			level := int(spec & 0x0f)
			env := frameEnv
			for i := 0; i < level; i++ {
				if env == nil {
					return Nil, fmt.Errorf("mrbvm: broken ARGARY environment")
				}
				env = env.parent
			}
			if env == nil {
				return Nil, fmt.Errorf("mrbvm: broken ARGARY environment")
			}
			source := env.regs
			values := make([]Value, 0, m1+m2)
			for i := 0; i < m1; i++ {
				values = append(values, source[1+i])
			}
			if hasRest {
				if rest, ok := source[1+m1].obj.data.(*arrayData); ok {
					values = append(values, rest.values...)
				}
			}
			base := 1 + m1
			if hasRest {
				base++
			}
			for i := 0; i < m2; i++ {
				values = append(values, source[base+i])
			}
			regs[a] = ArrayValue(values)
			keywordIndex := base + m2
			if keywordIndex < len(source) {
				regs[a+1] = source[keywordIndex]
			}
			if hasKeyword && keywordIndex+1 < len(source) {
				regs[a+2] = source[keywordIndex+1]
			}

		case bytecode.OpSUPER:
			a := int(in.Operands[0])
			argInfo := in.Operands[1]
			if owner == nil {
				sig := &unwindSignal{kind: unwindException, value: newException(s.NoMethodErrorClass, "super called outside of method")}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			var m *method
			var ok bool
			if _, isClassMethod := owner.classMethods[mid]; isClassMethod {
				if owner.super != nil {
					m, ok = owner.super.lookupClassMethod(mid)
				}
				if !ok {
					m, ok = s.classOf(self).lookup(mid)
				}
			} else {
				chain := classAncestors(s.classOf(self))
				start := 0
				for i, ancestor := range chain {
					if ancestor == owner {
						start = i + 1
						break
					}
				}
				for i := start; i < len(chain); i++ {
					if candidate, found := chain[i].methods[mid]; found {
						m, ok = candidate, true
						break
					}
				}
			}
			if !ok {
				missingValue, missingErr := s.invokeMethodMissing(ctx, self, mid, args, kw, block)
				if missingErr == nil {
					regs[in.Operands[0]] = missingValue
					continue
				}
				sig, _ := missingErr.(*unwindSignal)
				if sig == nil {
					return Nil, missingErr
				}
				np, rv, done, fatal := handle(sig, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			args, kw, block, err := s.sendArguments(irep, regs, a, argInfo, true)
			if err != nil {
				return Nil, err
			}
			if line := lineAt[pc]; line != 0 {
				s.frames[len(s.frames)-1].line = int(line)
			}
			result, err := s.invoke(ctx, m, self, args, kw, block)
			s.releaseValues(args)
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = result

		case bytecode.OpSEND0, bytecode.OpSSEND0:
			recv := self
			if in.Op == bytecode.OpSEND0 {
				recv = regs[in.Operands[0]]
			}
			if s.privateMethodCount > 0 || s.protectedCount > 0 || s.classPrivateCount > 0 {
				if err := s.privateCallError(recv, self, irep.Syms[in.Operands[1]]); err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
			}
			if line := lineAt[pc]; line != 0 {
				s.frames[len(s.frames)-1].line = int(line)
			}
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], nil, Nil, Nil)
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[in.Operands[0]] = result

		case bytecode.OpSEND, bytecode.OpSSEND:
			a := int(in.Operands[0])
			recv := self
			if in.Op == bytecode.OpSEND {
				recv = regs[a]
			}
			// Operators on built-in values are the hottest sends. Evaluate
			// them directly instead of marshalling a one-element argument
			// slice; fall through for user-defined behaviour or errors.
			if op := binaryOps[pc]; op != binNone && s.privateMethodCount == 0 && !s.bopRedefined {
				right := regs[a+1]
				if recv.Kind() == KindInt && right.Kind() == KindInt {
					if result, ok := intBinaryOp(recv.Int(), right.Int(), op); ok {
						regs[a] = result
						continue
					}
				}
				if result, ok := binaryValueOp(recv, op, right); ok {
					regs[a] = result
					continue
				}
			}
			var args []Value
			kw := Nil
			block := Nil
			var err error
			switch in.Operands[2] {
			case 1:
				args = s.acquireValues(1)
				args[0] = regs[a+1]
			case 2:
				args = s.acquireValues(2)
				copy(args, regs[a+1:a+3])
			default:
				args, kw, block, err = s.sendArguments(irep, regs, a, in.Operands[2], false)
				if err != nil {
					return Nil, err
				}
			}
			if s.privateMethodCount > 0 || s.protectedCount > 0 || s.classPrivateCount > 0 {
				if err := s.privateCallError(recv, self, irep.Syms[in.Operands[1]]); err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
			}
			if line := lineAt[pc]; line != 0 {
				s.frames[len(s.frames)-1].line = int(line)
			}
			result, err := s.call(ctx, recv, irep.Syms[in.Operands[1]], args, kw, block)
			s.releaseValues(args)
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[in.Operands[0]] = result

		case bytecode.OpJMP:
			target := jumpAt[pc]
			if target < 0 {
				return Nil, fmt.Errorf("mrbvm: %s has an invalid jump target", in.Op)
			}
			pc = int(target) - 1
		case bytecode.OpJMPUW:
			target := int(jumpAt[pc])
			if target < 0 {
				return Nil, fmt.Errorf("mrbvm: %s has an invalid jump target", in.Op)
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindJump, jumpTarget: target}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpJMPIF:
			if regs[in.Operands[0]].IsTruthy() {
				target := jumpAt[pc]
				if target < 0 {
					return Nil, fmt.Errorf("mrbvm: %s has an invalid jump target", in.Op)
				}
				pc = int(target) - 1
			}
		case bytecode.OpJMPNOT:
			if !regs[in.Operands[0]].IsTruthy() {
				target := jumpAt[pc]
				if target < 0 {
					return Nil, fmt.Errorf("mrbvm: %s has an invalid jump target", in.Op)
				}
				pc = int(target) - 1
			}
		case bytecode.OpJMPNIL:
			if regs[in.Operands[0]].IsNil() {
				target := jumpAt[pc]
				if target < 0 {
					return Nil, fmt.Errorf("mrbvm: %s has an invalid jump target", in.Op)
				}
				pc = int(target) - 1
			}

		case bytecode.OpADD, bytecode.OpSUB, bytecode.OpMUL, bytecode.OpDIV,
			bytecode.OpEQ, bytecode.OpLT, bytecode.OpLE, bytecode.OpGT, bytecode.OpGE:
			a := int(in.Operands[0])
			if s.bopRedefined {
				result, err := s.call(ctx, regs[a], bytecodeArithName(in.Op), []Value{regs[a+1]}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				regs[a] = result
				continue
			}
			left := regs[a]
			right := regs[a+1]
			needDispatch := false
			if _, isString := stringPayload(left); isString {
				if in.Op == bytecode.OpADD {
					if m, ok := s.classOf(left).lookup("+"); !ok || m != s.stringPlusBuiltin {
						needDispatch = true
					}
				} else if in.Op == bytecode.OpEQ {
					if m, ok := s.classOf(left).lookup("=="); ok && m != s.objectEqBuiltin {
						needDispatch = true
					}
				}
			}
			if needDispatch {
				result, err := s.call(ctx, left, bytecodeArithName(in.Op), []Value{right}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				regs[a] = result
				continue
			}
			if left.Kind() == KindInt && right.Kind() == KindInt {
				l, r := left.Int(), right.Int()
				switch in.Op {
				case bytecode.OpADD:
					if sum, ok := addInt64(l, r); ok {
						regs[a] = IntValue(sum)
						continue
					}
				case bytecode.OpSUB:
					if diff, ok := subInt64(l, r); ok {
						regs[a] = IntValue(diff)
						continue
					}
				case bytecode.OpMUL:
					if product, ok := mulInt64(l, r); ok {
						regs[a] = IntValue(product)
						continue
					}
				case bytecode.OpDIV:
					if r != 0 && !(l == math.MinInt64 && r == -1) {
						regs[a] = IntValue(floorDiv(l, r))
						continue
					}
				case bytecode.OpEQ:
					regs[a] = BoolValue(l == r)
					continue
				case bytecode.OpLT:
					regs[a] = BoolValue(l < r)
					continue
				case bytecode.OpLE:
					regs[a] = BoolValue(l <= r)
					continue
				case bytecode.OpGT:
					regs[a] = BoolValue(l > r)
					continue
				case bytecode.OpGE:
					regs[a] = BoolValue(l >= r)
					continue
				}
			}
			value, ok := binaryValueOp(left, bytecodeArithOp(in.Op), right)
			if !ok {
				result, err := s.call(ctx, left, bytecodeArithName(in.Op), []Value{regs[in.Operands[0]+1]}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				value = result
			}
			regs[in.Operands[0]] = value
		case bytecode.OpADDI, bytecode.OpSUBI:
			a := int(in.Operands[0])
			name := "+"
			if in.Op == bytecode.OpSUBI {
				name = "-"
			}
			if s.bopRedefined {
				result, err := s.call(ctx, regs[a], name, []Value{IntValue(int64(in.Operands[1]))}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				regs[a] = result
				continue
			}
			if left := regs[a]; left.Kind() == KindInt {
				immediate := int64(in.Operands[1])
				if in.Op == bytecode.OpADDI {
					if sum, ok := addInt64(left.Int(), immediate); ok {
						regs[a] = IntValue(sum)
						continue
					}
				} else {
					if diff, ok := subInt64(left.Int(), immediate); ok {
						regs[a] = IntValue(diff)
						continue
					}
				}
			}
			left := regs[a]
			value, ok := binaryValue(left, name, IntValue(int64(in.Operands[1])))
			if !ok {
				result, err := s.call(ctx, left, name, []Value{IntValue(int64(in.Operands[1]))}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				value = result
			}
			regs[in.Operands[0]] = value
		case bytecode.OpADDILV, bytecode.OpSUBILV:
			a := int(in.Operands[0])
			name := "+"
			if in.Op == bytecode.OpSUBILV {
				name = "-"
			}
			if s.bopRedefined {
				result, err := s.call(ctx, regs[a], name, []Value{IntValue(int64(in.Operands[2]))}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				regs[a] = result
				continue
			}
			if left := regs[a]; left.Kind() == KindInt {
				immediate := int64(in.Operands[2])
				if in.Op == bytecode.OpADDILV {
					if sum, ok := addInt64(left.Int(), immediate); ok {
						regs[a] = IntValue(sum)
						continue
					}
				} else {
					if diff, ok := subInt64(left.Int(), immediate); ok {
						regs[a] = IntValue(diff)
						continue
					}
				}
			}
			left := regs[a]
			value, ok := binaryValue(left, name, IntValue(int64(in.Operands[2])))
			if !ok {
				result, err := s.call(ctx, left, name, []Value{IntValue(int64(in.Operands[2]))}, Nil, Nil)
				if err != nil {
					np, rv, done, fatal := dispatch(err, in.Offset)
					if fatal != nil {
						return Nil, fatal
					}
					if done {
						return rv, nil
					}
					pc = np - 1
					continue
				}
				value = result
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
			if err := s.validateRangeBound(regs[a], regs[a+1]); err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
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
			value, err := s.indexOp(ctx, regs[a], regs[a+1])
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = value
		case bytecode.OpARYPUSH:
			a := int(in.Operands[0])
			n := int(in.Operands[1])
			data, ok := regs[a].obj.data.(*arrayData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: ARYPUSH receiver is not an array")
			}
			if err := s.checkArraySize(len(data.values) + n); err != nil {
				return Nil, err
			}
			for i := 0; i < n; i++ {
				data.values = append(data.values, regs[a+1+i])
			}
		case bytecode.OpARYCAT:
			a := int(in.Operands[0])
			if regs[a].IsNil() {
				regs[a] = ArrayValue(nil)
			}
			dst, ok := regs[a].obj.data.(*arrayData)
			if !ok {
				return Nil, fmt.Errorf("mrbvm: ARYCAT receiver is not an array")
			}
			srcValue := regs[a+1]
			if srcValue.IsNil() {
				break
			}
			if srcValue.obj != nil {
				if src, ok := srcValue.obj.data.(*arrayData); ok {
					dst.values = append(dst.values, src.values...)
					break
				}
				if _, ok := s.classOf(srcValue).lookup("to_a"); ok {
					converted, err := s.call(ctx, srcValue, "to_a", nil, Nil, Nil)
					if err != nil {
						np, rv, done, fatal := dispatch(err, in.Offset)
						if fatal != nil {
							return Nil, fatal
						}
						if done {
							return rv, nil
						}
						pc = np - 1
						continue
					}
					if converted.obj != nil {
						if src, ok := converted.obj.data.(*arrayData); ok {
							dst.values = append(dst.values, src.values...)
							break
						}
					}
				}
			}
			dst.values = append(dst.values, srcValue)
		case bytecode.OpARYSPLAT:
			a := int(in.Operands[0])
			if regs[a].obj != nil {
				if data, ok := regs[a].obj.data.(*arrayData); ok {
					values := make([]Value, len(data.values))
					copy(values, data.values)
					regs[a] = ArrayValue(values)
					break
				}
			}
			if regs[a].IsNil() {
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

		case bytecode.OpSETIDX:
			a := int(in.Operands[0])
			if _, err := s.setIndexOp(ctx, regs[a], regs[a+1], regs[a+2]); err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}

		case bytecode.OpGETIDX0:
			a := int(in.Operands[0])
			b := int(in.Operands[1])
			value, err := s.indexOp(ctx, regs[b], IntValue(0))
			if err != nil {
				np, rv, done, fatal := dispatch(err, in.Offset)
				if fatal != nil {
					return Nil, fatal
				}
				if done {
					return rv, nil
				}
				pc = np - 1
				continue
			}
			regs[a] = value

		case bytecode.OpRETURN:
			if ensureAt(in.Offset, -1) == nil {
				return regs[in.Operands[0]], nil
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: regs[in.Operands[0]], target: frameEnv}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpRETNIL:
			if ensureAt(in.Offset, -1) == nil {
				return Nil, nil
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: Nil, target: frameEnv}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpRETSELF:
			if ensureAt(in.Offset, -1) == nil {
				return self, nil
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: self, target: frameEnv}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpRETTRUE:
			if ensureAt(in.Offset, -1) == nil {
				return True, nil
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: True, target: frameEnv}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1
		case bytecode.OpRETFALSE:
			if ensureAt(in.Offset, -1) == nil {
				return False, nil
			}
			np, rv, done, fatal := handle(&unwindSignal{kind: unwindReturn, value: False, target: frameEnv}, in.Offset)
			if fatal != nil {
				return Nil, fatal
			}
			if done {
				return rv, nil
			}
			pc = np - 1

		case bytecode.OpSTOP:
			return Nil, nil

		default:
			return Nil, fmt.Errorf("mrbvm: opcode %s is not implemented yet", in.Op)
		}
	}
	return Nil, nil
}

// resolveUpvar maps an upvar level to an environment. mruby counts
// environments above the current proc, and a block's proc shares the stack of
// the scope where it was defined, so the search starts at the captured
// environment. Official mrbc emits one extra level for implicit super
// arguments inside a lambda; the caller compensates for that instruction.
func resolveUpvar(frameEnv *environment, level int, adjust bool) *environment {
	env := frameEnv
	if env != nil {
		env = env.parent
	}
	if adjust && level > 0 {
		level--
	}
	for i := 0; i < level; i++ {
		if env == nil {
			return nil
		}
		env = env.parent
	}
	return env
}

func nearestMethodEnv(env *environment) *environment {
	for current := env; current != nil; current = current.parent {
		if current.methodFrame {
			return current
		}
	}
	return nil
}

func classVarTarget(class *Class) *Class {
	for class != nil && class.singletonOf != nil {
		class = class.singletonOf
	}
	return class
}

func classVarOwner(class *Class, name string) *Class {
	if class == nil {
		return nil
	}
	for _, current := range classAncestors(class) {
		if current == class {
			continue
		}
		if _, ok := current.classVars[name]; ok {
			return current
		}
	}
	if _, ok := class.classVars[name]; ok {
		return class
	}
	return nil
}

func lookupClassVar(class *Class, name string) Value {
	if owner := classVarOwner(class, name); owner != nil {
		return owner.classVars[name]
	}
	return Nil
}

func (s *State) setClassVar(class *Class, name string, value Value) error {
	owner := classVarOwner(class, name)
	if owner == nil {
		owner = class
	}
	if err := s.ensureMutable(s.classValue(owner)); err != nil {
		return err
	}
	owner.classVars[name] = value
	return nil
}

func lookupLexicalConstant(cref *Class, name string) (Value, bool) {
	for current := cref; current != nil; current = current.lexicalOuter {
		if value, ok := current.constants[name]; ok {
			return value, true
		}
	}
	for _, current := range classAncestors(cref) {
		if value, ok := current.constants[name]; ok {
			return value, true
		}
	}
	return Nil, false
}

func (s *State) lookupConstant(class *Class, name string) Value {
	for _, current := range classAncestors(class) {
		if value, ok := current.constants[name]; ok {
			return value
		}
	}
	return Nil
}

const freeSliceLimit = 64

// acquireSlice returns a zeroed slice of length n from the free list. Free
// lists avoid sync.Pool because storing a slice in an interface boxes its
// header, which allocates on every release.
func (s *State) acquireSlice(free *[][]Value, n int) []Value {
	if n <= 0 {
		return nil
	}
	if stack := *free; len(stack) > 0 {
		buffer := stack[len(stack)-1]
		*free = stack[:len(stack)-1]
		if cap(buffer) >= n {
			buffer = buffer[:n]
			clear(buffer)
			return buffer
		}
	}
	return make([]Value, n)
}

func (s *State) releaseSlice(free *[][]Value, values []Value) {
	if cap(values) == 0 || len(*free) >= freeSliceLimit {
		return
	}
	clear(values)
	*free = append(*free, values[:0])
}

func (s *State) acquireEnvironment() *environment {
	if n := len(s.envFree); n > 0 {
		env := s.envFree[n-1]
		s.envFree = s.envFree[:n-1]
		return env
	}
	return &environment{}
}

func (s *State) releaseEnvironment(env *environment) {
	if len(s.envFree) >= freeSliceLimit {
		return
	}
	env.parent = nil
	env.regs = nil
	env.block = Nil
	env.methodFrame = false
	s.envFree = append(s.envFree, env)
}

// acquireValues reuses an argument buffer without zeroing it: every caller
// overwrites the first n elements before reading.
func (s *State) acquireValues(n int) []Value {
	if n <= 0 {
		return nil
	}
	if stack := s.argFree; len(stack) > 0 {
		buffer := stack[len(stack)-1]
		s.argFree = stack[:len(stack)-1]
		if cap(buffer) >= n {
			return buffer[:n]
		}
	}
	return make([]Value, n)
}

func (s *State) releaseValues(values []Value) {
	if cap(values) == 0 || len(s.argFree) >= freeSliceLimit {
		return
	}
	s.argFree = append(s.argFree, values[:0])
}

func (s *State) sendArguments(irep *Irep, regs []Value, a int, argInfo uint32, hasBlock bool) ([]Value, Value, Value, error) {
	n := int(argInfo & 0x0f)
	k := int((argInfo >> 4) & 0x0f)
	var args []Value
	positionalRegs := n
	if n == 15 {
		positionalRegs = 1
		array := regs[a+1]
		data, ok := array.obj.data.(*arrayData)
		if !ok {
			return nil, Nil, Nil, fmt.Errorf("mrbvm: packed arguments are not an array")
		}
		args = s.acquireValues(len(data.values))
		copy(args, data.values)
	} else {
		args = s.acquireValues(n)
		copy(args, regs[a+1:a+1+n])
	}
	kwRegs := 0
	kw := Nil
	if k == 15 {
		kwRegs = 1
		kw = regs[a+1+positionalRegs]
	} else if k > 0 {
		kwRegs = 2 * k
		entries := make([]hashEntry, k)
		for i := 0; i < k; i++ {
			entries[i] = hashEntry{
				key:   regs[a+1+positionalRegs+2*i],
				value: regs[a+1+positionalRegs+2*i+1],
			}
		}
		kw = HashValue(entries)
	}
	block := Nil
	if hasBlock {
		block = regs[a+1+positionalRegs+kwRegs]
	}
	return args, kw, block, nil
}

func (s *State) call(ctx context.Context, recv Value, name string, args []Value, kw Value, block Value) (Value, error) {
	// A Ruby redefinition of an operator wins over the native fast paths
	// below. This is what makes send(:+), aliases, and method_missing agree
	// with normal method dispatch.
	if s.bopRedefined && builtinOperatorNames[name] {
		if m, ok := s.classOf(recv).lookup(name); ok && !m.builtin {
			return s.invoke(ctx, m, recv, args, kw, block)
		}
	}
	if len(args) == 0 {
		switch name {
		case "nil?":
			return BoolValue(recv.IsNil()), nil
		case "!":
			return BoolValue(!recv.IsTruthy()), nil
		case "-@":
			switch recv.Kind() {
			case KindInt:
				value := recv.Int()
				if value == math.MinInt64 {
					return normalizedBigInt(new(big.Int).Neg(big.NewInt(value))), nil
				}
				return IntValue(-value), nil
			case KindBigInt:
				value, _ := recv.BigInt()
				return normalizedBigInt(new(big.Int).Neg(value)), nil
			case KindFloat:
				return FloatValue(-recv.Float()), nil
			}
		case "~":
			switch recv.Kind() {
			case KindInt:
				return IntValue(^recv.Int()), nil
			case KindBigInt:
				value, _ := recv.BigInt()
				return normalizedBigInt(new(big.Int).Not(value)), nil
			}
		}
	}
	if class, ok := s.asClass(recv); ok {
		if m, ok := class.lookupClassMethod(name); ok {
			return s.invoke(ctx, m, recv, args, kw, block)
		}
	}
	if (name == "/" || name == "%") && len(args) == 1 && args[0].Kind() == KindInt && args[0].Int() == 0 &&
		(recv.Kind() == KindInt || recv.Kind() == KindBigInt) {
		return Nil, &unwindSignal{kind: unwindException, value: newException(s.ZeroDivisionErrorClass, "divided by 0")}
	}
	if name == "<=>" && len(args) == 1 {
		if cmp, ok := compareNative(recv, args[0]); ok {
			return IntValue(int64(cmp)), nil
		}
	}
	if name == "===" && len(args) == 1 {
		if m, ok := s.classOf(recv).lookup(name); ok {
			return s.invoke(ctx, m, recv, args, kw, block)
		}
		return BoolValue(valuesEqual(recv, args[0])), nil
	}
	skipBinary := false
	if _, isString := stringPayload(recv); isString {
		switch name {
		case "+":
			if m, ok := s.classOf(recv).lookup("+"); !ok || m != s.stringPlusBuiltin {
				skipBinary = true
			}
		case "==":
			if m, ok := s.classOf(recv).lookup("=="); ok && m != s.objectEqBuiltin {
				skipBinary = true
			}
		}
	}
	if !skipBinary {
		if result, ok := binaryCall(recv, name, args); ok {
			return result, nil
		}
	}
	if len(args) == 1 && (recv.Kind() == KindInt || recv.Kind() == KindFloat || recv.Kind() == KindBigInt) {
		switch name {
		case "+", "-", "*", "/", "%", "**", "<<", ">>", "&", "|", "^":
			other := args[0]
			if other.Kind() != KindInt && other.Kind() != KindFloat && other.Kind() != KindBigInt {
				return Nil, &unwindSignal{kind: unwindException, value: newException(s.TypeErrorClass,
					"no implicit conversion of "+s.classOf(other).name+" into Integer")}
			}
		}
	}
	m, ok := s.classOf(recv).lookup(name)
	if !ok {
		return s.invokeMethodMissing(ctx, recv, name, args, kw, block)
	}
	return s.invoke(ctx, m, recv, args, kw, block)
}

func (s *State) invoke(ctx context.Context, m *method, recv Value, args []Value, kw Value, block Value) (Value, error) {
	if m.native != nil {
		// Callers may pass a pooled buffer that is reused once the call
		// returns. Native methods must copy the slice if they retain it; see
		// kernelP and DefineFunction.
		return m.native(ctx, recv, args, kw, block)
	}
	methodCref := m.cref
	if methodCref == nil {
		methodCref = m.owner
	}
	return s.executeFrame(ctx, m.irep, recv, m.owner, args, kw, block, nil, m.name, m.owner, methodCref, true)
}

type binaryOp uint8

const (
	binNone binaryOp = iota
	binEq
	binLt
	binLe
	binGt
	binGe
	binAdd
	binSub
	binMul
	binDiv
	binMod
	binPow
	binShl
	binShr
	binAnd
	binOr
	binXor
)

var binaryOpsByName = map[string]binaryOp{
	"==": binEq,
	"<":  binLt,
	"<=": binLe,
	">":  binGt,
	">=": binGe,
	"+":  binAdd,
	"-":  binSub,
	"*":  binMul,
	"/":  binDiv,
	"%":  binMod,
	"**": binPow,
	"<<": binShl,
	">>": binShr,
	"&":  binAnd,
	"|":  binOr,
	"^":  binXor,
}

func binaryOpForName(name string) binaryOp {
	return binaryOpsByName[name]
}

func binaryCall(recv Value, name string, args []Value) (Value, bool) {
	if len(args) != 1 {
		return Nil, false
	}
	return binaryValue(recv, name, args[0])
}

func addInt64(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

func subInt64(a, b int64) (int64, bool) {
	diff := a - b
	if (b < 0 && diff < a) || (b > 0 && diff > a) {
		return 0, false
	}
	return diff, true
}

func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, false
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
}

// intBinaryOp evaluates the common integer/integer operators directly.
// Unsigned shifts and exponentiation fall back to the general path because
// they can produce big integers.
func intBinaryOp(left, right int64, op binaryOp) (Value, bool) {
	switch op {
	case binAdd:
		if sum, ok := addInt64(left, right); ok {
			return IntValue(sum), true
		}
		return Nil, false
	case binSub:
		if diff, ok := subInt64(left, right); ok {
			return IntValue(diff), true
		}
		return Nil, false
	case binMul:
		if product, ok := mulInt64(left, right); ok {
			return IntValue(product), true
		}
		return Nil, false
	case binDiv:
		if right == 0 || (left == math.MinInt64 && right == -1) {
			return Nil, false
		}
		return IntValue(floorDiv(left, right)), true
	case binMod:
		if right == 0 {
			return Nil, false
		}
		rem := left % right
		if rem != 0 && (rem < 0) != (right < 0) {
			rem += right
		}
		return IntValue(rem), true
	case binEq:
		return BoolValue(left == right), true
	case binLt:
		return BoolValue(left < right), true
	case binLe:
		return BoolValue(left <= right), true
	case binGt:
		return BoolValue(left > right), true
	case binGe:
		return BoolValue(left >= right), true
	}
	return Nil, false
}

func bytecodeArithName(op bytecode.Opcode) string {
	switch op {
	case bytecode.OpADD:
		return "+"
	case bytecode.OpSUB:
		return "-"
	case bytecode.OpMUL:
		return "*"
	case bytecode.OpDIV:
		return "/"
	case bytecode.OpEQ:
		return "=="
	case bytecode.OpLT:
		return "<"
	case bytecode.OpLE:
		return "<="
	case bytecode.OpGT:
		return ">"
	case bytecode.OpGE:
		return ">="
	}
	return ""
}

func bytecodeArithOp(op bytecode.Opcode) binaryOp {
	switch op {
	case bytecode.OpADD:
		return binAdd
	case bytecode.OpSUB:
		return binSub
	case bytecode.OpMUL:
		return binMul
	case bytecode.OpDIV:
		return binDiv
	case bytecode.OpEQ:
		return binEq
	case bytecode.OpLT:
		return binLt
	case bytecode.OpLE:
		return binLe
	case bytecode.OpGT:
		return binGt
	case bytecode.OpGE:
		return binGe
	}
	return binNone
}

func binaryValue(left Value, name string, right Value) (Value, bool) {
	return binaryValueOp(left, binaryOpForName(name), right)
}

func binaryValueOp(left Value, op binaryOp, right Value) (Value, bool) {
	switch op {
	case binEq:
		if builtinComparable(left) && builtinComparable(right) {
			return BoolValue(valuesEqual(left, right)), true
		}
		return Nil, false
	case binLt, binLe, binGt, binGe:
		if left.Kind() == KindFloat && math.IsNaN(left.Float()) {
			return False, true
		}
		if right.Kind() == KindFloat && math.IsNaN(right.Float()) {
			return False, true
		}
		var cmp int
		if li, ri, ok := bothBigInts(left, right); ok {
			cmp = li.Cmp(ri)
		} else if li, ri, ok := bothInts(left, right); ok {
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
		} else if (left.Kind() == KindString || left.Kind() == KindSymbol) &&
			(right.Kind() == KindString || right.Kind() == KindSymbol) {
			cmp = strings.Compare(left.String(), right.String())
		} else {
			return Nil, false
		}
		switch op {
		case binLt:
			return BoolValue(cmp < 0), true
		case binLe:
			return BoolValue(cmp <= 0), true
		case binGt:
			return BoolValue(cmp > 0), true
		case binGe:
			return BoolValue(cmp >= 0), true
		}
	case binAdd:
		if left.Kind() == KindString && right.Kind() == KindString {
			return StringValue(left.String() + right.String()), true
		}
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).Add(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if sum, ok := addInt64(li, ri); ok {
				return IntValue(sum), true
			}
			return normalizedBigInt(new(big.Int).Add(big.NewInt(li), big.NewInt(ri))), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf + rf), true
		}
	case binSub:
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).Sub(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if diff, ok := subInt64(li, ri); ok {
				return IntValue(diff), true
			}
			return normalizedBigInt(new(big.Int).Sub(big.NewInt(li), big.NewInt(ri))), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf - rf), true
		}
	case binMul:
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).Mul(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if product, ok := mulInt64(li, ri); ok {
				return IntValue(product), true
			}
			return normalizedBigInt(new(big.Int).Mul(big.NewInt(li), big.NewInt(ri))), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf * rf), true
		}
	case binMod:
		if li, ri, ok := bothBigInts(left, right); ok {
			if ri.Sign() == 0 {
				return Nil, false
			}
			rem := new(big.Int).Rem(li, ri)
			if rem.Sign() != 0 && rem.Sign() != ri.Sign() {
				rem.Add(rem, ri)
			}
			return normalizedBigInt(rem), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if ri == 0 {
				return Nil, false
			}
			rem := li % ri
			if rem != 0 && (rem < 0) != (ri < 0) {
				rem += ri
			}
			return IntValue(rem), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			if rf == 0 {
				return Nil, false
			}
			rem := math.Mod(lf, rf)
			if rem != 0 && (rem < 0) != (rf < 0) {
				rem += rf
			}
			return FloatValue(rem), true
		}
	case binAnd:
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).And(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li & ri), true
		}
	case binOr:
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).Or(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li | ri), true
		}
	case binXor:
		if li, ri, ok := bothBigInts(left, right); ok {
			return normalizedBigInt(new(big.Int).Xor(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			return IntValue(li ^ ri), true
		}
	case binShl:
		if li, ok := integerBig(left); ok {
			if ri, ok := integerBig(right); ok && ri.Sign() >= 0 && ri.IsInt64() {
				return normalizedBigInt(new(big.Int).Lsh(li, uint(ri.Int64()))), true
			}
		}
	case binShr:
		if li, ok := integerBig(left); ok {
			if ri, ok := integerBig(right); ok && ri.Sign() >= 0 && ri.IsInt64() {
				return normalizedBigInt(new(big.Int).Rsh(li, uint(ri.Int64()))), true
			}
		}
	case binPow:
		if li, ri, ok := bothBigInts(left, right); ok {
			if ri.Sign() < 0 {
				return Nil, false
			}
			return normalizedBigInt(new(big.Int).Exp(li, ri, nil)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if ri < 0 {
				return FloatValue(math.Pow(float64(li), float64(ri))), true
			}
			return normalizedBigInt(new(big.Int).Exp(big.NewInt(li), big.NewInt(ri), nil)), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(math.Pow(lf, rf)), true
		}
	case binDiv:
		if li, ri, ok := bothBigInts(left, right); ok {
			if ri.Sign() == 0 {
				return Nil, false
			}
			return normalizedBigInt(new(big.Int).Quo(li, ri)), true
		}
		if li, ri, ok := bothInts(left, right); ok {
			if ri == 0 {
				return Nil, false
			}
			if li == math.MinInt64 && ri == -1 {
				return normalizedBigInt(new(big.Int).Quo(big.NewInt(li), big.NewInt(ri))), true
			}
			return IntValue(floorDiv(li, ri)), true
		}
		if lf, rf, ok := bothFloats(left, right); ok {
			return FloatValue(lf / rf), true
		}
	}
	return Nil, false
}

func builtinComparable(v Value) bool {
	if v.Kind() != KindObject {
		return true
	}
	if v.obj == nil {
		return true
	}
	switch v.obj.data.(type) {
	case *rangeData, *stringData:
		return true
	}
	return false
}

func integerBig(v Value) (*big.Int, bool) {
	switch v.Kind() {
	case KindInt:
		return big.NewInt(v.Int()), true
	case KindBigInt:
		return v.BigInt()
	}
	return nil, false
}

func bothBigInts(left, right Value) (*big.Int, *big.Int, bool) {
	if left.Kind() != KindBigInt && right.Kind() != KindBigInt {
		return nil, nil, false
	}
	li, ok1 := integerBig(left)
	ri, ok2 := integerBig(right)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	return li, ri, true
}

func normalizedBigInt(value *big.Int) Value {
	if value.IsInt64() {
		return IntValue(value.Int64())
	}
	return BigIntValue(value)
}

func compareNative(left, right Value) (int, bool) {
	if li, ri, ok := bothBigInts(left, right); ok {
		return li.Cmp(ri), true
	}
	if li, ri, ok := bothInts(left, right); ok {
		switch {
		case li < ri:
			return -1, true
		case li > ri:
			return 1, true
		default:
			return 0, true
		}
	}
	if lf, rf, ok := bothFloats(left, right); ok {
		if math.IsNaN(lf) || math.IsNaN(rf) {
			return 0, false
		}
		switch {
		case lf < rf:
			return -1, true
		case lf > rf:
			return 1, true
		default:
			return 0, true
		}
	}
	if (left.Kind() == KindString || left.Kind() == KindSymbol) && (right.Kind() == KindString || right.Kind() == KindSymbol) {
		return strings.Compare(left.String(), right.String()), true
	}
	return 0, false
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

type objectPair struct {
	left  *Object
	right *Object
}

func valuesEqual(left, right Value) bool {
	return valuesEqualSeen(left, right, make(map[objectPair]bool))
}

func stringPayload(v Value) (string, bool) {
	if v.obj == nil {
		return "", false
	}
	switch v.Kind() {
	case KindString, KindSymbol, KindObject:
		if data, ok := v.obj.data.(*stringData); ok {
			return data.str(), true
		}
	}
	return "", false
}

func valuesEqualSeen(left, right Value, seen map[objectPair]bool) bool {
	if leftString, leftOK := stringPayload(left); leftOK {
		if rightString, rightOK := stringPayload(right); rightOK {
			if (left.Kind() == KindSymbol) != (right.Kind() == KindSymbol) {
				return false
			}
			return leftString == rightString
		}
	}
	if left.Kind() != right.Kind() {
		if li, ri, ok := bothBigInts(left, right); ok {
			return li.Cmp(ri) == 0
		}
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
	case KindBigInt:
		li, _ := left.BigInt()
		ri, _ := right.BigInt()
		return li.Cmp(ri) == 0
	case KindFloat:
		return left.Float() == right.Float()
	case KindString, KindSymbol:
		return left.String() == right.String()
	case KindObject:
		if left.obj == nil || right.obj == nil {
			return left.obj == right.obj
		}
		pair := objectPair{left: left.obj, right: right.obj}
		if seen[pair] {
			return true
		}
		switch ldata := left.obj.data.(type) {
		case *arrayData:
			rdata, ok := right.obj.data.(*arrayData)
			if !ok || len(ldata.values) != len(rdata.values) {
				return false
			}
			seen[pair] = true
			defer delete(seen, pair)
			for i := range ldata.values {
				if !valuesEqualSeen(ldata.values[i], rdata.values[i], seen) {
					return false
				}
			}
			return true
		case *hashData:
			rdata, ok := right.obj.data.(*hashData)
			if !ok || len(ldata.entries) != len(rdata.entries) {
				return false
			}
			seen[pair] = true
			defer delete(seen, pair)
			for _, entry := range ldata.entries {
				found := false
				for _, other := range rdata.entries {
					if valuesEqualSeen(entry.key, other.key, seen) && valuesEqualSeen(entry.value, other.value, seen) {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
			return true
		case *rangeData:
			rdata, ok := right.obj.data.(*rangeData)
			return ok && ldata.exclusive == rdata.exclusive &&
				valuesEqualSeen(ldata.low, rdata.low, seen) && valuesEqualSeen(ldata.high, rdata.high, seen)
		case *stringData:
			rdata, ok := right.obj.data.(*stringData)
			return ok && ldata.str() == rdata.str()
		default:
			return left.obj == right.obj
		}
	default:
		return false
	}
}

func constAssignedSoon(instructions []bytecode.Instruction, pc int, symbol uint32) bool {
	limit := pc + 8
	if limit > len(instructions) {
		limit = len(instructions)
	}
	for i := pc + 1; i < limit; i++ {
		if instructions[i].Op == bytecode.OpSETCONST && instructions[i].Operands[1] == symbol {
			return true
		}
	}
	return false
}

func definedNext(instructions []bytecode.Instruction, pc int, syms []string) bool {
	if pc+1 >= len(instructions) {
		return false
	}
	next := instructions[pc+1]
	switch next.Op {
	case bytecode.OpSEND, bytecode.OpSSEND, bytecode.OpSEND0, bytecode.OpSSEND0:
		index := int(next.Operands[1])
		return index >= 0 && index < len(syms) && syms[index] == "defined?"
	}
	return false
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
		return normalizedBigInt(value), nil
	default:
		return Nil, fmt.Errorf("mrbvm: unsupported pool entry kind %d", entry.Kind)
	}
}
