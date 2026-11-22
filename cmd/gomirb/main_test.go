package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func TestRunREPLNonInteractive(t *testing.T) {
	var out, errOut bytes.Buffer
	state := mrbvm.New(mrbvm.Options{Stdout: &out, Stderr: &errOut})
	input := strings.NewReader("x = 1\nx + 2\n")
	if err := runREPL(context.Background(), input, &out, &errOut, state, false, false); err != nil {
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
	if err := runREPL(context.Background(), input, &out, &errOut, state, true, false); err != nil {
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
