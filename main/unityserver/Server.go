package main

import (
	"Ginx/gcore"
	"Ginx/gface"
	"Ginx/gnet"
	"fmt"
)

// UnityServer 保存 Unity MMO 示例服务端的共享游戏状态。
type UnityServer struct {
	World *gcore.UnityWorld
}

// UnityChatRouter 处理 Unity 客户端的聊天消息，消息 ID 为 2。
type UnityChatRouter struct {
	gnet.BaseRouter
	World *gcore.UnityWorld
}

func (router *UnityChatRouter) Handle(request gface.IRequest) {
	talk := &gcore.UnityTalk{}
	if err := talk.Unmarshal(request.GetData()); err != nil {
		fmt.Println("Unity talk decode error: ", err)
		return
	}

	message, err := buildUnityBroadcast(int32(request.GetConnection().GetConnId()), 1, talk.Content, nil, 0)
	if err != nil {
		fmt.Println("Unity talk encode error: ", err)
		return
	}
	if errorsFound := router.World.Broadcast(gcore.UnityMsgBroadCast, message); len(errorsFound) > 0 {
		fmt.Println("Unity talk broadcast error: ", errorsFound[0])
	}
}

// UnityMoveRouter 处理 Unity 客户端的移动消息，消息 ID 为 3。
type UnityMoveRouter struct {
	gnet.BaseRouter
	World *gcore.UnityWorld
}

func (router *UnityMoveRouter) Handle(request gface.IRequest) {
	position := &gcore.UnityPosition{}
	if err := position.Unmarshal(request.GetData()); err != nil {
		fmt.Println("Unity move decode error: ", err)
		return
	}

	player, err := router.World.UpdatePlayer(request.GetConnection().GetConnId(), *position, 0)
	if err != nil {
		fmt.Println("Unity move update error: ", err)
		return
	}
	message, err := buildUnityBroadcast(player.PID, 3, "", &player.Position, player.ActionData)
	if err != nil {
		fmt.Println("Unity move encode error: ", err)
		return
	}
	if errorsFound := router.World.Broadcast(gcore.UnityMsgBroadCast, message); len(errorsFound) > 0 {
		fmt.Println("Unity move broadcast error: ", errorsFound[0])
	}
}

func buildUnityBroadcast(pid int32, tp int32, content string, position *gcore.UnityPosition, actionData int32) ([]byte, error) {
	return (&gcore.UnityBroadCast{
		PID:        pid,
		TP:         tp,
		Content:    content,
		P:          position,
		ActionData: actionData,
	}).Marshal()
}

func main() {
	unityServer := &UnityServer{
		World: gcore.NewUnityWorld(),
	}
	server := gnet.NewServer()
	server.AddRouter(gcore.UnityMsgTalk, &UnityChatRouter{World: unityServer.World})
	server.AddRouter(gcore.UnityMsgMove, &UnityMoveRouter{World: unityServer.World})
	server.SetOnConnStart(func(connection gface.IConnection) {
		player, oldPlayers, err := unityServer.World.AddPlayer(connection)
		if err != nil {
			fmt.Println("Unity player add error: ", err)
			connection.Stop()
			return
		}

		pidData, _ := (&gcore.UnitySyncPID{PID: player.PID}).Marshal()
		if err := connection.SendBuffMsg(gcore.UnityMsgSyncPID, pidData); err != nil {
			fmt.Println("Unity sync pid error: ", err)
			return
		}

		players := make([]gcore.UnityPlayer, 0, len(oldPlayers))
		for _, oldPlayer := range oldPlayers {
			players = append(players, gcore.UnityPlayer{PID: oldPlayer.PID, P: oldPlayer.Position})
		}
		playersData, _ := (&gcore.UnitySyncPlayers{Players: players}).Marshal()
		if err := connection.SendBuffMsg(gcore.UnityMsgSyncPlayers, playersData); err != nil {
			fmt.Println("Unity sync players error: ", err)
			return
		}

		joinedData, _ := (&gcore.UnityBroadCast{
			PID: player.PID,
			TP:  2,
			P:   &player.Position,
		}).Marshal()
		if errorsFound := unityServer.World.Broadcast(gcore.UnityMsgBroadCast, joinedData); len(errorsFound) > 0 {
			fmt.Println("Unity join broadcast error: ", errorsFound[0])
		}
		fmt.Println("Unity player connected, pid = ", player.PID)
	})
	server.SetOnConnStop(func(connection gface.IConnection) {
		player, err := unityServer.World.RemovePlayer(connection.GetConnId())
		if err != nil {
			return
		}
		pidData, _ := (&gcore.UnitySyncPID{PID: player.PID}).Marshal()
		if errorsFound := unityServer.World.Broadcast(gcore.UnityMsgPlayerLeave, pidData); len(errorsFound) > 0 {
			fmt.Println("Unity leave broadcast error: ", errorsFound[0])
		}
		fmt.Println("Unity player disconnected, pid = ", player.PID)
	})

	server.Serve()
}
