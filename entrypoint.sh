#!/bin/sh
set -e
CERTS=/mosquitto/certs
mkdir -p "$CERTS"

needs_cert() {
    [ ! -f "$CERTS/bayrol-server.crt" ] || \
    ! openssl x509 -checkend 2592000 -noout -in "$CERTS/bayrol-server.crt" 2>/dev/null
}

if needs_cert; then
    echo "[certgen] generating self-signed cert for mqtt1.bayrol-poolaccess.de"
    openssl genrsa -out "$CERTS/bayrol-ca.key" 2048 2>/dev/null
    openssl req -new -x509 -days 3650 \
        -key "$CERTS/bayrol-ca.key" \
        -out "$CERTS/bayrol-ca.crt" \
        -subj "/CN=Root/C=DE" 2>/dev/null
    openssl genrsa -out "$CERTS/bayrol-server.key" 2048 2>/dev/null
    openssl req -new \
        -key "$CERTS/bayrol-server.key" \
        -out "$CERTS/bayrol-server.csr" \
        -subj "/CN=mqtt1.bayrol-poolaccess.de/C=DE" 2>/dev/null
    openssl x509 -req \
        -in "$CERTS/bayrol-server.csr" \
        -CA "$CERTS/bayrol-ca.crt" \
        -CAkey "$CERTS/bayrol-ca.key" \
        -CAcreateserial \
        -out "$CERTS/bayrol-server.crt" \
        -days 3650 2>/dev/null
    echo "[certgen] done, expires $(openssl x509 -enddate -noout -in $CERTS/bayrol-server.crt)"
else
    echo "[certgen] cert valid, skipping"
fi

/usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf &

exec /bridge /config.yaml
