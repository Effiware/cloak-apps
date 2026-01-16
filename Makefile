-include .env

### Execute on local machine
.PHONY: gen-certs prep build-local build test swag templ notify-templ-proxy air

gen-certs:
	@openssl req -newkey rsa:2048 -nodes -x509 -days 3650 \
		-keyout keycloak-server.key.pem \
		-out keycloak-server.crt.pem \
		-subj "/CN=localhost" \
		-addext "subjectAltName=DNS:localhost,DNS:keycloak,IP:127.0.0.1"

prep:
	@go get -tool github.com/a-h/templ/cmd/templ@latest
	@go get -tool github.com/air-verse/air@latest
	@go get -tool github.com/swaggo/swag/cmd/swag@latest
	@npm install
	@make gen-certs
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
	make docker-up-keycloak & sleep 10; \
	make templ & sleep 1; \
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
