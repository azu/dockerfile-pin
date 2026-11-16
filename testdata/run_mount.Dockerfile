# Multi-stage build that also mounts files from external images during RUN.
FROM golang:1.22 AS builder

# from an external image
RUN --mount=type=bind,from=nginx:1.27,source=/etc/nginx,target=/mnt/nginx \
    cp -r /mnt/nginx /etc/nginx

# several mounts on one RUN: a cache, a stage, and two external images
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,from=builder,source=/src,target=/src \
    --mount=type=bind,from=busybox:1.36,target=/busybox \
    --mount=type=cache,from=registry.example.com:5000/cache:1.0,target=/cache \
    go build -o /app

# the from value in quotes, and the whole option list in quotes
RUN --mount=type=bind,from="nginx:1.27",target=/mnt/nginx true
RUN --mount='type=bind,from=busybox:1.36,target=/busybox' true

# a form that is not supported is left as it is
RUN --mount=type=bind,from=nginx:1.27,target="/mnt/nginx" true

# already pinned
RUN --mount=from=alpine:3.19@sha256:aaaa1111,target=/alpine cat /alpine/etc/alpine-release

# BuildKit rejects a variable in from=
ARG NGINX_VERSION=1.27
RUN --mount=from=nginx:${NGINX_VERSION},target=/mnt true

# an ONBUILD trigger still names a real image
ONBUILD RUN --mount=from=curlimages/curl:8.11.1,target=/curl /curl/usr/bin/curl --version

# no from=: the build context and a secret
RUN --mount=type=bind,source=go.mod,target=go.mod --mount=type=secret,id=token go mod download
