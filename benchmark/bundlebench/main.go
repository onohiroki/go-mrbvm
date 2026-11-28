// Command bundlebench measures gomrbpack bundle startup time and binary size
// against the plain gomruby runner.
//
// It builds gomruby and gomrbpack from the checkout, packages the sample app
// in benchmark/bundle, builds a single binary, and reports wall-clock timing
// for three ways to run the same program:
//
//	gomruby main.rb   compile source at startup and run it
//	gomruby app.mrbb  run the archive (no compile)
//	built app         run the generated single binary
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", "", "repository root (default: search upward for go.mod)")
	runs := flag.Int("runs", 20, "measured runs per command")
	warmup := flag.Int("warmup", 3, "warmup runs per command")
	work := flag.String("work", "", "work directory (default: a temporary directory)")
	output := flag.String("o", "", "write the report to this file as well as stdout")
	smoke := flag.Bool("smoke", false, "build and verify only, without timing")
	flag.Parse()

	if err := execute(*root, *work, *output, *runs, *warmup, *smoke); err != nil {
		fmt.Fprintln(os.Stderr, "bundlebench:", err)
		os.Exit(1)
	}
}

func execute(rootArg, workArg, output string, runs, warmup int, smoke bool) error {
	if runs < 1 {
		return errors.New("-runs must be at least 1")
	}
	if warmup < 0 {
		return errors.New("-warmup must not be negative")
	}
	root, err := findRoot(rootArg)
	if err != nil {
		return err
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		return errors.New("the go command is required")
	}

	work, remove, err := workDir(workArg)
	if err != nil {
		return err
	}
	if remove {
		defer os.RemoveAll(work)
	}
	binDir := filepath.Join(work, "bin")
	appDir := filepath.Join(work, "app")
	if err := os.MkdirAll(filepath.Join(appDir, "lib"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}

	gomruby := filepath.Join(binDir, exeName("gomruby"))
	gomrbpack := filepath.Join(binDir, exeName("gomrbpack"))
	builtApp := filepath.Join(binDir, exeName("app"))
	if err := build(root, goBin, gomruby, "./cmd/gomruby"); err != nil {
		return err
	}
	if err := build(root, goBin, gomrbpack, "./cmd/gomrbpack"); err != nil {
		return err
	}

	srcDir := filepath.Join(root, "benchmark", "bundle")
	copies := [][2]string{
		{filepath.Join(srcDir, "main.rb"), filepath.Join(appDir, "main.rb")},
		{filepath.Join(srcDir, "lib", "helper.rb"), filepath.Join(appDir, "lib", "helper.rb")},
	}
	for _, copy := range copies {
		if err := copyFile(copy[0], copy[1]); err != nil {
			return err
		}
	}

	archivePath := filepath.Join(appDir, "app.mrbb")
	if err := runCommand(root, gomrbpack, "pack", "--root", appDir, "--entry", "main.rb",
		"--quiet", "-o", archivePath, filepath.Join(appDir, "main.rb"), filepath.Join(appDir, "lib")); err != nil {
		return err
	}
	if err := runCommand(root, gomrbpack, "build", "--root", appDir, "--entry", "main.rb",
		"--replace", root, "--quiet", "-o", builtApp, filepath.Join(appDir, "main.rb"), filepath.Join(appDir, "lib")); err != nil {
		return err
	}

	cases := []struct {
		name string
		argv []string
	}{
		{"gomruby main.rb", []string{gomruby, filepath.Join(appDir, "main.rb")}},
		{"gomruby app.mrbb", []string{gomruby, archivePath}},
		{"built app", []string{builtApp}},
	}

	// Every command must print the same thing; use the first run as the
	// reference and fail fast on a difference.
	var reference []byte
	for i := range cases {
		out, err := capture(cases[i].argv)
		if err != nil {
			return fmt.Errorf("%s: %w", cases[i].name, err)
		}
		if i == 0 {
			reference = out
			continue
		}
		if !bytes.Equal(out, reference) {
			return fmt.Errorf("%s: output %q differs from gomruby main.rb %q", cases[i].name, out, reference)
		}
	}

	gomrubySize, err := fileSize(gomruby)
	if err != nil {
		return err
	}
	appSize, err := fileSize(builtApp)
	if err != nil {
		return err
	}
	archiveSize, err := fileSize(archivePath)
	if err != nil {
		return err
	}

	if smoke {
		fmt.Printf("bundle smoke: OK (%d bytes archive, %d bytes app, %d bytes gomruby)\n", archiveSize, appSize, gomrubySize)
		return nil
	}

	results := make([]timing, 0, len(cases))
	for _, tc := range cases {
		durations, err := measure(tc.argv, runs, warmup)
		if err != nil {
			return fmt.Errorf("%s: %w", tc.name, err)
		}
		results = append(results, timing{name: tc.name, durations: durations})
	}

	report := renderReport(root, runs, warmup, gomrubySize, appSize, archiveSize, results)
	fmt.Print(report)
	if output != "" {
		if err := os.WriteFile(output, []byte(report), 0o644); err != nil {
			return err
		}
	}
	return nil
}

type timing struct {
	name      string
	durations []time.Duration
}

func (t timing) median() time.Duration {
	sorted := append([]time.Duration(nil), t.durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func (t timing) min() time.Duration {
	sorted := append([]time.Duration(nil), t.durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[0]
}

func (t timing) mean() time.Duration {
	var total time.Duration
	for _, duration := range t.durations {
		total += duration
	}
	return total / time.Duration(len(t.durations))
}

func renderReport(root string, runs, warmup int, gomrubySize, appSize, archiveSize int64, results []timing) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# gomrbpack bundle benchmark\n")
	fmt.Fprintf(&b, "date: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "commit: %s\n", gitCommit(root))
	fmt.Fprintf(&b, "go: %s\n", runtime.Version())
	fmt.Fprintf(&b, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "runs: %d (warmup %d)\n", runs, warmup)
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "sizes:\n")
	fmt.Fprintf(&b, "  gomruby      %10s  (%d bytes)\n", humanBytes(gomrubySize), gomrubySize)
	fmt.Fprintf(&b, "  built app    %10s  (%d bytes)\n", humanBytes(appSize), appSize)
	fmt.Fprintf(&b, "  app.mrbb     %10s  (%d bytes)\n", humanBytes(archiveSize), archiveSize)
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "wall clock (median, min, mean):\n")
	for _, result := range results {
		fmt.Fprintf(&b, "  %-18s %8.2f ms  (%6.2f, %6.2f)\n",
			result.name,
			milliseconds(result.median()),
			milliseconds(result.min()),
			milliseconds(result.mean()))
	}
	return b.String()
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/unit)
}

func build(root, goBin, output, pkg string) error {
	return runCommand(root, goBin, "build", "-trimpath", "-o", output, pkg)
}

func runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func capture(argv []string) ([]byte, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w\n%s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func measure(argv []string, runs, warmup int) ([]time.Duration, error) {
	for i := 0; i < warmup; i++ {
		if _, err := capture(argv); err != nil {
			return nil, err
		}
	}
	durations := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdout = io.Discard
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		start := time.Now()
		err := cmd.Run()
		durations = append(durations, time.Since(start))
		if err != nil {
			return nil, fmt.Errorf("%w\n%s", err, stderr.String())
		}
	}
	return durations, nil
}

func findRoot(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found; pass -root")
		}
		dir = parent
	}
}

func workDir(requested string) (string, bool, error) {
	if requested != "" {
		abs, err := filepath.Abs(requested)
		if err != nil {
			return "", false, err
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", false, err
		}
		return abs, false, nil
	}
	dir, err := os.MkdirTemp("", "bundlebench-*")
	if err != nil {
		return "", false, err
	}
	return dir, true, nil
}

func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func fileSize(name string) (int64, error) {
	info, err := os.Stat(name)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func gitCommit(root string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
