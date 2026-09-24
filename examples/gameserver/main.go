package main

import (
	"Ginx/examples/gameapp"
	"Ginx/examples/sqlitestore"
	"Ginx/gnet"
	"Ginx/persist"
	"Ginx/utils"
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	httpAddr := flag.String("http", "127.0.0.1:8080", "HTTP listen address")
	configPath := flag.String("config", "config/ginx.json", "TCP config file")
	accountsPath := flag.String("accounts", "config/accounts.local.json", "account file with bcrypt hashes")
	playersPath := flag.String("players", "", "player storage path (default depends on -storage)")
	storage := flag.String("storage", "sqlite", "sqlite (full example) or json (legacy login/rooms only)")
	ttl := flag.Duration("token-ttl", time.Hour, "absolute token lifetime")
	hashPassword := flag.Bool("hash-password", false, "read a password line from stdin and print its bcrypt hash")
	flag.Parse()
	if *hashPassword {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			log.Fatal("read password: ", err)
		}
		password := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if len(password) == 0 || len(password) > 72 {
			log.Fatal("password must contain 1 to 72 bytes")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(hash))
		return
	}
	data, err := os.ReadFile(*accountsPath)
	if err != nil {
		log.Fatal("read account config: ", err)
	}
	var accounts []gameapp.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		log.Fatal("invalid account config: ", err)
	}
	if *playersPath == "" {
		if *storage == "json" {
			*playersPath = "data/players.json"
		} else {
			*playersPath = "data/players.db"
		}
	}
	var store persist.PlayerStore
	switch *storage {
	case "sqlite":
		var database *sqlitestore.Store
		database, err = sqlitestore.Open(*playersPath)
		if err == nil {
			defer database.Close()
			store = database
		}
	case "json":
		store, err = persist.NewJSONFileStore(*playersPath)
	default:
		log.Fatal("storage must be sqlite or json")
	}
	if err != nil {
		log.Fatal(err)
	}
	service, err := gameapp.New(accounts, store, *ttl, []gameapp.RoomConfig{{ID: 1, MaxPlayers: 100}})
	if err != nil {
		log.Fatal(err)
	}
	gin.SetMode(gin.ReleaseMode)
	config, err := utils.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	tcp := gnet.NewServerWithConfig(config)
	gameapp.RegisterTCP(tcp, service)
	listener, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	log.Printf("HTTP %s; TCP %s:%d", listener.Addr(), tcp.IP, tcp.Port)
	if err := gameapp.Serve(ctx, listener, tcp, service); err != nil {
		log.Fatal(err)
	}
}
