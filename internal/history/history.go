package history

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Record struct {
	Date          string
	Sessions      int
	Messages      int
	ActiveMinutes int
	CLIUsagePct   float64
	IDEUsagePct   float64
}

const csvPath = "~/.kiromon/history.csv"

var header = []string{"date", "sessions", "messages", "active_minutes", "cli_usage_pct", "ide_usage_pct"}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

func Load() ([]Record, error) {
	f, err := os.Open(expandPath(csvPath))
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	var records []Record
	for _, row := range rows[1:] { // skip header
		if len(row) < 6 {
			continue
		}
		sessions, _ := strconv.Atoi(row[1])
		messages, _ := strconv.Atoi(row[2])
		activeMinutes, _ := strconv.Atoi(row[3])
		cliPct, _ := strconv.ParseFloat(row[4], 64)
		idePct, _ := strconv.ParseFloat(row[5], 64)
		records = append(records, Record{
			Date:          row[0],
			Sessions:      sessions,
			Messages:      messages,
			ActiveMinutes: activeMinutes,
			CLIUsagePct:   cliPct,
			IDEUsagePct:   idePct,
		})
	}
	return records, nil
}

func Save(records []Record) error {
	path := expandPath(csvPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Write(header)
	for _, r := range records {
		w.Write([]string{
			r.Date,
			strconv.Itoa(r.Sessions),
			strconv.Itoa(r.Messages),
			strconv.Itoa(r.ActiveMinutes),
			strconv.FormatFloat(r.CLIUsagePct, 'f', -1, 64),
			strconv.FormatFloat(r.IDEUsagePct, 'f', -1, 64),
		})
	}
	w.Flush()
	return w.Error()
}

func Upsert(r Record) error {
	records, err := Load()
	if err != nil {
		return err
	}

	found := false
	for i, rec := range records {
		if rec.Date == r.Date {
			records[i] = r
			found = true
			break
		}
	}
	if !found {
		records = append(records, r)
	}

	cutoff := time.Now().AddDate(0, 0, -365).Format("2006-01-02")
	trimmed := records[:0]
	for _, rec := range records {
		if rec.Date >= cutoff {
			trimmed = append(trimmed, rec)
		}
	}

	return Save(trimmed)
}
