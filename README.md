# go-mrbvm

[![CI](https://github.com/onohiroki/go-mrbvm/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/onohiroki/go-mrbvm/actions/workflows/ci.yml)

A pure-Go implementation of the mruby 4.0.0 virtual machine.

`go-mrbvm` executes **RITE 04.00** bytecode produced by
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) or by mruby 4.0.0's `mrbc`.
It is the runtime side of the macro-language stack; `go-mrbc` remains the
compiler and the reference implementation is always C mruby 4.0.0.

## Background

I started this project after attending hadashiA's talk at RubyKaigi 2026,
[mruby on C#: From VM Implementation to Game Scripting](https://rubykaigi.org/2026/presentations/hadashiA.html).
The talk presents [ChibiRuby](https://github.com/hadashiA/ChibiRuby), a pure-C#
mruby VM, and argues that running mruby without C compilation makes it much
easier to embed in cross-platform applications. I came away with the conclusion
that, when mruby is used as an application's macro language, it should be
implemented in the language the application itself is written in. For a Go
application, that means an mruby implementation in Go.

I first built only the VM. It could execute RITE bytecode, but my intended use —
macro scripts supplied as Ruby source — also needed a bytecode compiler. I
discarded that VM and restarted from the compiler.
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) is the compiler, and this
repository is the VM I rebuilt afterwards to run its output.

The compiler was feasible because
[go-ruby-prism](https://github.com/danielgatis/go-ruby-prism) exposes the Prism
parser to pure Go (Prism itself is embedded as WebAssembly), so no Ruby parser
had to be written from scratch. The implementation was written entirely with AI
coding assistance.

## Status

**VM-4 through VM-6 are implemented; VM-7 has started.** The VM loads RITE 04.00 through
`go-mrbc/bytecode` and supports:

- values, objects, classes, methods, blocks, closures, and upvars
- locals, arrays, hashes, ranges, strings, symbols, arbitrary-precision
  integers
- `ENTER` argument binding (required/optional/rest/post/keyword/block/noblock)
- methods, `yield`, `BLOCK` / `BLKPUSH` / `BLKCALL`, `Proc#call`
- classes, modules, inheritance, `super`, `ARGARY`, singleton classes,
  constants, class variables
- arithmetic/comparison opcodes, including official `mrbc`'s optimized
  `ADD`, `ADDI`, `ADDILV`, `SUB`, `EQ`, `LT`, `LE`, `GT`, `GE`
- `raise` / `rescue` / `ensure`, catch handlers, `EXCEPT` / `RESCUE` /
  `RAISEIF`, `MATCHERR`
- non-local `return`, lambda `return` / `break`, `retry` / `redo`,
  and ensure unwinding for `return` / `break` / `next`
- exception classes, `message`, and rescue by class
- array/hash access and pattern helpers
- core `Array`, `Hash`, `String`, `Integer`, `Float`, `Range`, and
  `Object` methods, including `Enumerable`-style `map` / `select` /
  `reject` / `find` / `inject` / `min` / `max` / `sort`, `group_by`,
  `sort_by`, `each_with_object`, `zip`, and `flat_map`
- a native `Enumerable` module that works with user-defined `each`
- lazy `Enumerator` for blockless `each` / `map` / `times` and friends:
  `next`, `peek`, `feed`, `rewind`, `with_index`, `with_object`,
  `Enumerator.new`, `Enumerator.produce`, and the Yielder/Generator classes
- a native `Comparable` module, `Symbol#to_proc`, and working
  `attr_reader` / `attr_writer` / `attr_accessor`
- singleton classes for arbitrary objects (`class << obj`, `def obj.m`),
  `class_eval`, `alias_method`, `remove_method`, Array class constructors,
  subclass arrays, `Module#include?`, splat rescue, and Float/Range
  indexing
- redefinition-aware optimized indexing (`GETIDX` / `SETIDX`), array
  slice assignment, and cycle-safe `inspect`, `join`, and equality
- `NilClass`, `TrueClass`, and `FalseClass` with their logical operators
- exception `backtrace` / `full_message` / `cause`, `$!`, and `FrozenError`
- reflection: `instance_variables`, `instance_variable_get` / `set`,
  `const_get` / `set`, `ancestors`, and `methods`
- an opt-in precompiled mrblib subset (`Options.LoadMrblib`):
  `10error`, `compar`, `enum`, `array`, `hash`, `string`, `range`;
  native implementations stay authoritative where names overlap, so
  `String#sub` / `gsub` keep their Regexp support
- a host API: `DefineFunction`, `DefineClass`, `DefineModule`,
  `DefineConst`, `Send`, `Raise`, and value conversion helpers
- sandbox limits: instructions, wall time, call depth, output bytes
  (stdout and stderr), array/hash/string sizes, active Fiber/Enumerator
  counts, and a coarse `MaxMemory` budget; budgets reset for each
  `Run` / `Execute`
- a capability policy (`sandbox.Policy`) enforced for files, `eval`, `load`,
  and `require`; standard I/O is host-controlled
- `State.Close` stops the fiber scheduler, waits for internal fiber/enumerator
  goroutines, and closes files still open through `File`
- deterministic `Time.now` / `object_id`, active Fiber/Enumerator limits, and a
  coarse `MaxMemory` budget
- seeded random differential runs (`vmcompat -random`) and nightly soak /
  benchmark workflows
- `Rational` (arbitrary-precision exact parts), `Complex`, and the `Math`
  module
- `Method` / `UnboundMethod` reflection (`call`, `arity`, `parameters`,
  `owner`, `receiver`, `unbind` / `bind` / `bind_call`, `super_method`)
- injected-compiler `eval`, `Binding`, `instance_eval` / `class_eval`
  strings, and `load` / `require` / `require_relative`
- `Time` (`now`, `at`, components, `strftime`, arithmetic)
- `JSON` (`parse`, `generate`, `dump`, `pretty_generate`)
- `File` (capability-gated) and `STDIN` / `STDOUT` / `STDERR`
  (host-controlled) with read/write/open/close, path helpers, size, and
  delete
- `Regexp` / `MatchData` backed by the pure-Go `dlclark/regexp2` engine,
  including Ruby option semantics, `inspect` / `to_s` folding, `escape`,
  `union`, named captures, `$~` and `$1`-style globals, and regexp
  `String#index` / `rindex` / `match` / `scan` / `sub` / `gsub`
- cooperative `Fiber` (`current` / `resume` / `yield` / `transfer` /
  `raise`) with per-fiber VM stacks, caller tracking, ensure cleanup, and
  `Fiber[]` / `storage`
- `gomirb`, a persistent-State REPL with multi-line continuation,
  history persistence, and completion candidates
- `gomruby` / `gomirb` switch parsing compatible with mruby / mirb,
  including stdin programs, `-r`, `ARGV`, and `$DEBUG`; `gomruby` also
  supports `-e` and initializes `$0`
- `gomrbpack`, which compiles multiple `.rb` / `.mrb` inputs into a `.mrbb`
  bundle and builds a single executable that embeds it
- an injectable read-only filesystem (`Options.SourceFS`, `Options.LoadPaths`)
  used by `load`, `require`, and `require_relative`, plus `.mrb` support in
  `require`
- VM-7 benchmark tooling, register pooling, and a GC shim
- `cmd/vmcompat` for differential runs against the reference mruby
- `cmd/mrbtest` plus `testdata/official.txt` for official mruby tests

The shared conformance corpus (53 cases) lives in the sibling `go-mrbc`
repository as the importable `conformance` package. Every case passes with
bytecode from both `go-mrbc` and the official mruby 4.0.0 `mrbc`.

The VM is tested against the official mruby 4.0.0 core and mrbgem suites.
The current manifests cover **43 core files** and **35 mrbgem test files**.
`mruby-eval` passes 45/48 in `eval.rb` (one C-helper skip, two stress cases
not run) and 14/14 in `binding.rb`. Remaining Regexp exclusions are listed in
the roadmap; they are constructs the pure-Go `regexp2` engine cannot express,
such as Onigmo subroutine calls and absent repeaters.

Design and plan:

- [docs/ecosystem.md](docs/ecosystem.md) — how go-mrbc and go-mrbvm fit together
- [docs/ecosystem.ja.md](docs/ecosystem.ja.md) — リポジトリの関係（日本語）
- [docs/releasing.md](docs/releasing.md) — release order and module tagging
- [docs/release-notes/v0.1.2.md](docs/release-notes/v0.1.2.md) — v0.1.2 hardening notes
- [docs/architecture.md](docs/architecture.md) — architecture and design
- [docs/implementation-plan.ja.md](docs/implementation-plan.ja.md) — implementation plan (Japanese)
- [docs/roadmap.ja.md](docs/roadmap.ja.md) — phases, sprint plan, and quality gates (Japanese)
- [docs/official-tests.md](docs/official-tests.md) — official suite coverage and exclusions
- [docs/benchmarks.ja.md](docs/benchmarks.ja.md) — microbenchmarks and C mruby comparison (Japanese)
- [benchmark/README.md](benchmark/README.md) — VM benchmark baseline and comparison workflow
- [docs/bytecode-format.md](docs/bytecode-format.md) — RITE 04.00 format
- [docs/bundling.ja.md](docs/bundling.ja.md) — `.mrbb` bundles and single-binary builds (Japanese)
- [CONTRIBUTING.md](CONTRIBUTING.md) — patch checklist and repository routing

## Relationship to go-mrbc

```text
Ruby source ──▶ go-mrbc ──▶ RITE 04.00 ──▶ go-mrbvm
                                      └──▶ mruby 4.0.0 (reference)
```

```text
go-mrbvm ──imports──▶ go-mrbc/bytecode
go-mrbc  ──no dependency on──▶ go-mrbvm
```

`go-mrbc` is the compiler; `go-mrbvm` is the runtime. The VM imports only
`go-mrbc/bytecode`, the shared RITE model and opcode codec. It does not import
the parser or compiler. `eval`, `load`, and `require` compile through
`Options.Compiler`, so an embedding that does not use those operations does not
link a compiler.

The `gomruby` and `gomirb` commands do link both modules: they accept Ruby
source, compile it with `go-mrbc`, and execute the result with `mrbvm`.

The shared contract is deliberately small:

- RITE 04.00 container and `Irep`
- pools, symbols, catch handlers, and debug lines
- instruction encoding/decoding
- the conformance corpus owned by `go-mrbc` and consumed by both

The released module pins `github.com/onohiroki/go-mrbc` v0.1.1. During
coordinated development, a working tree may instead use a `replace` for the
sibling `../go-mrbc` checkout; a published release removes the replace and
pins a tagged compiler version. The order is described in
[docs/releasing.md](docs/releasing.md) and the full split in
[docs/ecosystem.md](docs/ecosystem.md).

## Layout

```text
go-mrbvm/
├── mrbvm/                 public embedding API, interpreter, object model,
│                          core library, Regexp, Fiber, eval, Set/Struct/Data
├── bundle/                .mrbb archive format and embedded-program runtime
├── sandbox/               resource limits and capability policy
├── cmd/                   gomruby, gomirb, gomrbpack, vmcompat, mrbtest,
│                          vmbench
├── docs/                  architecture, ecosystem, roadmap, release process
├── examples/              sample macro scripts
├── testdata/              official mruby test manifests
└── internal/              toolchain checks used by tests
```

The mruby and ChibiRuby directories used by the reference test harness are
not part of the module and are not required to build it.

## Getting started

The v0.1.2 release is available from the Go module proxy:

```sh
go install github.com/onohiroki/go-mrbvm/cmd/gomruby@v0.1.2
go install github.com/onohiroki/go-mrbvm/cmd/gomirb@v0.1.2
```

`gomrbpack` is newer than v0.1.2; build it from a checkout as shown below.

Or build all commands from a checkout:

```sh
git clone https://github.com/onohiroki/go-mrbvm.git
cd go-mrbvm
make build
./bin/gomruby -v
```

`gomruby` runs `.mrb` bytecode directly and compiles `.rb` (and any file that
is neither `.mrb` nor RITE bytecode) through the linked `go-mrbc`. It accepts
the mruby switches `-b`, `-c`, `-d`, `-e`, `-r`, `-v`, `--verbose`,
`--version`, `--copyright`, and `-h`; with no program file it reads standard
input, and trailing arguments become `ARGV`. `$0` and `$DEBUG` are also
initialized.

`gomirb` is the persistent REPL. It accepts mirb's `-d`, `-r`, `-v`,
`--verbose`, `--version`, and `--copyright`, plus the go-mrbvm extensions
`-q` (quiet results) and `-H` (history file). `-r` loads `.rb` or `.mrb`
libraries, and may be repeated. A program file and trailing arguments are
supported, with the arguments exposed as `ARGV`.

### Bundling an application

`gomrbpack` compiles multiple sources into a `.mrbb` archive and can build a
single executable from it:

```sh
gomrbpack pack  --entry main.rb -I lib -o app.mrbb main.rb lib/
gomrbpack run   app.mrbb arg1 arg2
gomrbpack build --entry main.rb -I lib -o app main.rb lib/
./app arg1 arg2
```

The `.rb` inputs are compiled to RITE bytecode at pack time, so the built
binary does not link the compiler (Prism/wazero) and starts without parsing
source. Inside the bundle, `require` / `require_relative` resolve against the
archive through `Options.SourceFS`; the host filesystem is not consulted. The
default capability policy allows only `require` and `load`, so bundled
programs cannot read host files. `gomruby app.mrbb` runs the same archive.

`build` generates a small `main.go` that `//go:embed`s the archive and calls
`bundle.Run`, then runs `go mod tidy` and `go build`. Use `--replace` to point
at a local checkout during development, and `--goos` / `--goarch` for
cross-compilation. See [docs/bundling.ja.md](docs/bundling.ja.md) for the
archive format, path resolution, and embedding without `gomrbpack build`.

## Development

Requirements:

- Go 1.24 or newer
- mruby 4.0.0 (`mruby`, `mrbc`) for differential tests

```sh
gofmt -l .
go vet ./...
go test ./...

# Differential run against the reference mruby.
go run ./cmd/vmcompat -strict -stderr -corpus
go run ./cmd/vmcompat -strict -stderr -random 100 -seed 1

# Run the official mruby test manifests that currently pass.
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

The VM uses UTF-8 character indexing for `String` (`bytesize` and the byte APIs
remain byte-based). The core manifest covers **43** official test files and the
gem manifest (`testdata/official-gems.txt`) covers **35** mrbgem test files,
including Array/Hash/String/Enumerable extensions, Enumerator, Method,
metaprogramming, Rational, Complex, Regexp/MatchData, Fiber, Set, Struct,
Data, and eval/Binding.

The manifests are allowlists of suites that currently pass, not claims that
every mruby test passes. The roadmap records each intentional exclusion and
the reason.

## Host API

```go
state := mrbvm.New(mrbvm.Options{})
state.DefineFunction("host_greet", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
	return mrbvm.StringValue("hi " + args[0].String()), nil
})
state.DefineConst(nil, "ANSWER", mrbvm.IntValue(42))
result, err := state.Send(ctx, state.TopSelf, "ruby_method", mrbvm.IntValue(1))
```

`SetGlobal` initializes global variables such as `$DEBUG` and `$0`; `DefineConst`
can define top-level constants such as `ARGV`.

Sandbox limits are set through `mrbvm.Options`:

```go
policy := sandbox.New().Permit(sandbox.FileRead)
state := mrbvm.New(mrbvm.Options{
	Policy:          policy,
	MaxInstructions: 1_000_000,
	MaxDepth:        100,
	MaxOutputBytes:  64 * 1024,
	MaxArraySize:    10_000,
	MaxHashSize:     10_000,
	MaxStringSize:   1 << 20,
	MaxFibers:       64,
	MaxEnumerators:  64,
	MaxMemory:       16 << 20,
	MaxTime:         2 * time.Second,
	Deterministic:   true,
})
defer state.Close()
```

`Policy` is enforced for `File` reads/writes and for `eval`, `load`, and
`require`. With no `Policy`, those capabilities are denied; the `gomruby` and
`gomirb` commands opt into `sandbox.AllowAll()` because they run local scripts.
`STDIN` / `STDOUT` / `STDERR` remain available so that hosts can collect script
output; use a dedicated writer for isolation.

`Deterministic` fixes `Time.now` and `object_id` for reproducible runs; `Clock` supplies an explicit clock when needed. `State` is owned by one goroutine. `Run` / `Execute` limits are per call, and
`Close` must be called after execution finishes; it is not safe to call
`Close` concurrently with a running script. `Close` is idempotent. A call
after `Close` returns `ErrStateClosed`; a recovered internal panic marks the
State unusable and returns `ErrStatePanicked`.

The conformance corpus is embedded by the sibling `go-mrbc` repository's
`conformance` package, so compiler and VM always run the same cases.

## Commands

- `gomruby` — mruby-compatible runner: `-b`, `-c`, `-d`, `-e`, `-r`, `-v`,
  `--verbose`, `--version`, `--copyright`, stdin, and `ARGV`
- `gomirb` — persistent-State REPL: `-d`, `-r` (`.rb` / `.mrb`),
  `-v`, `--verbose`, `--version`, `--copyright`, plus `-q` / `-H`,
  a program file, and `ARGV`
- `vmcompat` — compare go-mrbc/mrbc bytecode against the reference `mruby`
- `mrbtest` — run official mruby test files with `assert.rb`
- `vmbench` — measure a script and optionally compare with reference `mruby`

## Author

Created and maintained by [onohiroki](https://github.com/onohiroki).

## License

The original source code and documentation in this repository are licensed
under the **MIT License**; see [LICENSE](LICENSE).

The `mrbvm` library consumes [go-mrbc](https://github.com/onohiroki/go-mrbc)'s
`bytecode` package (MIT) and links [dlclark/regexp2](https://github.com/dlclark/regexp2)
(MIT) for regular expressions. The `gomruby` and `gomirb` commands can also
link the compiler stack:

- [go-ruby-prism](https://github.com/danielgatis/go-ruby-prism) — MIT
- [Prism](https://github.com/ruby/prism) — MIT, embedded as `prism.wasm`
- [wazero](https://github.com/tetratelabs/wazero) — Apache-2.0

See [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) and
[third_party/](third_party/) for the notices and full license texts. mruby is
referenced as the bytecode and VM specification; no mruby C code is linked.
ChibiRuby is referenced as prior art; no code is copied.

`.mrb` files generated from user Ruby programs are not claimed by this
project's MIT License. Their licensing is determined by the input program and
the user's own distribution terms.
