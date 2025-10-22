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
VOLUME ["/app/data"]
ENV KTN_HOSTNAME="" \
    KTN_SYSTEM_ADMIN_EMAIL="" \
    KTN_TLS_KEY="" \
    KTN_TLS_CERTIFICATE="" \
    KTN_DATA_DIRECTORY="/app/data/" \
    KTN_ENVIRONMENT="production" \
    KTN_SMTP_PORT="25" \
    KTN_HTTP_PORT="8080" \
    KTN_RUN_TYPE="all"
EXPOSE 8080 25 2525
ENTRYPOINT ["ktn"]
