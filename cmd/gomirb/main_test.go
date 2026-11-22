package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestRunREPLNonInteractive(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	input := strings.NewReader("x = 1\nx + 2\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, "", false, false); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if got := out.String(); got != "=> 3\n" {
		t.Fatalf("out = %q, want %q", got, "=> 3\n")
	}
}

func TestRunREPLInteractivePersistsState(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	input := strings.NewReader("$x = 4\n$x * 2\nexit\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, "", true, false); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "=> 8\n") {
		t.Fatalf("out = %q, missing result", out.String())
	}
}

func TestIncompleteSource(t *testing.T) {
	if !incompleteSource("unexpected end-of-input") {
		t.Fatal("expected incomplete source")
	}
	if incompleteSource("syntax error") {
		t.Fatal("plain syntax error should not continue")
	}
}

func TestHistoryAndCompletion(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	historyPath := t.TempDir() + "/history"
	input := strings.NewReader("1 + 1\nhistory\ncomplete File\nexit\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, historyPath, true, false); err != nil {
		t.Fatalf("runREPL: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "1: 1 + 1") {
		t.Fatalf("out = %q, missing history", out.String())
	}
	if !strings.Contains(out.String(), "File") {
		t.Fatalf("out = %q, missing completion", out.String())
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "1 + 1") {
		t.Fatalf("history file = %q", data)
	}
}
