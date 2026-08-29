package docker

import (
	"strings"
	"testing"
)

const sampleDockerPsJSON = `{"ID":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789","Names":"web","Image":"nginx:1.27","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:80->80/tcp","CreatedAt":"2026-01-02 03:04:05 +0000 UTC"}
{"ID":"1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff","Names":"db","Image":"postgres:16","State":"exited","Status":"Exited (0) 3 days ago","Ports":"","CreatedAt":"2026-01-01 12:00:00 +0000 UTC"}
not actually json
{"ID":"short","Names":"tiny","Image":"alpine","State":"created","Status":"Created","Ports":"","CreatedAt":""}`

func TestParseListJSON(t *testing.T) {
	containers, err := parseListJSON(strings.NewReader(sampleDockerPsJSON))
	if err != nil {
		t.Fatalf("parseListJSON: %v", err)
	}

	if got := len(containers); got != 3 {
		t.Fatalf("len(containers): got %d want 3", got)
	}

	// Sort is by Name, so: db, tiny, web
	if containers[0].Name != "db" {
		t.Errorf("[0].Name: got %q want db", containers[0].Name)
	}
	if containers[1].Name != "tiny" {
		t.Errorf("[1].Name: got %q want tiny", containers[1].Name)
	}
	if containers[2].Name != "web" {
		t.Errorf("[2].Name: got %q want web", containers[2].Name)
	}

	web := containers[2]
	if web.ShortID != "abcdef012345" {
		t.Errorf("ShortID: got %q want abcdef012345", web.ShortID)
	}
	if web.State != "running" {
		t.Errorf("State: got %q want running", web.State)
	}
	if web.Image != "nginx:1.27" {
		t.Errorf("Image: got %q want nginx:1.27", web.Image)
	}
	if web.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be parsed, got zero value")
	}
	if web.CreatedAt.Year() != 2026 {
		t.Errorf("CreatedAt year: got %d want 2026", web.CreatedAt.Year())
	}

	// Short ID handling when raw ID is < 12 chars: should fall back to raw ID.
	tiny := containers[1]
	if tiny.ShortID != "short" {
		t.Errorf("Short ShortID fallback: got %q want short", tiny.ShortID)
	}
	if !tiny.CreatedAt.IsZero() {
		t.Errorf("CreatedAt: expected zero on empty input, got %v", tiny.CreatedAt)
	}
}

func TestParseListJSONEmpty(t *testing.T) {
	containers, err := parseListJSON(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parseListJSON empty: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("len: got %d want 0", len(containers))
	}
}

func TestNormalizeStopTimeout(t *testing.T) {
	cases := []struct {
		name    string
		input   int
		want    int
		wantErr bool
	}{
		{"zero defaults to 10", 0, 10, false},
		{"positive passes through", 5, 5, false},
		{"max ok", 3600, 3600, false},
		{"negative rejected", -1, 0, true},
		{"too big rejected", 3601, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeStopTimeout(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestTruncateTail(t *testing.T) {
	in := []byte("abcdefghij") // 10 bytes
	got, truncated := truncateTail(in, 5)
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if got != "fghij" {
		t.Errorf("trailing tail: got %q want %q", got, "fghij")
	}

	got2, t2 := truncateTail(in, 100)
	if t2 {
		t.Fatal("expected truncated=false when budget >= len")
	}
	if got2 != string(in) {
		t.Errorf("untruncated: got %q want %q", got2, in)
	}
}
