FROM eclipse-mosquitto:2
RUN apk add --no-cache openssl
COPY entrypoint.sh /entrypoint.sh
COPY bayrol.conf /mosquitto/config/mosquitto.conf
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
