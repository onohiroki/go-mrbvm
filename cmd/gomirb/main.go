// Command gomirb is a small mirb-style REPL over go-mrbc and go-mrbvm.
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/internal/cli"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

var optionSpecs = []cli.Option{
	{Short: 'd'},
	{Short: 'r', HasArg: true},
	{Short: 'v'},
	{Short: 'q'},
	{Short: 'H', HasArg: true},
	{Short: 'h'},
	{Long: "help"},
	{Long: "verbose"},
	{Long: "version"},
	{Long: "copyright"},
}

type config struct {
	debug       bool
	libs        []string
	verbose     bool
	quiet       bool
	historyPath string
}

type replConfig struct {
	sourceName  string
	historyPath string
	interactive bool
	quiet       bool
	verbose     bool
}

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	prog := "gomirb"
	if len(argv) > 0 && argv[0] != "" {
		prog = argv[0]
	}
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}

	cfg := config{historyPath: defaultHistoryPath()}
	matches, rest, parseErr := cli.Parse(args, optionSpecs)
	for _, match := range matches {
		switch {
		case match.Option.Short == 'd':
			cfg.debug = true
		case match.Option.Short == 'r':
			cfg.libs = append(cfg.libs, match.Arg)
		case match.Option.Short == 'v':
			if !cfg.verbose {
				printVersion(stdout)
			}
			cfg.verbose = true
		case match.Option.Short == 'q':
			cfg.quiet = true
		case match.Option.Short == 'H':
			cfg.historyPath = match.Arg
		case match.Option.Short == 'h', match.Option.Long == "help":
			printUsage(stdout, prog)
			return 0
		case match.Option.Long == "verbose":
			cfg.verbose = true
		case match.Option.Long == "version":
			printVersion(stdout)
			return 0
		case match.Option.Long == "copyright":
			printCopyright(stdout)
			return 0
		}
	}
	if parseErr != nil {
		return reportParseError(stderr, prog, parseErr)
	}

	programPath := ""
	var programArgs []string
	if len(rest) > 0 {
		programPath = rest[0]
		programArgs = rest[1:]
	}

	ctx := context.Background()
	state := mrbvm.New(mrbvm.Options{
		Stdin:    stdin,
		Stdout:   stdout,
		Stderr:   stderr,
		Compiler: compileEvalSource,
	})
	state.SetGlobal("$DEBUG", mrbvm.BoolValue(cfg.debug))
	state.DefineConst(nil, "ARGV", stringArray(programArgs))

	interactive := programPath == "" && isTerminal(stdin)
	if interactive {
		fmt.Fprintln(stdout, "gomirb - Embeddable Interactive Ruby Shell")
		fmt.Fprintln(stdout)
	}

	// Load libraries before the program or the REPL.
	for _, lib := range cfg.libs {
		data, openFailed, err := readFile(lib)
		if err != nil {
			if openFailed {
				fmt.Fprintf(stderr, "Cannot open library file. (%s)\n", lib)
			} else {
				fmt.Fprintf(stderr, "Cannot read library file. (%s)\n", lib)
			}
			return 1
		}
		if err := loadLibrary(ctx, state, data, lib, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if programPath != "" {
		file, err := os.Open(programPath)
		if err != nil {
			fmt.Fprintf(stderr, "Cannot open program file. (%s)\n", programPath)
			return 1
		}
		defer file.Close()
		if info, statErr := file.Stat(); statErr == nil && info.IsDir() {
			fmt.Fprintf(stderr, "Cannot read program file. (%s)\n", programPath)
			return 1
		}
		err = runREPL(ctx, file, stdout, stderr, state, replConfig{
			sourceName:  programPath,
			historyPath: cfg.historyPath,
			quiet:       cfg.quiet,
			verbose:     cfg.verbose,
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	err := runREPL(ctx, stdin, stdout, stderr, state, replConfig{
		sourceName:  "(mirb)",
		historyPath: cfg.historyPath,
		interactive: interactive,
		quiet:       cfg.quiet,
		verbose:     cfg.verbose,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func isTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func defaultHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.gomirb_history"
}

func loadHistory(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var history []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			history = append(history, line)
		}
	}
	if len(history) > 1000 {
		history = history[len(history)-1000:]
	}
	return history
}

func saveHistory(path string, history []string) {
	if path == "" || len(history) == 0 {
		return
	}
	_ = os.WriteFile(path, []byte(strings.Join(history, "\n")+"\n"), 0o600)
}

func runREPL(ctx context.Context, input io.Reader, output, errOutput io.Writer, state *mrbvm.State, cfg replConfig) error {
	sourceName := cfg.sourceName
	if sourceName == "" {
		sourceName = "(mirb)"
	}
	if !cfg.interactive {
		source, err := io.ReadAll(input)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(source))) == 0 {
			return nil
		}
		result, err := execute(ctx, state, source, sourceName, cfg.verbose, output)
		if err == nil && !cfg.quiet && !result.IsNil() {
			fmt.Fprintf(output, "=> %s\n", result.Inspect())
		}
		return err
	}

	history := loadHistory(cfg.historyPath)
	defer func() { saveHistory(cfg.historyPath, history) }()
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
			if trimmed == "history" {
				for i, entry := range history {
					fmt.Fprintf(output, "%d: %s\n", i+1, entry)
				}
				continue
			}
			if strings.HasPrefix(trimmed, "complete ") {
				prefix := strings.TrimSpace(strings.TrimPrefix(trimmed, "complete "))
				for _, name := range state.Complete(prefix) {
					fmt.Fprintln(output, name)
				}
				continue
			}
		}
		if buffer.Len() > 0 {
			buffer.WriteByte('\n')
		}
		buffer.WriteString(line)

		result, err := execute(ctx, state, []byte(buffer.String()), sourceName, cfg.verbose, output)
		if err != nil && incompleteSource(err.Error()) {
			continue
		}
		if err != nil {
			fmt.Fprintln(errOutput, err)
		} else {
			source := buffer.String()
			if len(history) == 0 || history[len(history)-1] != source {
				history = append(history, source)
			}
			if !cfg.quiet && !result.IsNil() {
				fmt.Fprintf(output, "=> %s\n", result.Inspect())
			}
		}
		buffer.Reset()
	}
	return scanner.Err()
}

func execute(ctx context.Context, state *mrbvm.State, source []byte, filename string, verbose bool, output io.Writer) (mrbvm.Value, error) {
	if filename == "" {
		filename = "(mirb)"
	}
	irep, err := mrbc.Compile(ctx, source, mrbc.Options{FilePath: filename, DebugInfo: true})
	if err != nil {
		return mrbvm.Nil, err
	}
	if verbose {
		if err := bytecode.Disassemble(output, irep); err != nil {
			return mrbvm.Nil, err
		}
	}
	return state.Execute(ctx, irep)
}

// loadLibrary executes a -r library. A .mrb extension or a RITE header selects
// the bytecode loader; everything else is compiled as Ruby source.
func loadLibrary(ctx context.Context, state *mrbvm.State, data []byte, filename string, output io.Writer) error {
	if filepath.Ext(filename) == ".mrb" || bytes.HasPrefix(data, []byte("RITE")) {
		irep, err := mrbvm.DecodeBytecode(data)
		if err != nil {
			return err
		}
		_, err = state.Execute(ctx, irep)
		return err
	}
	_, err := execute(ctx, state, data, filename, false, output)
	return err
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

// readFile reports whether the failure happened while opening the file, so
// the caller can distinguish "Cannot open" from "Cannot read" like mirb.
func readFile(path string) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, true, err
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr == nil && info.IsDir() {
		return nil, false, fmt.Errorf("is a directory")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

func stringArray(values []string) mrbvm.Value {
	items := make([]mrbvm.Value, len(values))
	for i, value := range values {
		items[i] = mrbvm.StringValue(value)
	}
	return mrbvm.ArrayValue(items)
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "gomirb %s (go-mrbvm, %s)\n", mrbvm.Version, mrbvm.Target)
}

func printCopyright(w io.Writer) {
	fmt.Fprintln(w, "go-mrbvm - Copyright (c) 2026 go-mrbvm authors")
}

func printUsage(w io.Writer, prog string) {
	fmt.Fprintf(w, "Usage: %s [switches] [programfile] [arguments]\n", prog)
	fmt.Fprintln(w, "  switches:")
	fmt.Fprintln(w, "  -d           set $DEBUG to true (same as `mruby -d`)")
	fmt.Fprintln(w, "  -h           print this help")
	fmt.Fprintln(w, "  -r library   same as `mruby -r`")
	fmt.Fprintln(w, "  -v           print version number, then run in verbose mode")
	fmt.Fprintln(w, "  --verbose    run in verbose mode")
	fmt.Fprintln(w, "  --version    print the version")
	fmt.Fprintln(w, "  --copyright  print the copyright")
	fmt.Fprintln(w, "  -q           do not print expression results")
	fmt.Fprintln(w, "  -H file      history file (empty disables history)")
}

func reportParseError(w io.Writer, prog string, err error) int {
	parseErr, ok := err.(*cli.ParseError)
	if !ok {
		fmt.Fprintf(w, "%s: %v\n", prog, err)
		return 1
	}
	if parseErr.Missing {
		if parseErr.Option.Short == 'r' {
			fmt.Fprintf(w, "%s: No library specified for -r\n", prog)
		} else {
			fmt.Fprintf(w, "%s: option requires an argument -- %c\n", prog, parseErr.Option.Short)
		}
		return 1
	}
	if parseErr.Long {
		fmt.Fprintf(w, "%s: invalid option --%s\n", prog, parseErr.Name)
	} else {
		fmt.Fprintf(w, "%s: invalid option -%s\n", prog, parseErr.Name)
	}
	return 1
}

func compileEvalSource(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
	return mrbc.Compile(ctx, source, mrbc.Options{FilePath: opts.FilePath, LocalNames: opts.LocalNames, LineOffset: opts.LineOffset, Eval: opts.Eval, EvalInMethod: opts.EvalInMethod})
}
