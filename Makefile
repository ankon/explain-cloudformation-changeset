BINARY_NAME=explain-cloudformation-changeset

SOURCES:=$(shell find . -type f -name '*.go')
AWS_EXAMPLES_SOURCES:=$(wildcard aws-examples/SampleChangeSet*.json)
AWS_EXAMPLES_OUTPUTS:=$(patsubst %.json, %.png, $(AWS_EXAMPLES_SOURCES))

CURRENT_GOOS:=$(shell go env GOOS)
CURRENT_GOARCH:=$(shell go env GOARCH)

all: build test aws-examples
 
build: ${BINARY_NAME}

release: ${BINARY_NAME}.darwin-amd64
${BINARY_NAME}.darwin-amd64: ${SOURCES}
	GOOS=darwin GOARCH=amd64 go build -o "$@" main.go

${BINARY_NAME}: ${BINARY_NAME}.${CURRENT_GOOS}-${CURRENT_GOARCH}
	ln "$<" "$@"

lint:
	go vet .

test:
	go test -v main.go
 
run:
	go build -o ${BINARY_NAME} main.go
	./${BINARY_NAME}

aws-examples: ${AWS_EXAMPLES_OUTPUTS}
aws-examples/%.png: ${BINARY_NAME} aws-examples/%.json
	./${BINARY_NAME} --cache-dir aws-examples --graph-output "$@" --change-set-name "$*"

deps:
	go mod download

clean:
	go clean
	rm -f ${BINARY_NAME} ${BINARY_NAME}.*