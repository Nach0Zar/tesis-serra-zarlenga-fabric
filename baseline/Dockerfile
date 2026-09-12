# syntax=docker/dockerfile:1

FROM golang:1.23.5-alpine AS build

WORKDIR /src

COPY domain/go.mod domain/go.mod
COPY baseline/go.mod baseline/go.sum baseline/
RUN cd baseline && go mod download

COPY domain domain
COPY baseline baseline

RUN cd baseline \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/snt-baseline ./cmd/snt-baseline \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/snt-seed ./cmd/snt-seed

FROM alpine:3.21

RUN apk add --no-cache bash ca-certificates postgresql-client \
    && addgroup -S snt \
    && adduser -S -D -H -G snt snt

WORKDIR /app

COPY --from=build --chmod=0555 /out/snt-baseline /usr/local/bin/snt-baseline
COPY --from=build --chmod=0555 /out/snt-seed /usr/local/bin/snt-seed
COPY --chmod=0555 baseline/scripts/migrate.sh baseline/scripts/container-entrypoint.sh /app/scripts/
COPY baseline/migrations /app/migrations
RUN chmod -R a=rX /app/migrations

USER snt
EXPOSE 8080

ENTRYPOINT ["/app/scripts/container-entrypoint.sh"]
