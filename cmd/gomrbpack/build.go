package main

import (
	"context"
	"debug/buildinfo"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const archiveName = "app.mrbb"

type buildConfig struct {
	workdir       string
	keepWork      bool
	replace       string
	moduleVersion string
	goos          string
	goarch        string
	tags          string
	ldflags       string
	strip         bool
	noTrimpath    bool
	verbose       bool
}

func (cfg *buildConfig) register(fs *flag.FlagSet) {
	fs.StringVar(&cfg.workdir, "workdir", "", "build directory (default: a temporary directory)")
	fs.BoolVar(&cfg.keepWork, "keep-work", false, "keep the temporary build directory")
	fs.StringVar(&cfg.replace, "replace", "", "replace go-mrbvm with a local module directory")
	fs.StringVar(&cfg.moduleVersion, "module-version", "", "go-mrbvm version to require (default: this tool's version)")
	fs.StringVar(&cfg.goos, "goos", "", "target operating system (GOOS)")
	fs.StringVar(&cfg.goarch, "goarch", "", "target architecture (GOARCH)")
	fs.StringVar(&cfg.tags, "tags", "", "build tags")
	fs.StringVar(&cfg.ldflags, "ldflags", "", "linker flags")
	fs.BoolVar(&cfg.strip, "strip", false, "omit the symbol table and DWARF data")
	fs.BoolVar(&cfg.noTrimpath, "no-trimpath", false, "keep full file paths in the binary")
	fs.BoolVar(&cfg.verbose, "verbose", false, "print the generated files and go commands")
}

func runBuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var packCfg packConfig
	packCfg.register(flags)
	var cfg buildConfig
	cfg.register(flags)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if packCfg.output == "" {
		fmt.Fprintln(stderr, "gomrbpack: build requires -o")
		return 2
	}

	archive, entry, count, err := buildArchive(ctx, &packCfg, flags.Args())
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}

	version := cfg.moduleVersion
	if version == "" {
		version = ownModuleVersion()
	}
	if version == "" {
		if cfg.replace == "" {
			fmt.Fprintln(stderr, "gomrbpack: cannot determine the go-mrbvm version; pass --module-version or --replace")
			return 1
		}
		version = "v0.0.0"
	}
	replaceDir := ""
	if cfg.replace != "" {
		replaceDir, err = filepath.Abs(cfg.replace)
		if err != nil {
			fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
			return 1
		}
		if _, err := os.Stat(filepath.Join(replaceDir, "go.mod")); err != nil {
			fmt.Fprintf(stderr, "gomrbpack: --replace %s: %v\n", cfg.replace, err)
			return 1
		}
	}

	output, err := filepath.Abs(packCfg.output)
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}

	dir, remove, err := buildDir(cfg.workdir)
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if remove {
		defer os.RemoveAll(dir)
	}

	if err := os.WriteFile(filepath.Join(dir, archiveName), archive, 0o644); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(generatedMain), 0o644); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	goMod := goModFile(version, replaceDir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if cfg.verbose {
		fmt.Fprintf(stdout, "gomrbpack: workdir %s\n", dir)
		fmt.Fprintf(stdout, "gomrbpack: entry %s (%d files)\n", entry, count)
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "gomrbpack: the go command is required for build")
		return 1
	}
	env := buildEnv(&cfg)

	if cfg.verbose {
		fmt.Fprintln(stdout, "gomrbpack: go mod tidy")
	}
	if err := runGo(ctx, goBin, dir, env, stderr, stdout, cfg.verbose, "mod", "tidy"); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: go mod tidy failed: %v\n", err)
		return 1
	}

	buildArgs := []string{"build", "-o", output}
	if !cfg.noTrimpath {
		buildArgs = append(buildArgs, "-trimpath")
	}
	ldflags := cfg.ldflags
	if cfg.strip {
		if ldflags != "" {
			ldflags += " "
		}
		ldflags += "-s -w"
	}
	if ldflags != "" {
		buildArgs = append(buildArgs, "-ldflags", ldflags)
	}
	if cfg.tags != "" {
		buildArgs = append(buildArgs, "-tags", cfg.tags)
	}
	buildArgs = append(buildArgs, ".")

	if cfg.verbose {
		fmt.Fprintf(stdout, "gomrbpack: go %s\n", strings.Join(buildArgs, " "))
	}
	if err := runGo(ctx, goBin, dir, env, stderr, stdout, cfg.verbose, buildArgs...); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: go build failed: %v\n", err)
		return 1
	}

	if !packCfg.quiet {
		fmt.Fprintf(stdout, "gomrbpack: wrote %s (entry %s, %d files)\n", output, entry, count)
	}
	return 0
}

func buildDir(requested string) (string, bool, error) {
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
	dir, err := os.MkdirTemp("", "gomrbpack-build-*")
	if err != nil {
		return "", false, err
	}
	return dir, true, nil
}

func buildEnv(cfg *buildConfig) []string {
	env := os.Environ()
	if cfg.goos != "" {
		env = append(env, "GOOS="+cfg.goos)
	}
	if cfg.goarch != "" {
		env = append(env, "GOARCH="+cfg.goarch)
	}
	return env
}

func runGo(ctx context.Context, goBin, dir string, env []string, stderr, verboseOut io.Writer, verbose bool, args ...string) error {
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	cmd.Env = env
	if verbose {
		cmd.Stdout = verboseOut
		cmd.Stderr = verboseOut
	} else {
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

func goModFile(version, replaceDir string) string {
	var b strings.Builder
	b.WriteString("module gomrbpack.app\n\n")
	b.WriteString("go " + goLanguageVersion() + "\n\n")
	b.WriteString("require github.com/onohiroki/go-mrbvm " + version + "\n")
	if replaceDir != "" {
		b.WriteString("\nreplace github.com/onohiroki/go-mrbvm => " + filepath.ToSlash(replaceDir) + "\n")
	}
	return b.String()
}

func goLanguageVersion() string {
	version := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// ownModuleVersion reports the go-mrbvm version this tool was built from, or
// an empty string for a development build.
func ownModuleVersion() string {
	info, err := buildinfo.ReadFile(os.Args[0])
	if err != nil {
		return ""
	}
	version := info.Main.Version
	if version == "" || version == "(devel)" {
		return ""
	}
	return version
}

const generatedMain = `// Code generated by gomrbpack. DO NOT EDIT.
package main

import (
	"context"
	_ "embed"
	"os"

	"github.com/onohiroki/go-mrbvm/bundle"
)

//go:embed app.mrbb
var archive []byte

func main() {
	os.Exit(bundle.Run(context.Background(), archive, os.Args, os.Stdin, os.Stdout, os.Stderr, bundle.Options{}))
}
`
