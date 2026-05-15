VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/CarriedWorldUniverse/acp-claude-pty/internal/version.Version=$(VERSION)

.PHONY: build test vet version clean

build:
	go build -ldflags '$(LDFLAGS)' -o bin/acp-claude-pty ./cmd/acp-claude-pty

test:
	go test -race ./...

vet:
	go vet ./...

version:
	@echo $(VERSION)

clean:
	rm -rf bin/
