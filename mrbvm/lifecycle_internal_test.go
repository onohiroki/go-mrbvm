package mrbvm

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestCloseClosesTrackedFiles(t *testing.T) {
	state := New(Options{Stdout: &bytes.Buffer{}})
	file, err := os.CreateTemp(t.TempDir(), "close-*")
	if err != nil {
		t.Fatal(err)
	}
	state.newFileValue(file, file.Name())
	state.Close()

	state.filesMu.Lock()
	remaining := len(state.openFiles)
	state.filesMu.Unlock()
	if remaining != 0 {
		t.Fatalf("openFiles = %d, want 0", remaining)
	}
	if _, err := file.WriteString("x"); err == nil {
		t.Fatal("file is still writable after Close")
	}
}

func TestRecordPanicMarksStateUnusable(t *testing.T) {
	state := New(Options{Stdout: &bytes.Buffer{}})
	defer state.Close()
	if err := state.recordPanic("boom"); err == nil {
		t.Fatal("recordPanic returned nil")
	}
	if _, err := state.Run(context.Background(), nil); err == nil {
		t.Fatal("Run succeeded after panic")
	}
}
