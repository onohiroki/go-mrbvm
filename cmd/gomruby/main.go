// Command gomruby runs mruby 4.0.0 bytecode with the pure-Go go-mrbvm VM.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/bundle"
	"github.com/onohiroki/go-mrbvm/internal/cli"
	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

var optionSpecs = []cli.Option{
	{Short: 'b'},
	{Short: 'c'},
	{Short: 'd'},
	{Short: 'e', HasArg: true},
	{Short: 'h'},
	{Long: "help"},
	{Short: 'r', HasArg: true},
	{Short: 'v'},
	{Long: "verbose"},
	{Long: "version"},
	{Long: "copyright"},
}

type config struct {
	bytecode    bool
	checkSyntax bool
	debug       bool
	code        []string
	hasCode     bool
	libs        []string
	verbose     bool
	versionSeen bool
}

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	prog := "gomruby"
	if len(argv) > 0 && argv[0] != "" {
		prog = argv[0]
	}
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}

	matches, rest, parseErr := cli.Parse(args, optionSpecs)
	cfg := config{}
	for _, match := range matches {
		switch {
		case match.Option.Short == 'b':
			cfg.bytecode = true
		case match.Option.Short == 'c':
			cfg.checkSyntax = true
		case match.Option.Short == 'd':
			cfg.debug = true
		case match.Option.Short == 'e':
			cfg.hasCode = true
			cfg.code = append(cfg.code, match.Arg)
		case match.Option.Short == 'r':
			cfg.libs = append(cfg.libs, match.Arg)
		case match.Option.Short == 'v':
			if !cfg.verbose {
				printVersion(stdout)
				cfg.versionSeen = true
			}
			cfg.verbose = true
		case match.Option.Long == "verbose":
			cfg.verbose = true
		case match.Option.Long == "version":
			printVersion(stdout)
			return 0
		case match.Option.Long == "copyright":
			printCopyright(stdout)
			return 0
		case match.Option.Short == 'h', match.Option.Long == "help":
			printUsage(stdout, prog)
			return 0
		}
	}
	if parseErr != nil {
		return reportParseError(stderr, prog, parseErr)
	}

	// `-v` alone prints the version and exits; `mruby -v` does not read
	// standard input. With a program or -e it continues in verbose mode.
	if cfg.versionSeen && !cfg.hasCode && len(rest) == 0 {
		return 0
	}

	ctx := context.Background()

	// A .mrbb bundle is a self-contained program; run it through the bundle
	// runtime so that its embedded filesystem and entry point are used.
	if !cfg.hasCode && len(rest) > 0 && rest[0] != "-" && strings.EqualFold(filepath.Ext(rest[0]), bundle.Extension) {
		if len(cfg.libs) > 0 {
			fmt.Fprintf(stderr, "%s: -r cannot be combined with a %s bundle\n", prog, bundle.Extension)
			return 1
		}
		return runBundleProgram(ctx, prog, rest, cfg, stdin, stdout, stderr)
	}

	programName := "-"
	var programArgs []string
	var programPath string
	var programFile *os.File
	programReader := stdin
	if cfg.hasCode {
		programName = "-e"
		programArgs = rest
	} else if len(rest) > 0 {
		programPath = rest[0]
		programName = programPath
		programArgs = rest[1:]
		if programPath != "-" {
			file, err := os.Open(programPath)
			if err != nil {
				if info, statErr := os.Stat(programPath); statErr == nil && info.IsDir() {
					fmt.Fprintf(stderr, "%s: Cannot read program file: %s\n", prog, programPath)
				} else {
					fmt.Fprintf(stderr, "%s: Cannot open program file: %s\n", prog, programPath)
				}
				return 1
			}
			if info, statErr := file.Stat(); statErr == nil && info.IsDir() {
				file.Close()
				fmt.Fprintf(stderr, "%s: Cannot read program file: %s\n", prog, programPath)
				return 1
			}
			programFile = file
			programReader = file
		}
	}
	if programFile != nil {
		defer programFile.Close()
	}

	state := mrbvm.New(mrbvm.Options{
		Stdin:    stdin,
		Stdout:   stdout,
		Stderr:   stderr,
		Compiler: compileEvalSource,
		Policy:   sandbox.AllowAll(),
	})
	defer state.Close()
	state.SetGlobal("$DEBUG", mrbvm.BoolValue(cfg.debug))
	state.SetGlobal("$0", mrbvm.StringValue(programName))
	state.DefineConst(nil, "ARGV", stringArray(programArgs))

	for _, lib := range cfg.libs {
		data, openFailed, err := readFile(lib)
		if err != nil {
			if openFailed {
				fmt.Fprintf(stderr, "%s: Cannot open library file: %s\n", prog, lib)
			} else {
				fmt.Fprintf(stderr, "%s: Cannot read library file: %s\n", prog, lib)
			}
			return 1
		}
		if err := runChunk(ctx, state, data, lib, false, false, cfg, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if cfg.hasCode {
		source := []byte(strings.Join(cfg.code, "\n"))
		if err := runChunk(ctx, state, source, "-e", false, true, cfg, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if cfg.checkSyntax {
			fmt.Fprintln(stdout, "Syntax OK")
		}
		return 0
	}

	data, err := io.ReadAll(programReader)
	if err != nil {
		fmt.Fprintf(stderr, "%s: Cannot read program file: %s\n", prog, programName)
		return 1
	}
	if err := runChunk(ctx, state, data, programName, cfg.bytecode, false, cfg, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if cfg.checkSyntax {
		fmt.Fprintln(stdout, "Syntax OK")
	}
	return 0
}

// runBundleProgram executes a .mrbb archive, setting ARGV from the arguments
// after the bundle path.
func runBundleProgram(ctx context.Context, prog string, argv []string, cfg config, stdin io.Reader, stdout, stderr io.Writer) int {
	path := argv[0]
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s: Cannot open program file: %s\n", prog, path)
		return 1
	}
	app, err := bundle.Open(data)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return 1
	}
	if cfg.checkSyntax {
		fmt.Fprintln(stdout, "Syntax OK")
		return 0
	}
	if cfg.verbose {
		manifest := app.Manifest()
		fmt.Fprintf(stdout, "bundle entry: %s\n", manifest.Entry)
		for _, file := range manifest.Files {
			fmt.Fprintf(stdout, "  %-6s %8d %s\n", file.Kind, file.Size, file.Path)
		}
	}
	return app.Run(ctx, argv, stdin, stdout, stderr, bundle.Options{
		Compiler: compileEvalSource,
		Policy:   sandbox.AllowAll(),
		Debug:    cfg.debug,
	})
}

// runChunk compiles or decodes one program chunk, optionally disassembles it,
// and executes it unless -c was given. forceSource is used for -e, which is
// always Ruby source even if the text happens to start with RITE.
func runChunk(ctx context.Context, state *mrbvm.State, data []byte, name string, forceBytecode, forceSource bool, cfg config, stdout io.Writer) error {
	isBytecode := !forceSource && (forceBytecode || filepath.Ext(name) == ".mrb" || bytes.HasPrefix(data, []byte("RITE")))
	var (
		irep *mrbvm.Irep
		err  error
	)
	if isBytecode {
		irep, err = mrbvm.DecodeBytecode(data)
	} else {
		irep, err = mrbc.Compile(ctx, data, mrbc.Options{FilePath: name, DebugInfo: true})
	}
	if err != nil {
		return err
	}
	if cfg.verbose {
		if err := bytecode.Disassemble(stdout, irep); err != nil {
			return err
		}
	}
	if cfg.checkSyntax {
		return nil
	}
	_, err = state.Execute(ctx, irep)
	return err
}

// readFile reports whether the failure happened while opening the file, so
// callers can distinguish "Cannot open" from "Cannot read" as mruby does.
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
	fmt.Fprintf(w, "gomruby %s (go-mrbvm, %s)\n", mrbvm.Version, mrbvm.Target)
}

func printCopyright(w io.Writer) {
	fmt.Fprintln(w, "go-mrbvm - Copyright (c) 2026 go-mrbvm authors")
}

func printUsage(w io.Writer, prog string) {
	fmt.Fprintf(w, "Usage: %s [switches] [programfile] [arguments]\n", prog)
	fmt.Fprintln(w, "  switches:")
	fmt.Fprintln(w, "  -b           load and execute RiteBinary (mrb) file")
	fmt.Fprintln(w, "  -c           check syntax only")
	fmt.Fprintln(w, "  -d           set debugging flags (set $DEBUG to true)")
	fmt.Fprintln(w, "  -e 'command' one line of script")
	fmt.Fprintln(w, "  -h           print this help")
	fmt.Fprintln(w, "  -r library   load the library before executing your script")
	fmt.Fprintln(w, "  -v           print version number, then run in verbose mode")
	fmt.Fprintln(w, "  --verbose    run in verbose mode")
	fmt.Fprintln(w, "  --version    print the version")
	fmt.Fprintln(w, "  --copyright  print the copyright")
}

func reportParseError(w io.Writer, prog string, err error) int {
	parseErr, ok := err.(*cli.ParseError)
	if !ok {
		fmt.Fprintf(w, "%s: %v\n", prog, err)
		return 1
	}
	if parseErr.Missing {
		switch parseErr.Option.Short {
		case 'e':
			fmt.Fprintf(w, "%s: No code specified for -e\n", prog)
		case 'r':
			fmt.Fprintf(w, "%s: No library specified for -r\n", prog)
		default:
			fmt.Fprintf(w, "%s: option requires an argument -- %c\n", prog, parseErr.Option.Short)
		}
		return 1
	}
	if parseErr.Long {
		fmt.Fprintf(w, "%s: invalid option --%s (-h will show valid options)\n", prog, parseErr.Name)
	} else {
		fmt.Fprintf(w, "%s: invalid option -%s (-h will show valid options)\n", prog, parseErr.Name)
	}
	return 1
}

func compileEvalSource(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
	return mrbc.Compile(ctx, source, mrbc.Options{FilePath: opts.FilePath, LocalNames: opts.LocalNames, LineOffset: opts.LineOffset, Eval: opts.Eval, EvalInMethod: opts.EvalInMethod})
}
