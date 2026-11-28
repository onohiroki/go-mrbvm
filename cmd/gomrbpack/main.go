// Command gomrbpack packages Ruby sources and RITE bytecode into a .mrbb
// bundle and builds a single executable from it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "pack":
		return runPack(ctx, args[1:], stdout, stderr)
	case "build":
		return runBuild(ctx, args[1:], stdout, stderr)
	case "run":
		return runBundle(ctx, args[1:], stdin, stdout, stderr)
	case "list":
		return runList(args[1:], stdout, stderr)
	case "info":
		return runInfo(args[1:], stdout, stderr)
	case "extract":
		return runExtract(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		printVersion(stdout)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "gomrbpack: unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func printVersion(w io.Writer) {
	version := ownModuleVersion()
	if version == "" {
		version = "devel"
	}
	fmt.Fprintf(w, "gomrbpack %s (go-mrbvm %s, %s)\n", version, mrbvm.Version, mrbvm.Target)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: gomrbpack <command> [options] [files...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  pack     compile .rb files and write a .mrbb bundle")
	fmt.Fprintln(w, "  build    build a single executable that embeds the bundle")
	fmt.Fprintln(w, "  run      run a .mrbb bundle")
	fmt.Fprintln(w, "  list     list the files in a bundle")
	fmt.Fprintln(w, "  info     print the bundle manifest")
	fmt.Fprintln(w, "  extract  write the bundle members to a directory")
	fmt.Fprintln(w, "  version  print the version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Example:")
	fmt.Fprintln(w, "  gomrbpack build -o app main.rb lib/")
}
