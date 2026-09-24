// Package gameprotocol 演示在可选 GameMessage 信封上定义业务消息编号。
// 与 gameapp 的 JSON 协议和 Unity 协议分别使用，不能混用编号含义。
package gameprotocol

import "Ginx/gcore"

const (
	GameMsgLoginRequest      gcore.GameMessageID = 1001
	GameMsgLoginResponse     gcore.GameMessageID = 1002
	GameMsgEnterRoomRequest  gcore.GameMessageID = 2001
	GameMsgEnterRoomResponse gcore.GameMessageID = 2002
	GameMsgLeaveRoomRequest  gcore.GameMessageID = 2003
	GameMsgRoomBroadcast     gcore.GameMessageID = 2004
	GameMsgPlayerMove        gcore.GameMessageID = 3001
	GameMsgAOIChange         gcore.GameMessageID = 3002
	GameMsgHeartbeat         gcore.GameMessageID = 9001
)
