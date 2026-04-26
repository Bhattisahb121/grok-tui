.PHONY: build clean run install

BINARY := grok-tui
PKG := ./cmd/grok-tui

build:
	go build -o $(BINARY) $(PKG)

run: build
	./$(BINARY)

install:
	go install $(PKG)

clean:
	rm -f $(BINARY)
	rm -rf dist/

vet:
	go vet ./...

fmt:
	gofmt -w .
