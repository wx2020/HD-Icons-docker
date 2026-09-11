FROM golang:1.22-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/hd-icons .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/hd-icons /app/hd-icons
VOLUME /app/icons
EXPOSE 50560
ENV PORT=50560 ICONS_DIR=/app/icons
CMD ["/app/hd-icons"]
