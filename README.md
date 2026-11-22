# go-mrbvm

A pure-Go implementation of the mruby 4.0.0 virtual machine.

`go-mrbvm` executes **RITE 04.00** bytecode produced by
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) or by mruby 4.0.0's `mrbc`.
It is the runtime side of the macro-language stack; `go-mrbc` remains the
compiler and the reference implementation is always C mruby 4.0.0.

## Status

**VM-4b, VM-4c, VM-5, and the VM-6 core are implemented; VM-7 has started.** The VM loads RITE 04.00 through
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
- a minimal eager `Enumerator` for blockless `each` / `map` / `times`
  and friends, including `next`, `peek`, `rewind`, `size`, and `with_index`
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
  `10error`, `compar`, `enum`, `array`, `hash`, `string`, `range`
- a host API: `DefineFunction`, `DefineClass`, `DefineModule`,
  `DefineConst`, `Send`, `Raise`, and value conversion helpers
- sandbox limits: instructions, wall time, call depth, output bytes,
  array/hash/string sizes
- a capability policy package (`sandbox.Policy`) for IO, files, network,
  process, OS, eval, load, and require
- `Time` (`now`, `at`, components, `strftime`, arithmetic)
- `JSON` (`parse`, `generate`, `dump`, `pretty_generate`)
- capability-gated `IO` / `File` with `STDIN` / `STDOUT` / `STDERR`,
  read/write/open/close, path helpers, size, and delete
- `Regexp` / `MatchData` backed by Go regexp, including flags, named
  captures, `String#=~`, `#match`, `#scan`, and regexp `sub` / `gsub`
- cooperative `Fiber` (`current` / `resume` / `yield` / `raise`) with
  nested stacks and ensure cleanup
- `gomirb`, a persistent-State REPL with multi-line continuation,
  history persistence, and completion candidates
- VM-7 benchmark tooling, register pooling, and a GC shim
- `cmd/vmcompat` for differential runs against the reference mruby
- `cmd/mrbtest` plus `testdata/official.txt` for official mruby tests

The shared conformance corpus (53 cases) lives in the sibling `go-mrbc`
repository as the importable `conformance` package. Every case passes with
bytecode from both `go-mrbc` and the official mruby 4.0.0 `mrbc`.

VM-3 leftovers are backtraces and richer exception introspection. VM-4 has
started with the native core library and the differential runner; the next
step is loading mruby's Ruby-level `mrblib` and widening official test
coverage.

Design and plan:

- [docs/architecture.md](docs/architecture.md) — architecture and design
- [docs/implementation-plan.ja.md](docs/implementation-plan.ja.md) — implementation plan (Japanese)
- [docs/roadmap.ja.md](docs/roadmap.ja.md) — phases, sprint plan, and quality gates (Japanese)
- [docs/benchmarks.ja.md](docs/benchmarks.ja.md) — microbenchmarks and C mruby comparison (Japanese)
- [docs/bytecode-format.md](docs/bytecode-format.md) — RITE 04.00 format

## Relationship to go-mrbc

```text
go-mrbvm ──imports──▶ go-mrbc/bytecode
go-mrbc  ──no dependency on──▶ go-mrbvm
```

The shared contract is the `bytecode` package:

- RITE 04.00 container
- `Irep`, pools, symbols, catch handlers
- instruction encoding/decoding

`go-mrbvm` does not copy the bytecode implementation. During local
development, `go.mod` uses a `replace` directive for the sibling
`../go-mrbc` checkout. Once `go-mrbc` has a tagged release, the replace will
be removed.

## Layout

```text
go-mrbvm/
├── mrbvm/                 public embedding API
├── internal/              interpreter, objects, GC, core library
├── sandbox/               resource limits and capability policy
├── cmd/                   gomruby, gomirb, vmcompat, mrbtest, vmbench
├── docs/                  plan and bytecode format
├── examples/              sample macro scripts
└── testdata/official.txt  manifest of passing official mruby tests
```

## Development

Requirements:

- Go 1.24 or newer
- mruby 4.0.0 (`mruby`, `mrbc`) for differential tests

```sh
go build ./...
go test ./...

# Differential run against the reference mruby.
go run ./cmd/vmcompat -corpus

# Run the manifest of official mruby tests that currently pass.
go run ./cmd/mrbtest -manifest testdata/official.txt
```

The manifest currently covers **31** official test files, including
`array`, `object`, `superclass`, `version`, `platform`, the exception-class
tests, `nameerror`, and `nomethoderror`.

## Host API

```go
state := mrbvm.New(mrbvm.Options{})
state.DefineFunction("host_greet", func(ctx context.Context, s *mrbvm.State, self mrbvm.Value, args []mrbvm.Value, block mrbvm.Value) (mrbvm.Value, error) {
	return mrbvm.StringValue("hi " + args[0].String()), nil
})
state.DefineConst(nil, "ANSWER", mrbvm.IntValue(42))
result, err := state.Send(ctx, state.TopSelf, "ruby_method", mrbvm.IntValue(1))
```

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
	MaxTime:         2 * time.Second,
})
```

The conformance corpus is embedded by the sibling `go-mrbc` repository's
`conformance` package, so compiler and VM always run the same cases.

## Commands

- `gomruby` — run `.mrb` bytecode, or compile `.rb` with `go-mrbc`
- `vmcompat` — compare go-mrbc/mrbc bytecode against the reference `mruby`
- `mrbtest` — run official mruby test files with `assert.rb`
- `gomirb` — persistent-State REPL
- `vmbench` — measure a script and optionally compare with reference `mruby`

## License

The original source code and documentation in this repository are licensed
under the **MIT License**; see [LICENSE](LICENSE).

The `mrbvm` library consumes [go-mrbc](https://github.com/onohiroki/go-mrbc)'s
`bytecode` package (MIT). The `gomruby` command can also link the compiler
stack:

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
