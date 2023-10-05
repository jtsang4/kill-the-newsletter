#!/usr/bin/env node

import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import url from "node:url";
import timers from "node:timers/promises";
import os from "node:os";
import * as commander from "commander";
import express from "express";
import sql, { Database } from "@leafac/sqlite";
// import { Database } from 'bun:sqlite';
import html, { HTML } from "@leafac/html";
import css, { localCSS } from "@leafac/css";
import { localJavaScript } from "@leafac/javascript";
import lodash from "lodash";
import { execa, ExecaChildProcess } from "execa";
import dedent from "dedent";
import { version } from '../package.json';
import { configuration } from './configuration.mts'


if (process.env.TEST === "kill-the-newsletter") {
  delete process.env.TEST;

  assert.equal(1 + 1, 2);

  process.exit(0);
}

await commander.program
  .name("kill-the-newsletter")
  .description("Convert email newsletters into Atom feeds")
  .addOption(
    new commander.Option("--process-type <process-type>")
      .default("main")
      .hideHelp()
  )
  .addOption(
    new commander.Option("--process-number <process-number>").hideHelp()
  )
  .version(version)
  .addHelpText(
    "after",
    "\n" +
      dedent`
      Configuration:
        See ‘https://github.com/courselore/courselore/blob/main/documentation/self-hosting.md’ for instructions, and ‘https://github.com/courselore/courselore/blob/main/configuration/example.mjs’ for an example.
    `
  )
  .allowExcessArguments(false)
  .showHelpAfterError()
  .action(
    async (
      {
        processType,
        processNumber,
      }: {
        processType: "main" | "web" | "email";
        processNumber: string;
      }
    ) => {
        console.log("🚀 ~ file: index.mts:62 ~ processType:", processType)
        console.log("🚀 ~ file: index.mts:62 ~ processNumber:", processNumber)
      console.log("🚀 ~ file: index.mts:62 ~ configuration:", configuration)
      const stop = new Promise<void>((resolve) => {
        const processKeepAlive = new AbortController();
        timers
          .setInterval(1 << 30, undefined, {
            signal: processKeepAlive.signal,
          })
          [Symbol.asyncIterator]()
          .next()
          .catch(() => {});
        for (const event of [
          "exit",
          "SIGHUP",
          "SIGINT",
          "SIGQUIT",
          "SIGTERM",
          "SIGUSR2",
          "SIGBREAK",
        ])
          process.on(event, () => {
            processKeepAlive.abort();
            resolve();
          });
      });

      const application: {
        name: string;
        version: string;
        process: {
          id: string;
          type: "main" | "web" | "email";
          number: number;
        };
        configuration: {
          hostname: string;
          dataDirectory: string;
          administratorEmail: string;
          environment: "production" | "development" | "other";
          tunnel: boolean;
          alternativeHostnames: string[];
          hstsPreload: boolean;
          caddy: string;
        };
        static: {
          [path: string]: string;
        };
        ports: {
          web: number[];
        };
        web: Omit<express.Express, "locals"> & Function;
        email: "TODO";
        log(...messageParts: string[]): void;
        database: Database;
      } = {
        name: "kill-the-newsletter",
        version,
        process: {
          id: Math.random().toString(36).slice(2),
          type: processType,
          number: (typeof processNumber === "string"
            ? Number(processNumber)
            : undefined) as number,
        },
        configuration,
        static: JSON.parse(
          await fs.readFile(
            new URL("../build/static/paths.json", import.meta.url),
            "utf8"
          )
        ),
        ports: {
          web: lodash.times(
            os.cpus().length,
            (processNumber) => 6000 + processNumber
          ),
        },
        web: express(),
        email: "TODO",
      } as any;

      application.configuration.environment ??= "production";
      application.configuration.tunnel ??= false;
      application.configuration.alternativeHostnames ??= [];
      application.configuration.hstsPreload ??= false;
      application.configuration.caddy ??= dedent``;

      application.log = (...messageParts) => {
        console.log(
          [
            new Date().toISOString(),
            application.process.type,
            application.process.number,
            application.process.id,
            ...messageParts,
          ].join(" \t")
        );
      };

      application.log(
        "STARTED",
        ...(application.process.type === "main"
          ? [
              application.name,
              application.version,
              `https://${application.configuration.hostname}`,
            ]
          : [])
      );

      process.once("exit", () => {
        application.log("STOPPED");
      });

      type ResponseLocalsLogging = {
        log(...messageParts: string[]): void;
      };

      application.web.enable("trust proxy");

      application.web.use<{}, any, {}, {}, ResponseLocalsLogging>(
        (request, response, next) => {
          if (response.locals.log !== undefined) return next();

          const id = Math.random().toString(36).slice(2);
          const time = process.hrtime.bigint();
          response.locals.log = (...messageParts) => {
            application.log(
              id,
              `${(process.hrtime.bigint() - time) / 1_000_000n}ms`,
              request.ip,
              request.method,
              request.originalUrl,
              ...messageParts
            );
          };
          const log = response.locals.log;

          log("STARTING...");

          response.once("close", () => {
            const contentLength = response.getHeader("Content-Length");
            log(
              "FINISHED",
              String(response.statusCode),
              ...(typeof contentLength === "string"
                ? [`${Math.ceil(Number(contentLength) / 1000)}kB`]
                : [])
            );
          });

          next();
        }
      );

      await fs.mkdir(application.configuration.dataDirectory, {
        recursive: true,
      });
      application.database = new Database(
        path.join(
          application.configuration.dataDirectory,
          `${application.name}.db`
        )
      );

      process.once("exit", () => {
        application.database.close();
      });

      if (application.process.type === "main") {
        application.log("DATABASE MIGRATION", "STARTING...");

        application.database.exec("PRAGMA journal_mode = WAL;");

        // TODO: STOP USING DEFAULT VALUES.
        // TODO: DOUBLE-CHECK THAT THE OLD MIGRATION SYSTEM IS COMPATIBLE WITH THIS, USING SQLITE’S ‘PRAGMA USER_DATA’
        await application.database.migrate(
          sql`
            CREATE TABLE "feeds" (
              "id" INTEGER PRIMARY KEY AUTOINCREMENT,
              "createdAt" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
              "updatedAt" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
              "reference" TEXT NOT NULL UNIQUE,
              "title" TEXT NOT NULL
            );

            CREATE TABLE "entries" (
              "id" INTEGER PRIMARY KEY AUTOINCREMENT,
              "createdAt" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
              "reference" TEXT NOT NULL UNIQUE,
              "feed" INTEGER NOT NULL REFERENCES "feeds",
              "title" TEXT NOT NULL,
              "author" TEXT NOT NULL,
              "content" TEXT NOT NULL
            );
          `,
          sql`
            CREATE INDEX "entriesFeed" ON "entries" ("feed");
          `
        );

        application.log("DATABASE MIGRATION", "FINISHED");
      }

      type ResponseLocalsBase = ResponseLocalsLogging & {
        css: ReturnType<typeof localCSS>;
        javascript: ReturnType<typeof localJavaScript>;
      };

      application.web.use<{}, any, {}, {}, ResponseLocalsBase>(
        (request, response, next) => {
          response.locals.css = localCSS();
          response.locals.javascript = localJavaScript();

          if (
            !["GET", "HEAD", "OPTIONS", "TRACE"].includes(request.method) &&
            request.header("CSRF-Protection") !== "true"
          )
            next("Cross-Site Request Forgery");

          next();
        }
      );

      application.web.use<{}, any, {}, {}, ResponseLocalsBase>(
        express.urlencoded({ extended: true })
      );

      const layout = ({
        request,
        response,
        head,
        body,
      }: {
        request: express.Request<{}, HTML, {}, {}, ResponseLocalsBase>;
        response: express.Response<HTML, ResponseLocalsBase>;
        head: HTML;
        body: HTML;
      }) => {
        const layoutBody = html`
          <body
            css="${response.locals.css(css`
              font-family: "JetBrains MonoVariable",
                var(--font-family--monospace);
              font-size: var(--font-size--xs);
              background-color: var(--color--cyan--50);
              color: var(--color--cyan--900);
              @media (prefers-color-scheme: dark) {
                background-color: var(--color--cyan--900);
                color: var(--color--cyan--50);
              }
              position: absolute;
              top: 0;
              right: 0;
              bottom: 0;
              left: 0;
            `)}"
          >
            <div
              css="${response.locals.css(css`
                min-height: 100%;
                display: flex;
                justify-content: center;
                align-items: center;
              `)}"
            >
              <div
                css="${response.locals.css(css`
                  text-align: center;
                  max-width: var(--width--prose);
                  margin: var(--space--4) var(--space--2);
                  display: flex;
                  flex-direction: column;
                  gap: var(--space--2);
                  align-items: center;
                `)}"
              >
                <h1>
                  <a href="https://${application.configuration.hostname}/"
                    >Kill the Newsletter!</a
                  >
                </h1>
                $${body}
              </div>
            </div>
          </body>
        `;

        return html`
          <!DOCTYPE html>
          <html lang="en">
            <head>
              <meta name="version" content="${application.version}" />

              <meta
                name="description"
                content="Convert email newsletters into Atom feeds"
              />

              <meta
                name="viewport"
                content="width=device-width, initial-scale=1, maximum-scale=1"
              />
              <link
                rel="stylesheet"
                href="https://${application.configuration
                  .hostname}/${application.static["index.css"]}"
              />
              $${response.locals.css.toString()}

              <script
                src="https://${application.configuration.hostname}/${application
                  .static["index.mjs"]}"
                defer
              ></script>

              $${head}
            </head>

            $${layoutBody} $${response.locals.javascript.toString()}
          </html>
        `;
      };

      application.web.get<{}, any, {}, {}, ResponseLocalsBase>(
        "/",
        (request, response) => {
          response.send(
            layout({
              request,
              response,
              head: html`<title>Kill the Newsletter!</title>`,
              body: html`
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
                <p>
                  Lorem ipsum dolor sit amet, consectetur adipiscing elit.
                  Aenean dictum dui quis magna mollis, vel interdum felis
                  consectetur.
                </p>
              `,
            })
          );
        }
      );

      switch (application.process.type) {
        case "main": {
          const childProcesses = new Set<ExecaChildProcess>();
          let restartChildProcesses = true;
          for (const execaArguments of [
            ...Object.entries({ web: os.cpus().length, email: 1 }).flatMap(
              ([processType, processCount]) =>
                lodash.times(processCount, (processNumber) => ({
                  file: process.argv[0],
                  arguments: [
                    process.argv[1],
                    "--process-type",
                    processType,
                    "--process-number",
                    processNumber,
                    configuration,
                  ],
                  options: {
                    preferLocal: true,
                    stdio: "inherit",
                    ...(application.configuration.environment === "production"
                      ? { env: { NODE_ENV: "production" } }
                      : {}),
                  },
                }))
            ),
            {
              file: "caddy",
              arguments: ["run", "--config", "-", "--adapter", "caddyfile"],
              options: {
                preferLocal: true,
                stdout: "ignore",
                stderr: "ignore",
                input: dedent`
                  {
                    admin off
                    ${
                      application.configuration.environment === "production"
                        ? `email ${application.configuration.administratorEmail}`
                        : `local_certs`
                    }
                  }

                  (common) {
                    header Cache-Control no-store
                    header Content-Security-Policy "default-src https://${
                      application.configuration.hostname
                    }/ 'unsafe-inline' 'unsafe-eval'; frame-ancestors 'none'; object-src 'none'"
                    header Cross-Origin-Embedder-Policy require-corp
                    header Cross-Origin-Opener-Policy same-origin
                    header Cross-Origin-Resource-Policy same-origin
                    header Referrer-Policy no-referrer
                    header Strict-Transport-Security "max-age=31536000; includeSubDomains${
                      application.configuration.hstsPreload ? `; preload` : ``
                    }"
                    header X-Content-Type-Options nosniff
                    header Origin-Agent-Cluster "?1"
                    header X-DNS-Prefetch-Control off
                    header X-Frame-Options DENY
                    header X-Permitted-Cross-Domain-Policies none
                    header -Server
                    header -X-Powered-By
                    header X-XSS-Protection 0
                    header Permissions-Policy "interest-cohort=()"
                    encode zstd gzip
                  }

                  ${[
                    ...(application.configuration.tunnel
                      ? []
                      : [application.configuration.hostname]),
                    ...application.configuration.alternativeHostnames,
                  ]
                    .map((hostname) => `http://${hostname}`)
                    .join(", ")} {
                    import common
                    redir https://{host}{uri} 308
                    handle_errors {
                      import common
                    }
                  }

                  ${
                    application.configuration.alternativeHostnames.length > 0
                      ? dedent`
                          ${application.configuration.alternativeHostnames
                            .map((hostname) => `https://${hostname}`)
                            .join(", ")} {
                            import common
                            redir https://${
                              application.configuration.hostname
                            }{uri} 307
                            handle_errors {
                              import common
                            }
                          }
                        `
                      : ``
                  }

                  http${application.configuration.tunnel ? `` : `s`}://${
                  application.configuration.hostname
                } {
                    route {
                      import common
                      route {
                        root * ${JSON.stringify(
                          url.fileURLToPath(
                            new URL("../static/", import.meta.url)
                          )
                        )}
                        @file_exists file
                        route @file_exists {
                          header Cache-Control "public, max-age=31536000, immutable"
                          file_server
                        }
                      }
                      reverse_proxy ${application.ports.web
                        .map((port) => `127.0.0.1:${port}`)
                        .join(" ")} {
                          lb_retries 1
                        }
                    }
                    handle_errors {
                      import common
                    }
                  }

                  ${application.configuration.caddy}
                `,
              },
            },
          ])
            (async () => {
              while (restartChildProcesses) {
                const childProcess = execa(
                  execaArguments.file,
                  execaArguments.arguments as any,
                  {
                    ...execaArguments.options,
                    reject: false,
                    cleanup: false,
                  } as any
                );
                childProcesses.add(childProcess);
                const childProcessResult = await childProcess;
                application.log(
                  "CHILD PROCESS RESULT",
                  JSON.stringify(childProcessResult, undefined, 2)
                );
                childProcesses.delete(childProcess);
              }
            })();
          await stop;
          restartChildProcesses = false;
          for (const childProcess of childProcesses) childProcess.cancel();
          break;
        }

        case "web": {
          const webApplication = application.web;
          webApplication.emit("start");
          const server = webApplication.listen(
            application.ports.web[application.process.number],
            "127.0.0.1"
          );
          await stop;
          server.close();
          webApplication.emit("stop");
          break;
        }

        case "email": {
          // TODO
          await stop;
          break;
        }
      }

      await timers.setTimeout(10 * 1000, undefined, { ref: false });
      process.exit(1);
    }
  )
  .parseAsync();
