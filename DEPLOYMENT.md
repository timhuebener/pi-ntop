# Docker Compose Deployment

This project can be deployed as a single container with Docker Compose.

There are two supported deployment modes:

- [docker-compose.yml](docker-compose.yml) for direct Linux deployment with `network_mode: host`
- [docker-compose.coolify.yml](docker-compose.coolify.yml) for Coolify or other reverse-proxy platforms

Use the direct Linux deployment when you want host-level interface visibility. Use the Coolify deployment when you want the app behind a domain managed by Coolify.

## Direct Linux start

```sh
docker compose up -d --build
```

The app listens on `:8090` by default and persists its SQLite database in `./data/pi-ntop.sqlite` on the host.

## Stop

```sh
docker compose down
```

## Coolify

Do not use [docker-compose.yml](docker-compose.yml) in Coolify. The host-networked deployment bypasses the container networking model that Coolify's reverse proxy expects, which can result in empty responses or an unreachable service.

Instead, deploy with [docker-compose.coolify.yml](docker-compose.coolify.yml) and configure the Coolify service/domain to target internal port `8090`.

Recommended Coolify settings:

- Compose file: [docker-compose.coolify.yml](docker-compose.coolify.yml)
- Domain: `network.pi.home`
- Service port / exposed port: `8090`
- Health check path: `/healthz`

Example launch outside of Coolify using the Coolify-compatible file:

```sh
docker compose -f docker-compose.coolify.yml up -d --build
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
- Coolify should use [docker-compose.coolify.yml](docker-compose.coolify.yml), not [docker-compose.yml](docker-compose.yml).
- The Coolify deployment runs in a normal container network namespace, so interface statistics describe the container's view of networking rather than the entire host.
- Docker Desktop on macOS does not provide the same host-network behavior as Linux, so the container will build there but will not observe the macOS host network stack the same way.
- The image installs `traceroute`, which the app requires for path discovery.
- The Compose service adds `NET_RAW` to avoid traceroute capability issues on stricter hosts.