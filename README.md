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

## Requirements

- Docker + Docker Compose
- DNS override capability (AdGuard, Pi-hole, or router DNS)
- Home Assistant with MQTT integration

## Setup

**1. Clone and configure**

```bash
git clone https://git.zk35.de/secalpha/bayrol-bridge
cd bayrol-bridge
cp bridge/config.yaml.example bridge/config.yaml
```

Edit `bridge/config.yaml`:

```yaml
bayrol_broker:
  host: mosquitto
  port: 1883
  serial: "YOUR-SERIAL"   # printed on device, format: 23ASE2-XXXXX

ha_broker:
  host: 192.168.1.x       # HA host IP
  port: 1883
  username: mqttuser
  password: yourpassword

output_prefix: bayrol/pool
```

**2. Start**

```bash
docker compose up -d
```

Mosquitto generates its TLS certificate automatically on first start.
Certificate is renewed automatically when less than 30 days remain.

**3. DNS override**

Point `mqtt1.bayrol-poolaccess.de` to your bridge host IP in AdGuard/Pi-hole.

**4. Verify**

```bash
docker compose logs -f mosquitto   # watch for device connect
docker compose logs -f bridge      # watch for topic forwarding
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
