package sip

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

// Config 是 sip.Client 所需参数(由上层从 config.SIPConfig 填)
type Config struct {
	Server    string // "127.0.0.1:5060"
	Username  string
	Password  string
	Domain    string // "127.0.0.1"
	LocalHost string // "127.0.0.1"
	LocalPort int
	Expiry    int // 注册有效期(秒)
}

// Client 是运行时的状态机
type Client struct {
	cfg     Config
	ua      *sipgo.UserAgent
	client  *sipgo.Client
	server  *sipgo.Server
	dialogs *sipgo.DialogClientCache
}

// New 构造 UA + client + dialog cache(绑定本地端口,Contact 才真实可回连)
func New(cfg Config) (*Client, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("deviceemu-" + cfg.Username))
	if err != nil {
		return nil, fmt.Errorf("new ua: %w", err)
	}
	// 造一个 Server，并真正向操作系统 bind(5066)
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("new server: %w", err)
	}

	// 强制监听 0.0.0.0 (所有网卡)。
	bindAddr := fmt.Sprintf("0.0.0.0:%d", cfg.LocalPort)
	go func() {
		if err := srv.ListenAndServe(context.Background(), "udp", bindAddr); err != nil {
			log.Printf("[sip] server listen error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 让 Client 自动“蹭” Server 已经建好的 5066 端口去发包，彻底杜绝随机端口和端口冲突！
	cli, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	contact := sip.ContactHeader{
		Address: sip.Uri{User: cfg.Username, Host: cfg.LocalHost, Port: cfg.LocalPort},
	}
	dialogs := sipgo.NewDialogClientCache(cli, contact)

	return &Client{
		cfg: cfg, ua: ua, client: cli, server: srv, dialogs: dialogs,
	}, nil
}

// Register 执行 REGISTER:首发 → 收 401 → 带 digest 重发 → 期望 200
func (c *Client) Register(ctx context.Context) error {
	recipient := sip.Uri{}
	if err := sip.ParseUri(fmt.Sprintf("sip:%s@%s", c.cfg.Username, c.cfg.Server), &recipient); err != nil {
		return fmt.Errorf("parse recipient uri: %w", err)
	}

	req := sip.NewRequest(sip.REGISTER, recipient)
	fromHeader := &sip.FromHeader{
		DisplayName: c.cfg.Username,
		Address:     sip.Uri{User: c.cfg.Username, Host: c.cfg.Domain},
		Params:      sip.NewParams(),
	}
	fromHeader.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(fromHeader)

	req.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<sip:%s@%s:%d>", c.cfg.Username, c.cfg.LocalHost, c.cfg.LocalPort)))
	req.AppendHeader(sip.NewHeader("Expires", fmt.Sprintf("%d", c.cfg.Expiry)))
	req.SetTransport("UDP")

	// 第一次发,期望 401
	tx, err := c.client.TransactionRequest(ctx, req, sipgo.ClientRequestBuild)
	if err != nil {
		return fmt.Errorf("register tx: %w", err)
	}
	res, err := waitResponse(ctx, tx)
	tx.Terminate()
	if err != nil {
		return err
	}
	log.Printf("[sip] REGISTER <= %d", res.StatusCode)

	// 收到 401:算 digest 重发
	if res.StatusCode == sip.StatusUnauthorized {
		wwwAuth := res.GetHeader("www-Authenticate")
		if wwwAuth == nil {
			return fmt.Errorf("401 without WWW-Authenticate")
		}
		chal, err := digest.ParseChallenge(wwwAuth.Value())
		if err != nil {
			return fmt.Errorf("parse challenge: %w", err)
		}
		cred, err := digest.Digest(chal, digest.Options{
			Method:   sip.REGISTER.String(),
			URI:      recipient.String(),
			Username: c.cfg.Username,
			Password: c.cfg.Password,
		})
		if err != nil {
			return fmt.Errorf("compute digest: %w", err)
		}

		newReq := req.Clone()
		newReq.RemoveHeader("Via")
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

		tx2, err := c.client.TransactionRequest(ctx, newReq, sipgo.ClientRequestIncreaseCSEQ, sipgo.ClientRequestAddVia)
		if err != nil {
			return fmt.Errorf("register auth tx: %w", err)
		}
		res, err = waitResponse(ctx, tx2)
		tx2.Terminate()
		if err != nil {
			return err
		}
		log.Printf("[sip] REGISTER (auth) <= %d", res.StatusCode)
	}

	if res.StatusCode != sip.StatusOK {
		return fmt.Errorf("register failed: status %d", res.StatusCode)
	}
	log.Printf("[sip] %s registered to %s", c.cfg.Username, c.cfg.Server)
	return nil
}

// Close 释放 UA / client
func (c *Client) Close() {
	if c.server != nil {
		_ = c.server.Close()
	}
	if c.client != nil {
		_ = c.client.Close()
	}
	if c.ua != nil {
		_ = c.ua.Close()
	}
}

// waitResponse 从事务里等一个响应(或 ctx 取消 / 事务终止)
func waitResponse(ctx context.Context, tx sip.ClientTransaction) (*sip.Response, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-tx.Done():
		return nil, fmt.Errorf("transaction terminates before response")
	case res := <-tx.Responses():
		return res, nil
	}
}
