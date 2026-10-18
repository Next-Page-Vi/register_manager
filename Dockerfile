# syntax=docker/dockerfile:1

FROM node:26-alpine AS web
WORKDIR /src/web
RUN npm install --global pnpm@12.11.2
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.27-alpine AS server
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22
RUN adduser -D -u 10001 app && mkdir /data && chown app /data
COPY --from=server /out/server /usr/local/bin/server
USER app
ENV RM_DB_PATH=/data/register_manager.db \
    RM_LISTEN=:8080 \
    GOMEMLIMIT=96MiB
# The database must live on a local volume, never a network file system.
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["server"]
CMD ["serve"]
