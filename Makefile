.PHONY: run build test tidy docker-up docker-down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./... -v

tidy:
	go mod tidy

docker-up:
	docker run -d --name bookify-postgres \
		-e POSTGRES_DB=bookify \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD=postgres \
		-p 5432:5432 postgres:16-alpine
	@echo "PostgreSQL started. Waiting for ready..."
	@sleep 2

docker-down:
	docker stop bookify-postgres && docker rm bookify-postgres

clean:
	rm -rf bin/
