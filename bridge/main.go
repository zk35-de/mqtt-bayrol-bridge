package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Config struct {
	BayrolBroker struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"bayrol_broker"`
	HABroker struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"ha_broker"`
	OutputPrefix    string `yaml:"output_prefix"`
	DiscoveryPrefix string `yaml:"discovery_prefix"`
	Web             struct {
		Port int `yaml:"port"`
	} `yaml:"web"`
	Mosquitto struct {
		CertPath string `yaml:"cert_path"`
	} `yaml:"mosquitto"`
	Debug struct {
		Enabled    bool `yaml:"enabled"`
		RawLogSize int  `yaml:"raw_log_size"`
	} `yaml:"debug"`
}

// applyEnvOverrides overlays environment variables on top of config file values.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BAYROL_HOST"); v != "" {
		cfg.BayrolBroker.Host = v
	}
	if v := os.Getenv("BAYROL_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.BayrolBroker.Port = p
		}
	}
	if v := os.Getenv("HA_HOST"); v != "" {
		cfg.HABroker.Host = v
	}
	if v := os.Getenv("HA_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.HABroker.Port = p
		}
	}
	if v := os.Getenv("HA_USERNAME"); v != "" {
		cfg.HABroker.Username = v
	}
	if v := os.Getenv("HA_PASSWORD"); v != "" {
		cfg.HABroker.Password = v
	}
	if v := os.Getenv("OUTPUT_PREFIX"); v != "" {
		cfg.OutputPrefix = v
	}
	if v := os.Getenv("DISCOVERY_PREFIX"); v != "" {
		cfg.DiscoveryPrefix = v
	}
	if v := os.Getenv("WEB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Web.Port = p
		}
	}
	if v := os.Getenv("MOSQUITTO_CERT_PATH"); v != "" {
		cfg.Mosquitto.CertPath = v
	}
	if v := os.Getenv("DEBUG"); v == "true" || v == "1" {
		cfg.Debug.Enabled = true
	}
	if v := os.Getenv("DEBUG_RAW_LOG_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Debug.RawLogSize = n
		}
	}
}

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
	// serial is learned from the first MQTT message; empty until then.
	serialMu sync.RWMutex
	serial   string
	store    *valueStore
	status   *connStatus
	rawLog   *rawLogger
}

func (b *bridge) getSerial() string {
	b.serialMu.RLock()
	defer b.serialMu.RUnlock()
	return b.serial
}

// learnSerial stores the serial on first call and returns true if newly learned.
func (b *bridge) learnSerial(s string) bool {
	if s == "" {
		return false
	}
	b.serialMu.Lock()
	defer b.serialMu.Unlock()
	if b.serial == "" {
		b.serial = s
		return true
	}
	return false
}

func (b *bridge) publish(subTopic, value string) {
	b.store.set(subTopic, value)
	topic := b.prefix + "/" + subTopic
	t := b.ha.Publish(topic, 0, true, value)
	t.Wait()
	if t.Error() != nil {
		log.Printf("publish %s: %v", topic, t.Error())
	}
}

func (b *bridge) handle(_ mqtt.Client, msg mqtt.Message) {
	b.rawLog.log(msg.Topic(), msg.Payload())
	serial, pubs := transform(msg.Topic(), msg.Payload())
	if b.learnSerial(serial) {
		log.Printf("serial learned from MQTT: %s", serial)
		b.publishDiscovery()
	}
	for _, pub := range pubs {
		b.publish(pub.SubTopic, pub.Value)
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

	applyEnvOverrides(&cfg)

	if cfg.OutputPrefix == "" {
		cfg.OutputPrefix = "bayrol/pool"
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}
	if cfg.Mosquitto.CertPath == "" {
		cfg.Mosquitto.CertPath = "/mosquitto/certs/bayrol-server.crt"
	}
	if cfg.Debug.RawLogSize <= 0 {
		cfg.Debug.RawLogSize = 200
	}

	b := &bridge{
		cfg:    cfg,
		prefix: cfg.OutputPrefix,
		store:  newValueStore(),
		status: &connStatus{startedAt: time.Now()},
		rawLog: newRawLogger(cfg.Debug.Enabled, cfg.Debug.RawLogSize),
	}

	haBroker := fmt.Sprintf("tcp://%s:%d", cfg.HABroker.Host, cfg.HABroker.Port)
	log.Printf("connecting HA broker %s", haBroker)
	b.ha = connect(haBroker, "bayrol-bridge-ha", cfg.HABroker.Username, cfg.HABroker.Password, func(c mqtt.Client) {
		log.Println("HA broker connected")
		b.status.setHA(true)
		// Discovery only if serial already known (reconnect case)
		if b.getSerial() != "" {
			b.publishDiscovery()
		}
	})

	bayrolBroker := fmt.Sprintf("tcp://%s:%d", cfg.BayrolBroker.Host, cfg.BayrolBroker.Port)
	log.Printf("connecting Bayrol broker %s", bayrolBroker)

	connect(bayrolBroker, "bayrol-bridge-local", "", "", func(c mqtt.Client) {
		log.Println("Bayrol broker connected, subscribing d02/+/v/#")
		b.status.setBayrol(true)
		if t := c.Subscribe("d02/+/v/#", 0, b.handle); t.Wait() && t.Error() != nil {
			log.Printf("subscribe: %v", t.Error())
		}
	})

	go b.startWebServer(fmt.Sprintf(":%d", cfg.Web.Port), cfgPath)

	log.Println("bridge running")
	select {}
}
