package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// GoalState reads the native goal through the CLI's protocol rather than its
// private SQLite schema. exec success alone does not prove a goal was set or
// completed (a model may ignore the activation instruction).
func GoalState(ctx context.Context, path, threadID string, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "app-server", "--enable", "goals")
	cmd.Env, cmd.Stderr = env, io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() { in.Close(); cancel(); _ = cmd.Wait() }()
	enc, scan := json.NewEncoder(in), bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 4*1024*1024)
	read := func(id int, dst any) error {
		for scan.Scan() {
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(scan.Bytes(), &msg) != nil || msg.ID != id {
				continue
			}
			if msg.Error != nil {
				return fmt.Errorf("goal status: %s", msg.Error.Message)
			}
			return json.Unmarshal(msg.Result, dst)
		}
		if err := scan.Err(); err != nil {
			return err
		}
		return fmt.Errorf("goal status unavailable")
	}
	if err := enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "nacre", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}}); err != nil {
		return "", err
	}
	var init json.RawMessage
	if err := read(1, &init); err != nil {
		return "", err
	}
	if err := enc.Encode(map[string]any{"method": "initialized"}); err != nil {
		return "", err
	}
	if err := enc.Encode(map[string]any{"id": 2, "method": "thread/goal/get", "params": map[string]string{"threadId": threadID}}); err != nil {
		return "", err
	}
	var result struct {
		Goal *struct {
			Status string `json:"status"`
		} `json:"goal"`
	}
	if err := read(2, &result); err != nil {
		return "", err
	}
	if result.Goal == nil {
		return "", fmt.Errorf("the session did not activate a native goal")
	}
	return result.Goal.Status, nil
}
