package history

import (
	"time"
)

type ForecastResult struct {
	DaysLeft int
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

	// 이번 달 1일부터 오늘까지 경과 일수 기반
	days := float64(now.Day())

	calc := func(pct float64) ForecastResult {
		avg := pct / days
		if avg <= 0 {
			return ForecastResult{-1}
		}
		return ForecastResult{int((100 - pct) / avg)}
	}

	return calc(currentCLIPct), calc(currentIDEPct)
}
