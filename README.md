# Ginx

Ginx is a lightweight Go game-server framework with length-prefixed messages, message-ID routing, per-connection read/write goroutines, and router lifecycle hooks.

## Quick start

Run commands from the repository root so `config/ginx.json` is loaded:

```powershell
go run ./main/server
```

The example server registers message IDs `0` and `1`. The matching example clients are:

```powershell
go run ./main/client0
go run ./main/client1
```

Run the automated checks with:

```powershell
go test ./...
go vet ./...
```

The full game-server usage guide is in [docs/game-server-guide.md](docs/game-server-guide.md).
