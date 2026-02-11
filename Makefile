.PHONY: build run clean deps test

BINARY := cc-web
GO := go

build: deps
	$(GO) build -o $(BINARY) ./cmd/gateway

run: build
	@if [ ! -f configs/config.local.yaml ]; then \
		echo "Error: configs/config.local.yaml not found."; \
		echo "Copy configs/config.yaml and set a real auth_token:"; \
		echo "  cp configs/config.yaml configs/config.local.yaml"; \
		exit 1; \
	fi
	./$(BINARY) -config configs/config.local.yaml

deps:
	$(GO) mod tidy

test:
	$(GO) test ./...

clean:
	rm -f $(BINARY)
	rm -f sessions.json
