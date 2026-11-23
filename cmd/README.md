# Commands

- `gomruby` — run mruby 4.0.0 bytecode. A `.rb` argument is compiled with the
  linked `go-mrbc` first. `-v` prints the version.
- `gomirb` — persistent-State REPL with multi-line continuation, history, and
  completion. `-q` suppresses expression results, `-r FILE` preloads a Ruby
  file, and `-H FILE` selects the history file.
- `vmcompat` — differential runner against the reference mruby. `-corpus` runs
  the shared conformance corpus; `-v` prints each comparison; `-strict` fails
  when the reference tools are missing.
- `mrbtest` — runs official mruby test files with `assert.rb`. `-manifest FILE`
  runs a manifest, `-v` prints every assertion label, and `-stress` enables
  stress-only cases.
- `vmbench` — measures a script and optionally compares against reference
  mruby. `-n` sets iterations, `-compare` enables the reference run,
  `-mrbc` compiles with the reference `mrbc`, and the profile flags write CPU
  and heap profiles.

Build any command with:

```sh
go build ./cmd/gomruby
go build ./cmd/gomirb
go build ./cmd/vmcompat
go build ./cmd/mrbtest
go build ./cmd/vmbench
```

The VM library and the compiler are separate repositories; see
[../docs/ecosystem.md](../docs/ecosystem.md).
