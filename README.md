# Kill the Newsletter!

> This project is a complete reimplementation of [`leafac/kill-the-newsletter`](https://github.com/leafac/kill-the-newsletter) written in Go with first-class Docker deployment support.

Kill the Newsletter! is a self-hosted service that transforms newsletter emails into Atom feeds. Create a unique inbox for each subscription, receive the newsletter by mail, and read it via feed readers or the built-in web UI.

This repository ships a ready-to-use Docker image plus a `compose.yml` sample so you can run the HTTP and SMTP servers in either production or development mode.

## Feature Overview

- **HTTP server**: Listens on `:8080` by default, serving the home page, feed pages, and ` /feeds/<id>.xml` output.
- **SMTP server**: Listens on `:25` in production or `:2525` in development (configurable), and only accepts mail addressed to `feedPublicID@<hostname>`.
- **Attachments and inlines**: Saved as enclosures under `dataDirectory/files/` and exposed via `/files/` routes.
- **Size management and throttling**: Individual messages are limited to roughly 512 KB. Feed content is trimmed to ~512 KB cumulatively, and Atom fetches / WebSub callbacks have simple rate limits.

## Quick Start (Docker)

1. Prepare directories on the host:

```
./data/        # Persistent application data (SQLite + attachments)
```

2. Run the container (production example):

```bash
docker run -d \
  --name kill-the-newsletter \
  -p 8080:8080 \
  -p 25:25 \
  -v "$PWD/data:/app/data" \
  -e KTN_HOSTNAME=newsletters.example.com \
  -e KTN_ENVIRONMENT=production \
  -e KTN_SMTP_PORT=25 \
  -e KTN_HTTP_PORT=8080 \
  -e KTN_RUN_TYPE=all \
  ghcr.io/jtsang4/kill-the-newsletter:latest
```

Notes:

- `KTN_HOSTNAME` must match the domain used in DNS MX records.
- `KTN_SMTP_PORT` defaults to `25` in production and `2525` in development if unset.
- For SMTP STARTTLS, mount certificate files and set `KTN_TLS_CERTIFICATE` and `KTN_TLS_KEY` to their in-container paths.
- The HTTP server does not include TLS; see “Reverse proxy / HTTPS”.

3. Run the container:

```bash
docker run -d \
  --name kill-the-newsletter \
  -p 8080:8080 \
  -p 25:25 \
  -v "$PWD/data:/app/data" \
  -e KTN_HOSTNAME=newsletters.example.com \
  ghcr.io/jtsang4/kill-the-newsletter:latest
```

Then open `http://<server-ip-or-domain>:8080/`.

## Deploy with Docker Compose

The repository includes a `compose.yml` you can copy as-is or adapt:

```yaml
services:
  ktn:
    image: ghcr.io/jtsang4/kill-the-newsletter:latest
    container_name: kill-the-newsletter
    restart: unless-stopped
    ports:
      - '8080:8080' # HTTP
      - '25:25' # SMTP (production)
      - '2525:2525' # SMTP (development)
    volumes:
      - ./data:/app/data
    environment:
      - KTN_HOSTNAME=newsletters.example.com
      - KTN_ENVIRONMENT=production
      - KTN_SMTP_PORT=25
      - KTN_HTTP_PORT=8080
      - KTN_RUN_TYPE=all
    healthcheck:
      test:
        [
          'CMD-SHELL',
          'wget -qO- http://127.0.0.1:8080/ >/dev/null 2>&1 || exit 1',
        ]
      interval: 30s
      timeout: 5s
      retries: 3
```

Important: the application only listens on one SMTP port—the one set via `KTN_SMTP_PORT`. The dual `EXPOSE 25 2525` in the `Dockerfile` only documents capabilities. In production you typically publish `25:25`; in development you can expose `2525:2525` and set `KTN_ENVIRONMENT=development` (or set `KTN_SMTP_PORT=2525`).

Start with:

```bash
docker compose -f compose.yml up -d
```

## Configuration (Environment Variables)

Set the following environment variables (names map to `internal/config/config.go`):

- `KTN_HOSTNAME` (required): Public domain name.
- `KTN_SYSTEM_ADMIN_EMAIL` (optional): Address shown for administrative contact.
- `KTN_TLS_KEY` / `KTN_TLS_CERTIFICATE` (optional): Paths to SMTP STARTTLS key and certificate inside the container/host.
- `KTN_DATA_DIRECTORY` (optional, default: `./data/` or `/app/data/` in Docker examples).
- `KTN_ENVIRONMENT` (optional): `production` or `development` (default: `production`).
- `KTN_SMTP_PORT` (optional): SMTP listening port. Defaults to `25` in production and `2525` in development if unset.
- `KTN_HTTP_PORT` (optional, default: `8080`).
- `KTN_RUN_TYPE` (optional): `server`, `email`, `background`, or `all` (default).

Development example:

```bash
export KTN_HOSTNAME=localhost
export KTN_ENVIRONMENT=development
export KTN_SMTP_PORT=2525
export KTN_HTTP_PORT=8080
export KTN_RUN_TYPE=all
```

## Cloudflare DNS Example (`example.com`)

Goal: allow third parties to deliver newsletters to your server on port 25 while keeping the web UI reachable.

1. In the Cloudflare DNS dashboard:

   - **A record**:
     - Name: `newsletters`
     - IPv4: your server’s public IP
     - Proxy status: **DNS only** (grey cloud). Cloudflare does not proxy SMTP/25.
   - **AAAA record** (optional): point `newsletters` to your IPv6 address, also **DNS only**.
   - **MX record**:
     - Name: `@` (root domain `example.com`)
     - Mail server: `newsletters.example.com`
     - Priority: `10`
     - Remember: Cloudflare never proxies MX, but the host it targets must resolve to “DNS only” A/AAAA records.

2. Firewalls and ports:

   - Allow inbound **TCP 25** (SMTP) and **TCP 8080** (HTTP) on the server.
   - Many VPS providers block port 25 by default. Use a provider that permits SMTP or request an unblocking exception.

3. Web access:
   - Users can visit `http://newsletters.example.com:8080/` directly.
   - For HTTPS/443, put a reverse proxy in front (see below).

Tip: the service only _receives_ mail and serves feeds; it does not send email. SPF, DKIM, and DMARC records are therefore optional for this domain.

## Reverse Proxy / HTTPS

The HTTP server listens on `:8080` without TLS. Run a reverse proxy (Nginx, Caddy, Traefik, etc.) on ports 80/443 and proxy to `ktn:8080` in the Compose network:

- Issue and renew TLS certificates on the proxy.
- Keep the MX host `newsletters.example.com` as **DNS only**. If you want a Cloudflare-proxied web hostname, map another record (e.g. `rss.example.com`) to the proxy.

## Usage Workflow

1. Visit `http://<hostname>:8080/` and create a feed.
2. A `feedPublicID` is generated; its mailbox is `feedPublicID@<hostname>`.
3. Change the newsletter subscription to deliver to that mailbox.
4. Subscribe to `https://<hostname>/feeds/<feedPublicID>.xml` in your reader.
5. Attachments and inline images are saved as enclosures and linked on the entry page.

## Data Persistence and Backups

- Data directory: `dataDirectory` (mounted as `./data/` in Docker examples).
- SQLite database: `kill-the-newsletter.db`.
- Attachments: `dataDirectory/files/`.
- Backup strategy: regularly archive the entire `./data/` directory.

## Troubleshooting

- **Mail never arrives**:
  - Ensure Cloudflare A/AAAA records are **DNS only**.
  - Confirm the MX record points to `hostname`, and TCP 25 is reachable.
  - Check whether your provider blocks port 25.
  - Verify `KTN_HOSTNAME` and `KTN_SMTP_PORT` are set correctly.
- **Development SMTP (`2525`) unreachable**:
  - Set `KTN_ENVIRONMENT=development` or explicitly `KTN_SMTP_PORT=2525`.
  - Publish the port mapping `2525:2525` in Docker / Compose.
- **Web UI unavailable**:
  - Confirm the `8080:8080` mapping and health check status.
  - If using a reverse proxy, double-check upstream settings and TLS certificates.

## Build from Source (optional)

```bash
docker build -t kill-the-newsletter:local .
docker run -d \
  -p 8080:8080 -p 25:25 \
  -v "$PWD/data:/app/data" \
  -e KTN_HOSTNAME=localhost -e KTN_ENVIRONMENT=development -e KTN_SMTP_PORT=2525 -e KTN_HTTP_PORT=8080 -e KTN_RUN_TYPE=all \
  kill-the-newsletter:local
```

## License

See `LICENSE.md`.
