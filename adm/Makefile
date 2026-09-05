.PHONY: build test vet lint demo

build:
	go build ./...

vet:
	go vet ./...

test:
	go test -race -count=1 ./...

# Exercise the whole control plane against the in-memory demo fleet.
demo: build
	go run ./cmd/admctl --demo propose "I want my fleet to be secure"
	go run ./cmd/admctl --demo plans
