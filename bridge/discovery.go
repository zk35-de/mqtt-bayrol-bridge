package main

import (
	"encoding/json"
	"fmt"
	"log"
)

type haDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer"`
}

type sensorDiscovery struct {
	Name              string   `json:"name"`
	StateTopic        string   `json:"state_topic"`
	UniqueID          string   `json:"unique_id"`
	Device            haDevice `json:"device"`
	DeviceClass       string   `json:"device_class,omitempty"`
	UnitOfMeasurement string   `json:"unit_of_measurement,omitempty"`
}

type binarySensorDiscovery struct {
	Name        string   `json:"name"`
	StateTopic  string   `json:"state_topic"`
	UniqueID    string   `json:"unique_id"`
	Device      haDevice `json:"device"`
	DeviceClass string   `json:"device_class,omitempty"`
	PayloadOn   string   `json:"payload_on"`
	PayloadOff  string   `json:"payload_off"`
}

type sensorDef struct {
	subTopic    string
	name        string
	deviceClass string
	unit        string
	binary      bool
}

var sensorDefs = []sensorDef{
	{"temperatur", "Pool Temperatur", "temperature", "°C", false},
	{"temperatur_ref", "Pool Temperatur Referenz", "temperature", "°C", false},
	{"ph", "Pool pH", "", "", false},
	{"ph_mv", "Pool pH Spannung", "", "mV", false},
	{"redox", "Pool Redox", "", "mV", false},
	{"salzgehalt", "Pool Salzgehalt", "", "g/l", false},
	{"salzgehalt_gpl", "Pool Salzgehalt g/l (Alarm)", "", "g/l", false},
	{"se_produktion", "Pool SE Produktion", "", "%", false},
	{"se_betriebsstunden", "Pool SE Betriebsstunden", "", "", false},
	{"alarm_subject", "Pool Alarm", "", "", false},
	{"filterpumpe", "Pool Filterpumpe", "power", "", true},
}

func discoveryPayload(discoveryPrefix, serial, stateTopic, subTopic string, def sensorDef) (topic string, payload []byte, err error) {
	dev := haDevice{
		Identifiers:  []string{"bayrol_" + serial},
		Name:         "Bayrol Automatic SALT",
		Manufacturer: "Bayrol",
	}
	uniqueID := "bayrol_" + serial + "_" + subTopic

	if def.binary {
		topic = fmt.Sprintf("%s/binary_sensor/bayrol_%s/config", discoveryPrefix, subTopic)
		payload, err = json.Marshal(binarySensorDiscovery{
			Name:        def.name,
			StateTopic:  stateTopic,
			UniqueID:    uniqueID,
			Device:      dev,
			DeviceClass: def.deviceClass,
			PayloadOn:   "ON",
			PayloadOff:  "OFF",
		})
	} else {
		topic = fmt.Sprintf("%s/sensor/bayrol_%s/config", discoveryPrefix, subTopic)
		payload, err = json.Marshal(sensorDiscovery{
			Name:              def.name,
			StateTopic:        stateTopic,
			UniqueID:          uniqueID,
			Device:            dev,
			DeviceClass:       def.deviceClass,
			UnitOfMeasurement: def.unit,
		})
	}
	return
}

func (b *bridge) publishDiscovery() {
	prefix := b.cfg.DiscoveryPrefix
	if prefix == "" {
		prefix = "homeassistant"
	}
	for _, def := range sensorDefs {
		stateTopic := b.prefix + "/" + def.subTopic
		topic, payload, err := discoveryPayload(prefix, b.serial, stateTopic, def.subTopic, def)
		if err != nil {
			log.Printf("discovery marshal %s: %v", def.subTopic, err)
			continue
		}
		t := b.ha.Publish(topic, 0, true, payload)
		t.Wait()
		if t.Error() != nil {
			log.Printf("discovery publish %s: %v", topic, t.Error())
		}
	}
	log.Printf("discovery: published %d entries", len(sensorDefs))
}
