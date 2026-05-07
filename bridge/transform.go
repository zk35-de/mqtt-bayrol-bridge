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
// Returns the serial extracted from the topic (may be empty on parse failure).
// Pure function with no side-effects.
func transform(topic string, payload []byte) (serial string, pubs []Publication) {
	// topic format: d02/{serial}/v/{id}
	parts := strings.SplitN(topic, "/", 4)
	if len(parts) != 4 || parts[0] != "d02" || parts[2] != "v" {
		return "", nil
	}
	serial = parts[1]
	id := parts[3]

	switch id {
	case "1":
		var m struct {
			V string `json:"v"`
		}
		if err := json.Unmarshal(payload, &m); err == nil && m.V != "" {
			pubs = []Publication{{"temperatur_ref", m.V}}
		}

	case "4.82":
		if v, ok := numericVal(payload); ok {
			pubs = []Publication{{"redox", v}}
		}

	case "4.78":
		// Excel: e_num_var_ph → pH-Wert × 10 (z.B. 74 = 7.4 pH)
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				ph := math.Round(raw/10.0*100) / 100
				pubs = []Publication{{"ph", strconv.FormatFloat(ph, 'f', 2, 64)}}
			}
		}

	case "4.92":
		if v, ok := numericVal(payload); ok {
			pubs = []Publication{{"se_produktion", v}}
		}

	case "4.98":
		// community finding (harb70/bayrolas5-nodered, same AS5 model): v/4.98 = temperature × 10
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				temp := math.Round(raw/10.0*10) / 10
				pubs = []Publication{{"temperatur", strconv.FormatFloat(temp, 'f', 1, 64)}}
			}
		}

	case "4.100":
		// community finding: v/4.100 = salt content × 10 in g/l
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				gpl := math.Round(raw/10.0*10) / 10
				pubs = []Publication{{"salzgehalt", strconv.FormatFloat(gpl, 'f', 1, 64)}}
			}
		}

	case "4.182":
		// Excel: e_num_var_ph_minus → pH-Minus-Dosiermenge (Einheit unbekannt, ÷10)
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				val := math.Round(raw/10.0*100) / 100
				pubs = []Publication{{"ph_minus", strconv.FormatFloat(val, 'f', 2, 64)}}
			}
		}

	case "4.176":
		// unit is minutes; convert to hours
		var m map[string]interface{}
		if err := json.Unmarshal(payload, &m); err == nil {
			if raw, ok := m["v"].(float64); ok {
				hours := int64(math.Round(raw / 60.0))
				pubs = []Publication{{"se_betriebsstunden", strconv.FormatInt(hours, 10)}}
			}
		}

	case "2":
		var info struct {
			TypeName  string `json:"type_name"`
			Serial    string `json:"serial_no"`
			SWVersion string `json:"sw_version"`
		}
		if err := json.Unmarshal(payload, &info); err == nil {
			pubs = []Publication{
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
			pubs = []Publication{{"filterpumpe", state}}
		}

	case "16":
		var m struct {
			Subject string `json:"subject"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(payload, &m); err != nil {
			return serial, nil
		}
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
	}

	return serial, pubs
}
