# updash — multi-stage Docker image
# Prints system state; useful for CI/CD or headless runs on servers.
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /updash ./cmd/updash/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /updash /usr/local/bin/updash
ENTRYPOINT ["updash"]
CMD ["--check"]
