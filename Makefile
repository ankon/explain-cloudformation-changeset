BINARY_NAME=explain-cloudformation-changeset

SOURCES:=$(shell find . -type f -name '*.go')
AWS_EXAMPLES_SOURCES:=$(wildcard aws-examples/SampleChangeSet*.json)
AWS_EXAMPLES_OUTPUTS:=$(patsubst %.json, %.png, $(AWS_EXAMPLES_SOURCES))

all: build test aws-examples
 
build: ${BINARY_NAME}

${BINARY_NAME}: ${SOURCES}
	go build -o ${BINARY_NAME} main.go

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
	rm ${BINARY_NAME}