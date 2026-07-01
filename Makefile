.PHONY: build run test proto clean

build:
	go build -o ratchet ./cmd/ratchet

run: build
	./ratchet

test:
	go test ./...

proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/ratchet.proto

clean:
	rm -f ratchet
