package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanProjectsRange(t *testing.T) {
	tmpDir := t.TempDir()

	// Reference time: midnight 2026-05-15 local
	loc := time.Local
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	at := func(y int, m time.Month, d, h, min int) int64 {
		return time.Date(y, m, d, h, min, 0, 0, loc).Unix()
	}

	// Helper to write a .jsonl + matching .json sidecar.
	writeSession := func(name, cwd string, msgs []struct {
		kind string
		ts   int64
	}) {
		jsonlPath := filepath.Join(tmpDir, name+".jsonl")
		jsonPath := filepath.Join(tmpDir, name+".json")

		var content string
		for _, m := range msgs {
			content += fmt.Sprintf(
				`{"version":"v1","kind":%q,"data":{"meta":{"timestamp":%d}}}`,
				m.kind, m.ts,
			) + "\n"
		}
		if err := os.WriteFile(jsonlPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write jsonl: %v", err)
		}
		// Sidecar holds cwd. Also tickle mtime to "now-ish" so the mtime fast-path
		// doesn't skip the file.
		sidecar := fmt.Sprintf(`{"cwd":%q}`, cwd)
		if err := os.WriteFile(jsonPath, []byte(sidecar), 0o644); err != nil {
			t.Fatalf("write json: %v", err)
		}
		// Set mtime to last message ts (or now if no msgs)
		var mtime time.Time
		for _, m := range msgs {
			if t := time.Unix(m.ts, 0); t.After(mtime) {
				mtime = t
			}
		}
		if mtime.IsZero() {
			mtime = time.Now()
		}
		_ = os.Chtimes(jsonlPath, mtime, mtime)
	}

	// Project A: 3 messages on 2026-05-14, 2 on 2026-05-15
	writeSession("a", "/Users/mark/projA", []struct {
		kind string
		ts   int64
	}{
		{"Prompt", at(2026, 5, 14, 10, 0)},
		{"AssistantMessage", at(2026, 5, 14, 10, 1)},
		{"Prompt", at(2026, 5, 14, 11, 0)},
		{"Prompt", at(2026, 5, 15, 9, 0)},
		{"AssistantMessage", at(2026, 5, 15, 9, 1)},
	})

	// Project B: 4 messages on 2026-05-15
	writeSession("b", "/Users/mark/projB", []struct {
		kind string
		ts   int64
	}{
		{"Prompt", at(2026, 5, 15, 12, 0)},
		{"AssistantMessage", at(2026, 5, 15, 12, 1)},
		{"Prompt", at(2026, 5, 15, 13, 0)},
		{"AssistantMessage", at(2026, 5, 15, 13, 1)},
	})

	// Project A again, different session: 1 message on 2026-05-15
	writeSession("a2", "/Users/mark/projA", []struct {
		kind string
		ts   int64
	}{
		{"Prompt", at(2026, 5, 15, 14, 0)},
	})

	// Out-of-range project: messages only on 2026-05-13
	writeSession("c-old", "/Users/mark/projC", []struct {
		kind string
		ts   int64
	}{
		{"Prompt", at(2026, 5, 13, 10, 0)},
	})

	// Range: 2026-05-15 single day → should only count msgs on 2026-05-15
	stats, err := ScanProjectsRange(tmpDir, day(2026, 5, 15), day(2026, 5, 15))
	if err != nil {
		t.Fatalf("ScanProjectsRange(single day): %v", err)
	}

	wantOrder := []struct {
		project  string
		sessions int
		messages int
	}{
		{"projB", 1, 4},
		{"projA", 2, 3}, // session a (2 msgs in range) + session a2 (1 msg in range)
	}
	if len(stats) != len(wantOrder) {
		t.Fatalf("single-day: got %d projects, want %d: %+v", len(stats), len(wantOrder), stats)
	}
	for i, w := range wantOrder {
		if stats[i].Project != w.project {
			t.Errorf("single-day[%d].Project = %q, want %q", i, stats[i].Project, w.project)
		}
		if stats[i].Sessions != w.sessions {
			t.Errorf("single-day[%d].Sessions = %d, want %d", i, stats[i].Sessions, w.sessions)
		}
		if stats[i].Messages != w.messages {
			t.Errorf("single-day[%d].Messages = %d, want %d", i, stats[i].Messages, w.messages)
		}
	}

	// Range: 2026-05-13 to 2026-05-15 → all three projects
	stats3, err := ScanProjectsRange(tmpDir, day(2026, 5, 13), day(2026, 5, 15))
	if err != nil {
		t.Fatalf("ScanProjectsRange(3 days): %v", err)
	}
	want3 := map[string]int{
		"projA": 5, // 3 on 5/14 + 2 on 5/15 + 1 on 5/15 from a2 → wait, a2 has 1 on 5/15
		"projB": 4,
		"projC": 1,
	}
	// projA total in range: a(5) + a2(1) = 6
	want3["projA"] = 6

	if len(stats3) != 3 {
		t.Fatalf("3-day: got %d projects, want 3: %+v", len(stats3), stats3)
	}
	for _, s := range stats3 {
		if got := want3[s.Project]; got != s.Messages {
			t.Errorf("3-day project %q: messages = %d, want %d", s.Project, s.Messages, got)
		}
	}
}

func TestScanProjectsRange_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	stats, err := ScanProjectsRange(tmpDir, time.Now().AddDate(0, 0, -7), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty slice, got %+v", stats)
	}
}

func TestFindCwd_SidecarPreferredOverToolUse(t *testing.T) {
	tmpDir := t.TempDir()
	jsonl := filepath.Join(tmpDir, "x.jsonl")
	json := filepath.Join(tmpDir, "x.json")

	// jsonl: AssistantMessage with toolUse.working_dir = "/from/tooluse"
	content := `{"version":"v1","kind":"AssistantMessage","data":{"meta":{"timestamp":1},"content":[{"kind":"toolUse","data":{"input":{"working_dir":"/from/tooluse"}}}]}}`
	if err := os.WriteFile(jsonl, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// sidecar: cwd = "/from/sidecar"
	if err := os.WriteFile(json, []byte(`{"cwd":"/from/sidecar"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := ParseFile(jsonl)
	if err != nil {
		t.Fatal(err)
	}
	got := findCwd(jsonl, entries)
	if got != "/from/sidecar" {
		t.Errorf("findCwd = %q, want /from/sidecar (sidecar should be preferred)", got)
	}

	// Now remove sidecar; ToolUse fallback should kick in.
	_ = os.Remove(json)
	got = findCwd(jsonl, entries)
	if got != "/from/tooluse" {
		t.Errorf("findCwd (no sidecar) = %q, want /from/tooluse", got)
	}
}
