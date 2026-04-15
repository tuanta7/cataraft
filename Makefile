.PHONY: buf
BUF_VERSION?=1.64.0
GOBIN:=$(shell go env GOPATH)/bin
MOCKGEN:=$(GOBIN)/mockgen

build:
	go build ./cmd/cataraft

buf-install:
	go install github.com/bufbuild/buf/cmd/buf@v${BUF_VERSION}
	buf --version

buf-export:
	buf dep update
	buf export buf.build/bufbuild/protovalidate --output=.

buf-gen:
	buf dep update
	buf generate

mock-install:
	go install go.uber.org/mock/mockgen@latest
	$(MOCKGEN) --version

mock-gen:
	$(MOCKGEN) -source=./internal/storage/writer/cow/buffer.go -destination=./mocks/cow/copy_on_write.go -package=mockcow

endian-check:
	lscpu | grep "Byte Order"
