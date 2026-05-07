#!/bin/sh
set -e
CERTS=/mosquitto/certs
mkdir -p "$CERTS"
mkdir -p /data

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
    chown -R mosquitto:mosquitto "$CERTS"
else
    echo "[certgen] cert valid, skipping"
fi

if [ ! -s /config.yaml ]; then
    echo "[config] /config.yaml fehlt oder leer – schreibe Vorlage"
    cat > /config.yaml <<'EOF'
bayrol_broker:
  host: localhost
  port: 1883

ha_broker:
  host: ""
  port: 1883
  username: ""
  password: ""

output_prefix: bayrol/pool
discovery_prefix: homeassistant

web:
  port: 8080
EOF
fi

/usr/sbin/mosquitto -c /mosquitto/config/mosquitto.conf &

exec /bridge /config.yaml
