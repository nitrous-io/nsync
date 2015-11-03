NO_COLOR=\x1b[0m
OK_COLOR=\x1b[32;01m
ERROR_COLOR=\x1b[31;01m
WARN_COLOR=\x1b[33;01m

export ROOTDIR=$(CURDIR)

all:
	@mkdir -p dist/
	@echo "$(OK_COLOR)--> Compiling...$(NO_COLOR)"
	go build -v -o dist/nsync
	@cp -a bin dist

deps:
	go get -d -v ./...

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)--> Testing...$(NO_COLOR)"
	@go list -f '{{range .TestImports}}{{.}}\
		{{end}}' ./... | xargs -n1 go get -d
	go test ./...

.PHONY: all format test deps
