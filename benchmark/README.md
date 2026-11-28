# Benchmarks

Microbenchmarks live in `mrbvm/benchmark_test.go`. They measure VM execution
with precompiled bytecode and report allocations per operation.

```sh
make bench
```

The command writes `benchmark/latest.txt`. Keep a baseline for each release
when the machine and Go version are recorded alongside it. Benchmarks are
machine-dependent; do not use cross-machine numbers as a hard CI gate.

## Bundle benchmark

`make bundle-bench` runs `benchmark/bundlebench`, which builds `gomruby`,
`gomrbpack`, and a sample single binary from `benchmark/bundle/`, then
compares wall-clock startup for `gomruby main.rb`, `gomruby app.mrbb`, and the
built binary. The output also records the three binary sizes. It writes
`benchmark/bundle-latest.txt`; the first baseline is
`bundle-baseline-2026-11-28.txt`.

`make bundle-smoke` performs the same build and runs each form once, checking
that all three produce identical output. It is the CI gate; timing numbers are
informational only.

To compare two runs, install `benchstat` and run:

```sh
benchstat benchmark/baseline-v0.1.1.txt benchmark/latest.txt
```

A release should investigate a consistent regression of more than roughly
10–20% in the main loops (`ArithmeticLoop`, `MethodCall`, `ArrayMap`,
`HashWrite`, `StringAppend`, `Rescue`). The nightly benchmark workflow uploads
the output as an artifact for trend tracking; it does not fail the build on
noise.
