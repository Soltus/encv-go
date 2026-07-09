.PHONY: build run clean test

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/preview-gateway ./cmd/preview-gateway

run: build
	./bin/preview-gateway

test:
	go test ./... -v -count=1

clean:
	rm -rf bin/
