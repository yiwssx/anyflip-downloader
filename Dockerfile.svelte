# SUI-04 candidate only. The existing Dockerfile / Production release stays
# unchanged. Build the Svelte assets in an isolated Node stage and embed them
# directly in Go; Node is NOT installed in the runtime container.
FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
ENV SUI_EMBED_BUILD=1
RUN npm run check && npm run build

FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-build /src/frontend/dist/ ./web/svelte/
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/anyflip-downloader .

FROM alpine:3.22
ARG SOURCE_COMMIT=unknown
LABEL org.opencontainers.image.source="https://github.com/yiwssx/anyflip-downloader" \
      org.opencontainers.image.revision="$SOURCE_COMMIT" \
      org.opencontainers.image.title="RCAT AnyFlip Downloader Svelte UI"
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 anyflip \
    && adduser -S -D -H -u 10001 -G anyflip anyflip \
    && mkdir -p /data/jobs \
    && chown -R 10001:10001 /data
COPY --from=build /out/anyflip-downloader /usr/local/bin/anyflip-downloader
USER 10001:10001
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/anyflip-downloader"]
CMD ["--help"]
