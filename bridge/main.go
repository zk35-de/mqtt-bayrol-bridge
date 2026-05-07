package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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
	OutputPrefix    string `yaml:"output_prefix"`
	DiscoveryPrefix string `yaml:"discovery_prefix"`
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
	for _, pub := range transform(b.serial, msg.Topic(), msg.Payload()) {
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
	if cfg.OutputPrefix == "" {
		cfg.OutputPrefix = "bayrol/pool"
	}

	b := &bridge{
		cfg:    cfg,
		prefix: cfg.OutputPrefix,
		serial: cfg.BayrolBroker.Serial,
	}

	haBroker := fmt.Sprintf("tcp://%s:%d", cfg.HABroker.Host, cfg.HABroker.Port)
	log.Printf("connecting HA broker %s", haBroker)
	b.ha = connect(haBroker, "bayrol-bridge-ha", cfg.HABroker.Username, cfg.HABroker.Password, func(c mqtt.Client) {
		log.Println("HA broker connected")
		b.publishDiscovery()
	})

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
