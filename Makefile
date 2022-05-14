BINARY_NAME=explain-cloudformation-changeset

SOURCES:=$(shell find . -type f -name '*.go')

all: build test
 
build: ${BINARY_NAME}

${BINARY_NAME}: ${SOURCES}
	go build -o ${BINARY_NAME} main.go

test:
	go test -v main.go
 
run:
	go build -o ${BINARY_NAME} main.go
	./${BINARY_NAME}

deps:
	go mod download

clean:
	go clean
	rm ${BINARY_NAME}