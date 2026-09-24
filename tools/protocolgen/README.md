# Ginx protocolgen

通用构建期协议工具，Python 3.10+ 标准库实现。不会进入 Go 服务端运行时。

```powershell
python tools/protocolgen/generate.py --validate
python tools/protocolgen/generate.py
python tools/protocolgen/generate.py --check
```

自定义应用使用 `--schema <file.json> --out <directory>`。提供 `--baseline` 可以检查
相对已发布定义的保守兼容性。完整定义格式、类型、退出码和限制见
[协议工具链说明](../../docs/protocol-toolchain.md)。测试统一位于根 `test/`。
