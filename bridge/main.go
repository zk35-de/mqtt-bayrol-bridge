package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Config struct {
	BayrolBroker struct {
		Host   string `yaml:"host"`
		Port   int    `yaml:"port"`
		Serial string `yaml:"serial"`
	} `yaml:"bayrol_broker"`
	HABroker struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"ha_broker"`
	OutputPrefix string `yaml:"output_prefix"`
}

var (
	rePH    = regexp.MustCompile(`pH\t: ([0-9.]+)`)
	reTemp  = regexp.MustCompile(`Temp\.\t: ([0-9.]+)`)
	reSalz  = regexp.MustCompile(`Salz\t: ([0-9.]+) g/l`)
	reRedox = regexp.MustCompile(`Redox\t: ([0-9]+) mV`)
)

// numericVal extracts "v" field as string from Bayrol JSON {"t":"x","v":123,...}
func numericVal(payload []byte) (string, bool) {
	var msg map[string]interface{}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return "", false
	}
	switch v := msg["v"].(type) {
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case string:
		return v, true
	}
	return "", false
}

type bridge struct {
	cfg    Config
	ha     mqtt.Client
	prefix string
	serial string
}

func (b *bridge) publish(subTopic, value string) {
	topic := b.prefix + "/" + subTopic
	t := b.ha.Publish(topic, 0, true, value)
	t.Wait()
	if t.Error() != nil {
		log.Printf("publish %s: %v", topic, t.Error())
	}
}

func (b *bridge) handle(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Strip d02/<serial>/v/
	vPrefix := "d02/" + b.serial + "/v/"
	if !strings.HasPrefix(topic, vPrefix) {
		return
	}
	id := strings.TrimPrefix(topic, vPrefix)

	switch id {
	case "1":
		// Kalibrierungsreferenztemperatur
		var m struct {
			V string `json:"v"`
		}
		if err := json.Unmarshal(payload, &m); err == nil {
			b.publish("temperatur_ref", m.V)
		}

	case "4.82":
		if v, ok := numericVal(payload); ok {
			b.publish("redox", v)
		}

	case "4.78":
		if v, ok := numericVal(payload); ok {
			b.publish("salzgehalt_pct", v)
		}

	case "4.92":
		if v, ok := numericVal(payload); ok {
			b.publish("se_produktion", v)
		}

	case "4.98":
		if v, ok := numericVal(payload); ok {
			b.publish("ph_mv", v)
		}

	case "4.176":
		if v, ok := numericVal(payload); ok {
			b.publish("se_betriebsstunden", v)
		}

	case "2":
		// Device info – direkt weiterleiten
		b.publish("device_info", string(payload))

	case "10":
		// Filterpumpe: "8.5" in Array → OFF
		var m struct {
			V []string `json:"v"`
		}
		if err := json.Unmarshal(payload, &m); err == nil {
			state := "ON"
			for _, id := range m.V {
				if id == "8.5" {
					state = "OFF"
					break
				}
			}
			b.publish("filterpumpe", state)
		}

	case "16":
		// Alert-Text – parst pH, Temp, Salz, Redox und Betreff
		var m struct {
			Subject string `json:"subject"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(payload, &m); err != nil {
			return
		}
		if m.Subject != "" {
			b.publish("alarm_subject", m.Subject)
		}
		if ms := rePH.FindStringSubmatch(m.Text); len(ms) == 2 {
			b.publish("ph", ms[1])
		}
		if ms := reTemp.FindStringSubmatch(m.Text); len(ms) == 2 {
			b.publish("temperatur", ms[1])
		}
		if ms := reSalz.FindStringSubmatch(m.Text); len(ms) == 2 {
			b.publish("salzgehalt_gpl", ms[1])
		}
		if ms := reRedox.FindStringSubmatch(m.Text); len(ms) == 2 {
			b.publish("redox_alert", ms[1])
		}
	}
}

func connect(broker, clientID, user, pass string, onConnect mqtt.OnConnectHandler) mqtt.Client {
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetUsername(user).
		SetPassword(pass).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10 * time.Second).
		SetOnConnectHandler(onConnect)

	c := mqtt.NewClient(opts)
	for {
		if t := c.Connect(); t.Wait() && t.Error() != nil {
			log.Printf("connect %s: %v – retry in 10s", broker, t.Error())
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}
	return c
}

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("config parse: %v", err)
	}
	if cfg.OutputPrefix == "" {
		cfg.OutputPrefix = "bayrol/pool"
	}

	b := &bridge{
		cfg:    cfg,
		prefix: cfg.OutputPrefix,
		serial: cfg.BayrolBroker.Serial,
	}

	// Connect to HA MQTT first
	haBroker := fmt.Sprintf("tcp://%s:%d", cfg.HABroker.Host, cfg.HABroker.Port)
	log.Printf("connecting HA broker %s", haBroker)
	b.ha = connect(haBroker, "bayrol-bridge-ha", cfg.HABroker.Username, cfg.HABroker.Password, func(c mqtt.Client) {
		log.Println("HA broker connected")
	})

	// Connect to local Mosquitto (Bayrol-facing)
	bayrolBroker := fmt.Sprintf("tcp://%s:%d", cfg.BayrolBroker.Host, cfg.BayrolBroker.Port)
	subTopic := fmt.Sprintf("d02/%s/v/#", cfg.BayrolBroker.Serial)
	log.Printf("connecting Bayrol broker %s, subscribing %s", bayrolBroker, subTopic)

	connect(bayrolBroker, "bayrol-bridge-local", "", "", func(c mqtt.Client) {
		log.Println("Bayrol broker connected, subscribing")
		if t := c.Subscribe(subTopic, 0, b.handle); t.Wait() && t.Error() != nil {
			log.Printf("subscribe: %v", t.Error())
		}
	})

	log.Println("bridge running")
	select {}
}
