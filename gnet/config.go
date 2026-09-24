package gnet

import "Ginx/utils"

// Config 是实例级配置；构造时复制，不在运行期间读取配置文件。
type Config = utils.GlobalObj

func DefaultConfig() Config { return utils.DefaultConfig() }

// nil 仅用于旧构造函数，保留其全局配置兼容语义。
func effectiveConfig(config *Config) Config {
	if config != nil {
		return *config
	}
	return *utils.GlobalObject
}
