// Command vmbench measures go-mrbvm execution and optionally compares it with
// the reference mruby interpreter.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/onohiroki/go-mrbc/mrbc"
	localtoolchain "github.com/onohiroki/go-mrbvm/internal/toolchain"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

func main() {
	iterations := flag.Int("n", 5, "number of iterations")
	compare := flag.Bool("compare", false, "compare with the reference mruby")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: vmbench [-n N] [-compare] file.rb")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)
	ctx := context.Background()
	data, err := mrbc.CompileFile(ctx, path, mrbc.Options{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	irep, err := mrbvm.DecodeBytecode(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	start := time.Now()
	for i := 0; i < *iterations; i++ {
		state := mrbvm.New(mrbvm.Options{Stdout: io.Discard, Stderr: io.Discard})
		if _, err := state.Execute(ctx, irep); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	goDuration := time.Since(start)
	fmt.Printf("go-mrbvm  %d iterations  %s  %.2f ops/s\n", *iterations, goDuration, float64(*iterations)/goDuration.Seconds())

	if !*compare {
		return
	}
	mrubyPath, err := localtoolchain.Find("mruby")
	if err != nil {
		fmt.Fprintln(os.Stderr, "vmbench: reference mruby not found on PATH")
		return
	}
	start = time.Now()
	for i := 0; i < *iterations; i++ {
		command := exec.Command(mrubyPath, path)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	cDuration := time.Since(start)
	fmt.Printf("mruby     %d iterations  %s  %.2f ops/s\n", *iterations, cDuration, float64(*iterations)/cDuration.Seconds())
	fmt.Printf("ratio     go-mrbvm/mruby = %.2fx\n", goDuration.Seconds()/cDuration.Seconds())
}
