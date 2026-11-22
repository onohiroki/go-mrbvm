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
)

type limitWriter struct {
	writer    io.Writer
	remaining int
	exceeded  bool
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		w.exceeded = true
		return len(p), nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
		w.exceeded = true
	}
	n, err := w.writer.Write(p)
	w.remaining -= n
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

func (s *State) checkExecutionLimits() error {
	s.instructionCount++
	if s.maxInstructions > 0 && s.instructionCount > s.maxInstructions {
		return errInstructionLimit
	}
	if s.output != nil && s.output.exceeded {
		return errOutputLimit
	}
	if s.maxTime > 0 && s.instructionCount%1024 == 0 && time.Since(s.startTime) > s.maxTime {
		return errTimeLimit
	}
	return nil
}
