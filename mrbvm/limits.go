package mrbvm

import (
	"errors"
	"io"
	"time"
)

var (
	errInstructionLimit = errors.New("mrbvm: instruction limit exceeded")
	errTimeLimit        = errors.New("mrbvm: time limit exceeded")
	errOutputLimit      = errors.New("mrbvm: output limit exceeded")
	errArrayLimit       = errors.New("mrbvm: array size limit exceeded")
	errHashLimit        = errors.New("mrbvm: hash size limit exceeded")
	errStringLimit      = errors.New("mrbvm: string size limit exceeded")
	errFiberLimit       = errors.New("mrbvm: fiber limit exceeded")
	errEnumeratorLimit  = errors.New("mrbvm: enumerator limit exceeded")
)

type outputBudget struct {
	remaining int
	exceeded  bool
}

type limitWriter struct {
	writer io.Writer
	budget *outputBudget
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.budget == nil {
		return w.writer.Write(p)
	}
	if w.budget.remaining <= 0 {
		w.budget.exceeded = true
		return len(p), nil
	}
	if len(p) > w.budget.remaining {
		p = p[:w.budget.remaining]
		w.budget.exceeded = true
	}
	n, err := w.writer.Write(p)
	w.budget.remaining -= n
	return n, err
}

func (s *State) checkArraySize(size int) error {
	if s.maxArraySize > 0 && size > s.maxArraySize {
		return errArrayLimit
	}
	return nil
}

func (s *State) checkHashSize(size int) error {
	if s.maxHashSize > 0 && size > s.maxHashSize {
		return errHashLimit
	}
	return nil
}

func (s *State) checkStringSize(size int) error {
	if s.maxStringSize > 0 && size > s.maxStringSize {
		return errStringLimit
	}
	return nil
}

func (s *State) checkedStringValue(text string) (Value, error) {
	if err := s.checkStringSize(len(text)); err != nil {
		return Nil, err
	}
	return StringValue(text), nil
}

func (s *State) reserveFiber() error {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	if s.maxFibers > 0 && s.activeFibers >= s.maxFibers {
		return errFiberLimit
	}
	s.activeFibers++
	return nil
}

func (s *State) releaseFiber() {
	s.resourceMu.Lock()
	if s.activeFibers > 0 {
		s.activeFibers--
	}
	s.resourceMu.Unlock()
}

func (s *State) reserveEnumerator() error {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	if s.maxEnumerators > 0 && s.activeEnumerators >= s.maxEnumerators {
		return errEnumeratorLimit
	}
	s.activeEnumerators++
	return nil
}

func (s *State) releaseEnumerator() {
	s.resourceMu.Lock()
	if s.activeEnumerators > 0 {
		s.activeEnumerators--
	}
	s.resourceMu.Unlock()
}

func (s *State) checkExecutionLimits() error {
	s.instructionCount++
	if s.maxInstructions > 0 && s.instructionCount > s.maxInstructions {
		return errInstructionLimit
	}
	if s.outputBudget != nil && s.outputBudget.exceeded {
		return errOutputLimit
	}
	if s.maxTime > 0 && s.instructionCount%1024 == 0 && time.Since(s.startTime) > s.maxTime {
		return errTimeLimit
	}
	return nil
}

// resetLimits starts a fresh per-execution budget. Instruction and wall-clock
// limits are scoped to one Execute/Run call, not to the lifetime of a State.
func (s *State) resetLimits() {
	s.instructionCount = 0
	s.startTime = time.Now()
	if s.outputBudget != nil {
		s.outputBudget.remaining = s.maxOutputBytes
		s.outputBudget.exceeded = false
	}
}
