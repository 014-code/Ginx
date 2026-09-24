package gcore

// 以下常量只为旧源码兼容保留，框架不注册这些路由。
const (
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgLoginRequest GameMessageID = 1001
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgLoginResponse GameMessageID = 1002
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgEnterRoomRequest GameMessageID = 2001
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgEnterRoomResponse GameMessageID = 2002
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgLeaveRoomRequest GameMessageID = 2003
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgRoomBroadcast GameMessageID = 2004
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgPlayerMove GameMessageID = 3001
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgAOIChange GameMessageID = 3002
	// Deprecated: 使用 examples/gameprotocol 或自行定义业务编号。
	GameMsgHeartbeat GameMessageID = 9001
)
