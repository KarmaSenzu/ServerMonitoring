package pm2

import (
	"strings"
	"testing"
	"time"
)

func TestParseJListOnlineAndStopped(t *testing.T) {
	// Build the online entry's pm_uptime so it's roughly "now - 30 seconds"
	// — the parser computes uptime relative to time.Now().
	nowMs := time.Now().UnixMilli()
	startMs := nowMs - 30_000

	fixture := `[
	  {
	    "name": "alpha",
	    "pid": 12345,
	    "pm2_env": {
	      "status": "online",
	      "pm_uptime": ` + itoa64(startMs) + `,
	      "restart_time": 3,
	      "exec_interpreter": "node",
	      "pm_exec_path": "/srv/alpha/server.js",
	      "pm_cwd": "/srv/alpha"
	    },
	    "monit": { "cpu": 1.5, "memory": 12345678 }
	  },
	  {
	    "name": "bravo",
	    "pid": 0,
	    "pm2_env": {
	      "status": "stopped",
	      "pm_uptime": 0,
	      "restart_time": 0,
	      "exec_interpreter": "python3",
	      "pm_exec_path": "/srv/bravo/app.py",
	      "pm_cwd": "/srv/bravo"
	    },
	    "monit": { "cpu": 0, "memory": 0 }
	  },
	  {
	    "name": "",
	    "pid": 99,
	    "pm2_env": { "status": "online", "pm_uptime": ` + itoa64(startMs) + ` },
	    "monit": { "cpu": 0, "memory": 0 }
	  }
	]`

	got, err := parseJList(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseJList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (skipping empty-name), got %d", len(got))
	}

	a := got[0]
	if a.Name != "alpha" {
		t.Errorf("[0].Name=%q want alpha", a.Name)
	}
	if a.Status != "online" {
		t.Errorf("[0].Status=%q want online", a.Status)
	}
	if a.PID != 12345 {
		t.Errorf("[0].PID=%d want 12345", a.PID)
	}
	if a.Restarts != 3 {
		t.Errorf("[0].Restarts=%d want 3", a.Restarts)
	}
	if a.Interpreter != "node" {
		t.Errorf("[0].Interpreter=%q want node", a.Interpreter)
	}
	if a.ScriptPath != "/srv/alpha/server.js" {
		t.Errorf("[0].ScriptPath=%q", a.ScriptPath)
	}
	if a.Cwd != "/srv/alpha" {
		t.Errorf("[0].Cwd=%q", a.Cwd)
	}
	if a.CPUPercent != 1.5 {
		t.Errorf("[0].CPUPercent=%v want 1.5", a.CPUPercent)
	}
	if a.MemoryBytes != 12345678 {
		t.Errorf("[0].MemoryBytes=%d want 12345678", a.MemoryBytes)
	}
	// Uptime should be approximately 30 seconds.
	if a.Uptime < 25 || a.Uptime > 60 {
		t.Errorf("[0].Uptime=%d want ~30s", a.Uptime)
	}

	b := got[1]
	if b.Name != "bravo" {
		t.Errorf("[1].Name=%q want bravo", b.Name)
	}
	if b.Status != "stopped" {
		t.Errorf("[1].Status=%q want stopped", b.Status)
	}
	if b.Uptime != 0 {
		t.Errorf("[1].Uptime=%d want 0", b.Uptime)
	}
}

func TestParseJListEmpty(t *testing.T) {
	got, err := parseJList(strings.NewReader(`[]`))
	if err != nil {
		t.Fatalf("parseJList: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestParseJListSkipsBanner(t *testing.T) {
	// PM2 sometimes prints "[PM2] Spawning daemon" lines to stdout before
	// the JSON payload. parseJList must skip past them.
	in := "[PM2] Spawning PM2 daemon with pm2_home=/foo\n[PM2] PM2 Successfully daemonized\n[]"
	got, err := parseJList(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseJList: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// itoa64 avoids strconv just to keep the test fixture readable.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

func TestValidateProcessName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"alpha", true},
		{"alpha-beta", true},
		{"alpha_beta.svc", true},
		{"AlphaBeta123", true},
		{"a", true},
		{"", false},
		{"-leading-dash", false},
		{".leading-dot", false},
		{"_leading-underscore", false},
		{"contains space", false},
		{"semi;colon", false},
		{"slash/here", false},
		{"way-too-long-name-that-exceeds-the-sixty-four-character-allowance-set", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProcessName(tc.name)
			if tc.ok && err != nil {
				t.Errorf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
