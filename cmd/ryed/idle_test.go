package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestIdleWatch_idle(t *testing.T) {
	start := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	quiet := `{"sessions":[],"states":{}}`
	// changedAt returns a list reply with one session last changed d after start.
	changedAt := func(d time.Duration) string {
		return `{"sessions":[{"lastModified":` + strconv.FormatInt(start.Add(d).UnixMilli(), 10) + `}],"states":{}}`
	}
	// The watch has a 30-minute limit. last and now are offsets from start.
	tests := []struct {
		name    string
		last    time.Duration
		now     time.Duration
		reply   string
		listErr error
		want    bool
	}{
		{name: "quiet within limit", now: 20 * time.Minute, reply: quiet, want: false},
		{name: "quiet past limit", now: 35 * time.Minute, reply: quiet, want: true},
		{name: "running session", now: 60 * time.Minute,
			reply: `{"sessions":[],"states":{"s":{"running":true,"pending":0,"background":0}}}`, want: false},
		{name: "pending approval", now: 60 * time.Minute, reply: `{"states":{"s":{"pending":1}}}`, want: false},
		{name: "background task", now: 60 * time.Minute, reply: `{"states":{"s":{"background":2}}}`, want: false},
		{name: "change 20 minutes ago", now: 60 * time.Minute, reply: changedAt(40 * time.Minute), want: false},
		{name: "change 35 minutes ago", now: 60 * time.Minute, reply: changedAt(25 * time.Minute), want: true},
		{name: "later change moves last forward", now: 40 * time.Minute, reply: changedAt(15 * time.Minute), want: false},
		{name: "earlier change keeps last", last: 30 * time.Minute, now: 40 * time.Minute, reply: changedAt(0), want: false},
		{name: "agent-manager unreachable past limit", now: 35 * time.Minute, listErr: errors.New("agent-manager stopped"), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reply map[string]json.RawMessage
			if tt.listErr == nil {
				if err := json.Unmarshal([]byte(tt.reply), &reply); err != nil {
					t.Fatal(err)
				}
			}
			now := start.Add(tt.now)
			w := &idleWatch{
				limit: 30 * time.Minute,
				now:   func() time.Time { return now },
				last:  start.Add(tt.last),
				list:  func(context.Context) (map[string]json.RawMessage, error) { return reply, tt.listErr },
			}
			if got := w.idle(context.Background()); got != tt.want {
				t.Errorf("idle() with last %s, now %s = %v, want %v", tt.last, tt.now, got, tt.want)
			}
		})
	}
}
