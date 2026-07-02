package sip

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

// 最小合法 SDP(注意 SIP 要求 CRLF 行尾)。
const minimalSDP = "v=0\r\n" +
	"o=deviceemu 0 0 IN IP4 127.0.0.1\r\n" +
	"s=DeviceEmu\r\n" +
	"c=IN IP4 127.0.0.1\r\n" +
	"t=0 0\r\n" +
	"m=audio 16000 RTP/AVP 0\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n"

// Config 是 sip.Client 所需参数(由上层从 config.SIPConfig 填)
type Config struct {
	Server        string // "192.168.1.10:5060"
	Username      string
	Password      string
	Domain        string // "192.168.1.10":From 域 / realm 必须落在 FreeSWITCH 认的域上
	LocalHost     string // "192.168.1.10":Contact 对外宣告的 IP
	LocalPort     int    // 5066:本机 UA 监听端口(收响应 / NOTIFY / INVITE)
	Expiry        int    // 注册有效期(秒)
	RTPPort       int
	AnswerEnabled bool
}

// Client 是运行时的状态机
type Client struct {
	cfg        Config
	ua         *sipgo.UserAgent
	client     *sipgo.Client
	server     *sipgo.Server
	dialogs    *sipgo.DialogClientCache
	dialogSrv  *sipgo.DialogServerCache // 被叫对话
	contact    sip.ContactHeader        // 主/被叫共用的 Contact
	toneHz     float64                  // 本设备音调频率,可区分哪路
	registered atomic.Bool              // 原子布尔值,供 Device 遥测安全读取
}

// New 构造 UA + server(监听端口)+ client。
func New(cfg Config) (*Client, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("deviceemu-" + cfg.Username))
	if err != nil {
		return nil, fmt.Errorf("new ua: %w", err)
	}

	srv, err := sipgo.NewServer(ua)
	if err != nil {
		ua.Close()
		return nil, fmt.Errorf("new server: %w", err)
	}

	// 对 FreeSWITCH 注册成功后主动推送的 NOTIFY
	// 直接回 200 OK,不做业务处理——避免命中 sipgo 默认的 "handler not found" WARN 日志
	srv.OnRequest(sip.NOTIFY, func(req *sip.Request, tx sip.ServerTransaction) {
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		if err := tx.Respond(res); err != nil {
			log.Printf("[sip] respond to NOTIFY: %v", err)
		}
	})

	// 同步绑定本地监听端口:立刻完成 bind。
	// 绑 0.0.0.0 而非具体 IP,可避开「IP 不在网卡上 → cannot assign requested address」。
	listenAddr := fmt.Sprintf("0.0.0.0:%d", cfg.LocalPort)
	conn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		ua.Close()
		return nil, fmt.Errorf("sip bind %s: %w", listenAddr, err)
	}

	// socket 已就绪,后台收包(注册响应 / FreeSWITCH 的 NOTIFY / Day3 的 INVITE)。
	// 没有 time.Sleep:bind 已同步完成,执行顺序确定,无并发竞态。
	go func() {
		if err := srv.ServeUDP(conn); err != nil {
			log.Printf("[sip] serve udp stopped: %v", err)
		}
	}()

	// 不传 WithClientAddr,Via 如实 → 响应凭 received 回到本 socket。
	cli, err := sipgo.NewClient(ua)
	if err != nil {
		conn.Close()
		ua.Close()
		return nil, fmt.Errorf("new client: %w", err)
	}

	contact := sip.ContactHeader{
		Address: sip.Uri{User: cfg.Username, Host: cfg.LocalHost, Port: cfg.LocalPort},
	}
	dialogs := sipgo.NewDialogClientCache(cli, contact)
	c := &Client{
		cfg: cfg, ua: ua, client: cli, server: srv, dialogs: dialogs,
		contact: contact,
		// 每设备一个可区分频率:350~1300Hz 之间,靠 RTP 端口散开
		toneHz: 350 + float64(cfg.RTPPort%20)*48,
	}
	if cfg.AnswerEnabled {
		c.enableAnswer()
	}
	return c, nil
}

// Register 执行 REGISTER:首发 → 收 401 → 带 digest 重发 → 期望 200
func (c *Client) Register(ctx context.Context) error {
	recipient := sip.Uri{}
	if err := sip.ParseUri(fmt.Sprintf("sip:%s@%s", c.cfg.Username, c.cfg.Server), &recipient); err != nil {
		return fmt.Errorf("parse recipient uri: %w", err)
	}

	req := sip.NewRequest(sip.REGISTER, recipient)

	// 显式钉死 From 域 = FreeSWITCH 认的域(cfg.Domain)。
	// FreeSWITCH 默认 challenge-realm=auto_from:realm 跟着 From 域走。
	// 若 From 域是 sipgo 默认的 localhost,realm 就成 localhost,
	// FreeSWITCH 去 localhost 域里找用户 1001 找不到 → 403。
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
			URI:      recipient.String(), // 与 Request-URI 一致即可,带不带用户名服务器都按本字段重算
			Username: c.cfg.Username,
			Password: c.cfg.Password, // 注意:容器镜像可能改了默认密码,以 vars.xml 实查为准
		})
		if err != nil {
			return fmt.Errorf("compute digest: %w", err)
		}

		newReq := req.Clone()
		newReq.RemoveHeader("Via") // 交给 ClientRequestAddVia 重新生成
		newReq.AppendHeader(sip.NewHeader("Authorization", cred.String()))

		tx2, err := c.client.TransactionRequest(ctx, newReq,
			sipgo.ClientRequestIncreaseCSEQ, // CSeq 必须递增
			sipgo.ClientRequestAddVia)       // 重发要带新的 Via
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

// Call 发起一次完整呼叫:INVITE → 等 2xx → ACK →(保持 hold)→ BYE
func (c *Client) Call(ctx context.Context, target string, hold time.Duration) error {
	recipient := sip.Uri{}
	if err := sip.ParseUri(target, &recipient); err != nil {
		return fmt.Errorf("parse callee uri %q: %w", target, err)
	}

	session, err := c.dialogs.Invite(ctx, recipient, []byte(minimalSDP),
		sip.NewHeader("Content-Type", "application/sdp"))
	if err != nil {
		return fmt.Errorf("invite: %w", err)
	}
	defer session.Close()

	// 等应答:内部处理 100/180,拿到 2xx 才返回;遇 407 用账号自动带认证
	if err := session.WaitAnswer(ctx, sipgo.AnswerOptions{
		Username: c.cfg.Username,
		Password: c.cfg.Password,
		OnResponse: func(res *sip.Response) error {
			log.Printf("[sip] call <= %d", res.StatusCode)
			return nil
		},
	}); err != nil {
		return fmt.Errorf("wait answer: %w", err)
	}

	// 收到 200 必须发 ACK,否则对端重传 200 然后超时挂断
	if err := session.Ack(ctx); err != nil {
		return fmt.Errorf("ack: %w", err)
	}
	log.Printf("[sip] call established, holding %s", hold)

	select {
	case <-time.After(hold):
	case <-ctx.Done(): // hangup / 关机
	}
	byeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.Bye(byeCtx); err != nil {
		return fmt.Errorf("bye: %w", err)
	}
	log.Printf("[sip] call ended (BYE sent)")
	return nil
}

// Close 释放 server / client / ua(三层都要关,否则监听端口和 goroutine 泄漏)
func (c *Client) Close() {
	if c.server != nil {
		_ = c.server.Close() // 关掉监听 socket
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
