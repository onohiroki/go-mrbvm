# go-mrbvm

A pure-Go implementation of the mruby 4.0.0 virtual machine.

`go-mrbvm` executes **RITE 04.00** bytecode produced by
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) or by mruby 4.0.0's `mrbc`.
It is the runtime side of the macro-language stack; `go-mrbc` remains the
compiler and the reference implementation is always C mruby 4.0.0.

## Status

**VM-1 is in progress.** The VM loads RITE 04.00 through
`go-mrbc/bytecode` and supports:

- values, objects, classes, methods, blocks, closures, and upvars
- locals, arrays, hashes, ranges, strings, symbols
- `ENTER` argument binding (required/optional/rest/post/block)
- methods, `yield`, `BLOCK` / `BLKPUSH` / `BLKCALL`, `Proc#call`
- arithmetic/comparison opcodes, including official `mrbc`'s optimized
  `ADD`, `ADDI`, `ADDILV`, `SUB`, `EQ`, `LT`, `LE`, `GT`, `GE`
- array/hash access and pattern helpers

The following 20 shared conformance cases pass with bytecode from both
`go-mrbc` and the official mruby 4.0.0 `mrbc`:

`advanced_classes`, `alias_undef`, `assignment_targets`, `basic_hello`,
`blocks`, `case_and_logic`, `classes`, `collections`, `control_flow`,
`for_blocks`, `for_loops`, `keywords`, `methods`, `modern_syntax`,
`multiple_assignment`, `operator_assignment`, `parameters`,
`pattern_matching`, `scalars`, `splat`.

VM-2 is largely implemented: classes, modules, inheritance, `super`,
class methods, `class << self`, class variables, keyword arguments, and
pattern-matching helpers. The remaining conformance failures are centred
on exceptions, big integers, and a few block/lambda edge cases.

Next: **VM-3** (`raise` / `rescue` / `ensure` and non-local control flow).

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
```

The `testdata/conformance` corpus is copied from `go-mrbc` for bootstrapping.
It will later move to an importable shared package.

## Planned commands

- `gomruby` — run `.mrb` bytecode, or compile `.rb` with `go-mrbc`
- `gomirb` — interactive shell over `go-mrbc` and `go-mrbvm`

## License

MIT. See [LICENSE](LICENSE). mruby, Prism, go-mrbc, and ChibiRuby are
distributed under their respective licenses.
