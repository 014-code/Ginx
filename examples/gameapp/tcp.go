package gameapp

import (
	"Ginx/examples/gameapp/protocol"
	"Ginx/gface"
	"Ginx/gnet"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const (
	// 保留原有公开名称；编号统一来自 schema 生成物。
	MsgAuthenticate  = protocol.MsgAuthenticate
	MsgHeartbeat     = protocol.MsgHeartbeat
	MsgJoinRoom      = protocol.MsgJoinRoom
	MsgLeaveRoom     = protocol.MsgLeaveRoom
	MsgProgress      = protocol.MsgProgress
	MsgStarterReward = protocol.MsgStarterReward
	MsgUseItem       = protocol.MsgUseItem
)

type Reply struct {
	Code string `json:"code"`
	Data any    `json:"data,omitempty"`
}

// RegisterTCP 只注册路由和连接 Hook，必须在 server.Start 前调用。
// 该入口拥有这两个 Hook；其他业务初始化可在 Service 外组合后注册。
func RegisterTCP(server gface.IServer, service *Service) {
	server.SetOnConnStart(service.Connected)
	server.SetOnConnStop(service.Disconnected)
	for _, id := range []uint32{MsgAuthenticate, MsgHeartbeat, MsgJoinRoom, MsgLeaveRoom, MsgProgress, MsgStarterReward, MsgUseItem} {
		server.AddRouter(id, &tcpRouter{service: service})
	}
}

type tcpRouter struct {
	gnet.BaseRouter
	service *Service
}

func (r *tcpRouter) Handle(request gface.IRequest) {
	conn := request.GetConnection()
	id := conn.GetConnId()
	var result any
	var err error
	switch request.GetMsgID() {
	case MsgAuthenticate:
		var body protocol.AuthenticateRequest
		if !decodeRequest(request.GetData(), &body) || body.Token == "" || len(body.Token) > 128 {
			r.reply(request, Reply{Code: "invalid_request"})
			return
		}
		var playerID uint64
		playerID, err = r.service.Authenticate(id, body.Token)
		result = map[string]uint64{"player_id": playerID}
	case MsgJoinRoom:
		var body protocol.JoinRoomRequest
		if !decodeRequest(request.GetData(), &body) || body.RoomID == 0 {
			r.reply(request, Reply{Code: "invalid_request"})
			return
		}
		result, err = r.service.Join(id, body.RoomID)
	case MsgUseItem:
		var body protocol.UseItemRequest
		if !decodeRequest(request.GetData(), &body) || body.ItemID != "potion" {
			r.reply(request, Reply{Code: "invalid_request"})
			return
		}
		result, err = r.service.UseItem(gface.RequestContext(request), id, body.ItemID)
	case MsgLeaveRoom, MsgHeartbeat, MsgProgress, MsgStarterReward:
		var body struct{}
		if !decodeRequest(request.GetData(), &body) {
			r.reply(request, Reply{Code: "invalid_request"})
			return
		}
		switch request.GetMsgID() {
		case MsgLeaveRoom:
			err = r.service.Leave(id)
		case MsgHeartbeat:
			err = r.service.Heartbeat(id)
		case MsgProgress:
			result, err = r.service.ConnectionProgress(gface.RequestContext(request), id)
		case MsgStarterReward:
			result, err = r.service.ClaimStarter(gface.RequestContext(request), id)
		}
	}
	if err != nil {
		code := "internal_error"
		for _, expected := range []error{ErrUnauthorized, ErrRoomNotFound, ErrRoomUnavailable, ErrNotInRoom, ErrRewardClaimed, ErrItemUnavailable} {
			if errors.Is(err, expected) {
				code = expected.Error()
				break
			}
		}
		r.reply(request, Reply{Code: code})
		return
	}
	r.reply(request, Reply{Code: "ok", Data: result})
}

func (r *tcpRouter) reply(request gface.IRequest, value Reply) {
	data, err := json.Marshal(value)
	if err == nil {
		err = request.GetConnection().SendBuffMsg(request.GetMsgID(), data)
	}
	if err != nil {
		request.GetConnection().Stop()
	}
}

func decodeRequest(data []byte, value any) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return false
	}
	return decoder.Decode(new(any)) == io.EOF
}
