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
	"runtime/pprof"
	"time"

	"github.com/onohiroki/go-mrbc/mrbc"
	localtoolchain "github.com/onohiroki/go-mrbvm/internal/toolchain"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// compile produces RITE bytecode with go-mrbc or the reference mrbc.
func compile(ctx context.Context, path string, official bool) ([]byte, error) {
	if !official {
		return mrbc.CompileFile(ctx, path, mrbc.Options{})
	}
	mrbcPath, err := localtoolchain.Find("mrbc")
	if err != nil {
		return nil, fmt.Errorf("vmbench: reference mrbc not found: %w", err)
	}
	file, err := os.CreateTemp("", "vmbench-*.mrb")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	file.Close()
	command := exec.Command(mrbcPath, "-o", file.Name(), path)
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(file.Name())
}

func main() {
	iterations := flag.Int("n", 5, "number of iterations")
	compare := flag.Bool("compare", false, "compare with the reference mruby")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile to this file")
	memProfile := flag.String("memprofile", "", "write a heap profile to this file")
	useMrbc := flag.Bool("mrbc", false, "compile with the reference mrbc instead of go-mrbc")
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
	data, err := compile(ctx, path, *useMrbc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	irep, err := mrbvm.DecodeBytecode(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *cpuProfile != "" {
		file, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer file.Close()
		if err := pprof.StartCPUProfile(file); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
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
	if *memProfile != "" {
		file, err := os.Create(*memProfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer file.Close()
		if err := pprof.WriteHeapProfile(file); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

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
