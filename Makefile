BINARY_NAME=explain-cloudformation-changeset

SOURCES:=$(shell find . -type f -name '*.go')
AWS_EXAMPLES_SOURCES:=$(wildcard aws-examples/SampleChangeSet*.json)
AWS_EXAMPLES_OUTPUTS:=$(patsubst %.json, %.png, $(AWS_EXAMPLES_SOURCES))

all: build test aws-examples
 
build: ${BINARY_NAME}

release: ${BINARY_NAME}.darwin-amd64

${BINARY_NAME}.%: clean
	GOOS=$(shell echo "$*" | cut -f 1 -d -) GOARCH=$(shell echo "$*" | cut -f 2 -d -) make build && mv ${BINARY_NAME} "$@"

${BINARY_NAME}: ${SOURCES} deps
	go build -o "$@" main.go

lint:
	go vet .

test:
	go test -v main.go
 
run: ${BINARY_NAME}
	./${BINARY_NAME}

aws-examples: ${AWS_EXAMPLES_OUTPUTS}
aws-examples/%.png: ${BINARY_NAME} aws-examples/%.json
	./${BINARY_NAME} --cache-dir aws-examples --graph-output "$@" --change-set-name "$*"

deps:
	go mod download

clean:
	go clean

distclean: clean
	rm -f ${BINARY_NAME} ${BINARY_NAME}.*