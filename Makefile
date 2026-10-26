GO ?= go

.PHONY: build release install cli test vet
build:
	$(GO) build -o yati .

release:
	$(GO) build -trimpath -ldflags='-s -w' -o yati .

install: release
	install -Dm755 yati "$$(dirname -- "$${XDG_DATA_HOME:-$$HOME/.local/share}")/bin/yati"

cli:
	CGO_ENABLED=0 $(GO) build -o yati .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
