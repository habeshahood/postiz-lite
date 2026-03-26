FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /postiz-lite ./cmd/postiz-lite/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /postiz-lite /usr/local/bin/postiz-lite

# Upload directory
RUN mkdir -p /uploads
VOLUME /uploads

# Inside Docker, 0.0.0.0 is safe — the host port binding controls external exposure.
ENV BIND_ADDR=0.0.0.0

EXPOSE 5000
ENTRYPOINT ["postiz-lite"]
