package device

import (
	"encoding/json"
	"errors"
)

var (
	errInvalidJSON = errors.New("invalid json")
	errNoAction    = errors.New("missing action")
)

// command 是下行命令的线格式
type command struct {
	RequestID string    `json:"request_id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Interval  int       `json:"interval_seconds"`
	Duration  int       `json:"duration_seconds"`
	Fault     FaultKind `json:"fault"`
}

// parseCommand 解析并做基本校验;返回错误说明拒绝原因
func parseCommand(b []byte) (command, error) {
	var c command
	if err := json.Unmarshal(b, &c); err != nil {
		return c, errInvalidJSON
	}
	if c.Action == "" {
		return c, errNoAction
	}
	return c, nil
}
