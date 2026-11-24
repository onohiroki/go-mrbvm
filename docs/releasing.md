# Releasing go-mrbc and go-mrbvm

The two modules are released in dependency order. `go-mrbvm` imports
`go-mrbc/bytecode`, so the compiler tag must exist before the VM tag.

Replace the example version `v0.1.0` with the version being released.

`origin` is the public GitHub remote and `private` is the Gitea
development remote. Push release tags to both.

## 1. Prepare a release candidate

Both repositories:

```sh
git status --short          # must be empty
gofmt -l .                  # must print nothing
go vet ./...
go test ./...
```

Run the reference suites from a checkout that has mruby 4.0.0 on `PATH`:

```sh
# go-mrbc
go run ./cmd/compat -fail mruby/test mruby/mrbgems
go run ./cmd/runtimecompat -fail -assert mruby/test/assert.rb mruby/test/t mruby/mrbgems
```

```sh
# go-mrbvm, with the sibling ../go-mrbc checkout
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

Update the status counts and known exclusions in the README and roadmap before
tagging. Do not tag a tree whose documentation describes a different test
result.

## 2. Bump the version

In `go-mrbc`, update `mrbc.Version`. In `go-mrbvm`, update
`mrbvm.Version`; `gomruby -v` and `gomirb -v` print it.

Confirm the version constants agree with the tag:

```sh
go run ./cmd/gomrbc --version
go run ./cmd/gomruby -v
go run ./cmd/gomirb -v
```

## 3. Tag and publish go-mrbc

```sh
cd go-mrbc
git tag -a v0.1.0 -m "go-mrbc v0.1.0"
git push private v0.1.0
git push origin v0.1.0
```

Verify that the module proxy can resolve the tag:

```sh
GOPROXY=https://proxy.golang.org go list -m github.com/onohiroki/go-mrbc@v0.1.0
```

If the module contents are not public yet, configure the repository as
public before this step. `go-mrbvm`'s CI uses the repository token fallback,
but external users cannot install an unreleased module.

## 4. Pin go-mrbvm to the compiler tag

```sh
cd go-mrbvm
go mod edit -dropreplace=github.com/onohiroki/go-mrbc
go get github.com/onohiroki/go-mrbc@v0.1.0
go mod tidy
```

Check both the direct `bytecode` import and the command dependencies:

```sh
go list -m all | grep go-mrbc
go test ./...
go run ./cmd/vmcompat -corpus
go run ./cmd/mrbtest -manifest testdata/official.txt
go run ./cmd/mrbtest -manifest testdata/official-gems.txt
```

Commit the `go.mod`, `go.sum`, and release-document changes separately from
implementation work.

## 5. Tag and publish go-mrbvm

```sh
cd go-mrbvm
git tag -a v0.1.0 -m "go-mrbvm v0.1.0"
git push private v0.1.0
git push origin v0.1.0
```

Verify the install path with a clean module cache and a temporary `GOBIN`:

```sh
tmp=$(mktemp -d)
GOBIN="$tmp" GOPROXY=https://proxy.golang.org \
  go install github.com/onohiroki/go-mrbvm/cmd/gomruby@v0.1.0
"$tmp/gomruby" -v
```

Run a small bytecode program through the installed command, then remove the
temporary directory.

## 6. Release notes

Each release should state:

- the `go-mrbc` version required by `go-mrbvm`
- the mruby and RITE versions targeted
- new opcodes or bytecode-format changes, if any
- the official test counts and documented exclusions
- any intentional differences from C mruby, such as UTF-8 `String`
  indexing in `go-mrbvm`

## Compatibility policy

- RITE 04.00 and mruby 4.0.0 are the fixed target for the current series.
- A change to opcode semantics requires a compiler commit, a VM commit, and a
  shared conformance case in the same release cycle.
- A `go-mrbc` release that changes the supported bytecode subset should note
  the minimum `go-mrbvm` version in its release notes.
- Removing the local replace is a release step, not a development step; keep
  `replace ../go-mrbc` in the working tree until the compiler tag exists.

## Rollback

Never move or delete a tag that the module proxy may already have served.
Publish a corrected patch release instead. To warn users away from a bad
version, add a `retract` directive to the latest `go.mod`:

```text
retract (
    v0.1.0 // released with a broken bytecode table
)
```

Then tag the fixed release, for example `v0.1.1`, and update the dependent
`go-mrbvm` module. `go help retract` describes the directive in detail.
