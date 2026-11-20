GO ?= go
BINARY_DIR ?= bin

.PHONY: build test vet fmt clean

build:
	$(GO) build -o $(BINARY_DIR)/ ./cmd/...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BINARY_DIR)
