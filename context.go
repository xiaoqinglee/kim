package kim

import (
	"sync"

	"github.com/klintcheng/kim/logger"
	"github.com/klintcheng/kim/wire"
	"github.com/klintcheng/kim/wire/pkt"
	"google.golang.org/protobuf/proto"
)

// Session read only
type Session interface {
	GetChannelId() string
	GetGateId() string
	GetAccount() string
	GetZone() string
	GetIsp() string
	GetRemoteIP() string
	GetDevice() string
	GetApp() string
	GetTags() []string
}

type Context interface {
	Dispather
	SessionStorage
	Header() *pkt.Header
	ReadBody(val proto.Message) error
	Session() Session
	RespWithError(status pkt.Status, err error) error
	Resp(status pkt.Status, body proto.Message) error
	Dispatch(body proto.Message, recvs ...*Location) error
}

// HandlerFunc defines the handler used
type HandlerFunc func(Context)

// HandlersChain HandlersChain
type HandlersChain []HandlerFunc

// ContextImpl is the most important part of kim
type ContextImpl struct {
	sync.Mutex
	Dispather
	SessionStorage

	handlers HandlersChain
	index    int
	request  *pkt.LogicPkt
	session  Session
}

func BuildContext() Context {
	return &ContextImpl{}
}

// Next execute next handler
func (c *ContextImpl) Next() {
	if c.index >= len(c.handlers) {
		return
	}
	f := c.handlers[c.index]

	f(c)
	c.index++
}

// RespWithError response with error
func (c *ContextImpl) RespWithError(status pkt.Status, err error) error {
	return c.Resp(status, &pkt.ErrorResp{Message: err.Error()})
}

// Resp send a response message to sender, the header of packet copied from request
func (c *ContextImpl) Resp(status pkt.Status, body proto.Message) error {
	packet := pkt.NewFrom(&c.request.Header)
	packet.Status = status
	packet.WriteBody(body)
	packet.Flag = pkt.Flag_Response
	logger.Debugf("<-- Resp to %s command:%s  status: %v body: %s", c.Session().GetAccount(), &c.request.Header, status, body)

	err := c.Push(c.Session().GetGateId(), []string{c.Session().GetChannelId()}, packet)
	if err != nil {
		logger.Error(err)
	}
	return err
}

// Dispatch the packet to the Destination of request,
// the header flag of this packet will be set with FlagDelivery
// exceptMe:  exclude self if self is false
func (c *ContextImpl) Dispatch(body proto.Message, recvs ...*Location) error {
	if len(recvs) == 0 {
		return nil
	}
	packet := pkt.NewFrom(&c.request.Header)
	packet.Flag = pkt.Flag_Push
	packet.WriteBody(body)

	logger.Debugf("<-- Dispatch to %d users command:%s", len(recvs), &c.request.Header)

	// the receivers group by the destination of gateway
	group := make(map[string][]string)
	for _, recv := range recvs {
		if recv.ChannelId == c.Session().GetChannelId() {
			continue
		}
		if _, ok := group[recv.GateId]; !ok {
			group[recv.GateId] = make([]string, 0)
		}
		group[recv.GateId] = append(group[recv.GateId], recv.ChannelId)
	}
	for gateway, ids := range group {
		err := c.Push(gateway, ids, packet)
		if err != nil {
			logger.Error(err)
		}
		return err
	}
	return nil
}

func (c *ContextImpl) reset() {
	c.request = nil
	c.index = 0
	c.handlers = nil
	c.session = nil
}

func (c *ContextImpl) Header() *pkt.Header {
	return &c.request.Header
}

func (c *ContextImpl) ReadBody(val proto.Message) error {
	return c.request.ReadBody(val)
}

func (c *ContextImpl) Session() Session {
	if c.session == nil {
		server, _ := c.request.GetMeta(wire.MetaDestServer)
		c.session = &pkt.Session{
			ChannelId: c.request.ChannelId,
			GateId:    server.(string),
			Tags:      []string{"AutoGenerated"},
		}
	}
	return c.session
}
