.PHONY: test build integration fmt vet

test:
	go test -race -coverprofile=coverage.out ./...

build:
	go build -trimpath -o caddy ./cmd/caddy

fmt:
	gofmt -w *.go cmd/caddy/*.go

vet:
	go vet ./...

# Explicitly opt in with environment variables; this target never reads .env.
integration: build
	CADDY_BINARY="$(CURDIR)/caddy" go test -tags integration -run '^TestLive' -v -timeout 12m .
