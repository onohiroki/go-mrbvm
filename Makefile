GO ?= go
BINARY_DIR ?= bin

.PHONY: build test vet fmt clean corpus

build:
	$(GO) build -o $(BINARY_DIR)/ ./cmd/...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

corpus:
	$(GO) run ./cmd/vmcompat -corpus

clean:
	rm -rf $(BINARY_DIR)
