# --- build -----------------------------------------------------------------
FROM golang:1.23-alpine AS build

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary; the schema is embedded, so the runtime image needs no SQL files.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w" \
    -o /out/api ./cmd/api

# --- runtime ---------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 ironnote

COPY --from=build /out/api /usr/local/bin/api

USER ironnote
EXPOSE 8080

# No shell wrapper: the binary itself answers the compose healthcheck.
ENTRYPOINT ["/usr/local/bin/api"]
