# Official test triage

The manifests in `testdata/` are allowlists of suites that pass on go-mrbvm.
They are not a claim that the full upstream suite passes.

## Current coverage

| Area | Included | Available | Notes |
| --- | ---: | ---: | --- |
| Core (`mruby/test/t`) | 43 | 50 | missing: `codegen`, `env`, `gc`, `refinement`, `syntax`, `sysfail`, `vformat` |
| mrbgem test files | 35 | 110 | 19 gems; many host-facing gems are not implemented |

`range.rb` and `mruby-string-ext/test/range.rb` were added after a triage run.

## Core exclusions

| File | Reason |
| --- | --- |
| `codegen.rb` | go-mrbc rejects the >255-target multiple assignment case; compiler limitation |
| `env.rb` | depends on the C test helpers `__env_*` and mruby's env internals |
| `gc.rb` | depends on C GC internals and `ObjectSpace` |
| `refinement.rb` | upstream test skips unless `MRB_USE_REFINEMENTS`; no assertions run |
| `syntax.rb` | contains parser/generator constructs go-mrbc does not currently accept |
| `sysfail.rb` | C test helper / GC failure injection |
| `vformat.rb` | depends on the C `TestVFormat` helper |

## mrbgem categories

The missing mrbgem files fall into these groups:

- **Host integration not implemented**: `mruby-socket`, `mruby-dir`,
  `mruby-env`, `mruby-process`, `mruby-signal`, `mruby-io`, `mruby-task`,
  `mruby-random`, `mruby-sleep`.
- **C test helpers / GC internals**: `mruby-objectspace*`,
  `mruby-os-memsize`, `mruby-test-inline-struct`, `mruby-warning`.
- **Encoding / formatting**: `mruby-encoding*`, `mruby-sprintf`,
  `mruby-strftime`, `mruby-pack`.
- **Remaining Regexp cases**: several `mruby-regexp` files use constructs the
  pure-Go `regexp2` engine cannot express (subroutine calls, some lookaround
  combinations, absent repeaters). These should stay documented exclusions
  until an engine change.
- **Feature extensions that may be fixable**: `mruby-proc-ext`,
  `mruby-range-ext`, `mruby-numeric-ext`, `mruby-enum-lazy`,
  `mruby-enum-chain`, `mruby-error`, `mruby-exit`, `mruby-toplevel-ext`.

## Policy

- Add a file only when it contains meaningful assertions and passes.
- Do not add tests whose only result is a skip or an empty total.
- Record a concrete exclusion reason in this file and in
  `docs/roadmap.ja.md`.
- Re-run the complete inventory after adding a feature, not just the
  manifest.
