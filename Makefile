.PHONY: buf
BUF_VERSION?=1.64.0

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
	#export PATH=$PATH:$(go env GOPATH)/bin
	mockgen --version

mock-gen:
	mockgen -source=./storage/storage.go -destination=./mock/storage.go -package=mock

endian-check:
	lscpu | grep "Byte Order"