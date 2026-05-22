package history

import (
	"time"
)

type ForecastResult struct {
	DaysLeft float64 // -1 means N/A (insufficient data)
}

func Forecast(records []Record, currentCLIPct, currentIDEPct float64) (ForecastResult, ForecastResult) {
	now := time.Now()
	prefix := now.Format("2006-01")

	hasData := false
	for _, r := range records {
		if len(r.Date) >= 7 && r.Date[:7] == prefix {
			hasData = true
			break
		}
	}

	na := ForecastResult{-1}
	if !hasData {
		return na, na
	}

	days := float64(now.Day())

	calc := func(pct float64) ForecastResult {
		avg := pct / days
		if avg <= 0 {
			return ForecastResult{-1}
		}
		return ForecastResult{(100 - pct) / avg}
	}

	return calc(currentCLIPct), calc(currentIDEPct)
}
