package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/onohiroki/go-mrbc/bytecode"
	"github.com/onohiroki/go-mrbc/mrbc"
	"github.com/onohiroki/go-mrbvm/bundle"
	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// packConfig holds the flags shared by pack and build.
type packConfig struct {
	output    string
	entry     string
	root      string
	loadPaths repeatable
	includes  repeatable
	quiet     bool
}

func (cfg *packConfig) register(fs *flag.FlagSet) {
	fs.StringVar(&cfg.output, "o", "", "output path")
	fs.StringVar(&cfg.output, "output", "", "output path")
	fs.StringVar(&cfg.entry, "entry", "", "entry file (default: first file or main.rb)")
	fs.StringVar(&cfg.root, "root", ".", "root directory for bundle paths")
	fs.Var(&cfg.loadPaths, "I", "add a require search path (repeatable)")
	fs.Var(&cfg.loadPaths, "load-path", "add a require search path (repeatable)")
	fs.Var(&cfg.includes, "include", "add a data file or directory (repeatable)")
	fs.BoolVar(&cfg.quiet, "quiet", false, "suppress the summary line")
}

type repeatable []string

func (r *repeatable) String() string { return strings.Join(*r, ",") }

func (r *repeatable) Set(value string) error {
	*r = append(*r, value)
	return nil
}

type inputFile struct {
	abs    string
	bundle string
	// data marks a member stored as an opaque asset instead of code.
	data bool
}

func runPack(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var cfg packConfig
	cfg.register(flags)
	if err := flags.Parse(args); err != nil {
		return 2
	}

	archive, entry, count, err := buildArchive(ctx, &cfg, flags.Args())
	if err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	output := cfg.output
	if output == "" {
		output = strings.TrimSuffix(entry, ".mrb") + bundle.Extension
	}
	if err := os.WriteFile(output, archive, 0o644); err != nil {
		fmt.Fprintf(stderr, "gomrbpack: %v\n", err)
		return 1
	}
	if !cfg.quiet {
		fmt.Fprintf(stdout, "gomrbpack: wrote %s (%d files, %d bytes)\n", output, count, len(archive))
	}
	return 0
}

// buildArchive collects, compiles, and encodes the inputs.
func buildArchive(ctx context.Context, cfg *packConfig, args []string) ([]byte, string, int, error) {
	inputs, err := collectInputs(cfg.root, args, cfg.includes)
	if err != nil {
		return nil, "", 0, err
	}
	var codeInputs []inputFile
	for _, input := range inputs {
		if !input.data {
			codeInputs = append(codeInputs, input)
		}
	}
	entry, err := chooseEntry(cfg, args, codeInputs)
	if err != nil {
		return nil, "", 0, err
	}

	entries := make([]bundle.Entry, 0, len(inputs))
	destinations := make(map[string]string, len(inputs))
	entryDest := ""
	for _, input := range inputs {
		member, err := buildMember(ctx, input)
		if err != nil {
			return nil, "", 0, err
		}
		if previous, exists := destinations[member.Path]; exists {
			return nil, "", 0, fmt.Errorf("%s and %s both map to %s", previous, input.bundle, member.Path)
		}
		destinations[member.Path] = input.bundle
		entries = append(entries, member)
		if input.bundle == entry {
			entryDest = member.Path
		}
	}
	if entryDest == "" {
		return nil, "", 0, fmt.Errorf("entry %s is not one of the input files", entry)
	}

	var buf bytes.Buffer
	if err := bundle.Write(&buf, entryDest, cfg.loadPaths, entries); err != nil {
		return nil, "", 0, err
	}
	return buf.Bytes(), entryDest, len(entries), nil
}

// buildMember compiles .rb to .mrb, validates existing .mrb files, and
// passes data members through unchanged.
func buildMember(ctx context.Context, input inputFile) (bundle.Entry, error) {
	if input.data {
		data, err := os.ReadFile(input.abs)
		if err != nil {
			return bundle.Entry{}, err
		}
		return bundle.Entry{Path: input.bundle, Kind: bundle.KindData, Data: data}, nil
	}
	switch strings.ToLower(filepath.Ext(input.bundle)) {
	case ".rb":
		source, err := os.ReadFile(input.abs)
		if err != nil {
			return bundle.Entry{}, err
		}
		irep, err := mrbc.Compile(ctx, source, mrbc.Options{FilePath: input.bundle, DebugInfo: true})
		if err != nil {
			return bundle.Entry{}, fmt.Errorf("%s: %w", input.bundle, err)
		}
		data, err := mrbc.DumpWithOptions(irep, bytecode.DumpOptions{DebugInfo: true})
		if err != nil {
			return bundle.Entry{}, fmt.Errorf("%s: %w", input.bundle, err)
		}
		return bundle.Entry{
			Path: strings.TrimSuffix(input.bundle, ".rb") + ".mrb",
			Kind: bundle.KindBytecode,
			Data: data,
		}, nil
	case ".mrb":
		data, err := os.ReadFile(input.abs)
		if err != nil {
			return bundle.Entry{}, err
		}
		if _, err := mrbvm.DecodeBytecode(data); err != nil {
			return bundle.Entry{}, fmt.Errorf("%s: %w", input.bundle, err)
		}
		return bundle.Entry{Path: input.bundle, Kind: bundle.KindBytecode, Data: data}, nil
	default:
		return bundle.Entry{}, fmt.Errorf("%s: unsupported file type (want .rb or .mrb)", input.abs)
	}
}

func collectInputs(rootArg string, args, includes []string) ([]inputFile, error) {
	if len(args) == 0 && len(includes) == 0 {
		return nil, fmt.Errorf("no input files")
	}
	rootAbs, err := filepath.Abs(rootArg)
	if err != nil {
		return nil, err
	}

	var files []inputFile
	seen := make(map[string]string)
	// add collects name. Explicit code files reject unsupported types, code
	// directory walks skip everything that is not .rb or .mrb, and --include
	// entries are stored as data regardless of extension.
	add := func(name string, explicit, data bool) error {
		abs, err := filepath.Abs(name)
		if err != nil {
			return err
		}
		if !data {
			ext := strings.ToLower(filepath.Ext(abs))
			if ext != ".rb" && ext != ".mrb" {
				if explicit {
					return fmt.Errorf("%s: unsupported file type (want .rb or .mrb)", name)
				}
				return nil
			}
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s: outside --root %s", name, rootArg)
		}
		bundlePath, err := bundle.CleanPath(filepath.ToSlash(rel))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		current := "code"
		if data {
			current = "data"
		}
		if previous, exists := seen[abs]; exists {
			if previous != current {
				return fmt.Errorf("%s: listed as both code and data", name)
			}
			return nil
		}
		seen[abs] = current
		files = append(files, inputFile{abs: abs, bundle: bundlePath, data: data})
		return nil
	}

	collect := func(arg string, data bool) error {
		matches := []string{arg}
		if hasGlobMeta(arg) {
			var err error
			matches, err = filepath.Glob(arg)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				return fmt.Errorf("%s: no matches", arg)
			}
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				if err := add(match, true, data); err != nil {
					return err
				}
				continue
			}
			walkErr := filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if path != match && d.Name() == ".git" {
						return fs.SkipDir
					}
					return nil
				}
				return add(path, false, data)
			})
			if walkErr != nil {
				return walkErr
			}
		}
		return nil
	}

	for _, arg := range args {
		if err := collect(arg, false); err != nil {
			return nil, err
		}
	}
	for _, include := range includes {
		if err := collect(include, true); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no input files found")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].bundle < files[j].bundle })
	return files, nil
}

func chooseEntry(cfg *packConfig, args []string, inputs []inputFile) (string, error) {
	find := func(name string) (string, bool) {
		for _, input := range inputs {
			if input.bundle == name {
				return input.bundle, true
			}
		}
		return "", false
	}

	if cfg.entry != "" {
		name, err := bundle.CleanPath(filepath.ToSlash(strings.TrimPrefix(cfg.entry, "./")))
		if err != nil {
			return "", fmt.Errorf("--entry %s: %w", cfg.entry, err)
		}
		candidates := []string{name}
		if filepath.Ext(name) == "" {
			candidates = append(candidates, name+".rb", name+".mrb")
		}
		for _, candidate := range candidates {
			if found, ok := find(candidate); ok {
				return found, nil
			}
		}
		return "", fmt.Errorf("--entry %s is not one of the input files", cfg.entry)
	}

	for _, arg := range args {
		if hasGlobMeta(arg) {
			continue
		}
		info, err := os.Stat(arg)
		if err != nil || info.IsDir() {
			continue
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			continue
		}
		rootAbs, err := filepath.Abs(cfg.root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if name, err := bundle.CleanPath(filepath.ToSlash(rel)); err == nil {
			if found, ok := find(name); ok {
				return found, nil
			}
		}
	}
	for _, candidate := range []string{"main.rb", "main.mrb"} {
		if found, ok := find(candidate); ok {
			return found, nil
		}
	}
	if len(inputs) == 0 {
		return "", fmt.Errorf("no entry point: only data files were given")
	}
	return inputs[0].bundle, nil
}

func hasGlobMeta(name string) bool {
	return strings.ContainsAny(name, "*?[")
}

// compileEvalSource compiles Ruby source for eval/load in the dev runner.
func compileEvalSource(ctx context.Context, source []byte, opts mrbvm.CompileOptions) (*mrbvm.Irep, error) {
	return mrbc.Compile(ctx, source, mrbc.Options{
		FilePath:     opts.FilePath,
		LocalNames:   opts.LocalNames,
		LineOffset:   opts.LineOffset,
		Eval:         opts.Eval,
		EvalInMethod: opts.EvalInMethod,
	})
}
