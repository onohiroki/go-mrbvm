package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/onohiroki/go-mrbvm/bundle"
	"github.com/onohiroki/go-mrbvm/sandbox"
)

func runBundle(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "gomrbpack: run requires a bundle path")
		return 2
	}
	b, err := bundle.OpenFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	return b.Run(ctx, args, stdin, stdout, stderr, bundle.Options{
		Compiler: compileEvalSource,
		Policy:   sandbox.AllowAll(),
	})
}

func runList(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "gomrbpack: list requires one bundle path")
		return 2
	}
	b, err := bundle.OpenFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	m := b.Manifest()
	fmt.Fprintf(stdout, "entry: %s\n", m.Entry)
	for _, file := range m.Files {
		marker := " "
		if file.Path == m.Entry {
			marker = "*"
		}
		fmt.Fprintf(stdout, "%s %-6s %8d %s\n", marker, file.Kind, file.Size, file.Path)
	}
	return 0
}

func runInfo(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "gomrbpack: info requires one bundle path")
		return 2
	}
	b, err := bundle.OpenFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(b.Manifest(), "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

func runExtract(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("extract", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("o", "", "output directory")
	flags.StringVar(output, "output", "", "output directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	rest := flags.Args()
	if len(rest) != 1 || *output == "" {
		fmt.Fprintln(stderr, "gomrbpack: extract requires -o DIR and one bundle path")
		return 2
	}
	b, err := bundle.OpenFile(rest[0])
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if err := b.Extract(*output); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "gomrbpack: extracted %d files to %s\n", len(b.Files()), *output)
	return 0
}
