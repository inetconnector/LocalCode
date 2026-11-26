# Multi-stage hardened container for LocalCode Zero-Knowledge E2EE Relay
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY src/go.mod src/go.sum ./src/
WORKDIR /build/src
RUN go mod download
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/localcode-relay ./cmd/localcode-relay

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/localcode-relay /localcode-relay
USER 65532:65532
EXPOSE 8443
ENTRYPOINT ["/localcode-relay", "-listen", ":8443"]
