package serv

import (
	"bytes"
	"strings"
	"time"

	"github.com/klintcheng/kim"
	"github.com/klintcheng/kim/container"
	"github.com/klintcheng/kim/logger"
	"github.com/klintcheng/kim/wire"
	"github.com/klintcheng/kim/wire/pkt"
	"google.golang.org/protobuf/proto"
)

var log = logger.WithFields(logger.Fields{
	"service": wire.SNChat,
	"pkg":     "serv",
})

// ServHandler ServHandler
type ServHandler struct {
	r          *kim.Router
	cache      kim.SessionStorage
	dispatcher *ServerDispatcher
}

func NewServHandler(r *kim.Router, cache kim.SessionStorage) *ServHandler {
	return &ServHandler{
		r:          r,
		dispatcher: &ServerDispatcher{},
		cache:      cache,
	}
}

// Accept this connection
func (h *ServHandler) Accept(conn kim.Conn, timeout time.Duration) (string, kim.Meta, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	frame, err := conn.ReadFrame()
	if err != nil {
		return "", nil, err
	}

	var req pkt.InnerHandshakeReq
	_ = proto.Unmarshal(frame.GetPayload(), &req)
	log.Info("Accept -- ", req.ServiceId)

	return req.ServiceId, nil, nil
}

// Receive default listener
func (h *ServHandler) Receive(ag kim.Agent, payload []byte) {
	buf := bytes.NewBuffer(payload)
	packet, err := pkt.MustReadLogicPkt(buf)
	if err != nil {
		log.Error(err)
		return
	}
	var session *pkt.Session
	if packet.Command == wire.CommandLoginSignIn {
		server, _ := packet.GetMeta(wire.MetaDestServer)
		session = &pkt.Session{
			ChannelId: packet.ChannelId,
			GateId:    server.(string),
			Tags:      []string{"AutoGenerated"},
		}
	} else {
		// TODO：优化点
		session, err = h.cache.Get(packet.ChannelId)
		if err == kim.ErrSessionNil {
			_ = RespErr(ag, packet, pkt.Status_SessionNotFound)
			return
		} else if err != nil {
			_ = RespErr(ag, packet, pkt.Status_SystemException)
			return
		}
	}
	log.Debugf("recv a message from %s  %s", session, &packet.Header)
	err = h.r.Serve(packet, h.dispatcher, h.cache, session)
	if err != nil {
		log.Warn(err)
	}

}

func RespErr(ag kim.Agent, p *pkt.LogicPkt, status pkt.Status) error {
	packet := pkt.NewFrom(&p.Header)
	packet.Status = status
	packet.Flag = pkt.Flag_Response

	p.AddStringMeta(wire.MetaDestChannels, p.Header.ChannelId)
	return container.Push(ag.ID(), p)
}

type ServerDispatcher struct {
}

func (d *ServerDispatcher) Push(gateway string, channels []string, p *pkt.LogicPkt) error {
	p.AddStringMeta(wire.MetaDestChannels, strings.Join(channels, ","))
	return container.Push(gateway, p)
}

// Disconnect default listener
func (h *ServHandler) Disconnect(id string) error {
	logger.Warnf("close event of %s", id)
	return nil
}
