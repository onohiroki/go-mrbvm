GO ?= go
BINARY_DIR ?= bin

.PHONY: build test vet fmt clean corpus soak bench

build:
	$(GO) build -o $(BINARY_DIR)/ ./cmd/...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

corpus:
	$(GO) run ./cmd/vmcompat -strict -stderr -corpus

soak:
	$(GO) test -tags soak ./mrbvm -run '^TestSoak' -count=1 -timeout 15m

bench:
	$(GO) test ./mrbvm -run '^$$' -bench . -benchmem -count=1 | tee benchmark/latest.txt

clean:
	rm -rf $(BINARY_DIR)
