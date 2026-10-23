# Build a self-contained Go application with its embedded Web GUI.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/anyflip-downloader .

# Use a non-root runtime with CA certificates for verified TLS.
FROM alpine:3.22
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
