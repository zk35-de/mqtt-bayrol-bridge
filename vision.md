# Vision – bayrol-bridge

## Why this exists

The Bayrol Automatic SALT pool controller sends its sensor data (pH, redox, salt content,
temperature, electrolysis production) exclusively to the Bayrol cloud. There is no
documented, supported local access.

bayrol-bridge intercepts this cloud traffic and forwards the data to a local MQTT broker
(e.g. Home Assistant) — no cloud dependency, no firmware modification, no physical
intervention on the device.

## What problem does it solve?

- **Cloud dependency:** If Bayrol shuts down their service or changes the API, all
  automations break. bayrol-bridge eliminates this dependency.
- **Privacy:** Pool usage data (when the pump runs, when it doses) does not go to a
  third party.
- **Integration:** Home Assistant and other local systems get real-time access to all
  sensor values.

## How it works

The device does not validate TLS certificates. A DNS override points
`mqtt1.bayrol-poolaccess.de` to the bridge host. A Mosquitto broker runs there
with a self-signed certificate — the device connects without noticing.
The Go bridge reads the raw data, transforms it into clean topics, and publishes
them to the local HA MQTT broker.

## What is explicitly out of scope?

- **Device control:** bayrol-bridge is read-only. No commands to the device.
- **Other Bayrol models:** Only Automatic SALT tested. Other models may work but
  are not the target.
- **Cloud replacement:** No reimplementation of the Bayrol portal, no app, no dashboard.
  That is what Home Assistant is for.
- **Firmware modification:** The device remains unchanged.

## This is not a hack

The device behaves exactly as it does with the cloud — it connects to an MQTT broker.
We run that broker ourselves. No reverse engineering of the firmware. No injection of
commands. The device is not aware of any difference. This is our right as the owner
of the hardware.

## Roadmap

### v0.1 – MVP
Bridge runs stably, all known topics handled, tests in place, E2E deployment confirmed.

### v0.2 – Configurable
Embedded web UI for MQTT connection parameters (Bayrol broker, HA broker, serial number,
output prefix). No YAML editing required. Simple auth. Status display: device connected?
Last topics?

### v0.3 – Public Release
Clean documentation, GitHub release, community can contribute other Bayrol models.
Automatic serial number detection from the MQTT connect packet (no manual config).

### v1.0 – Final form
Fully self-configuring: DNS hint in the UI, automatic cert rotation visible,
pH calibration (mV → pH reference points), complete boolean topic mapping
(filter pump, dosing, operating modes). One container, one `docker run`, done.
