import path from "node:path";

export const configuration = {
  hostname: process.env.HOSTNAME ?? "localhost",
  dataDirectory: path.resolve(process.cwd(), process.env.DATA_DIR ?? "data"),
  administratorEmail: process.env.ADMIN_EMAIL ?? "info@jtsang.me",
  environment: process.env.ENVIRONMENT ?? "other",
  tunnel: process.env.TUNNEL === '1',
  alternativeHostnames: process.env.ALT_HOSTNAMES?.split(",") ?? [],
  hstsPreload: process.env.HSTS_PRELOAD === '1',
  caddy: process.env.CADDYFILE ?? '',
};
