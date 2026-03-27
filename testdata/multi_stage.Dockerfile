FROM golang:1.22 AS builder
FROM --platform=linux/amd64 debian:bookworm-slim AS runtime
COPY --from=builder /app /app
FROM scratch
FROM builder AS final
