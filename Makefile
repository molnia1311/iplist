.PHONY: check lint fmt vet build test tidy clean

# `check` uses only the Go toolchain — runs anywhere Go is installed.
check: fmt vet build test

# `lint` needs golangci-lint. CI always runs it; locally it may be absent.
lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	@test -z "$$(gofmt -l .)" || { echo "unformatted files:"; gofmt -l .; exit 1; }

vet:
	go vet ./...

build:
	go build ./...

test:
	go test -cover ./...

tidy:
	go mod tidy
	@git diff --exit-code go.mod go.sum || { echo "go.mod/go.sum not tidy"; exit 1; }

clean:
	go clean
	rm -rf dist/
