# Multi-stage build that also copies from external images.
FROM golang:1.22 AS builder
RUN go build -o /app

FROM gcr.io/distroless/base-debian12:nonroot

# from an earlier stage, by name and by index
COPY --from=builder /app /app
COPY --from=0 /go/bin/tool /usr/local/bin/tool

# from external images
COPY --from=nginx:1.27 /etc/nginx /etc/nginx
COPY --chown=65532:65532 --from=busybox:1.36 /bin/busybox /bin/busybox
COPY --from=registry.example.com:5000/tool:1.0 /tool /usr/local/bin/tool2

# already pinned
COPY --from=alpine:3.19@sha256:aaaa1111 /etc/alpine-release /etc/alpine-release

# BuildKit does not expand variables in --from
ARG NGINX_VERSION=1.27
COPY --from=nginx:${NGINX_VERSION} /etc/nginx /etc/nginx.orig

# from the build context
COPY ./config /config
