.PHONY: test build run docker clean

test:
	go test ./...

build:
	go build -v ./cmd/g9router

run:
	go run ./cmd/g9router

docker:
	docker build -t g9router:dev .

clean:
	rm -f g9router
	go clean
