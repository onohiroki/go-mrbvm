# Contributing

These repositories form one toolchain:

- [`go-mrbc`](https://github.com/onohiroki/go-mrbc) compiles Ruby to RITE 04.00
  bytecode.
- [`go-mrbvm`](https://github.com/onohiroki/go-mrbvm) executes that bytecode.

Read [`docs/ecosystem.md`](docs/ecosystem.md) before changing bytecode,
opcode semantics, or the shared conformance corpus.

## Where a change belongs

| Change | Repository |
| --- | --- |
| Parser, lowering, compiler, register allocation | `go-mrbc` |
| RITE writer, opcode codec, `bytecode` package | `go-mrbc` |
| Shared conformance corpus and expected output | `go-mrbc` |
| Interpreter, object model, core library | `go-mrbvm` |
| Sandbox, embedding API, commands | `go-mrbvm` |
| Official core or mrbgem test manifests | `go-mrbvm` |
| Instruction meaning or bytecode layout | both, with a conformance case |

An issue that spans both repositories should start with the compiler-side
change and a failing conformance case, then add the VM-side implementation.

## Development layout

`go-mrbvm/go.mod` expects the sibling checkout:

```text
workspace/
├── go-mrbc/
└── go-mrbvm/
```

Keep the `replace ../go-mrbc` directive until a compiler tag is being used for
a release. See [`docs/releasing.md`](docs/releasing.md).

## Before opening a pull request

Both repositories:

```sh
gofmt -l .
go vet ./...
go test ./...
```

Compiler changes, when mruby 4.0.0 is available:

```sh
go run ./cmd/compat -fail mruby/test mruby/mrbgems
go run ./cmd/runtimecompat -fail -assert mruby/test/assert.rb mruby/test/t mruby/mrbgems
```

VM changes:

```sh
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

Do not add a failing suite to a manifest. Record intentional exclusions in
`docs/roadmap.ja.md` (VM) or `docs/compatibility.md` (compiler) with the reason.

## Style

- Keep the code pure Go; do not add cgo or link C mruby.
- The compiler must not import the VM. The VM may import only
  `go-mrbc/bytecode`; evaluator compilation goes through `Options.Compiler`.
- Prefer native implementations over loading mruby mrblib fragments whose C
  half carries the semantics.
- Keep comments focused on observable behavior and compatibility reasons.
- Add a conformance case when fixing a compiler or VM semantic difference.

## License

By contributing, you agree that your contribution is licensed under the MIT
License of the repository. See `LICENSE`. mruby is used as the specification
and test reference; do not copy C mruby code into these repositories.
