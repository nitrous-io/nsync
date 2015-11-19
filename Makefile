NO_COLOR=\x1b[0m
OK_COLOR=\x1b[32;01m
ERROR_COLOR=\x1b[31;01m
WARN_COLOR=\x1b[33;01m

export ROOTDIR=$(CURDIR)

all: osx windows linux

osx:
	@mkdir -p dist/
	@echo "$(OK_COLOR)--> Compiling for OSX...$(NO_COLOR)"
	go build -v -o dist/bin/osx64/nsync

windows:
	@mkdir -p dist/
	@echo "$(OK_COLOR)--> Compiling for Windows (win32)...$(NO_COLOR)"
	GOARCH=386 GOOS=windows go build -v -o dist/bin/win32/nsync.exe

linux:
	@mkdir -p dist/
	@echo "$(OK_COLOR)--> Compiling for Linux (amd64)...$(NO_COLOR)"
	GOARCH=amd64 GOOS=linux go build -v -o dist/bin/lin64/nsync

deps:
	go get -d -v ./...

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)--> Testing...$(NO_COLOR)"
	@go list -f '{{range .TestImports}}{{.}}\
		{{end}}' ./... | xargs -n1 go get -d
	go test ./...

.PHONY: all osx windows linux format test deps
