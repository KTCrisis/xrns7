.PHONY: build test vet clean

build:
	go build -o bin/xrns7 ./cmd/xrns7

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin
