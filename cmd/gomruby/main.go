// Command gomruby runs mruby 4.0.0 bytecode with the pure-Go go-mrbvm VM.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func main() {
	version := flag.Bool("v", false, "print version")
	flag.Parse()

	if *version {
		fmt.Println("gomruby 0.0.0 (go-mrbvm, mruby 4.0.0 / RITE 04.00)")
		return
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gomruby [-v] file.mrb|file.rb")
		os.Exit(2)
	}

	path := flag.Arg(0)
	ctx := context.Background()
	data, err := load(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	state := mrbvm.New(mrbvm.Options{Compiler: compileEvalSource})
	if _, err := state.Run(ctx, data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func load(ctx context.Context, path string) ([]byte, error) {
	if filepath.Ext(path) == ".rb" {
		return mrbc.CompileFile(ctx, path, mrbc.Options{DebugInfo: true})
	}
	return os.ReadFile(path)
}

func compileEvalSource(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
	return mrbc.Compile(ctx, source, mrbc.Options{FilePath: opts.FilePath, LocalNames: opts.LocalNames, LineOffset: opts.LineOffset})
}
