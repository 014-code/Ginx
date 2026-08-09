package gnet

import (
	"Ginx/gface"
	"Ginx/utils"
	"errors"
	"sync"
)

type ConnManager struct {
	//连接信息管理
	connections map[uint32]gface.IConnection
	//读写锁
	connLock sync.RWMutex
}

func NewConnManager() *ConnManager {
	return &ConnManager{
		connections: make(map[uint32]gface.IConnection),
	}
}

// 添加连接
func (cm *ConnManager) Add(conn gface.IConnection) error {
	if conn == nil {
		return errors.New("connection is nil")
	}

	cm.connLock.Lock()
	defer cm.connLock.Unlock()

	connID := conn.GetConnId()
	//将连接添加到连接管理器中
	if _, ok := cm.connections[connID]; ok {
		return errors.New("connection id already exists")
	}
	if utils.GlobalObject.MaxConn > 0 && len(cm.connections) >= utils.GlobalObject.MaxConn {
		return errors.New("maximum connections reached")
	}

	cm.connections[connID] = conn
	return nil
}

// 删除连接
func (cm *ConnManager) Remove(connID uint32) {
	cm.connLock.Lock()
	//删除该连接
	delete(cm.connections, connID)
	cm.connLock.Unlock()
}

// 获取连接
func (cm *ConnManager) Get(connID uint32) (gface.IConnection, error) {
	cm.connLock.RLock()
	conn, ok := cm.connections[connID]
	cm.connLock.RUnlock()
	if !ok {
		return nil, errors.New("connection not found")
	}
	return conn, nil
}

// 获取当前连接数
func (cm *ConnManager) Len() int {
	cm.connLock.RLock()
	length := len(cm.connections)
	cm.connLock.RUnlock()
	return length
}

// 清除并停止所有连接
func (cm *ConnManager) ClearConn() {
	cm.connLock.Lock()
	connections := make([]gface.IConnection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connections = append(connections, conn)
	}
	cm.connections = make(map[uint32]gface.IConnection)
	cm.connLock.Unlock()

	for _, conn := range connections {
		conn.Stop()
	}
}
