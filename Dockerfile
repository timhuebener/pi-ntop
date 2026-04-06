FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pi-ntop ./cmd/pi-ntop

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl traceroute \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /out/pi-ntop /usr/local/bin/pi-ntop

RUN mkdir -p /data

ENV GO_ENV=production \
    PI_NTOP_HTTP_ADDR=:8090 \
    PI_NTOP_DB_PATH=/data/pi-ntop.sqlite

EXPOSE 8090

ENTRYPOINT ["pi-ntop"]