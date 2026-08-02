package gface

// 连接管理器接口
type IConnManager interface {
	Add(conn IConnection) error
	Remove(connID uint32)
	Get(connID uint32) (IConnection, error)
	Len() int
	ClearConn()
}
