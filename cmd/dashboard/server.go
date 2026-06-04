package main

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/flanoer/kiromon/internal/history"
	"github.com/flanoer/kiromon/internal/session"
	"github.com/flanoer/kiromon/internal/usage"
)

const (
	sessionsDir            = "~/.kiro/sessions/cli/"
	projectsCacheTTL       = 5 * time.Minute
	defaultHistoryDays     = 30
	maxHistoryDays         = 365
	defaultProjectsDays    = 30
	maxProjectsDays        = 365
	topProjectLimitDefault = 10
)

func routes() http.Handler {
	mux := http.NewServeMux()

	// Embedded static assets under /static/*
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		slog.Error("failed to scope static FS", "error", err)
	} else {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS))))
	}

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/today", todayHandler)
	mux.HandleFunc("/api/history", historyHandler)
	mux.HandleFunc("/api/forecast", forecastHandler)
	mux.HandleFunc("/api/projects", projectsHandler)
	mux.HandleFunc("/api/aggregate", aggregateHandler)
	mux.HandleFunc("/api/health", healthHandler)

	return mux
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
}

// /api/today — live snapshot for the current day.
func todayHandler(w http.ResponseWriter, _ *http.Request) {
	now := time.Now()
	summary, _, err := session.ScanSessions(sessionsDir, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session scan failed", err)
		return
	}

	cliPct, cliErr := usage.GetUsagePercentage()
	idePct, ideErr := usage.GetIDEUsagePercentage()

	resp := map[string]any{
		"date":             now.Format("2006-01-02"),
		"sessions":         summary.TodaySessions,
		"messages":         summary.TodayMessages,
		"active_minutes":   int(summary.TodayActiveTime.Minutes()),
		"this_week_messages": summary.ThisWeekMessages,
		"cli_usage_pct":    nullableFloat(cliPct, cliErr),
		"ide_usage_pct":    nullableFloat(idePct, ideErr),
	}
	if cliErr != nil {
		resp["cli_usage_error"] = cliErr.Error()
	}
	if ideErr != nil {
		resp["ide_usage_error"] = ideErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// /api/history?days=N — daily records from history.csv plus today's live snapshot.
func historyHandler(w http.ResponseWriter, r *http.Request) {
	days := parseDays(r, defaultHistoryDays, maxHistoryDays)
	records, err := history.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "history load failed", err)
		return
	}

	// Merge today's live snapshot so the chart reflects the current day before
	// the next saveHistory tick.
	today := time.Now()
	todayKey := today.Format("2006-01-02")
	summary, _, _ := session.ScanSessions(sessionsDir, today)
	cliPct, _ := usage.GetUsagePercentage()
	idePct, _ := usage.GetIDEUsagePercentage()
	live := history.Record{
		Date:          todayKey,
		Sessions:      summary.TodaySessions,
		Messages:      summary.TodayMessages,
		ActiveMinutes: int(summary.TodayActiveTime.Minutes()),
		CLIUsagePct:   cliPct,
		IDEUsagePct:   idePct,
	}
	records = upsertRecord(records, live)

	cutoff := today.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	filtered := make([]history.Record, 0, len(records))
	for _, rec := range records {
		if rec.Date >= cutoff {
			filtered = append(filtered, rec)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Date < filtered[j].Date })

	writeJSON(w, http.StatusOK, map[string]any{
		"days":    days,
		"records": filtered,
	})
}

// /api/forecast — billing-cycle days-left projection for CLI and IDE.
func forecastHandler(w http.ResponseWriter, _ *http.Request) {
	records, err := history.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "history load failed", err)
		return
	}
	cliPct, _ := usage.GetUsagePercentage()
	idePct, _ := usage.GetIDEUsagePercentage()
	cliF, ideF := history.Forecast(records, cliPct, idePct)

	now := time.Now()
	// Days remaining in current calendar month (a rough proxy for the billing cycle).
	endOfMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	daysInMonth := int(endOfMonth.Sub(now).Hours() / 24)

	writeJSON(w, http.StatusOK, map[string]any{
		"cli_days_left":      forecastValue(cliF.DaysLeft),
		"ide_days_left":      forecastValue(ideF.DaysLeft),
		"cli_pct":            cliPct,
		"ide_pct":            idePct,
		"calendar_days_left": daysInMonth,
	})
}

// /api/projects?days=N — top projects by message count over the last N days.
// Cached per-days-key for projectsCacheTTL since session-file scanning is the
// most expensive endpoint.
func projectsHandler(w http.ResponseWriter, r *http.Request) {
	days := parseDays(r, defaultProjectsDays, maxProjectsDays)
	limit := parsePositiveInt(r.URL.Query().Get("limit"), topProjectLimitDefault)

	stats, err := getProjectsCached(days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project scan failed", err)
		return
	}
	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":     days,
		"projects": stats,
	})
}

// /api/aggregate?period=week|month — averages and peaks over recent records.
func aggregateHandler(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	records, err := history.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "history load failed", err)
		return
	}

	// Merge today's live snapshot for accuracy.
	today := time.Now()
	summary, _, _ := session.ScanSessions(sessionsDir, today)
	cliPct, _ := usage.GetUsagePercentage()
	idePct, _ := usage.GetIDEUsagePercentage()
	records = upsertRecord(records, history.Record{
		Date:          today.Format("2006-01-02"),
		Sessions:      summary.TodaySessions,
		Messages:      summary.TodayMessages,
		ActiveMinutes: int(summary.TodayActiveTime.Minutes()),
		CLIUsagePct:   cliPct,
		IDEUsagePct:   idePct,
	})

	var days int
	switch period {
	case "week":
		days = 7
	case "month":
		days = 30
	case "year":
		days = 365
	default:
		days = 7
	}
	cutoff := today.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	var (
		count                                    int
		sumSessions, sumMessages, sumActive      int
		peakSessions, peakMessages, peakActive   int
		peakSessionsDate, peakMsgsDate, peakActDate string
	)
	for _, rec := range records {
		if rec.Date < cutoff {
			continue
		}
		count++
		sumSessions += rec.Sessions
		sumMessages += rec.Messages
		sumActive += rec.ActiveMinutes
		if rec.Sessions > peakSessions {
			peakSessions = rec.Sessions
			peakSessionsDate = rec.Date
		}
		if rec.Messages > peakMessages {
			peakMessages = rec.Messages
			peakMsgsDate = rec.Date
		}
		if rec.ActiveMinutes > peakActive {
			peakActive = rec.ActiveMinutes
			peakActDate = rec.Date
		}
	}

	avg := func(total int) float64 {
		if count == 0 {
			return 0
		}
		return float64(total) / float64(count)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"period":          period,
		"days":            days,
		"sample_size":     count,
		"avg_sessions":    avg(sumSessions),
		"avg_messages":    avg(sumMessages),
		"avg_active_min":  avg(sumActive),
		"peak_sessions":   peakSessions,
		"peak_sessions_date": peakSessionsDate,
		"peak_messages":   peakMessages,
		"peak_messages_date": peakMsgsDate,
		"peak_active_min": peakActive,
		"peak_active_date": peakActDate,
	})
}

// --- helpers ---

func parseDays(r *http.Request, def, max int) int {
	q := r.URL.Query().Get("days")
	if q == "" {
		return def
	}
	n, err := strconv.Atoi(q)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func nullableFloat(v float64, err error) any {
	if err != nil {
		return nil
	}
	return v
}

func forecastValue(daysLeft float64) any {
	if daysLeft < 0 {
		return nil
	}
	return daysLeft
}

func upsertRecord(records []history.Record, r history.Record) []history.Record {
	for i, rec := range records {
		if rec.Date == r.Date {
			records[i] = r
			return records
		}
	}
	return append(records, r)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string, err error) {
	slog.Warn(message, "error", err)
	writeJSON(w, status, map[string]any{
		"error":   message,
		"detail":  err.Error(),
	})
}

// --- projects cache ---

type projectsCacheEntry struct {
	expiresAt time.Time
	stats     []session.ProjectStats
}

var (
	projectsMu    sync.Mutex
	projectsByDay = map[int]projectsCacheEntry{}
)

func getProjectsCached(days int) ([]session.ProjectStats, error) {
	projectsMu.Lock()
	if entry, ok := projectsByDay[days]; ok && time.Now().Before(entry.expiresAt) {
		projectsMu.Unlock()
		return entry.stats, nil
	}
	projectsMu.Unlock()

	now := time.Now()
	start := now.AddDate(0, 0, -(days - 1))
	stats, err := session.ScanProjectsRange(sessionsDir, start, now)
	if err != nil {
		return nil, err
	}

	projectsMu.Lock()
	projectsByDay[days] = projectsCacheEntry{
		expiresAt: time.Now().Add(projectsCacheTTL),
		stats:     stats,
	}
	projectsMu.Unlock()
	return stats, nil
}
