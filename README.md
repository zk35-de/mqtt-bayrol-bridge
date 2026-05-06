# bayrol-bridge

MQTT bridge for Bayrol Automatic SALT pool controllers. Intercepts the device's
cloud MQTT connection and forwards processed sensor data to a local broker (e.g.
Home Assistant) – no cloud dependency, no firmware modification.

→ [vision.md](vision.md) for the why.

## How it works

```
Bayrol device  →  DNS override: mqtt1.bayrol-poolaccess.de → bridge host
               →  Mosquitto :8883 (TLS, self-signed, device does not validate)
               →  Go bridge: transform raw topics → clean HA topics
               →  Home Assistant MQTT broker
```

Single container: Mosquitto and the Go bridge run together.
Mosquitto generates its TLS certificate automatically on first start.

## Requirements

- Docker / Podman + Compose
- DNS override capability (AdGuard, Pi-hole, or router DNS)
- Home Assistant with MQTT integration

## Deployment (production)

**1. Create config.yaml**

```yaml
bayrol_broker:
  host: localhost         # Mosquitto runs in the same container
  port: 1883
  serial: "YOUR-SERIAL"  # printed on device, format: 23ASE2-XXXXX

ha_broker:
  host: 192.168.1.x      # HA host IP
  port: 1883
  username: mqttuser
  password: yourpassword

output_prefix: bayrol/pool
```

**2. docker-compose.yml** (only this file + config.yaml needed on the host)

```yaml
services:
  bayrol-bridge:
    image: git.zk35.de/secalpha/bayrol-bridge:latest
    ports:
      - "8883:8883"
    volumes:
      - ./config.yaml:/config.yaml:ro
      - certs:/mosquitto/certs
    restart: unless-stopped

volumes:
  certs:
```

**3. Start**

```bash
docker login git.zk35.de
docker compose pull
docker compose up -d
```

**4. DNS override**

AdGuard Home → Filters → DNS rewrites → Add:
- Domain: `mqtt1.bayrol-poolaccess.de`
- Answer: `<bridge-host-ip>`

**5. Verify**

```bash
docker compose logs -f bayrol-bridge
```

Expected when device connects:
```
New client connected from 10.35.5.68 as ...
Bayrol broker connected, subscribing
```

## Development

```bash
git clone https://git.zk35.de/secalpha/bayrol-bridge
cd bayrol-bridge
cp bridge/config.yaml.example bridge/config.yaml
# edit bridge/config.yaml (host: localhost)

# build and run locally
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d

# tests
cd bridge && go test ./...
```

## Output Topics

All topics are published to `<output_prefix>/` (default: `bayrol/pool/`).

| Topic | Content | Unit |
|---|---|---|
| `temperatur_ref` | Calibration reference temperature (~1.3°C below display) | °C |
| `redox` | Redox potential | mV |
| `salzgehalt_pct` | Salt content | % (0–99) |
| `se_produktion` | Electrolysis production | % |
| `ph_mv` | pH electrode raw voltage | mV |
| `se_betriebsstunden` | Electrolysis total runtime | h |
| `filterpumpe` | Filter pump state | `ON` / `OFF` |
| `ph` | pH value (from alert text, updated on alarm) | pH |
| `temperatur` | Display temperature (from alert text, updated on alarm) | °C |
| `salzgehalt_gpl` | Salt content (from alert text, updated on alarm) | g/l |
| `alarm_subject` | Last alarm description | string |
| `device_info` | Device info JSON | JSON |

## Tested devices

- Bayrol Automatic SALT (AS5), firmware v1.53

## License

MIT
