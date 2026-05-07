package main

import (
	"testing"
)

const serial = "23ASE2-06017"

func topic(id string) string {
	return "d02/" + serial + "/v/" + id
}

func TestTransform_UnknownTopic(t *testing.T) {
	pubs := transform(serial, "other/topic", []byte(`{}`))
	if len(pubs) != 0 {
		t.Fatalf("expected no publications, got %v", pubs)
	}
}

func TestTransform_TemperaturRef(t *testing.T) {
	pubs := transform(serial, topic("1"), []byte(`{"v":"17.2"}`))
	assertSingle(t, pubs, "temperatur_ref", "17.2")
}

func TestTransform_Redox(t *testing.T) {
	pubs := transform(serial, topic("4.82"), []byte(`{"v":750}`))
	assertSingle(t, pubs, "redox", "750")
}

func TestTransform_PH_v478(t *testing.T) {
	// Excel e_num_var_ph: raw=74 → 74/10 = 7.40 pH
	pubs := transform(serial, topic("4.78"), []byte(`{"v":74}`))
	assertSingle(t, pubs, "ph", "7.40")
}

func TestTransform_PH_v478_Low(t *testing.T) {
	// raw=0 → 0.00 pH
	pubs := transform(serial, topic("4.78"), []byte(`{"v":0}`))
	assertSingle(t, pubs, "ph", "0.00")
}

func TestTransform_SEProduktion(t *testing.T) {
	pubs := transform(serial, topic("4.92"), []byte(`{"v":85}`))
	assertSingle(t, pubs, "se_produktion", "85")
}

func TestTransform_Temperatur(t *testing.T) {
	// 185 / 10 = 18.5°C (community finding: harb70/bayrolas5-nodered, same AS5 model)
	pubs := transform(serial, topic("4.98"), []byte(`{"v":185}`))
	assertSingle(t, pubs, "temperatur", "18.5")
}

func TestTransform_SalzgehaltExact(t *testing.T) {
	// 60 / 10 = 6.0 g/l (community finding: harb70/bayrolas5-nodered)
	pubs := transform(serial, topic("4.100"), []byte(`{"v":60}`))
	assertSingle(t, pubs, "salzgehalt", "6.0")
}

func TestTransform_PHMinus(t *testing.T) {
	// Excel e_num_var_ph_minus: pH-Minus-Dosiermenge ÷10
	pubs := transform(serial, topic("4.182"), []byte(`{"v":73}`))
	assertSingle(t, pubs, "ph_minus", "7.30")
}

func TestTransform_SEBetriebsstunden(t *testing.T) {
	// 1200 minutes / 60 = 20 hours
	pubs := transform(serial, topic("4.176"), []byte(`{"v":1200}`))
	assertSingle(t, pubs, "se_betriebsstunden", "20")
}

func TestTransform_SEBetriebsstunden_Round(t *testing.T) {
	// 1342881 minutes / 60 = 22381.35 → 22381h
	pubs := transform(serial, topic("4.176"), []byte(`{"v":1342881}`))
	assertSingle(t, pubs, "se_betriebsstunden", "22381")
}

func TestTransform_DeviceInfo(t *testing.T) {
	payload := `{"type_name":"Automatic SALT","serial_no":"23ASE2-06017","sw_version":"v1.53 (230524)"}`
	pubs := transform(serial, topic("2"), []byte(payload))
	if len(pubs) != 3 {
		t.Fatalf("expected 3 publications, got %d", len(pubs))
	}
	want := map[string]string{
		"device_type":       "Automatic SALT",
		"device_serial":     "23ASE2-06017",
		"device_sw_version": "v1.53 (230524)",
	}
	for _, p := range pubs {
		if want[p.SubTopic] != p.Value {
			t.Errorf("subtopic %s: want %q got %q", p.SubTopic, want[p.SubTopic], p.Value)
		}
	}
}

func TestTransform_FilterpumpeON(t *testing.T) {
	pubs := transform(serial, topic("10"), []byte(`{"v":["1.0","2.0"]}`))
	assertSingle(t, pubs, "filterpumpe", "ON")
}

func TestTransform_FilterpumpeOFF(t *testing.T) {
	pubs := transform(serial, topic("10"), []byte(`{"v":["1.0","8.5","2.0"]}`))
	assertSingle(t, pubs, "filterpumpe", "OFF")
}

func TestTransform_Alert_AllFields(t *testing.T) {
	payload := `{
		"subject": "Chlor-Alarm",
		"text": "pH\t: 7.3\nTemp.\t: 18.5\nSalz\t: 4.2 g/l\nRedox\t: 750 mV"
	}`
	pubs := transform(serial, topic("16"), []byte(payload))

	want := map[string]string{
		"alarm_subject":  "Chlor-Alarm",
		"ph":             "7.3",
		"temperatur":     "18.5",
		"salzgehalt_gpl": "4.2",
		"redox_alert":    "750",
	}
	if len(pubs) != len(want) {
		t.Fatalf("expected %d publications, got %d: %v", len(want), len(pubs), pubs)
	}
	got := make(map[string]string, len(pubs))
	for _, p := range pubs {
		got[p.SubTopic] = p.Value
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("topic %q: want %q, got %q", k, v, got[k])
		}
	}
}

func TestTransform_Alert_NoSubject(t *testing.T) {
	payload := `{"subject":"","text":"pH\t: 7.1\nTemp.\t: 19.0\nSalz\t: 4.0 g/l\nRedox\t: 700 mV"}`
	pubs := transform(serial, topic("16"), []byte(payload))
	for _, p := range pubs {
		if p.SubTopic == "alarm_subject" {
			t.Error("alarm_subject should not be published when subject is empty")
		}
	}
	if len(pubs) != 4 {
		t.Fatalf("expected 4 publications, got %d", len(pubs))
	}
}

func TestTransform_Alert_InvalidJSON(t *testing.T) {
	pubs := transform(serial, topic("16"), []byte(`not json`))
	if len(pubs) != 0 {
		t.Fatalf("expected no publications on invalid JSON, got %v", pubs)
	}
}

func TestTransform_NumericVal_Float(t *testing.T) {
	v, ok := numericVal([]byte(`{"v":18.5}`))
	if !ok || v != "18.5" {
		t.Errorf("got ok=%v v=%q, want true 18.5", ok, v)
	}
}

func TestTransform_NumericVal_Integer(t *testing.T) {
	v, ok := numericVal([]byte(`{"v":750}`))
	if !ok || v != "750" {
		t.Errorf("got ok=%v v=%q, want true 750", ok, v)
	}
}

func TestTransform_NumericVal_String(t *testing.T) {
	v, ok := numericVal([]byte(`{"v":"17.2"}`))
	if !ok || v != "17.2" {
		t.Errorf("got ok=%v v=%q, want true 17.2", ok, v)
	}
}

func assertSingle(t *testing.T, pubs []Publication, subTopic, value string) {
	t.Helper()
	if len(pubs) != 1 {
		t.Fatalf("expected 1 publication, got %d: %v", len(pubs), pubs)
	}
	if pubs[0].SubTopic != subTopic {
		t.Errorf("SubTopic: want %q, got %q", subTopic, pubs[0].SubTopic)
	}
	if pubs[0].Value != value {
		t.Errorf("Value: want %q, got %q", value, pubs[0].Value)
	}
}
