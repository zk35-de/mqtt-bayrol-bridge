package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiscoveryPayload_Sensor(t *testing.T) {
	def := sensorDef{"temperatur", "Pool Temperatur", "temperature", "°C", false}
	topic, payload, err := discoveryPayload("homeassistant", "TESTSERIAL", "bayrol/pool/temperatur", "temperatur", def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic != "homeassistant/sensor/bayrol_temperatur/config" {
		t.Errorf("wrong topic: %s", topic)
	}
	var cfg sensorDiscovery
	if err := json.Unmarshal(payload, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.StateTopic != "bayrol/pool/temperatur" {
		t.Errorf("wrong state_topic: %s", cfg.StateTopic)
	}
	if cfg.UniqueID != "bayrol_TESTSERIAL_temperatur" {
		t.Errorf("wrong unique_id: %s", cfg.UniqueID)
	}
	if cfg.DeviceClass != "temperature" {
		t.Errorf("wrong device_class: %s", cfg.DeviceClass)
	}
	if cfg.UnitOfMeasurement != "°C" {
		t.Errorf("wrong unit: %s", cfg.UnitOfMeasurement)
	}
	if len(cfg.Device.Identifiers) == 0 || cfg.Device.Identifiers[0] != "bayrol_TESTSERIAL" {
		t.Errorf("wrong device identifier: %v", cfg.Device.Identifiers)
	}
}

func TestDiscoveryPayload_BinarySensor(t *testing.T) {
	def := sensorDef{"filterpumpe", "Pool Filterpumpe", "power", "", true}
	topic, payload, err := discoveryPayload("homeassistant", "TESTSERIAL", "bayrol/pool/filterpumpe", "filterpumpe", def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic != "homeassistant/binary_sensor/bayrol_filterpumpe/config" {
		t.Errorf("wrong topic: %s", topic)
	}
	var cfg binarySensorDiscovery
	if err := json.Unmarshal(payload, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.PayloadOn != "ON" || cfg.PayloadOff != "OFF" {
		t.Errorf("wrong payloads: on=%s off=%s", cfg.PayloadOn, cfg.PayloadOff)
	}
}

func TestDiscoveryPayload_NoDeviceClass(t *testing.T) {
	def := sensorDef{"ph", "Pool pH", "", "", false}
	_, payload, err := discoveryPayload("homeassistant", "TESTSERIAL", "bayrol/pool/ph", "ph", def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// device_class must be omitted (not "")
	if strings.Contains(string(payload), "device_class") {
		t.Errorf("device_class should be omitted when empty, got: %s", payload)
	}
}

func TestAllSensorDefsHavePayload(t *testing.T) {
	for _, def := range sensorDefs {
		_, payload, err := discoveryPayload("homeassistant", "SN123", "bayrol/pool/"+def.subTopic, def.subTopic, def)
		if err != nil {
			t.Errorf("sensor %s: marshal error: %v", def.subTopic, err)
		}
		if len(payload) == 0 {
			t.Errorf("sensor %s: empty payload", def.subTopic)
		}
	}
}
