# The go-mrbc / go-mrbvm toolchain

`go-mrbc` and `go-mrbvm` are the two halves of a pure-Go mruby toolchain.

| Repository | Role | Produces / consumes |
| --- | --- | --- |
| [`go-mrbc`](https://github.com/onohiroki/go-mrbc) | Ruby-to-bytecode compiler | Reads Ruby source, produces RITE 04.00 bytecode |
| [`go-mrbvm`](https://github.com/onohiroki/go-mrbvm) | mruby virtual machine | Reads RITE 04.00 bytecode, runs it |

Both target **mruby 4.0.0** and its **RITE 04.00** container. The reference
implementation for compatibility remains C mruby 4.0.0 (`mruby` and `mrbc`).

## Data flow

```text
Ruby source
    |
    v
go-mrbc  (Prism parser -> lowering -> register compiler -> RITE writer)
    |
    v
RITE 04.00 bytecode (.mrb in memory or on disk)
    |
    +-----------------------------+
    |                             |
    v                             v
go-mrbvm                      mruby 4.0.0
(pure-Go VM)                  (reference VM)
```

The compiler does not need the VM to produce bytecode. The VM does not need
the compiler to execute bytecode.

## One-way dependency

```text
go-mrbvm ----imports----> go-mrbc/bytecode
go-mrbc  ----no import----> go-mrbvm
```

The VM imports only `go-mrbc/bytecode`, the shared RITE model and opcode
codec. It does not import the parser, lowering pipeline, or compiler.

`eval`, `load`, and `require` are compiler-backed operations. The core
`mrbvm` package therefore takes a compiler through `Options.Compiler`; an
embedding that never calls those operations does not link the compiler. The
`gomruby` and `gomirb` commands do link both modules because they accept Ruby
source directly.

The dependency direction is deliberate: the compiler stays usable on its
own, and the VM can execute bytecode produced by either the Go compiler or
the reference `mrbc`.

## Division of responsibility

`go-mrbc` owns:

- the Prism AST lowering frontend
- the internal compiler IR, scopes, registers, and code generation
- the RITE 04.00 writer and reader (`bytecode` package)
- opcode definitions and the instruction codec
- the shared conformance corpus in `conformance/testdata/conformance`
- bytecode-level compatibility and compile/runtime parity tests

`go-mrbvm` owns:

- the bytecode interpreter and object model
- core Ruby classes, modules, and language semantics
- `Regexp` / `MatchData`, `Fiber`, `eval` / `Binding`, `Set`, `Struct`,
  `Data`, and the other native mrbgem implementations
- the sandbox and capability policy
- the embedding API in the `mrbvm` package
- the `gomruby`, `gomirb`, `vmcompat`, `mrbtest`, and `vmbench` commands
- official mruby core and mrbgem test manifests

Changes to the bytecode format or opcode semantics belong in both
repositories. A change that alters instruction meaning needs a compiler
commit, a VM commit, and a conformance case.

## Shared contracts

The public contract between the repositories is:

- the RITE 04.00 container layout
- the `bytecode.Irep`, pool, symbol, catch-handler, and debug-line model
- instruction encoding, decoding, and operand widths
- the conformance corpus and its expected stdout files (owned by `go-mrbc`,
  consumed by both)

The corpus is embedded by the `conformance` package in `go-mrbc`.
`go-mrbvm` runs the same cases in its own tests, so a compiler change cannot
silently pass only one side.

## Licensing and dependency files

Both repositories are released under the MIT License. The licensing files are:

| File | Purpose |
| --- | --- |
| `LICENSE` | MIT license for the original source and documentation |
| `NOTICE` | Attribution summary for the distributed binaries |
| `THIRD_PARTY_LICENSES.md` | Component list, versions, licenses, and copyright |
| `third_party/` | Full license texts and notices for bundled components |

Direct and linked dependencies:

| Component | Used by | License |
| --- | --- | --- |
| `go-mrbc/bytecode` | `go-mrbvm` core | MIT |
| [dlclark/regexp2](https://github.com/dlclark/regexp2) | `go-mrbvm` Regexp/MatchData | MIT |
| [go-ruby-prism](https://github.com/danielgatis/go-ruby-prism) | `go-mrbc`, `gomruby`, `gomirb` | MIT |
| [Prism](https://github.com/ruby/prism) (embedded as `prism.wasm`) | through go-ruby-prism | MIT |
| [wazero](https://github.com/tetratelabs/wazero) | WebAssembly runtime for Prism | Apache-2.0 |

mruby (MIT) is the bytecode and VM reference and test input; no mruby C code is
linked. ChibiRuby (MIT) is referenced as prior art; no code is copied.

A source distribution keeps `go.mod`, `go.sum`, `LICENSE`, `NOTICE`,
`THIRD_PARTY_LICENSES.md`, and `third_party/`. A binary distribution includes
the license and notice files for the code it links. The full component tables
are in each repository's `THIRD_PARTY_LICENSES.md`.

## Compatibility target

Both projects target:

- mruby 4.0.0
- RITE 04.00
- the default `mrbc` output format for the supported subset

The compiler's compatibility report is in
[`docs/compatibility-report.md`](https://github.com/onohiroki/go-mrbc/blob/main/docs/compatibility-report.md).
The VM's supported tests and remaining exclusions are in
[`docs/roadmap.ja.md`](https://github.com/onohiroki/go-mrbvm/blob/main/docs/roadmap.ja.md)
(Japanese).

The VM intentionally differs from mruby in one documented policy: `String`
uses UTF-8 character indexing throughout, while byte APIs remain byte-based.
This is part of the public VM contract.

## Development checkouts

During development, `go-mrbvm/go.mod` uses a local replace:

```text
replace github.com/onohiroki/go-mrbc => ../go-mrbc
```

The expected layout is therefore:

```text
workspace/
├── go-mrbc/
└── go-mrbvm/
```

CI checks out both repositories side by side. A published `go-mrbvm`
release removes the replace and pins a tagged `go-mrbc` version; see
[`docs/releasing.md`](releasing.md).

## Versioning and release order

`go-mrbc` is tagged first because `go-mrbvm` depends on its `bytecode`
package. A release sequence is:

1. tag `go-mrbc` (for example `v0.1.0`)
2. update `go-mrbvm` to require that tag and remove the local replace
3. run the full compiler and VM gates
4. tag `go-mrbvm`

The repositories do not share a version number. `go-mrbvm` pins a
semver-compatible `go-mrbc` release and records any required bytecode
compatibility in its release notes.

## Testing map

`go-mrbc`:

```sh
go test ./...
go run ./cmd/compat -fail mruby/test mruby/mrbgems
go run ./cmd/runtimecompat -fail -assert mruby/test/assert.rb mruby/test/t mruby/mrbgems
```

`go-mrbvm`:

```sh
go test ./...
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

The mruby 4.0.0 reference checkout is a test input; it is not vendored or
linked into either module.

## Where to file changes

- Parser, lowering, compiler, bytecode writer, conformance corpus:
  `go-mrbc`
- Interpreter, core library, sandbox, embedding API, official test manifests:
  `go-mrbvm`
- Instruction semantics, RITE layout, shared conformance behavior: both

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) for the patch checklist.
