# Commands

Planned CLI tools:

- `gomruby` — mruby-compatible runner.
  - `gomruby file.mrb`
  - `gomruby file.rb` (compile with `go-mrbc`)
  - `-e 'code'`, `-r library`, `-I path`, `-v`, `-c`
- `gomirb` — interactive shell over `go-mrbc` and `go-mrbvm`.

They will be implemented after the VM core can execute the conformance
corpus. See [../docs/go-mrbvm-plan.md](../docs/go-mrbvm-plan.md).
