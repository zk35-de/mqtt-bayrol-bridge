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

| Topic | Source | Content | Unit |
|---|---|---|---|
| `temperatur` | v/4.98 | Pool temperature (value ÷ 10) | °C |
| `temperatur_ref` | v/1 | Calibration reference temperature | °C |
| `redox` | v/4.82 | Redox potential | mV |
| `salzgehalt` | v/4.100 | Salt content (value ÷ 10) | g/l |
| `se_produktion` | v/4.92 | Electrolysis production | % |
| `se_betriebsstunden` | v/4.176 | Electrolysis total runtime (minutes ÷ 60) | h |
| `filterpumpe` | v/10 | Filter pump state (inferred from alarm list) | `ON` / `OFF` |
| `ph` | v/4.182 | pH-related value – **topic mapping under investigation** (see #15, #16) | – |
| `ph` | v/16 | pH value from alert text (updated on alarm) | pH |
| `temperatur` | v/16 | Temperature from alert text (updated on alarm) | °C |
| `salzgehalt_gpl` | v/16 | Salt content from alert text (updated on alarm) | g/l |
| `redox_alert` | v/16 | Redox from alert text (updated on alarm) | mV |
| `alarm_subject` | v/16 | Last alarm description | string |
| `device_type` | v/2 | Device type name | string |
| `device_serial` | v/2 | Device serial number | string |
| `device_sw_version` | v/2 | Firmware version | string |

> **Note on pH:** The exact MQTT topics for the live pH value are still being investigated
> (issues [#15](../../issues/15), [#16](../../issues/16)). Values from alert messages (v/16)
> are reliable but only update on alarm events.

## Tested devices

- Bayrol Automatic SALT (AS5), firmware v1.53

## License

MIT
