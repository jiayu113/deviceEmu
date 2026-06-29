package mqtt

import (
	"fmt"
	"log"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/jiayu113/deviceemu/internal/metrics"
)

// MessageHandler 是上层处理收到消息的回调
type MessageHandler func(topic string, payload []byte)

// Options 构造 Client 所需的全部参数
type Options struct {
	Broker    string
	ClientID  string
	Username  string
	Password  string
	Keepalive time.Duration

	// 遗嘱(LWT):连接异常断开时,broker 替设备发这条消息
	WillTopic    string
	WillPayload  []byte
	WillQoS      byte
	WillRetained bool
}

// Client 是对 paho 的薄封装,只向上暴露 Connect/Subscribe/Publish/Disconnect
type Client struct {
	cli      paho.Client
	clientID string
}

// New 构造(还没连)
func New(opts Options) *Client {
	pahoOpts := paho.NewClientOptions().
		AddBroker(opts.Broker).
		SetClientID(opts.ClientID).
		SetUsername(opts.Username).
		SetPassword(opts.Password).
		SetKeepAlive(opts.Keepalive).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectTimeout(10 * time.Second)

	if opts.WillTopic != "" {
		pahoOpts.SetBinaryWill(opts.WillTopic, opts.WillPayload, opts.WillQoS, opts.WillRetained)
	}
	pahoOpts.OnConnect = func(_ paho.Client) {
		log.Printf("[mqtt] %s connected", opts.ClientID)
	}
	pahoOpts.OnConnectionLost = func(_ paho.Client, err error) {
		log.Printf("[mqtt] %s connection lost: %v", opts.ClientID, err)
	}
	// 重连
	pahoOpts.SetReconnectingHandler(func(_ paho.Client, _ *paho.ClientOptions) {
		metrics.MQTTReconnects.Inc()
	})

	return &Client{
		cli: paho.NewClient(pahoOpts), clientID: opts.ClientID,
	}
}

// Connect 阻塞直到连上或超时
func (c *Client) Connect() error {
	tok := c.cli.Connect()
	if !tok.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("mqtt connect timeout")
	}
	return tok.Error()
}

// Subscribe 订阅 topic,收到消息回调 handler
func (c *Client) Subscribe(topic string, qos byte, handler MessageHandler) error {
	tok := c.cli.Subscribe(topic, qos, func(_ paho.Client, m paho.Message) {
		handler(m.Topic(), m.Payload())
	})
	tok.Wait()
	return tok.Error()
}

// Publish 发布一条消息
func (c *Client) Publish(topic string, qos byte, payload []byte, retained bool) error {
	tok := c.cli.Publish(topic, qos, retained, payload)
	tok.Wait()
	return tok.Error()
}

// Disconnect 优雅断开(等 250ms 让在途消息发完)
func (c *Client) Disconnect() {
	c.cli.Disconnect(250)
	log.Printf("[mqtt] %s disconnected", c.clientID)
}
