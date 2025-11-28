-include .env

### Execute on local machine
.PHONY: prep build-local build test swag templ notify-templ-proxy air

prep:
	@go get -tool github.com/a-h/templ/cmd/templ@latest
	@go get -tool github.com/air-verse/air@latest
	@go get -tool github.com/swaggo/swag/cmd/swag@latest
	@npm install
	@cp .env.example .env
	@cp config.example.yaml config.yaml

build-local:
	@go build -o ./bin/main cmd/server/main.go

build:
	@npm run build
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/main cmd/server/main.go

test:
	@go test ./... -cover

swag:
	@go tool swag init -g ./internal/embed.go -o ./internal/docs

templ:
	@go tool templ generate --watch --proxy=http://localhost:$(SERVER_PORT) --proxyport=$(TEMPL_PROXY_PORT) --open-browser=false --proxybind="0.0.0.0"

notify-templ-proxy:
	@go tool templ generate --notify-proxy --proxyport=$(TEMPL_PROXY_PORT)

air:
	@trap 'make docker-down; exit' INT TERM; \
	make templ & sleep 1; \
	make docker-up-keycloak; \
	go tool air; \
	make docker-down

### Execute using docker-compose
.PHONY: docker-build docker-up docker-up-keycloak docker-down

docker-build:
	@docker-compose -f docker-compose.yml --profile whole build --no-cache

docker-up:
	@docker-compose -f docker-compose.yml --profile whole up --no-recreate

docker-up-keycloak:
	@docker-compose -f docker-compose.yml up --remove-orphans --detach

docker-down:
	@docker-compose -f docker-compose.yml --profile whole down
