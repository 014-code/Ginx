package test

import (
	"Ginx/gcore"
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestLoggerFiltersLevelsAndWritesStructuredFields(t *testing.T) {
	var output bytes.Buffer
	logger := gcore.NewLogger("game", &output)
	logger.SetLevel(gcore.LogLevelInfo)

	if err := logger.Debug("hidden debug"); err != nil {
		t.Fatalf("Debug() error = %v", err)
	}
	if err := logger.Info("player login", gcore.Field("player_id", 1001), gcore.Field("room_id", 7)); err != nil {
		t.Fatalf("Info() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1", len(lines))
	}
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("Unmarshal() log error = %v", err)
	}
	if record["level"] != "INFO" || record["service"] != "game" || record["message"] != "player login" {
		t.Fatalf("log record = %v, want info game player login", record)
	}
}

func TestLoggerWithAndConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	logger := gcore.NewLogger("game", &output).With(gcore.Field("server_id", "s1"))
	var waitGroup sync.WaitGroup

	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			if err := logger.Info("tick", gcore.Field("index", index)); err != nil {
				t.Errorf("Info() error = %v", err)
			}
		}(i)
	}
	waitGroup.Wait()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 20 {
		t.Fatalf("log lines = %d, want 20", len(lines))
	}
	for _, line := range lines {
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("Unmarshal() concurrent log error = %v", err)
		}
	}
}
