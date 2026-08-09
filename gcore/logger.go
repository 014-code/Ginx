package gcore

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// LogLevel 表示日志级别，级别越小越优先输出。
type LogLevel uint8

const (
	// LogLevelDebug 用于开发调试信息。
	LogLevelDebug LogLevel = iota
	// LogLevelInfo 用于正常运行信息。
	LogLevelInfo
	// LogLevelWarn 用于可恢复的问题或风险提示。
	LogLevelWarn
	// LogLevelError 用于当前操作失败的信息。
	LogLevelError
)

// String 返回日志级别名称。
func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogField 表示一组结构化日志字段。
type LogField struct {
	Key   string
	Value interface{}
}

// Field 创建一个结构化日志字段。
func Field(key string, value interface{}) LogField {
	return LogField{Key: key, Value: value}
}

// Logger 是一个线程安全的 JSON 行日志记录器。
//
// 每次日志调用输出一行 JSON，方便本地调试，也方便后续接入日志采集系统。
// Logger 不负责日志文件轮转和远程传输，这些能力应交给部署环境或业务层。
type Logger struct {
	service    string
	output     io.Writer
	minLevel   atomic.Int32
	writeLock  sync.Mutex
	baseFields map[string]interface{}
}

type logRecord struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Service   string                 `json:"service,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewLogger 创建一个输出到指定 Writer 的日志记录器。
// output 传入 nil 时默认写到标准输出。
func NewLogger(service string, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}
	logger := &Logger{
		service:    service,
		output:     output,
		baseFields: make(map[string]interface{}),
	}
	logger.minLevel.Store(int32(LogLevelInfo))
	return logger
}

var defaultLogger = NewLogger("ginx", os.Stdout)

// DefaultLogger 返回框架默认日志记录器。
func DefaultLogger() *Logger {
	return defaultLogger
}

// SetLevel 设置最低输出级别。
func (l *Logger) SetLevel(level LogLevel) {
	l.minLevel.Store(int32(level))
}

// GetLevel 获取当前最低输出级别。
func (l *Logger) GetLevel() LogLevel {
	return LogLevel(l.minLevel.Load())
}

// With 创建一个带固定字段的子日志记录器。
func (l *Logger) With(fields ...LogField) *Logger {
	child := NewLogger(l.service, l.output)
	child.SetLevel(l.GetLevel())
	for key, value := range l.baseFields {
		child.baseFields[key] = value
	}
	for _, field := range fields {
		if field.Key != "" {
			child.baseFields[field.Key] = field.Value
		}
	}
	return child
}

// Log 输出一条结构化日志。
func (l *Logger) Log(level LogLevel, message string, fields ...LogField) error {
	if level < l.GetLevel() {
		return nil
	}

	allFields := make(map[string]interface{}, len(l.baseFields)+len(fields))
	for key, value := range l.baseFields {
		allFields[key] = value
	}
	for _, field := range fields {
		if field.Key != "" {
			allFields[field.Key] = field.Value
		}
	}

	record := logRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Service:   l.service,
		Message:   message,
		Fields:    allFields,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	_, err = l.output.Write(data)
	return err
}

// Debug 输出调试日志。
func (l *Logger) Debug(message string, fields ...LogField) error {
	return l.Log(LogLevelDebug, message, fields...)
}

// Info 输出普通运行日志。
func (l *Logger) Info(message string, fields ...LogField) error {
	return l.Log(LogLevelInfo, message, fields...)
}

// Warn 输出风险提示日志。
func (l *Logger) Warn(message string, fields ...LogField) error {
	return l.Log(LogLevelWarn, message, fields...)
}

// Error 输出错误日志。
func (l *Logger) Error(message string, fields ...LogField) error {
	return l.Log(LogLevelError, message, fields...)
}
