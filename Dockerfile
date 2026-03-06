FROM golang:1.25-alpine AS build
WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
ARG BUILD_TIME=1
ARG PKG="github.com/ServerPlace/iac-controller/pkg/version"
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath \
    -ldflags "-s -w \
      -X ${PKG}.Version=${VERSION} \
      -X ${PKG}.BuildTime=${BUILD_TIME}" \
    -o controller ./cmd/controller

FROM gcr.io/distroless/static-debian12
WORKDIR /app

COPY --from=build /app/controller /controller

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/controller"]
