BINARY := nimsforestmastery
MODULE := github.com/nimsforest/nimsforestmastery
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build build-linux run clean vet fmt test docker

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/nimsforestmastery

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY)-linux-amd64 ./cmd/nimsforestmastery

run: build
	./bin/$(BINARY)

clean:
	rm -rf bin/

vet:
	go vet ./...

fmt:
	gofmt -w .

test:
	go test -v ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t registry.nimsforest.com/$(BINARY):latest .
