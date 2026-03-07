.PHONY: build run test proto clean

build:
	go build -o ratchet ./cmd/ratchet

run: build
	./ratchet

test:
	go test ./...

proto:
	protoc --go_out=. --go-grpc_out=. internal/proto/ratchet.proto

clean:
	rm -f ratchet
