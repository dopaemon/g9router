.PHONY: test build run docker

test:
	go test ./...

build:
	go build ./cmd/g9router

run:
	go run ./cmd/g9router

docker:
	docker build -t g9router:dev .
