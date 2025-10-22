FROM golang:1.24.2-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/ktn ./cmd/ktn

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/ktn /usr/local/bin/ktn
COPY static/ ./static/
COPY configs/development.json /app/config/config.json
VOLUME ["/app/data", "/config"]
EXPOSE 8080 25 2525
ENTRYPOINT ["ktn"]
CMD ["-config", "/app/config/config.json"]
