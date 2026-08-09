package main

import (
	"Ginx/gcore"
	"Ginx/gface"
	"Ginx/gnet"
	"Ginx/session"
	"Ginx/unity"
	"fmt"
)

// UnityServer 保存 Unity MMO 示例服务端的共享游戏状态。
type UnityServer struct {
	World    *unity.UnityWorld
	Rooms    *gcore.RoomManager
	Sessions *session.Manager
}

// UnityChatRouter 处理 Unity 客户端的聊天消息，消息 ID 为 2。
type UnityChatRouter struct {
	gnet.BaseRouter
	World *unity.UnityWorld
}

func (router *UnityChatRouter) Handle(request gface.IRequest) {
	talk := &unity.UnityTalk{}
	if err := talk.Unmarshal(request.GetData()); err != nil {
		fmt.Println("Unity talk decode error: ", err)
		return
	}

	message, err := buildUnityBroadcast(int32(request.GetConnection().GetConnId()), 1, talk.Content, nil, 0)
	if err != nil {
		fmt.Println("Unity talk encode error: ", err)
		return
	}
	if errorsFound := router.World.Broadcast(unity.UnityMsgBroadCast, message); len(errorsFound) > 0 {
		fmt.Println("Unity talk broadcast error: ", errorsFound[0])
	}
}

// UnityMoveRouter 处理 Unity 客户端的移动消息，消息 ID 为 3。
type UnityMoveRouter struct {
	gnet.BaseRouter
	World *unity.UnityWorld
}

// UnityCreateRoomRouter 处理创建房间请求。
type UnityCreateRoomRouter struct {
	gnet.BaseRouter
	Server *UnityServer
}

func (router *UnityCreateRoomRouter) Handle(request gface.IRequest) {
	roomRequest := &unity.UnityRoomRequest{}
	if err := roomRequest.Unmarshal(request.GetData()); err != nil {
		sendRoomResponse(request.GetConnection(), unity.UnityMsgCreateRoom, roomRequest.RoomID, -1, err.Error(), 0)
		return
	}
	if roomRequest.RoomID == 0 {
		sendRoomResponse(request.GetConnection(), unity.UnityMsgCreateRoom, roomRequest.RoomID, -1, "room id is empty", 0)
		return
	}
	room, err := router.Server.Rooms.CreateRoom(roomRequest.RoomID, 0)
	if err != nil {
		sendRoomResponse(request.GetConnection(), unity.UnityMsgCreateRoom, roomRequest.RoomID, -1, err.Error(), 0)
		return
	}
	sendRoomResponse(request.GetConnection(), unity.UnityMsgCreateRoom, room.ID, 0, "room created", 0)
}

// UnityJoinRoomRouter 处理加入房间请求。
type UnityJoinRoomRouter struct {
	gnet.BaseRouter
	Server *UnityServer
}

func (router *UnityJoinRoomRouter) Handle(request gface.IRequest) {
	connection := request.GetConnection()
	roomRequest := &unity.UnityRoomRequest{}
	if err := roomRequest.Unmarshal(request.GetData()); err != nil {
		sendRoomResponse(connection, unity.UnityMsgJoinRoom, roomRequest.RoomID, -1, err.Error(), 0)
		return
	}
	room, err := router.Server.Rooms.GetRoom(roomRequest.RoomID)
	if err != nil {
		sendRoomResponse(connection, unity.UnityMsgJoinRoom, roomRequest.RoomID, -1, err.Error(), 0)
		return
	}
	if currentRoomID := getRoomID(connection); currentRoomID != 0 {
		if currentRoomID == roomRequest.RoomID {
			sendRoomResponse(connection, unity.UnityMsgJoinRoom, roomRequest.RoomID, -1, "connection already joined room", room.PlayerCount())
			return
		}
	}
	if err := room.Join(connection); err != nil {
		sendRoomResponse(connection, unity.UnityMsgJoinRoom, roomRequest.RoomID, -1, err.Error(), room.PlayerCount())
		return
	}
	if currentRoomID := getRoomID(connection); currentRoomID != 0 {
		if currentRoom, currentErr := router.Server.Rooms.GetRoom(currentRoomID); currentErr == nil {
			if leaveErr := currentRoom.Leave(connection.GetConnId()); leaveErr != nil {
				_ = room.Leave(connection.GetConnId())
				sendRoomResponse(connection, unity.UnityMsgJoinRoom, roomRequest.RoomID, -1, leaveErr.Error(), room.PlayerCount())
				return
			}
		}
	}
	connection.SetProperty("RoomID", roomRequest.RoomID)
	sendRoomResponse(connection, unity.UnityMsgJoinRoom, room.ID, 0, "room joined", room.PlayerCount())
}

// UnityLeaveRoomRouter 处理离开房间请求。
type UnityLeaveRoomRouter struct {
	gnet.BaseRouter
	Server *UnityServer
}

func (router *UnityLeaveRoomRouter) Handle(request gface.IRequest) {
	connection := request.GetConnection()
	roomID := getRoomID(connection)
	if roomID == 0 {
		sendRoomResponse(connection, unity.UnityMsgLeaveRoom, 0, -1, "connection is not in room", 0)
		return
	}
	room, err := router.Server.Rooms.GetRoom(roomID)
	if err != nil {
		connection.RemoveProperty("RoomID")
		sendRoomResponse(connection, unity.UnityMsgLeaveRoom, roomID, -1, err.Error(), 0)
		return
	}
	if err := room.Leave(connection.GetConnId()); err != nil {
		sendRoomResponse(connection, unity.UnityMsgLeaveRoom, roomID, -1, err.Error(), room.PlayerCount())
		return
	}
	connection.RemoveProperty("RoomID")
	sendRoomResponse(connection, unity.UnityMsgLeaveRoom, roomID, 0, "room left", room.PlayerCount())
}

// UnityRoomEventRouter 处理房间内广播请求。
type UnityRoomEventRouter struct {
	gnet.BaseRouter
	Server *UnityServer
}

func (router *UnityRoomEventRouter) Handle(request gface.IRequest) {
	connection := request.GetConnection()
	roomID := getRoomID(connection)
	if roomID == 0 {
		return
	}
	playerID := getPlayerID(connection)
	event := &unity.UnityRoomEvent{}
	if err := event.Unmarshal(request.GetData()); err != nil {
		return
	}
	event.RoomID = roomID
	event.PlayerID = playerID
	data, err := event.Marshal()
	if err != nil {
		return
	}
	room, err := router.Server.Rooms.GetRoom(roomID)
	if err != nil {
		return
	}
	if errorsFound := room.Broadcast(unity.UnityMsgRoomEvent, data); len(errorsFound) > 0 {
		fmt.Println("Unity room event broadcast error: ", errorsFound[0])
	}
}

func sendRoomResponse(connection gface.IConnection, msgID uint32, roomID uint32, code int32, message string, playerCount int) {
	data, err := (&unity.UnityRoomResponse{RoomID: roomID, Code: code, Message: message, PlayerCount: uint32(playerCount)}).Marshal()
	if err == nil {
		_ = connection.SendBuffMsg(msgID, data)
	}
}

func getRoomID(connection gface.IConnection) uint32 {
	value, err := connection.GetProperty("RoomID")
	if err != nil {
		return 0
	}
	roomID, ok := value.(uint32)
	if !ok {
		return 0
	}
	return roomID
}

func getPlayerID(connection gface.IConnection) int32 {
	value, err := connection.GetProperty("PlayerID")
	if err != nil {
		return 0
	}
	playerID, ok := value.(uint64)
	if !ok {
		return 0
	}
	return int32(playerID)
}

func (router *UnityMoveRouter) Handle(request gface.IRequest) {
	position := &unity.UnityPosition{}
	if err := position.Unmarshal(request.GetData()); err != nil {
		fmt.Println("Unity move decode error: ", err)
		return
	}

	result, err := router.World.UpdatePlayerWithVisibility(request.GetConnection().GetConnId(), *position, 0)
	if err != nil {
		fmt.Println("Unity move update error: ", err)
		return
	}

	joinData, err := buildUnityBroadcast(result.Player.PID, 2, "", &result.Player.Position, result.Player.ActionData)
	if err != nil {
		fmt.Println("Unity move join encode error: ", err)
		return
	}
	for _, enteredPlayer := range result.Entered {
		if err := enteredPlayer.Connection.SendBuffMsg(unity.UnityMsgBroadCast, joinData); err != nil {
			fmt.Println("Unity entered broadcast error: ", err)
		}
		enteredData, encodeErr := buildUnityBroadcast(enteredPlayer.PID, 2, "", &enteredPlayer.Position, enteredPlayer.ActionData)
		if encodeErr == nil {
			if err := request.GetConnection().SendBuffMsg(unity.UnityMsgBroadCast, enteredData); err != nil {
				fmt.Println("Unity visible player sync error: ", err)
			}
		}
	}
	for _, leftPlayer := range result.Left {
		leftData, encodeErr := (&unity.UnitySyncPID{PID: leftPlayer.PID}).Marshal()
		if encodeErr == nil {
			moverData, _ := (&unity.UnitySyncPID{PID: result.Player.PID}).Marshal()
			if err := leftPlayer.Connection.SendBuffMsg(unity.UnityMsgPlayerLeave, moverData); err != nil {
				fmt.Println("Unity leave broadcast error: ", err)
			}
			if err := request.GetConnection().SendBuffMsg(unity.UnityMsgPlayerLeave, leftData); err != nil {
				fmt.Println("Unity visible player leave error: ", err)
			}
		}
	}

	message, err := buildUnityBroadcast(result.Player.PID, 3, "", &result.Player.Position, result.Player.ActionData)
	if err != nil {
		fmt.Println("Unity move encode error: ", err)
		return
	}
	if errorsFound := router.World.BroadcastVisible(request.GetConnection().GetConnId(), unity.UnityMsgBroadCast, message, true); len(errorsFound) > 0 {
		fmt.Println("Unity move broadcast error: ", errorsFound[0])
	}
}

func buildUnityBroadcast(pid int32, tp int32, content string, position *unity.UnityPosition, actionData int32) ([]byte, error) {
	return (&unity.UnityBroadCast{
		PID:        pid,
		TP:         tp,
		Content:    content,
		P:          position,
		ActionData: actionData,
	}).Marshal()
}

func main() {
	unityServer := &UnityServer{
		World:    unity.NewUnityWorld(),
		Rooms:    gcore.NewRoomManager(),
		Sessions: session.NewManager(1000),
	}
	if _, err := unityServer.Rooms.CreateRoom(1, 100); err != nil {
		panic(err)
	}
	server := gnet.NewServer()
	server.AddRouter(unity.UnityMsgTalk, &UnityChatRouter{World: unityServer.World})
	server.AddRouter(unity.UnityMsgMove, &UnityMoveRouter{World: unityServer.World})
	server.AddRouter(unity.UnityMsgCreateRoom, &UnityCreateRoomRouter{Server: unityServer})
	server.AddRouter(unity.UnityMsgJoinRoom, &UnityJoinRoomRouter{Server: unityServer})
	server.AddRouter(unity.UnityMsgLeaveRoom, &UnityLeaveRoomRouter{Server: unityServer})
	server.AddRouter(unity.UnityMsgRoomEvent, &UnityRoomEventRouter{Server: unityServer})
	server.SetOnConnStart(func(connection gface.IConnection) {
		accountID := fmt.Sprintf("unity-conn-%d", connection.GetConnId())
		gameSession, _, err := unityServer.Sessions.Login(accountID, connection.GetConnId())
		if err != nil {
			fmt.Println("Unity session login error: ", err)
			connection.Stop()
			return
		}
		connection.SetProperty("AccountID", gameSession.AccountID)
		connection.SetProperty("SessionToken", gameSession.Token)
		connection.SetProperty("PlayerID", gameSession.PlayerID)

		player, oldPlayers, err := unityServer.World.AddPlayerWithPID(connection, int32(gameSession.PlayerID))
		if err != nil {
			fmt.Println("Unity player add error: ", err)
			connection.Stop()
			return
		}
		room, roomErr := unityServer.Rooms.GetRoom(1)
		if roomErr == nil && room.Join(connection) == nil {
			connection.SetProperty("RoomID", uint32(1))
		}

		pidData, _ := (&unity.UnitySyncPID{PID: player.PID}).Marshal()
		if err := connection.SendBuffMsg(unity.UnityMsgSyncPID, pidData); err != nil {
			fmt.Println("Unity sync pid error: ", err)
			return
		}

		players := make([]unity.UnityPlayer, 0, len(oldPlayers))
		for _, oldPlayer := range oldPlayers {
			players = append(players, unity.UnityPlayer{PID: oldPlayer.PID, P: oldPlayer.Position})
		}
		playersData, _ := (&unity.UnitySyncPlayers{Players: players}).Marshal()
		if err := connection.SendBuffMsg(unity.UnityMsgSyncPlayers, playersData); err != nil {
			fmt.Println("Unity sync players error: ", err)
			return
		}

		joinedData, _ := (&unity.UnityBroadCast{
			PID: player.PID,
			TP:  2,
			P:   &player.Position,
		}).Marshal()
		if errorsFound := unityServer.World.BroadcastVisible(player.ConnID, unity.UnityMsgBroadCast, joinedData, true); len(errorsFound) > 0 {
			fmt.Println("Unity join broadcast error: ", errorsFound[0])
		}
		fmt.Println("Unity player connected, pid = ", player.PID)
	})
	server.SetOnConnStop(func(connection gface.IConnection) {
		if roomID := getRoomID(connection); roomID != 0 {
			if room, err := unityServer.Rooms.GetRoom(roomID); err == nil {
				_ = room.Leave(connection.GetConnId())
			}
		}
		player, nearbyPlayers, err := unityServer.World.RemovePlayerWithVisibility(connection.GetConnId())
		if err != nil {
			_, _ = unityServer.Sessions.LogoutByConnID(connection.GetConnId())
			return
		}
		pidData, _ := (&unity.UnitySyncPID{PID: player.PID}).Marshal()
		for _, nearbyPlayer := range nearbyPlayers {
			if err := nearbyPlayer.Connection.SendBuffMsg(unity.UnityMsgPlayerLeave, pidData); err != nil {
				fmt.Println("Unity leave broadcast error: ", err)
			}
		}
		_, _ = unityServer.Sessions.LogoutByConnID(connection.GetConnId())
		fmt.Println("Unity player disconnected, pid = ", player.PID)
	})

	server.Serve()
}
