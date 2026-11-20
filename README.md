# go-mrbvm

A pure-Go implementation of the mruby 4.0.0 virtual machine.

`go-mrbvm` executes **RITE 04.00** bytecode produced by
[`go-mrbc`](https://github.com/onohiroki/go-mrbc) or by mruby 4.0.0's `mrbc`.
It is the runtime side of the macro-language stack; `go-mrbc` remains the
compiler and the reference implementation is always C mruby 4.0.0.

## Status

**VM-0** is implemented: RITE 04.00 loading through `go-mrbc/bytecode`, a
minimal value/object model, `STRING` / `MOVE` / `SSEND` / `RETURN` / `STOP`,
and `Kernel#puts`. `puts "hello"` runs from bytecode produced by both
`go-mrbc` and the official mruby 4.0.0 `mrbc`.

Next: **VM-1** (expressions, locals, arrays/hashes, methods, blocks, `yield`).

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
