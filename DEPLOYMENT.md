# Docker Compose Deployment

This project can be deployed as a single container with Docker Compose.

The provided [docker-compose.yml](docker-compose.yml) is intended for Linux hosts, including Raspberry Pi systems, because the application collects local interface metrics and runs `traceroute` against configured targets. For accurate host-level metrics, the container uses `network_mode: host`.

## Start

```sh
docker compose up -d --build
```

The app listens on `:8090` by default and persists its SQLite database in `./data/pi-ntop.sqlite` on the host.

## Stop

```sh
docker compose down
```

## Common overrides

You can override configuration at launch time:

```sh
PI_NTOP_HTTP_ADDR=:8091 \
PI_NTOP_HEALTHCHECK_URL=http://127.0.0.1:8091/healthz \
PI_NTOP_TRACE_TARGETS=1.1.1.1,8.8.8.8,9.9.9.9 \
docker compose up -d --build
```

Important notes:

- `network_mode: host` is the correct deployment mode on Linux when you want host interface visibility.
- Docker Desktop on macOS does not provide the same host-network behavior as Linux, so the container will build there but will not observe the macOS host network stack the same way.
- The image installs `traceroute`, which the app requires for path discovery.
- The Compose service adds `NET_RAW` to avoid traceroute capability issues on stricter hosts.