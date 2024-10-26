.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test: vet
	go test -v -count 1 ./...
