FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.24-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /build
COPY bridge/go.mod bridge/go.sum ./
RUN go mod download
COPY bridge/*.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o bridge .

FROM docker.io/library/eclipse-mosquitto:2
RUN apk add --no-cache openssl
COPY --from=builder /build/bridge /bridge
COPY --from=docker.io/library/golang:1.24-alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY mosquitto/bayrol.conf /mosquitto/config/mosquitto.conf
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
