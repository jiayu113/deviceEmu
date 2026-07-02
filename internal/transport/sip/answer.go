package sip

import (
	"log"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// enableAnswer 装配被叫(UAS)能力:
//   - OnInvite:读入呼叫 → 应答 200→ 后台等挂断
//   - OnAck / OnBye:交给 DialogServerCache 路由到对应会话
func (c *Client) enableAnswer() {
	c.dialogSrv = sipgo.NewDialogServerCache(c.client, c.contact)

	c.server.OnInvite(func(req *sip.Request, tx sip.ServerTransaction) {
		dlg, err := c.dialogSrv.ReadInvite(req, tx)
		if err != nil {
			log.Printf("[sip] %s read invite: %v", c.cfg.Username, err)
			return
		}
		log.Printf("[sip] %s <= INVITE (incoming call)", c.cfg.Username)

		answer := staticAnswerSDP(c.cfg.LocalHost, c.cfg.RTPPort)
		if err := dlg.RespondSDP(answer); err != nil {
			log.Printf("[sip] %s respond invite: %v", c.cfg.Username, err)
			_ = dlg.Close()
			return
		}
		log.Printf("[sip] %s answered call (200 OK)", c.cfg.Username)

		// 后台等挂断,不阻塞 OnInvite(N 路并发时每呼叫各自独立)。
		go func() {
			<-dlg.Context().Done()
			_ = dlg.Close()
			log.Printf("[sip] %s call ended", c.cfg.Username)
		}()
	})

	// ACK / BYE 交给对话缓存路由(按 Call-ID 找到对应会话)
	c.server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {
		if err := c.dialogSrv.ReadAck(req, tx); err != nil {
			log.Printf("[sip] %s read ack: %v", c.cfg.Username, err)
		}
	})
	c.server.OnBye(func(req *sip.Request, tx sip.ServerTransaction) {
		if err := c.dialogSrv.ReadBye(req, tx); err != nil {
			log.Printf("[sip] %s read bye: %v", c.cfg.Username, err)
		}
	})
}

func staticAnswerSDP(host string, rtpPort int) []byte {
	return []byte("v=0\r\n" +
		"o=deviceemu 0 0 IN IP4 " + host + "\r\n" +
		"s=DeviceEmu\r\n" +
		"c=IN IP4 " + host + "\r\n" +
		"t=0 0\r\n" +
		"m=audio " + itoa(rtpPort) + " RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
