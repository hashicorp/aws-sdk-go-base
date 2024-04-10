TIMEOUT ?= 30s

default: test lint

cleantidy:
	@echo "make: tidying Go mods..."
	@cd tools && go mod tidy && cd ..
	@cd v2/awsv1shim && go mod tidy && cd ../..
	@go mod tidy
	@echo "make: Go mods tidied"

fmt:
	gofmt -s -w ./

golangci-lint:
	@golangci-lint run ./...
	@cd v2/awsv1shim && golangci-lint run ./...

importlint:
	@impi --local . --scheme stdThirdPartyLocal ./...

lint: golangci-lint importlint

semgrep:
	@docker run --rm --volume "${PWD}:/src" returntocorp/semgrep semgrep --config .semgrep --no-rewrite-rule-ids

test:
	go test -timeout=$(TIMEOUT) -parallel=4 ./...
	cd v2/awsv1shim && go test -timeout=$(TIMEOUT) -parallel=4 ./...

tools:
	cd tools && go install github.com/golangci/golangci-lint/cmd/golangci-lint
	cd tools && go install github.com/pavius/impi/cmd/impi

# Please keep targets in alphabetical order
.PHONY: \
	cleantidy \
	fmt \
	golangci-lint \
	importlint \
	lint \
	test \
	test \
	tools \