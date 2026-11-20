# go-mrbvm: a pure-Go mruby VM

Status: initial project plan (2026-11-20)

This document is the starting point for **`go-mrbvm`**, a pure-Go
implementation of the mruby 4.0.0 virtual machine. It is kept in the
`go-mrbc` repository because the compiler and the VM share a bytecode
contract, but the VM itself is a separate project and repository.

New repository: `github.com/onohiroki/go-mrbvm`

Related repositories:

- `github.com/onohiroki/go-mrbc` — compiler, RITE writer/reader, conformance
  corpus, C mruby parity harnesses.
- `github.com/onohiroki/go-mrbvm` — VM, embedding API, sandbox, `gomruby`
  and `gomirb` commands.

The dependency direction is one-way:

```text
go-mrbvm ──imports──▶ go-mrbc/bytecode
go-mrbc  ──no dependency on──▶ go-mrbvm
```

## 1. Goals

- Implement a pure-Go mruby VM that executes RITE 04.00 bytecode produced by
  `go-mrbc` and by mruby 4.0.0's `mrbc`.
- Match the observable behavior of mruby 4.0.0 on the existing conformance
  corpus.
- Provide a practical embedding API for Go tools: define classes and methods
  in Go, call Ruby methods, convert values, map errors, propagate
  `context.Context`, and enforce resource limits.
- Provide `gomruby` (mruby-compatible runner) and `gomirb` (interactive
  shell) as first-class commands.
- Keep the VM usable without cgo or native dependencies.

## 2. Non-goals

- The mruby C API (`mrb_*`) or C extension ABI.
- Bytecode from mruby 3.x or other RITE versions.
- Full CRuby compatibility.
- JIT compilation.
- Running arbitrary C mrbgems.
- A thread-safe single `State`; one `State` per goroutine is the model.
- Reimplementing `go-mrbc`; the VM consumes bytecode.

## 3. Target and references

- Target VM behavior: **mruby 4.0.0**.
- Bytecode format: **RITE 04.00**.
- Reference implementation: mruby 4.0.0 source.
  - `src/vm.c` — opcode semantics.
  - `src/load.c`, `src/dump.c` — RITE format.
  - `src/class.c` — classes, modules, method lookup, `super`.
  - `src/gc.c` — GC behavior and `ObjectSpace`.
  - `src/error.c` — exception and backtrace behavior.
  - `mrbgems/mruby-compiler/src/codegen.c` — how mruby lowers Ruby.
- Compiler and shared bytecode: `go-mrbc`.
- Conformance corpus: `go-mrbc/testdata/conformance` and the mruby 4.0.0 test
  tree.
- Prior art: [ChibiRuby](https://github.com/hadashiA/ChibiRuby), a pure C#
  mruby 4.0 VM. It implements all opcodes, passes mruby's official test
  suite, and provides host bindings, Fibers, a debugger, optional
  Regexp/IO/JSON, and benchmarks. Use it as an architectural reference, not
  as a line-by-line port.

## 4. Repository layout

```text
go-mrbvm/
├── go.mod                       module github.com/onohiroki/go-mrbvm
├── mrbvm/                       public embedding API
│   ├── state.go                 State, options, lifecycle
│   ├── value.go                 Value and conversions
│   ├── class.go                 Class, Module, method definition
│   ├── error.go                 Ruby exception and Go error mapping
│   └── capability.go            optional capability registration
├── internal/
│   ├── vm/                      interpreter loop, frames, opcodes
│   ├── object/                  object model, strings, arrays, hashes
│   ├── core/                    core classes and methods
│   ├── gc/                      object registry, GC shims
│   └── bytecode/                adapters around go-mrbc/bytecode
├── sandbox/                     limits, capability policy, deterministic mode
├── host/                        optional host integrations
├── debugger/                    DAP/debug adapter (later)
├── cmd/
│   ├── gomruby/                 mruby-compatible runner
│   └── gomirb/                  interactive shell
├── testdata/                    VM-specific fixtures
├── docs/
└── Makefile
```

The public package should be small. Keep object-model and interpreter
details under `internal/` until they are proven.

## 5. Shared bytecode contract

`go-mrbvm` imports:

```go
import "github.com/onohiroki/go-mrbc/bytecode"
```

The VM needs:

- `bytecode.ReadBinary` / `ReadBinaryWithOptions`
- `bytecode.Irep`, `PoolEntry`, `CatchHandler`, `Instruction`
- `bytecode.Decode` / `ReadInstructionSequence`
- RITE 04.00 section parsing, including `LVAR` and `DBG`
- Pool kinds: string, int32, int64, float, bigint
- Symbol table and child ireps

Rules:

- The VM must not import `go-mrbc/internal/...`.
- `bytecode` changes that affect the VM must be coordinated with a
  `go-mrbc` release tag.
- During development, use a `go.work` that contains both repositories.
- If `bytecode` grows into a stable independent contract, extract it to
  `go-mruby-bytecode` or `go-rite`. Do not do that before the API settles.

## 6. VM architecture

### 6.1 Value representation

Start with a simple, GC-friendly tagged value:

```go
type Kind uint8

type Value struct {
    kind Kind
    bits uint64 // int64, float64 bits, symbol ID, or small immediate
    obj  *Object
}
```

`Object` is a pointer so the Go garbage collector can see it. Avoid storing
Go pointers in `uint64` (NaN boxing) initially: unsafe pointers are invisible
to the Go GC and make finalizers and memory accounting harder.

Immediate values:

- `nil`, `false`, `true`
- integer (int64; bigint as an object)
- float (float64 bits)
- symbol (interned ID)

Heap objects:

- `String` (UTF-8 bytes, encoding flag)
- `Array`
- `Hash`
- `Range`
- `Proc` / `Block`
- `Class` / `Module` / singleton class
- `Exception`
- `Fiber`
- `Data` (Go host object)
- `Regexp` / `MatchData` (optional)
- `BigInt`, `Rational`, `Complex` (optional)

Optimize the representation later only after benchmarks show it matters.

### 6.2 Object model

Every heap object has a class pointer and instance variables. Suggested
internal shape:

```go
type Object struct {
    class *Class
    ivars map[Symbol]Value
    flags uint32
    data  any // RString, RArray, RHash, ...
}
```

Use concrete structs for common objects (`String`, `Array`, `Hash`, `Proc`,
`Exception`) and an interface or tagged union for the generic `Object`.
Avoid `map[Symbol]Value` on every object; use a compact inline/side table
once profiling shows it matters.

Classes and modules need:

- name, superclass, included modules, prepended modules
- method table (Go funcs and IREP procs)
- constant table
- class variables
- singleton class
- visibility (public/private/protected)
- frozen/tainted flags as needed

Method lookup and `super` must follow mruby 4.0.0 semantics, including
singleton classes, modules, `prepend`, `extend`, and `method_missing`.

### 6.3 Interpreter and call frames

Core frame state:

- `self`
- current irep
- program counter
- register window
- locals/stack
- upvars (closure environment)
- current block / block handler
- method ID and target class for `super`
- catch-handler stack (rescue/ensure)
- call info for backtraces and `caller`

Implement all mruby 4.0 opcodes. Key groups:

- loads/moves/constants/symbols
- arrays, hashes, ranges, strings
- `SEND` / `SENDB` / `SSEND` / `SSENDB` / `SEND0` / `SSEND0`
- `ENTER`, `BLOCK`, `LAMBDA`, `BLKPUSH`, `BLKCALL`, `SUPER`, `ARGARY`
- `GETUPVAR` / `SETUPVAR`, `GETIDX` / `SETIDX`, ivars, cvars, globals
- jumps and boolean tests
- `EXCEPT`, `RESCUE`, `RAISEIF`, `JMPUW`, `MATCHERR`
- `EXT1` / `EXT2` / `EXT3` extended operands

The bytecode decoder can pre-decode instructions at load time. Keep an
un-decoded path for debugging and a decoded path for execution.

### 6.4 Core library

This is the largest part of the project. mruby's runtime is a mix of C core
classes and Ruby `mrblib` code. A pure-Go VM needs both.

Recommended strategy: **hybrid**.

1. Implement primitive and semantic classes in Go:
   - `BasicObject`, `Object`, `Kernel`, `Class`, `Module`
   - `NilClass`, `TrueClass`, `FalseClass`
   - `Integer`, `Float`, `String`, `Symbol`
   - `Array`, `Hash`, `Range`
   - `Proc`, `Method`, `UnboundMethod`
   - `Exception` hierarchy
2. Compile mruby's `mrblib` / gem `mrblib` sources with `go-mrbc` and load
   them at VM boot where practical.
3. Add Go natives for operations that must be fast or need host access.
4. Grow the method surface using mruby's official tests as the checklist.

ChibiRuby's `sig/*.rbs` files are a useful inventory of core classes and
methods. A Go equivalent can be a generated `docs/core-api.md` or a test
manifest.

Do not try to port every mrbgem at once. Start with the core needed by the
conformance corpus.

### 6.5 Exceptions and backtraces

- Implement `raise`, `rescue`, `ensure`, `retry`, `redo`, `return` unwinding.
- Implement mruby's catch-handler table and `JMPUW`.
- Preserve the current file/line using `DBG` and `LVAR` sections.
- Build Ruby backtrace arrays from call frames.
- Map Go panics in host functions to Ruby exceptions; never let a Go panic
  escape a `State` boundary.
- Implement `Exception#message`, `#backtrace`, `#cause`, `#inspect` with
  mruby-compatible behavior.

### 6.6 GC and memory

Start with Go's garbage collector managing memory.

- Keep an object registry for `ObjectSpace`, weak references, finalizers,
  and sandbox accounting.
- Expose `GC.start`, `GC.disable`, `GC.enable`, and the ratio/limit setters
  as best-effort shims.
- Document where behavior differs from mruby's C GC.
- For deterministic execution and strict memory limits, use a VM-level
  allocator/accounting layer rather than relying on `runtime/debug` global
  limits.

A full mark-and-sweep GC may be added later if `ObjectSpace`, finalizers, or
hard memory limits require it. Do not block the first VM on this.

### 6.7 Fibers and concurrency

Fibers are part of mruby and are used by some tests and libraries.

Options:

1. **Goroutine-backed fibers**: each Fiber runs on a goroutine and
   communicates through channels. Simple, but expensive and easy to leak.
2. **Interpreter state machine**: suspend/resume the interpreter loop with
   explicit frame state. More work, but deterministic and cheap.
3. **Defer Fibers** until the core VM and host API are stable.

Start with option 3 unless the conformance corpus needs Fibers early. Design
the call-frame model so option 2 remains possible. Provide `context.Context`
cancellation at instruction boundaries independently of Fibers.

Thread safety: one `State` is owned by one goroutine. Multiple goroutines
use separate `State` instances. Host functions run on the owning goroutine.

### 6.8 Regexp, bigint, and optional features

- **Bigint**: use `math/big` and the `BIGINT` pool entry.
- **Regexp**: Go's `regexp` is RE2 and lacks backreferences/lookbehind. Ruby
  regexp compatibility needs a dedicated engine or a third-party library.
  Make `Regexp` an opt-in capability and document the limitations.
- **Rational / Complex**: optional; use `math/big.Rat` and a complex pair.
- **JSON / IO / HTTP**: separate opt-in Go packages, analogous to
  ChibiRuby's optional `DefineJson` / `DefineIO` / `DefineHttp`.
- **Time / Random**: provide deterministic-mode shims.

## 7. Host API

The host API is what makes `go-mrbvm` a macro language for Go tools. The
first version should be small and explicit.

Provisional sketch:

```go
package mrbvm

type State struct{ /* ... */ }

type Options struct {
    Sandbox       *sandbox.Policy
    Deterministic bool
    LoadPaths     []string
}

func New(opts Options) *State
func (s *State) Close()

func (s *State) LoadBytecode(data []byte) (Value, error)
func (s *State) Execute(ctx context.Context, irep *bytecode.Irep) (Value, error)

func (s *State) DefineFunction(name string, fn Func)
func (s *State) DefineClass(name string, super *Class, configure func(*Class))
func (s *State) DefineModule(name string, configure func(*Module))
func (s *State) DefineConst(scope *Module, name string, value Value)

func (s *State) Send(ctx context.Context, recv Value, name string, args ...Value) (Value, error)
func (s *State) Raise(exceptionClass *Class, message string)

type Func func(ctx context.Context, s *State, self Value, args []Value, block Value) (Value, error)
```

Design points:

- Go `error` returned from a host function becomes a Ruby exception.
- `context.Context` cancellation stops the interpreter at safe points.
- Value conversion helpers: `AsInt`, `AsFloat`, `AsString`, `AsBool`,
  `AsArray`, `AsHash`, `AsGoData`.
- Host objects use a `Data` wrapper with an explicit Go pointer/value.
- Do not expose internal object pointers publicly.
- Keep the API stable enough for tools; version it with semantic tags.

ChibiRuby's host API is a useful ergonomics reference: `DefineClass`,
`DefineMethod`, `DefineConst`, `Send`, `Raise`, `EnsureArgumentCount`,
`MRubyValue`, and optional `DefineRegexp` / `DefineIO` / `DefineJson`.

## 8. Sandbox

A macro language for tools often executes untrusted or semi-trusted
scripts. Sandboxing is a first-class feature, not a later add-on.

Required limits:

- instruction budget (check every N instructions)
- wall-clock timeout via `context.Context`
- maximum call depth
- maximum array/hash/string size
- maximum output size
- memory accounting (coarse at first, precise later)

Capability policy:

- `io`, `file`, `network`, `process`, `os`, `eval`, `load`, `require`
- host-defined functions
- clock and random source
- regexp, bigint, fibers

Deterministic mode:

- fixed random seed
- virtual clock
- stable object IDs or no ID exposure
- stable hash iteration order (mruby hashes preserve insertion order)
- no ambient environment or filesystem access

## 9. Commands

### `gomruby`

mruby-compatible runner.

- `gomruby file.mrb` — load and execute bytecode.
- `gomruby file.rb` — compile with `go-mrbc` and execute.
- `-e 'code'` — compile and run a snippet.
- `-r library` / `-I path` — load precompiled libraries.
- `-v` — version.
- `-c` — compile only (delegates to `go-mrbc`).

### `gomirb`

Interactive shell.

- persistent `State`
- compile each input with `go-mrbc`
- print `inspect` of the result
- multiline input
- history
- `-r` preload
- sandbox option

`gomirb` is a thin CLI over `go-mrbc` and `go-mrbvm`; it does not need its
own repository initially.

## 10. Testing and compatibility

### Reuse the compiler corpus

Expose `go-mrbc` testdata as an importable package:

```go
package conformance

import "embed"

//go:embed testdata
var FS embed.FS
```

Then `go-mrbvm` can run the same cases without copying them.

### Differential execution

Add `cmd/vmcompat`:

1. Compile a fixture with `gomrbc`.
2. Run it with C mruby 4.0.0.
3. Run it with `go-mrbvm`.
4. Compare stdout, stderr, and exit status.

Use the same wrapping approach as `cmd/runtimecompat`: prepend
`test/assert.rb` for mruby's own tests, append a deterministic summary.

### Opcode tests

Every opcode needs a focused test:

- hand-written irep or a tiny Ruby fixture
- register state before/after
- jump offsets
- `EXT1/2/3`
- catch handlers
- upvars and blocks

### Fuzzing

- Fuzz the RITE loader with mutated binaries.
- Fuzz generated Ruby programs (reuse `TestRandomizedDifferential`).
- Fuzz host API argument conversion.
- Stress GC, deep recursion, large arrays/hashes, and exception unwinding.

### Performance

Benchmark against C mruby 4.0.0:

- Fibonacci / method calls
- loops and arithmetic
- string concatenation
- array/hash operations
- block/yield-heavy code
- startup and bytecode load time

ChibiRuby publishes a benchmark against native mruby; do the same and keep
results in `docs/benchmarks.md`.

## 11. Milestones

### VM-0: Skeleton and hello world

- repository scaffold
- import `go-mrbc/bytecode`
- RITE loader
- minimal value/object types
- instruction decoder
- `LOADI`, `STRING`, `SEND`, `RETURN`, `STOP`
- `puts "hello"` end to end

### VM-1: Expressions and calls

- locals, globals, constants
- integers, floats, strings, symbols, arrays, hashes, ranges
- arithmetic and comparison calls
- method calls, blocks, `yield`
- simple `if`/`while`

### VM-2: Object model

- classes, modules, inheritance
- instance/class variables
- singleton classes, `class << self`
- `super`, `define_method`
- keyword arguments, splats, `...`
- closures/upvars

### VM-3: Exceptions and control flow

- `raise`, `begin/rescue/ensure`
- `retry`, `redo`
- catch handlers, `JMPUW`
- backtraces with source lines
- pattern matching support (`MATCHERR`, `deconstruct`, `deconstruct_keys`)

### VM-4: Core library and conformance

- core classes and methods used by the corpus
- mrblib loading strategy
- run the 4.0.0 conformance corpus
- differential harness
- opcode coverage tests

### VM-5: Host API and commands

- `DefineFunction`, `DefineClass`, `Send`, `Raise`
- value conversion
- context cancellation
- sandbox limits
- `gomruby`
- `gomirb`

### VM-6: Optional features

- Fibers
- debugger/DAP
- Regexp
- IO/File
- JSON
- HTTP
- bigint/Rational/Complex completeness
- benchmarks and performance work

## 12. ChibiRuby lessons

ChibiRuby is a strong existence proof and a useful reference:

- A pure managed-language mruby VM can pass mruby 4.0's official tests.
- All opcodes matter; partial opcode support is not enough.
- Host API ergonomics are as important as raw execution:
  `DefineClass`, `DefineMethod`, `Send`, `Raise`, value conversion.
- Optional capabilities (Regexp, IO, JSON) should be opt-in so the default
  host has a smaller attack surface.
- Fibers and async host methods need explicit design; they are not an
  afterthought.
- A debugger (breakpoints, stepping, locals, expression evaluation) is a
  major usability feature.
- Benchmarks against native mruby keep performance honest.
- RBS signature files are a useful checklist of the supported core API.

Caveats:

- C# has a JIT and a mature GC; Go has different tradeoffs.
- ChibiRuby's compiler wrapper uses a native compiler; `go-mrbvm` should use
  `go-mrbc` and stay pure Go.
- Do not copy C# object layouts or async/await mechanics literally.
- ChibiRuby is MIT-licensed. If any code is ported, preserve its license
  notice and attribution.

## 13. Risks and open questions

- **Core library volume**: mruby's core is large. Budget for it explicitly.
- **Regexp semantics**: Go's RE2 is not Ruby-compatible. Decide early
  whether to implement, vendor, or limit Regexp.
- **Fibers**: goroutine-backed fibers are simple but expensive; a state
  machine is better but more work.
- **GC**: Go GC handles memory, but `ObjectSpace`, finalizers, and hard
  memory limits may need a VM-level registry or custom GC.
- **Performance**: interface boxing and dynamic dispatch can be costly.
  Benchmark early and avoid premature NaN boxing.
- **Sandbox memory limits**: Go's GC is global; per-VM accounting needs
  careful design.
- **Encoding**: mruby strings are byte strings with encoding metadata.
- **`eval`/`load`/`require`**: decide whether the VM embeds a runtime
  compiler or delegates to the host.
- **mrbgems**: C extensions are out of scope; pure-Ruby gems can be
  precompiled.
- **Debugger**: DAP is a large project; start with backtraces and a simple
  REPL.

## 14. Development workflow

- Create `go-mrbvm` as a separate repository and Go module.
- During development, use a `go.work` containing both repositories.
- Depend on a tagged `go-mrbc` version in releases.
- Keep the VM's C-mruby differential tests in CI.
- Run the compiler's conformance corpus against the VM as soon as VM-1 is
  usable.
- Do not modify the compiler to accommodate VM bugs unless the bytecode
  contract itself is wrong.

## 15. Definition of done for the first usable version

- `gomruby` runs the conformance corpus compiled by `go-mrbc`.
- The VM passes the mruby 4.0.0 core test subset used by `go-mrbc`.
- Go host functions and classes can be registered and called.
- `gomirb` supports multiline input and preloading.
- Sandbox limits (instructions/time/depth) work.
- Backtraces include source file and line.
- No cgo, no native dependencies.
- Benchmarks are published, even if slower than C mruby.
