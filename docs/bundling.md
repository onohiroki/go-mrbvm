# Bundles and single binaries (gomrbpack)

`gomrbpack` packages Ruby sources, RITE bytecode, and data files into one
`.mrbb` archive, then builds a single executable that embeds it.

`require`, `load`, and `require_relative` read the archive through a virtual
filesystem, and `File.read` / `File.exist?` / `File.size` prefer bundled data.
Scripts and assets do not need to be shipped next to the executable.

## Relationship to the other commands

- `gomruby` — runs Ruby source or `.mrb` directly (development runner)
- `gomirb` — interactive REPL
- `gomrbpack` — builds `.mrbb` archives and single executables

`gomrbpack run`, `gomruby app.mrbb`, and the generated binary share the
`bundle` runtime. The development runners pass a compiler and a wider
capability policy, so `eval` and host `File` access work there but not in the
default generated binary.

## Usage

```sh
# Build an archive (data/ is bundled for File.read).
gomrbpack pack --entry main.rb -I lib --include data -o app.mrbb main.rb lib/

# Run an archive.
gomrbpack run app.mrbb arg1 arg2
gomruby app.mrbb arg1 arg2

# Build a single executable.
gomrbpack build --entry main.rb -I lib --include data -o app main.rb lib/
./app arg1 arg2

# Link go-mrbc so eval works.
gomrbpack build --with-compiler -o app-with-eval main.rb
```

`build` writes a small `main.go` that `//go:embed`s the archive, runs
`go mod tidy`, and invokes `go build`. The result is an ordinary Go binary, so
code signing (including macOS), cross-compilation, and `-trimpath` behave as
usual.

## Inputs and entry point

- Code inputs are `.rb` and `.mrb`. Directories are walked recursively and
  globs are accepted. Non-Ruby files inside code directories are ignored.
- `--include PATH` adds files or directories as **data** (repeatable, globs
  allowed). A file cannot be both code and data, and the entry point must be
  executable code.
- Bundle paths are relative to `--root` (default `.`).
- Without `--entry`, the first file argument wins, then `main.rb` / `main.mrb`,
  then the first file in sorted order.
- `.rb` is compiled to RITE 04.00 **at pack time**. The archive contains only
  `.mrb`, so the default generated binary does not link the compiler
  (Prism/wazero).
- Existing `.mrb` inputs are validated with `DecodeBytecode`.

`-I` is recorded in the manifest as a require search path and seeds
`$LOAD_PATH` at run time. `gomruby` also accepts `-I`.

```sh
gomruby -I lib main.rb
```

## require resolution

Inside a bundle, `require "foo"` searches:

1. the bundle root for `foo`, `foo.rb`, `foo.mrb`
2. each `load_paths` directory in the manifest for the same names

`require_relative` only searches the directory of the requiring file. A file
that has already been required is recorded by its normalized archive path and
returns `false` the next time.

`-I` and the manifest's `load_paths` initialize `$LOAD_PATH`. Ruby code can
append to `$LOAD_PATH`; the next `require` sees the change.

When `SourceFS` is set, `require` does not fall back to the host filesystem.
Absolute paths and paths that escape the bundle root raise `LoadError`.

## Runtime semantics

- `ARGV` is the program's argument list (`os.Args[1:]`).
- `$0` is the entry's archive path (for example `main.mrb`).
- The default capability policy allows `require` and `load` only. Host
  `File`, network, and process access are denied.
- Bundled data is readable through `File.read` / `File.exist?` /
  `File.size` without a capability, because it is trusted application
  content. Bundled data wins over a host file with the same path.
  `File.open` remains host-only; virtual files are not supported yet.
- The default binary has no compiler, so `eval` and source `load` fail.
  `--with-compiler` links go-mrbc and permits `eval`.
- Unhandled exceptions are printed with a backtrace and exit with status 1.
- `gomruby` detects a bundle by the `.mrbb` extension or the ZIP magic, so a
  renamed archive still runs.

## Embedding in a Go application

`gomrbpack build` is optional. An existing Go application can embed the
archive itself.

```go
import (
    "context"
    _ "embed"
    "os"

    "github.com/onohiroki/go-mrbvm/bundle"
    "github.com/onohiroki/go-mrbvm/mrbvm"
)

//go:embed app.mrbb
var archive []byte

func main() {
    os.Exit(bundle.Run(context.Background(), archive, os.Args,
        os.Stdin, os.Stdout, os.Stderr, bundle.Options{
            Setup: func(state *mrbvm.State) error {
                state.DefineFunction("host_log", hostLog)
                return nil
            },
        }))
}
```

`bundle.Options.Setup` runs after the `State` is created and after `$0`,
`ARGV`, and `$DEBUG` are initialized, but before the entry point executes.
Register host functions, classes, and constants there.

`bundle.Options.Configure` adjusts `mrbvm.Options`, including execution
limits and `SourceFS` / `AssetFS`.

`mrbvm.Options.SourceFS` and `mrbvm.Options.AssetFS` accept any read-only
filesystem. `mrbvm.NewSourceFS` adapts an `io/fs.FS` such as `embed.FS` or
`fstest.MapFS`.

```go
state := mrbvm.New(mrbvm.Options{
    SourceFS:  mrbvm.NewSourceFS(embedFS),
    AssetFS:   mrbvm.NewSourceFS(assetFS),
    LoadPaths: []string{"lib"},
    Policy:    sandbox.New().Permit(sandbox.Require, sandbox.Load),
})
```

## Archive format

A `.mrbb` file is a ZIP whose first member is `manifest.json`.

```json
{
  "format": 1,
  "entry": "main.mrb",
  "load_paths": ["lib"],
  "files": [
    {
      "path": "lib/helper.mrb",
      "kind": "mrb",
      "sha256": "…",
      "size": 383
    },
    {
      "path": "data/config.json",
      "kind": "data",
      "sha256": "…",
      "size": 128
    }
  ]
}
```

- Members are sorted by path and timestamps are fixed; identical inputs
  produce identical bytes.
- `Open` verifies each member's size and SHA-256 and rejects duplicate,
  unlisted, or escaping members.
- `gomrbpack list`, `info`, and `extract` inspect and unpack an archive.

## How build works

`build` creates a temporary module and runs `go mod tidy` and `go build`.

```text
<workdir>/
├── app.mrbb
├── main.go
└── go.mod
```

- The required `go-mrbvm` version comes from `gomrbpack`'s build information.
- Use `--replace /path/to/go-mrbvm` against a local checkout (the version
  becomes `v0.0.0` in that case).
- `--workdir` keeps the generated files; otherwise a temporary directory is
  removed unless `--keep-work` is set.
- `--goos`, `--goarch`, `--tags`, `--ldflags`, `--strip`, and
  `--no-trimpath` are forwarded to `go build`. When the target is Windows and
  `-o` has no extension, `.exe` is appended automatically.
- `--with-compiler` links go-mrbc and permits `eval`. `go mod tidy` resolves
  the dependency. The binary is larger.
- The `go` command and module downloads (or a populated module cache) are
  required.

## Benchmark

`make bundle-bench` measures startup-to-exit time and binary size for the
sample app in `benchmark/bundle/`, writing `benchmark/bundle-latest.txt`.
`benchmark/bundlebench` compares three ways to run the same program:

- `gomruby main.rb` — compile source at startup
- `gomruby app.mrbb` — run the precompiled archive
- `built app` — the `gomrbpack build` single binary

The first baseline is
[`benchmark/bundle-baseline-2026-11-28.txt`](../benchmark/bundle-baseline-2026-11-28.txt).
On an Apple M4 / macOS arm64 / go1.27.4, with 20 runs, the medians were:

| Invocation | Median | Min |
| --- | ---: | ---: |
| `gomruby main.rb` | 193.62 ms | 189.57 ms |
| `gomruby app.mrbb` | 6.53 ms | 4.97 ms |
| `built app` | 6.35 ms | 3.56 ms |

Sizes: `gomruby` 14.5 MiB, the generated binary 8.9 MiB, and `app.mrbb`
1.1 KiB. Runtime startup is nearly identical; most of the difference is the
startup compile. Results are machine-dependent; re-run per release on the
same host.

CI runs `make bundle-smoke`, which packs, builds, and runs the sample and
checks that all three forms print the same output. The nightly Bench workflow
runs `make bundle-bench` and uploads both benchmark files as artifacts.

## Limitations

- The default binary has no `eval` compiler. Use `--with-compiler` or supply
  `bundle.Options.Compiler` in a custom wrapper.
- Bundled data supports `File.read`, `File.exist?`, and `File.size` only.
  `File.open`, directory listing, and writes still use the host filesystem
  and its capabilities.
- The self-extracting stub approach (appending the archive to an existing
  binary) is not used. `go build` is the supported path because appending
  bytes breaks macOS code signatures.
