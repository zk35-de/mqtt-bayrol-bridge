# Vision – bayrol-bridge

## Wozu existiert dieses Projekt?

Bayrol Automatic SALT Pool-Controller senden ihre Messdaten (pH, Redox, Salzgehalt,
Temperatur, SE-Produktion) ausschließlich an die Bayrol Cloud. Lokal gibt es keinen
dokumentierten, unterstützten Zugriff.

bayrol-bridge fängt diesen Cloud-Traffic ab und leitet die Daten an einen lokalen
MQTT-Broker (z.B. Home Assistant) weiter – ohne Cloud-Abhängigkeit, ohne
Firmware-Modifikation, ohne physischen Eingriff am Gerät.

## Welches Problem löst es?

- **Cloud-Abhängigkeit:** Wenn Bayrol den Dienst einstellt oder die API ändert, sind
  alle Automationen tot. bayrol-bridge eliminiert diese Abhängigkeit.
- **Datenschutz:** Pool-Nutzungsdaten (wann läuft die Pumpe, wann dosiert sie) gehen
  nicht an einen Drittanbieter.
- **Integration:** Home Assistant und andere lokale Systeme bekommen Echtzeit-Zugriff
  auf alle Sensorwerte.

## Wie funktioniert es?

Das Gerät validiert TLS-Zertifikate nicht. Ein DNS-Override leitet
`mqtt1.bayrol-poolaccess.de` auf den Bridge-Host um. Dort läuft ein Mosquitto-Broker
mit Self-Signed Cert – das Gerät verbindet sich ohne es zu merken. Die Go-Bridge
liest die Rohdaten, transformiert sie in saubere Topics und publiziert sie an den
lokalen HA MQTT-Broker.

## Was ist explizit außerhalb des Scopes?

- **Steuerung des Geräts:** bayrol-bridge ist read-only. Keine Befehle ans Gerät.
- **Andere Bayrol-Modelle:** Nur Automatic SALT getestet. Andere Modelle mögen
  funktionieren, sind aber nicht das Ziel.
- **Cloud-Replacement:** Kein Nachbau des Bayrol-Portals, keine App, kein Dashboard.
  Dafür gibt es Home Assistant.
- **Firmware-Modifikation:** Das Gerät bleibt unverändert.

## Wohin soll es langfristig gehen?

1. **v0.1 MVP** – Stabil laufende Bridge, alle bekannten Topics verarbeitet, Tests vorhanden
2. **v0.2 Public** – Sauber dokumentiert, GitHub-Release, Community kann andere
   Bayrol-Modelle beitragen
3. **v0.3 Discovery** – Automatische Erkennung der Seriennummer aus dem MQTT-Connect,
   keine manuelle Config nötig
4. **Offen** – pH-Kalibrierung (mV → pH) wenn Referenzpunkte bekannt,
   Filterpumpen-Mapping vervollständigen

## Was dieses Projekt nicht ist

Kein Hack, keine Sicherheitslücke, kein Angriff auf Bayrol. Das Gerät verhält sich
exakt wie mit der Cloud – es verbindet sich zu einem MQTT-Broker. Wir betreiben
diesen Broker selbst. Das ist unser gutes Recht.
