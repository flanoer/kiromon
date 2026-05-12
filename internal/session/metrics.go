package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionStat은 단일 세션(파일 1개)의 비즈니스 의미를 담는 도메인 모델입니다.
type SessionStat struct {
	Branch           string
	Cwd              string
	StartTime        time.Time
	EndTime          time.Time
	TotalMessages    int
	TodayMessages    int
	ThisWeekMessages int
	TodayActiveTime  time.Duration
	FirstTsToday     int64
	LastTsToday      int64
}

// Summary는 UI(메뉴바)에 직접 표시될 최종 집계 데이터(ViewModel)입니다.
type Summary struct {
	TodaySessions    int
	TodayMessages    int
	TodayActiveTime  time.Duration
	ThisWeekMessages int
}

// interval: 시간 구간을 나타내는 내부 구조체
type interval struct {
	start int64
	end   int64
}

// AnalyzeSession은 파싱된 로그 배열과 파일의 수정 시간(mtime)을 받아 SessionStat으로 변환합니다.
func AnalyzeSession(entries []LogEntry, fileModTime time.Time, startOfToday, startOfWeek time.Time) SessionStat {
	var stat SessionStat
	var firstTs, lastTs int64
	var firstTsToday, lastTsToday int64
	var workingDir string

	var lastKnownTs int64 // 🌟 핵심: 가장 최근에 발견된 유효한 타임스탬프를 기억

	if len(entries) == 0 {
		return stat
	}

	for _, entry := range entries {
		// 🌟 어떤 종류의 로그든 유효한 타임스탬프가 보이면 기억해 둡니다.
		if entry.Data.Meta.Timestamp > 0 {
			lastKnownTs = entry.Data.Meta.Timestamp
		}

		if entry.Kind == "Prompt" || entry.Kind == "AssistantMessage" {
			stat.TotalMessages++

			effectiveTs := entry.Data.Meta.Timestamp
			// 🌟 AssistantMessage처럼 ts가 0으로 들어오면 직전 기록(lastKnownTs)을 상속
			if effectiveTs == 0 {
				effectiveTs = lastKnownTs
			}

			if effectiveTs > 0 {
				if firstTs == 0 {
					firstTs = effectiveTs
				}
				lastTs = effectiveTs

				entryTime := time.Unix(effectiveTs, 0) // 초 단위 가정

				// 1. 오늘 발생한 이벤트만 따로 집계
				if !entryTime.Before(startOfToday) {
					stat.TodayMessages++
					if firstTsToday == 0 {
						firstTsToday = effectiveTs
					}
					lastTsToday = effectiveTs
				}

				// 2. 이번 주 발생한 이벤트 집계
				if !entryTime.Before(startOfWeek) {
					stat.ThisWeekMessages++
				}
			}
		}

		// 3. 정확한 계층 탐색 로직
		if workingDir == "" && entry.Kind == "AssistantMessage" && len(entry.Data.Content) > 0 {
			for _, content := range entry.Data.Content {
				if content.Kind == "toolUse" {

					// 🌟 밖에서 정의한 깔끔한 구조체 사용
					var toolData ToolUseData

					if err := json.Unmarshal(content.Data, &toolData); err == nil {
						if toolData.Input.WorkingDir != "" {
							workingDir = toolData.Input.WorkingDir
							break
						}
					}
				}
			}
		}
	}

	// 전체 세션의 시작/종료 시간 세팅
	if firstTs > 0 {
		stat.StartTime = time.Unix(firstTs, 0)
	} else {
		stat.StartTime = fileModTime
	}

	if lastTs > 0 {
		stat.EndTime = time.Unix(lastTs, 0)
	} else {
		stat.EndTime = fileModTime
	}

	// 로컬 변수값을 구조체 필드에 명시적으로 할당
	stat.FirstTsToday = firstTsToday
	stat.LastTsToday = lastTsToday

	// 3. 시작 시간 설정
	if firstTsToday > 0 && lastTsToday > firstTsToday {
		stat.TodayActiveTime = time.Unix(lastTsToday, 0).Sub(time.Unix(firstTsToday, 0))
	}

	// 🌟 [mtime 패치 적용 부분] 🌟
	// 마지막 시간이 갱신되었으면 그것을 쓰고,
	// 갱신되지 않았거나(firstTs와 동일) 없다면 OS가 알려준 파일 수정 시간(fileModTime)을 강제로 집어넣습니다.
	if lastTs > 0 && lastTs != firstTs {
		stat.EndTime = time.Unix(lastTs, 0)
	} else if !fileModTime.IsZero() {
		stat.EndTime = fileModTime // Fallback (대체)
	}

	// 🌟 브랜치 정보 조회
	if workingDir != "" {
		stat.Branch = getGitBranch(workingDir)
	}

	return stat
}

// ActiveTime은 세션의 활성 시간(마지막 시간 - 처음 시간)을 반환합니다.
func (s SessionStat) ActiveTime() time.Duration {
	// 시작과 종료 시간이 모두 존재하고, 종료 시간이 더 뒤일 때만 계산
	if !s.StartTime.IsZero() && !s.EndTime.IsZero() && s.EndTime.After(s.StartTime) {
		return s.EndTime.Sub(s.StartTime)
	}
	return 0
}

// CalculateSummary: 모든 세션을 분석하여 오늘과 이번 주의 통계를 산출합니다.
func CalculateSummary(sessions []SessionStat, now time.Time) Summary {
	var summary Summary
	startOfToday := getStartOfDay(now)
	var activeIntervals []interval // 오늘 활동 구간들을 모을 바구니

	for _, s := range sessions {
		// 끝난 시간이 오늘 이후라면 "오늘 활동한 세션"으로 간주
		if !s.EndTime.Before(startOfToday) {
			summary.TodaySessions++
			summary.TodayMessages += s.TodayMessages

			// 🌟 오늘치 활동 구간이 있다면 수집 (단순 합산 대신 사용)
			if s.FirstTsToday > 0 && s.LastTsToday > 0 {
				activeIntervals = append(activeIntervals, interval{
					start: s.FirstTsToday,
					end:   s.LastTsToday,
				})
			}
		}
		summary.ThisWeekMessages += s.ThisWeekMessages
	}

	// 🌟 구간 병합 알고리즘 적용 (중복 시간 제거)
	merged := mergeIntervals(activeIntervals)
	var totalActiveSeconds int64
	for _, m := range merged {
		totalActiveSeconds += (m.end - m.start)
	}

	summary.TodayActiveTime = time.Duration(totalActiveSeconds) * time.Second

	return summary
}

// getGitBranch는 .git/HEAD 파일을 직접 파싱하여 브랜치명을 가져옵니다.
func getGitBranch(workingDir string) string {
	if workingDir == "" {
		return ""
	}

	headPath := filepath.Join(workingDir, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "" // Git 저장소가 아니거나 읽을 수 없는 경우
	}

	content := strings.TrimSpace(string(data))
	// 예: "ref: refs/heads/main" -> "main"
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/")
	}
	// Detached HEAD인 경우 앞 7자리 커밋 해시
	if len(content) >= 7 {
		return content[:7]
	}
	return ""
}

// mergeIntervals: 겹치는 시간 구간을 합집합으로 병합합니다.
func mergeIntervals(intervals []interval) []interval {
	if len(intervals) <= 1 {
		return intervals
	}

	// 시작 시간 기준으로 정렬
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start < intervals[j].start
	})

	merged := []interval{intervals[0]}
	for i := 1; i < len(intervals); i++ {
		last := &merged[len(merged)-1]
		current := intervals[i]

		if current.start <= last.end {
			if current.end > last.end {
				last.end = current.end
			}
		} else {
			merged = append(merged, current)
		}
	}
	return merged
}

func getStartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func getStartOfWeek(t time.Time) time.Time {
	startOfDay := getStartOfDay(t)
	daysToSubtract := int(t.Weekday()) // 일요일(0) 기준
	return startOfDay.AddDate(0, 0, -daysToSubtract)
}
