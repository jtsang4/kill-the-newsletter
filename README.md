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
./config/      # Configuration files and certificates (mounted read-only to /app/config)
```

2. Create `./config/config.json` (production example):

```json
{
  "hostname": "newsletters.example.com",
  "tls": {
    "key": "/app/config/tls/tls.key",
    "certificate": "/app/config/tls/tls.crt"
  },
  "dataDirectory": "/app/data/",
  "environment": "production",
  "smtpPort": 25,
  "httpAddr": ":8080",
  "runType": "all"
}
```

Notes:

- `hostname` must match the domain used in DNS MX records.
- `smtpPort` defaults to `25` in production and `2525` in development, but can be set explicitly.
- `tls.key` and `tls.certificate` point to the SMTP STARTTLS certificate and key (optional but strongly recommended). Mount the certificate files inside the container, e.g. `./config/tls/tls.crt` and `./config/tls/tls.key`.
- The HTTP server does not include TLS; see “Reverse proxy / HTTPS”.

3. Run the container:

```bash
docker run -d \
  --name kill-the-newsletter \
  -p 8080:8080 \
  -p 25:25 \
  -v "$PWD/data:/app/data" \
  -v "$PWD/config:/app/config:ro" \
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
      - ./config:/app/config:ro
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

Important: the application only listens on one SMTP port—the one set in `config.json` as `smtpPort`. The dual `EXPOSE 25 2525` in the `Dockerfile` only documents capabilities. In production you typically publish `25:25`; in development you can expose `2525:2525` and set `environment` to `development` (or set `smtpPort: 2525`).

Start with:

```bash
docker compose -f compose.yml up -d
```

## Configuration (`/app/config/config.json`)

The structure matches `internal/config/config.go`:

- `hostname` (required): Public domain name.
- `systemAdministratorEmail` (optional): Address shown for administrative contact.
- `tls.key` / `tls.certificate` (optional): Paths to SMTP STARTTLS key and certificate.
- `dataDirectory`: Storage location; inside Docker use `/app/data/`.
- `environment`: `production` or `development` (default: `production`).
- `smtpPort`: SMTP listening port. Defaults to `25` in production and `2525` in development if omitted.
- `httpAddr`: HTTP listening address, defaults to `:8080`.
- `runType`: `server`, `email`, `background`, or `all` (default).

Development example:

```json
{
  "hostname": "localhost",
  "tls": { "key": "", "certificate": "" },
  "dataDirectory": "/app/data/",
  "environment": "development",
  "smtpPort": 2525,
  "httpAddr": ":8080",
  "runType": "all"
}
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
  - Verify `config.json` contains the correct `hostname` and `smtpPort`.
- **Development SMTP (`2525`) unreachable**:
  - Set `environment: development` or explicitly `smtpPort: 2525`.
  - Publish the port mapping `2525:2525` in Docker / Compose.
- **Web UI unavailable**:
  - Confirm the `8080:8080` mapping and health check status.
  - If using a reverse proxy, double-check upstream settings and TLS certificates.

## Build from Source (optional)

```bash
docker build -t kill-the-newsletter:local .
docker run -d \
  -p 8080:8080 -p 25:25 \
  -v "$PWD/data:/app/data" -v "$PWD/config:/app/config:ro" \
  kill-the-newsletter:local
```

## License

See `LICENSE.md`.
