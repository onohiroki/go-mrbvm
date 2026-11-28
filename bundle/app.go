package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/onohiroki/go-mrbvm/mrbvm"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

// Options controls Run.
type Options struct {
	// Compiler compiles Ruby source members. It is needed only for archives
	// that contain .rb files (or when a script calls eval); archives
	// produced by gomrbpack contain precompiled bytecode.
	Compiler mrbvm.CompileFunc

	// Policy is the sandbox capability allow-list. A nil Policy permits
	// Require and Load and denies everything else. Pass sandbox.AllowAll()
	// to reproduce the gomruby command's behavior.
	Policy *sandbox.Policy

	// Debug sets $DEBUG to true.
	Debug bool

	// Configure, when non-nil, runs against the mrbvm options after the
	// archive defaults are applied. It can set execution limits or override
	// any field.
	Configure func(*mrbvm.Options)

	// Setup, when non-nil, runs after the State is created and after $0,
	// ARGV, and $DEBUG are initialized, but before the entry point executes.
	// It is where an embedding registers host functions and classes.
	Setup func(*mrbvm.State) error
}

// DefaultPolicy returns the capability set used when Options.Policy is nil:
// a bundled program may require other bundled files but has no access to the
// host filesystem, network, or process environment.
func DefaultPolicy() *sandbox.Policy {
	return sandbox.New().Permit(sandbox.Require, sandbox.Load)
}

// Run decodes archive and executes its entry point. argv follows the
// conventional layout: argv[0] is the program name, remaining elements become
// ARGV. The returned value is a process exit status.
func Run(ctx context.Context, archive []byte, argv []string, stdin io.Reader, stdout, stderr io.Writer, opts Options) int {
	b, err := Open(archive)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return b.Run(ctx, argv, stdin, stdout, stderr, opts)
}

// Run executes a previously opened bundle. See Run.
func (b *Bundle) Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer, opts Options) int {
	programArgs := []string{}
	if len(argv) > 1 {
		programArgs = argv[1:]
	}

	policy := opts.Policy
	if policy == nil {
		policy = DefaultPolicy()
	}
	vmOpts := mrbvm.Options{
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
		Compiler:  opts.Compiler,
		Policy:    policy,
		SourceFS:  b.SourceFS(),
		AssetFS:   b.AssetFS(),
		LoadPaths: b.manifest.LoadPaths,
	}
	if opts.Configure != nil {
		opts.Configure(&vmOpts)
	}

	state := mrbvm.New(vmOpts)
	defer state.Close()
	state.SetGlobal("$DEBUG", mrbvm.BoolValue(opts.Debug))
	state.SetGlobal("$0", mrbvm.StringValue(b.manifest.Entry))
	state.DefineConst(nil, "ARGV", stringArray(programArgs))
	if opts.Setup != nil {
		if err := opts.Setup(state); err != nil {
			fmt.Fprintf(stderr, "bundle: setup: %v\n", err)
			return 1
		}
	}

	data := b.files[b.manifest.Entry]
	irep, err := b.compileEntry(ctx, data, opts.Compiler)
	if err == nil {
		_, err = state.Execute(ctx, irep)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func (b *Bundle) compileEntry(ctx context.Context, data []byte, compiler mrbvm.CompileFunc) (*mrbvm.Irep, error) {
	if bytes.HasPrefix(data, []byte("RITE")) {
		return mrbvm.DecodeBytecode(data)
	}
	if compiler == nil {
		return nil, fmt.Errorf("bundle: entry %s is Ruby source but no compiler is configured", b.manifest.Entry)
	}
	return compiler(ctx, data, mrbvm.CompileOptions{FilePath: b.manifest.Entry})
}

func stringArray(values []string) mrbvm.Value {
	items := make([]mrbvm.Value, len(values))
	for i, value := range values {
		items[i] = mrbvm.StringValue(value)
	}
	return mrbvm.ArrayValue(items)
}
