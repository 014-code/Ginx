// Package metrics 提供轻量的并发安全运行指标。
package metrics

import "sync/atomic"

// Snapshot 是某一时刻的指标快照。
type Snapshot struct {
	Connections  int64
	Messages     uint64
	BytesIn      uint64
	BytesOut     uint64
	RouterPanics uint64
}

// Metrics 保存框架运行期间的累计指标。
type Metrics struct {
	connections  atomic.Int64
	messages     atomic.Uint64
	bytesIn      atomic.Uint64
	bytesOut     atomic.Uint64
	routerPanics atomic.Uint64
}

// AddConnections 增减当前连接数。
func (m *Metrics) AddConnections(delta int64) {
	if m != nil {
		m.connections.Add(delta)
	}
}

// IncMessages 增加收到的消息数。
func (m *Metrics) IncMessages() {
	if m != nil {
		m.messages.Add(1)
	}
}

// AddBytesIn 增加收到的消息体字节数。
func (m *Metrics) AddBytesIn(value uint64) {
	if m != nil {
		m.bytesIn.Add(value)
	}
}

// AddBytesOut 增加发出的完整数据包字节数。
func (m *Metrics) AddBytesOut(value uint64) {
	if m != nil {
		m.bytesOut.Add(value)
	}
}

// IncRouterPanics 增加路由 panic 次数。
func (m *Metrics) IncRouterPanics() {
	if m != nil {
		m.routerPanics.Add(1)
	}
}

// Snapshot 返回当前指标快照。
func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	return Snapshot{
		Connections:  m.connections.Load(),
		Messages:     m.messages.Load(),
		BytesIn:      m.bytesIn.Load(),
		BytesOut:     m.bytesOut.Load(),
		RouterPanics: m.routerPanics.Load(),
	}
}
