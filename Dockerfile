# updash — multi-stage Docker image
# Prints system state; useful for CI/CD or headless runs on servers.
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Copy only source trees needed to build (avoids recursive COPY . / secrets).
COPY cmd/ ./cmd/
COPY internal/ ./internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /updash ./cmd/updash/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 updash
COPY --from=builder /updash /usr/local/bin/updash
USER updash
ENTRYPOINT ["updash"]
CMD ["--check"]
