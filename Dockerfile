FROM golang:1.25 AS builder
ARG BUILD_HASH=unknown
ARG SERVER_VERSION=0.0.0
ARG CGO_ENABLED=0
ARG GOOS=linux
ARG GOARCH=amd64

ENV BUILD_HASH=${BUILD_HASH}
ENV SERVER_VERSION=${SERVER_VERSION}
ENV CGO_ENABLED=${CGO_ENABLED}
ENV GOOS=${GOOS}
ENV GOARCH=${GOARCH}

WORKDIR /app

# Needs to install npm to be able to install the tailwind modules
RUN apt-get update && apt-get install -y npm

COPY . .

RUN go mod download
RUN npm ci

RUN npm run build

# Patch swagger.json version to match build version
RUN sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"${SERVER_VERSION}\"/" internal/docs/swagger.json
RUN go build \
    -ldflags "-X github.com/effiware/cloak-apps/internal/version.Version=${SERVER_VERSION} -X github.com/effiware/cloak-apps/internal/version.BuildHash=${BUILD_HASH}" \
    -o ./bin/main cmd/server/main.go

FROM alpine:latest
WORKDIR /app

# Copies binary from previous container
COPY --from=builder /app/bin/main .

EXPOSE 8080

CMD ["./main"]
