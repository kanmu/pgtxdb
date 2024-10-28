.PHONY: test
test: vet
	go test -v -count 1 ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run
