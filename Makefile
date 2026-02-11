.PHONY: build run clean deps test

BINARY := cc-web
GO := go

build: deps
	$(GO) build -o $(BINARY) ./cmd/gateway

run: build
	./$(BINARY) -config configs/config.yaml

deps:
	$(GO) mod tidy

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
	rm -f sessions.json
