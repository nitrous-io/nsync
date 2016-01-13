NO_COLOR=\x1b[0m
OK_COLOR=\x1b[32;01m
ERROR_COLOR=\x1b[31;01m
WARN_COLOR=\x1b[33;01m

export ROOTDIR=$(CURDIR)
export NSYNC_LDFLAGS="-X main.version=`cat VERSION`-`git rev-parse --short HEAD`"
export NSYNC_SSH_LDFLAGS="-X main.version=`cat nsync-ssh/VERSION`-`git rev-parse --short HEAD`"

all: osx windows linux

osx:
	@mkdir -p dist/
	@cp -af bin/osx64/ dist/osx64/
	@echo "$(OK_COLOR)--> Compiling for OSX...$(NO_COLOR)"
	GO15VENDOREXPERIMENT=1 GOARCH=amd64 GOOS=darwin go build -v -ldflags $(NSYNC_LDFLAGS) -o dist/osx64/nsync
	GO15VENDOREXPERIMENT=1 GOARCH=amd64 GOOS=darwin go build -v -ldflags $(NSYNC_SSH_LDFLAGS) -o dist/osx64/nsync-ssh github.com/nitrous-io/nsync/nsync-ssh

windows:
	@mkdir -p dist/
	@cp -af bin/win32/ dist/win32/
	@echo "$(OK_COLOR)--> Compiling for Windows (win32)...$(NO_COLOR)"
	GO15VENDOREXPERIMENT=1 GOARCH=386 GOOS=windows go build -v -ldflags $(NSYNC_LDFLAGS) -o dist/win32/nsync.exe
	GO15VENDOREXPERIMENT=1 GOARCH=386 GOOS=windows go build -v -ldflags $(NSYNC_SSH_LDFLAGS) -o dist/win32/nsync-ssh.exe github.com/nitrous-io/nsync/nsync-ssh

linux:
	@mkdir -p dist/
	@cp -af bin/lin64/ dist/lin64/
	@echo "$(OK_COLOR)--> Compiling for Linux (amd64)...$(NO_COLOR)"
	GO15VENDOREXPERIMENT=1 GOARCH=amd64 GOOS=linux go build -v -ldflags $(NSYNC_LDFLAGS) -o dist/lin64/nsync
	GO15VENDOREXPERIMENT=1 GOARCH=amd64 GOOS=linux go build -v -ldflags $(NSYNC_SSH_LDFLAGS) -o dist/lin64/nsync-ssh github.com/nitrous-io/nsync/nsync-ssh

deps:
	GO15VENDOREXPERIMENT=1 go get -d -v ./...

savedeps:
	GO15VENDOREXPERIMENT=1 godep save ./...

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)--> Testing...$(NO_COLOR)"
	@go list -f '{{range .TestImports}}{{.}}\
		{{end}}' ./... | xargs -n1 go get -d
	env GO15VENDOREXPERIMENT=1 go test ./nsync/...
	env GO15VENDOREXPERIMENT=1 go test ./pkg/...

.PHONY: all osx windows linux format test deps savedeps
