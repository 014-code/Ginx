// 客户端演示完整链路，密码从 stdin 读取，不通过参数或环境变量传递。
package main

import (
	"Ginx/examples/gameapp"
	"Ginx/gnet"
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	httpAddr := flag.String("http", "http://127.0.0.1:8080", "HTTP server URL")
	tcpAddr := flag.String("tcp", "127.0.0.1:7777", "TCP server address")
	account := flag.String("account", "alice", "account ID")
	flag.Parse()
	password, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	password = strings.TrimSuffix(strings.TrimSuffix(password, "\n"), "\r")
	if password == "" {
		return fmt.Errorf("provide the password via stdin")
	}
	body, err := json.Marshal(map[string]string{"account_id": *account, "password": password})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(strings.TrimRight(*httpAddr, "/")+"/api/v1/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	var login gameapp.LoginResult
	err = json.NewDecoder(response.Body).Decode(&login)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || login.Token == "" {
		return fmt.Errorf("login failed (HTTP %d)", response.StatusCode)
	}
	fmt.Printf("Logged in as player %d\n", login.PlayerID)
	conn, err := net.DialTimeout("tcp", *tcpAddr, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	steps := []struct {
		id   uint32
		body any
	}{
		{gameapp.MsgAuthenticate, map[string]string{"token": login.Token}},
		{gameapp.MsgJoinRoom, map[string]int{"room_id": 1}},
		{gameapp.MsgStarterReward, struct{}{}},
		{gameapp.MsgUseItem, map[string]string{"item_id": "potion"}},
		{gameapp.MsgProgress, struct{}{}},
	}
	for _, step := range steps {
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		body, err := json.Marshal(step.body)
		if err != nil {
			return err
		}
		packet, err := gnet.NewDataPackWithLimit(4096).Pack(gnet.NewMsgPackage(step.id, body))
		if err != nil {
			return err
		}
		if _, err := io.Copy(conn, bytes.NewReader(packet)); err != nil {
			return err
		}
		message, err := gnet.NewDataPackWithLimit(4096).ReadMessage(conn)
		if err != nil {
			return err
		}
		var reply gameapp.Reply
		if message.GetMsgID() != step.id || json.Unmarshal(message.GetData(), &reply) != nil {
			return fmt.Errorf("invalid TCP reply")
		}
		fmt.Printf("TCP %d: %s\n", step.id, message.GetData())
		if reply.Code != "ok" && !(step.id == gameapp.MsgStarterReward && reply.Code == "reward_already_claimed") && !(step.id == gameapp.MsgUseItem && reply.Code == "item_unavailable") {
			return fmt.Errorf("TCP request failed: %s", reply.Code)
		}
	}
	request, err := http.NewRequest("GET", strings.TrimRight(*httpAddr, "/")+"/api/v1/progress", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+login.Token)
	response, err = client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("progress query failed (HTTP %d)", response.StatusCode)
	}
	var progress gameapp.Progress
	if err := json.NewDecoder(response.Body).Decode(&progress); err != nil {
		return err
	}
	result, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	fmt.Printf("HTTP progress: %s\n", result)
	return nil
}
