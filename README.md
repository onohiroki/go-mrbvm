# go-mrbvm

A pure-Go implementation of the mruby 4.0.0 virtual machine.

`go-mrbvm` executes **RITE 04.00** bytecode produced by
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) or by mruby 4.0.0's `mrbc`.
It is the runtime side of the macro-language stack; `go-mrbc` remains the
compiler and the reference implementation is always C mruby 4.0.0.

## Status

**VM-4 is in progress.** The VM loads RITE 04.00 through
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
- `cmd/vmcompat` for differential runs against the reference mruby

The following 31 shared conformance cases pass with bytecode from both
`go-mrbc` and the official mruby 4.0.0 `mrbc`:

`advanced_classes`, `alias_undef`, `assignment_targets`, `basic_hello`,
`blocks`, `case_and_logic`, `classes`, `collections`, `control_flow`,
`core_library`, `core_library_extended`, `destructuring`, `ensure_control`, `exceptions`, `for_blocks`,
`for_loops`, `keywords`, `lambda`, `lambda_super`, `methods`,
`modern_syntax`, `multiple_assignment`, `operator_assignment`,
`parameters`, `pattern_matching`, `patterns_extended`, `retry_redo`,
`runtime_edges`, `scalars`, `semantic_edges`, `splat`.

VM-3 leftovers are backtraces and richer exception introspection. VM-4 has
started with the native core library and the differential runner; the next
step is loading mruby's Ruby-level `mrblib` and widening official test
coverage.

Design and plan:

- [docs/architecture.md](docs/architecture.md) — architecture and design
- [docs/implementation-plan.ja.md](docs/implementation-plan.ja.md) — implementation plan (Japanese)
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
├── cmd/                   gomruby, gomirb
├── docs/                  plan and bytecode format
├── examples/              sample macro scripts
└── testdata/conformance/  shared behaviour corpus
```

## Development

Requirements:

- Go 1.24 or newer
- mruby 4.0.0 (`mruby`, `mrbc`) for differential tests

```sh
go build ./...
go test ./...

# Differential run against the reference mruby.
go run ./cmd/vmcompat testdata/conformance/*/main.rb
```

The `testdata/conformance` corpus is copied from `go-mrbc` for bootstrapping.
It will later move to an importable shared package.

## Commands

- `gomruby` — run `.mrb` bytecode, or compile `.rb` with `go-mrbc`
- `vmcompat` — compare go-mrbc/mrbc bytecode against the reference `mruby`
- `gomirb` — planned interactive shell

## License

MIT. See [LICENSE](LICENSE). mruby, Prism, go-mrbc, and ChibiRuby are
distributed under their respective licenses.
