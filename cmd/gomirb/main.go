// Command gomirb is a small mirb-style REPL over go-mrbc and go-mrbvm.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func main() {
	version := flag.Bool("v", false, "print version")
	quiet := flag.Bool("q", false, "do not print expression results")
	preload := flag.String("r", "", "preload a Ruby file before the REPL")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gomirb [-v] [-q] [-r file]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println("gomirb 0.0.0 (go-mrbvm, mruby 4.0.0)")
		return
	}

	ctx := context.Background()
	state := mrbvm.New(mrbvm.Options{Stdout: os.Stdout, Stderr: os.Stderr})
	if *preload != "" {
		source, err := os.ReadFile(*preload)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if _, err := execute(ctx, state, source, false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	interactive := false
	if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		interactive = true
	}
	if err := runREPL(ctx, os.Stdin, os.Stdout, os.Stderr, state, interactive, *quiet); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runREPL(ctx context.Context, input io.Reader, output, errOutput io.Writer, state *mrbvm.State, interactive, quiet bool) error {
	if !interactive {
		source, err := io.ReadAll(input)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(source))) == 0 {
			return nil
		}
		result, err := execute(ctx, state, source, quiet)
		if err == nil && !quiet && !result.IsNil() {
			fmt.Fprintf(output, "=> %s\n", result.Inspect())
		}
		return err
	}

	scanner := bufio.NewScanner(input)
	var buffer strings.Builder
	prompt := func(continuation bool) {
		if continuation {
			fmt.Fprint(output, "... ")
		} else {
			fmt.Fprint(output, "gomirb> ")
		}
	}
	for {
		prompt(buffer.Len() > 0)
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if buffer.Len() == 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed == "exit" || trimmed == "quit" {
				break
			}
			if trimmed == "" {
				continue
			}
		}
		if buffer.Len() > 0 {
			buffer.WriteByte('\n')
		}
		buffer.WriteString(line)

		result, err := execute(ctx, state, []byte(buffer.String()), quiet)
		if err != nil && incompleteSource(err.Error()) {
			continue
		}
		if err != nil {
			fmt.Fprintln(errOutput, err)
		} else if !quiet && !result.IsNil() {
			fmt.Fprintf(output, "=> %s\n", result.Inspect())
		}
		buffer.Reset()
	}
	return scanner.Err()
}

func execute(ctx context.Context, state *mrbvm.State, source []byte, quiet bool) (mrbvm.Value, error) {
	irep, err := mrbc.Compile(ctx, source, mrbc.Options{FilePath: "(gomirb)", DebugInfo: true})
	if err != nil {
		return mrbvm.Nil, err
	}
	return state.Execute(ctx, irep)
}

func incompleteSource(message string) bool {
	for _, marker := range []string{
		"unexpected end-of-input",
		"unterminated",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
