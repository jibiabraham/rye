package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"time"

	"rye/internal/agentmanager"
)

// idleCheck is how often ryed -local asks agent-manager for activity.
const idleCheck = 5 * time.Minute

// idleWatch tracks the last agent-manager activity. A running turn, a waiting approval, a background task or a session
// change counts. An open SSH connection doesn't. The time is kept in memory and starts again at each boot.
type idleWatch struct {
	limit time.Duration
	now   func() time.Time
	list  func(context.Context) (map[string]json.RawMessage, error)
	last  time.Time
}

func newIdleWatch(limit time.Duration, dial agentmanager.Dialer) *idleWatch {
	list := func(ctx context.Context) (map[string]json.RawMessage, error) {
		return agentmanager.Call(ctx, dial, "", map[string]any{"type": "list"})
	}
	return &idleWatch{limit: limit, now: time.Now, list: list, last: time.Now()}
}

// idle asks agent-manager for activity and reports whether there has been none for the limit. No answer from
// agent-manager counts as no activity.
func (w *idleWatch) idle(ctx context.Context) bool {
	now := w.now()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if reply, err := w.list(ctx); err == nil {
		w.last = lastActivity(reply, now, w.last)
	}
	return now.Sub(w.last) >= w.limit
}

// lastActivity returns now when a list reply shows a live session at work. Otherwise it returns the latest session
// change, or last when that is later.
func lastActivity(reply map[string]json.RawMessage, now, last time.Time) time.Time {
	var states map[string]struct {
		Running    bool `json:"running"`
		Pending    int  `json:"pending"`
		Background int  `json:"background"`
	}
	_ = json.Unmarshal(reply["states"], &states)
	for _, s := range states {
		if s.Running || s.Pending > 0 || s.Background > 0 {
			return now
		}
	}
	var sessions []struct {
		LastModified int64 `json:"lastModified"`
	}
	_ = json.Unmarshal(reply["sessions"], &sessions)
	for _, s := range sessions {
		if changed := time.UnixMilli(s.LastModified); changed.After(last) {
			last = changed
		}
	}
	return last
}

// powerOffWhenIdle checks every idleCheck, and powers the machine off when w reports it idle.
func powerOffWhenIdle(ctx context.Context, w *idleWatch) {
	ticker := time.NewTicker(idleCheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !w.idle(ctx) {
			continue
		}
		slog.Info("powering off", "idle", w.limit)
		if out, err := exec.CommandContext(ctx, "sudo", "-n", "systemctl", "poweroff").CombinedOutput(); err != nil {
			slog.Error("poweroff", "err", err, "output", string(out))
		}
	}
}
