package mrbvm

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
)

// ErrStateClosed is returned when a State is used after Close.
var ErrStateClosed = errors.New("mrbvm: state is closed")

// ErrStatePanicked is returned after an internal panic has been recovered.
// The State must not be reused after this error.
var ErrStatePanicked = errors.New("mrbvm: state panicked")

func (s *State) recoverPanic(result *Value, err *error) {
	panicValue := recover()
	if panicValue == nil {
		return
	}
	*result = Nil
	*err = s.recordPanic(panicValue)
}

func (s *State) recordPanic(panicValue any) error {
	err := fmt.Errorf("%w: %v\n%s", ErrStatePanicked, panicValue, debug.Stack())
	s.panicMu.Lock()
	if s.panicErr == nil {
		s.panicErr = err
	}
	s.panicMu.Unlock()
	return err
}

func (s *State) statePanicErr() error {
	s.panicMu.Lock()
	err := s.panicErr
	s.panicMu.Unlock()
	return err
}

// Close stops the state's background fiber scheduler, waits for internal
// fiber and enumerator goroutines to exit, and closes files still open through
// the Ruby File API. It is idempotent.
//
// Close must not be called concurrently with Run or Execute. The supported
// lifecycle is: execute to completion, then Close.
func (s *State) Close() {
	if s == nil || s.closed == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.fiberStop)
	})
	s.closeOnce.Do(func() {
		close(s.closed)
	})
	s.wg.Wait()
	s.closeOpenFiles()
}

func (s *State) isClosed() bool {
	if s == nil || s.closed == nil {
		return false
	}
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *State) trackFile(file *os.File) {
	if file == nil {
		return
	}
	s.filesMu.Lock()
	if s.openFiles == nil {
		s.openFiles = make(map[*os.File]struct{})
	}
	s.openFiles[file] = struct{}{}
	s.filesMu.Unlock()
}

func (s *State) untrackFile(file *os.File) {
	if file == nil {
		return
	}
	s.filesMu.Lock()
	delete(s.openFiles, file)
	s.filesMu.Unlock()
}

func (s *State) closeOpenFiles() {
	s.filesMu.Lock()
	files := make([]*os.File, 0, len(s.openFiles))
	for file := range s.openFiles {
		files = append(files, file)
		delete(s.openFiles, file)
	}
	s.filesMu.Unlock()
	for _, file := range files {
		_ = file.Close()
	}
}
