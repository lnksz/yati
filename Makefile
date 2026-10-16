GO ?= go

.PHONY: build cli test vet
build:
	$(GO) build -o yati .

cli:
	CGO_ENABLED=0 $(GO) build -o yati .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
