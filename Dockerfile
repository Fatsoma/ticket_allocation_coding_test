# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget \
	&& adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/server /app/server

USER appuser

EXPOSE 3000

ENV PORT=3000

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=10 \
	CMD wget -qO- http://127.0.0.1:3000/_health >/dev/null || exit 1

ENTRYPOINT ["/app/server"]
