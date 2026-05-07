package main

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	rePH    = regexp.MustCompile(`pH\t: ([0-9.]+)`)
	reTemp  = regexp.MustCompile(`Temp\.\t: ([0-9.]+)`)
	reSalz  = regexp.MustCompile(`Salz\t: ([0-9.]+) g/l`)
	reRedox = regexp.MustCompile(`Redox\t: ([0-9]+) mV`)
)

// Publication is a single MQTT output message.
type Publication struct {
	SubTopic string
	Value    string
}

// transform converts a Bayrol MQTT message into zero or more HA publications.
// It is a pure function with no side-effects.
func transform(serial, topic string, payload []byte) []Publication {
	vPrefix := "d02/" + serial + "/v/"
	if !strings.HasPrefix(topic, vPrefix) {
		return nil
	}
	id := strings.TrimPrefix(topic, vPrefix)

	switch id {
	case "1":
		var m struct {
			V string `json:"v"`
		}
		if err := json.Unmarshal(payload, &m); err == nil && m.V != "" {
			return []Publication{{"temperatur_ref", m.V}}
		}

	case "4.82":
		if v, ok := numericVal(payload); ok {
			return []Publication{{"redox", v}}
		}

	case "4.78":
		// raw value 0-99 index; linear approximation: 99 ≈ 8 g/l (from reverse engineering)
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				gpl := math.Round(raw*8.0/99.0*10) / 10
				return []Publication{{"salzgehalt", strconv.FormatFloat(gpl, 'f', 1, 64)}}
			}
		}

	case "4.92":
		if v, ok := numericVal(payload); ok {
			return []Publication{{"se_produktion", v}}
		}

	case "4.98":
		// community finding (harb70/bayrolas5-nodered, same AS5 model): v/4.98 = temperature × 10
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				temp := math.Round(raw/10.0*10) / 10
				return []Publication{{"temperatur", strconv.FormatFloat(temp, 'f', 1, 64)}}
			}
		}

	case "4.100":
		// community finding: v/4.100 = salt content × 10 in g/l
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				gpl := math.Round(raw/10.0*10) / 10
				return []Publication{{"salzgehalt", strconv.FormatFloat(gpl, 'f', 1, 64)}}
			}
		}

	case "4.182":
		// community finding: v/4.182 = pH × 10
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				ph := math.Round(raw/10.0*100) / 100
				return []Publication{{"ph", strconv.FormatFloat(ph, 'f', 2, 64)}}
			}
		}

	case "4.176":
		// unit is minutes; convert to hours
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				hours := int64(math.Round(raw / 60.0))
				return []Publication{{"se_betriebsstunden", strconv.FormatInt(hours, 10)}}
			}
		}

	case "2":
		var info struct {
			TypeName  string `json:"type_name"`
			Serial    string `json:"serial_no"`
			SWVersion string `json:"sw_version"`
		}
		if err := json.Unmarshal(payload, &info); err == nil {
			return []Publication{
				{"device_type", info.TypeName},
				{"device_serial", info.Serial},
				{"device_sw_version", info.SWVersion},
			}
		}

	case "10":
		var m struct {
			V []string `json:"v"`
		}
		if err := json.Unmarshal(payload, &m); err == nil {
			state := "ON"
			for _, item := range m.V {
				if item == "8.5" {
					state = "OFF"
					break
				}
			}
			return []Publication{{"filterpumpe", state}}
		}

	case "16":
		var m struct {
			Subject string `json:"subject"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(payload, &m); err != nil {
			return nil
		}
		var pubs []Publication
		if m.Subject != "" {
			pubs = append(pubs, Publication{"alarm_subject", m.Subject})
		}
		if ms := rePH.FindStringSubmatch(m.Text); len(ms) == 2 {
			pubs = append(pubs, Publication{"ph", ms[1]})
		}
		if ms := reTemp.FindStringSubmatch(m.Text); len(ms) == 2 {
			pubs = append(pubs, Publication{"temperatur", ms[1]})
		}
		if ms := reSalz.FindStringSubmatch(m.Text); len(ms) == 2 {
			pubs = append(pubs, Publication{"salzgehalt_gpl", ms[1]})
		}
		if ms := reRedox.FindStringSubmatch(m.Text); len(ms) == 2 {
			pubs = append(pubs, Publication{"redox_alert", ms[1]})
		}
		return pubs
	}

	return nil
}
