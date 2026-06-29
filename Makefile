.PHONY: build run test vet up down clean
build:
	go build -o bin/deviceemu ./cmd/deviceemu
run:
	go run ./cmd/deviceemu --config configs/config.yaml
test:
	go test ./...
vet:
	go vet ./...
up:
	docker-compose -f deploy/docker-compose.yaml up -d
down:
	docker-compose -f deploy/docker-compose.yaml down
clean:
	rm -rf bin/