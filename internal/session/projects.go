package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProjectStats is a per-project aggregate over a date range.
// "Project" is derived as the basename of the session's working directory.
type ProjectStats struct {
	Project    string    `json:"project"`     // basename(cwd)
	Cwd        string    `json:"cwd"`         // full working directory
	Sessions   int       `json:"sessions"`    // sessions touching the range
	Messages   int       `json:"messages"`    // messages within the range
	LastActive time.Time `json:"last_active"` // last message timestamp within the range
}

// ScanProjectsRange walks .jsonl files in dirPath and aggregates per-project stats
// for messages whose timestamp falls within [start, end] at day granularity, interpreted
// in start.Location().
//
// A session contributes to a project's Sessions count if any of its messages fall within
// the range. Messages count is the number of Prompt/AssistantMessage entries in range.
//
// Note: this function reads every session file's content (no mtime fast-path beyond the
// start-of-range), so callers should cache results.
func ScanProjectsRange(dirPath string, start, end time.Time) ([]ProjectStats, error) {
	if strings.HasPrefix(dirPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dirPath = filepath.Join(homeDir, dirPath[1:])
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	startOfRange := getStartOfDay(start)
	endOfRangeExclusive := getStartOfDay(end).AddDate(0, 0, 1)

	type agg struct {
		cwd        string
		project    string
		sessions   int
		messages   int
		lastActive int64 // unix seconds
	}
	aggMap := make(map[string]*agg)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// mtime fast-path: a file last modified before the range can't have messages in range.
		if info.ModTime().Before(startOfRange) {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		logEntries, err := ParseFile(fullPath)
		if err != nil || len(logEntries) == 0 {
			continue
		}

		cwd := findCwd(fullPath, logEntries)
		if cwd == "" {
			continue // can't attribute to a project
		}

		var (
			messagesInRange int
			lastTsInRange   int64
			lastKnownTs     int64
			touchedRange    bool
		)
		for _, le := range logEntries {
			if le.Data.Meta.Timestamp > 0 {
				lastKnownTs = le.Data.Meta.Timestamp
			}
			if le.Kind != "Prompt" && le.Kind != "AssistantMessage" {
				continue
			}
			ts := le.Data.Meta.Timestamp
			if ts == 0 {
				ts = lastKnownTs
			}
			if ts == 0 {
				continue
			}
			t := time.Unix(ts, 0)
			if t.Before(startOfRange) || !t.Before(endOfRangeExclusive) {
				continue
			}
			messagesInRange++
			if ts > lastTsInRange {
				lastTsInRange = ts
			}
			touchedRange = true
		}

		if !touchedRange {
			continue
		}

		a := aggMap[cwd]
		if a == nil {
			a = &agg{cwd: cwd, project: filepath.Base(cwd)}
			aggMap[cwd] = a
		}
		a.sessions++
		a.messages += messagesInRange
		if lastTsInRange > a.lastActive {
			a.lastActive = lastTsInRange
		}
	}

	result := make([]ProjectStats, 0, len(aggMap))
	for _, a := range aggMap {
		result = append(result, ProjectStats{
			Project:    a.project,
			Cwd:        a.cwd,
			Sessions:   a.sessions,
			Messages:   a.messages,
			LastActive: time.Unix(a.lastActive, 0),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Messages != result[j].Messages {
			return result[i].Messages > result[j].Messages
		}
		return result[i].Project < result[j].Project
	})
	return result, nil
}

// findCwd locates the session's working directory. It first checks the .json sidecar
// (e.g., session.jsonl → session.json with a "cwd" field), then falls back to scanning
// ToolUse entries for "working_dir".
func findCwd(jsonlPath string, entries []LogEntry) string {
	jsonPath := strings.TrimSuffix(jsonlPath, "l") // ".jsonl" → ".json"
	if data, err := os.ReadFile(jsonPath); err == nil {
		var meta struct {
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(data, &meta) == nil && meta.Cwd != "" {
			return meta.Cwd
		}
	}

	for _, le := range entries {
		if le.Kind != "AssistantMessage" {
			continue
		}
		for _, c := range le.Data.Content {
			if c.Kind != "toolUse" {
				continue
			}
			var td ToolUseData
			if err := json.Unmarshal(c.Data, &td); err == nil && td.Input.WorkingDir != "" {
				return td.Input.WorkingDir
			}
		}
	}
	return ""
}
