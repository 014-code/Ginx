# 协议定义

此目录存放应用拥有的协议定义，不增加框架内置消息或业务规则。

- `gameapp.json`：现有 HTTP/TCP 示例的 TCP JSON 消息，输出到 `examples/gameapp/protocol/`。
- 教程文本协议、Unity Protobuf、可选 `GameMessage` 信封各自独立，本轮不迁移。

生成器位于 `tools/protocolgen/generate.py`，可以用于任意应用提供的同格式定义。
第一版使用 JSON，不需要 YAML 依赖。具体格式与兼容性约定见
[协议工具链说明](../docs/protocol-toolchain.md)。
