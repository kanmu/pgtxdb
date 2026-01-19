.PHONY: test
test: vet
	go tool gotest -v -count 1 ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run
