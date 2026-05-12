package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ScanSessions는 지정된 디렉토리에서 조건에 맞는 JSONL 파일을 찾아 최종 통계를 반환합니다.
func ScanSessions(dirPath string, now time.Time) (Summary, []SessionStat, error) {
	// 1. '~' (홈 디렉토리) 경로 확장 처리
	// 아키텍트 팁: Go의 파일 입출력은 bash 쉘과 달리 '~'를 자동으로 인식하지 않습니다.
	if strings.HasPrefix(dirPath, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return Summary{}, nil, err
		}
		dirPath = filepath.Join(homeDir, dirPath[1:])
	}

	// 2. 디렉토리 안의 파일 목록 읽기
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return Summary{}, nil, err // 폴더가 없거나 권한이 없으면 Fail-fast
	}

	// 3. 이번 주 월요일 자정 계산 (필터링 기준점)
	daysSinceMonday := int(now.Weekday()) - int(time.Monday)
	if daysSinceMonday < 0 {
		daysSinceMonday += 7
	}
	startOfToday := getStartOfDay(now)
	startOfWeek := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -daysSinceMonday)

	var sessionStats []SessionStat

	// 4. 파일 순회 및 1차 필터링
	for _, entry := range entries {
		// 디렉토리이거나 .jsonl 파일이 아니면 무시
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // 파일 정보를 읽지 못하는 예외 상황은 무시하고 다음 파일로 진행
		}

		// ⭐️ 핵심 최적화: 파일 수정 시간(mtime)이 이번 주 월요일 이전이라면 파싱(I/O) 생략
		if info.ModTime().Before(startOfWeek) {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())

		// 5. 이전 단계에서 만든 파서(Parser)와 분석기(Analyzer) 파이프라인 연결
		logEntries, err := ParseFile(fullPath)
		if err != nil {
			continue
		}

		// 🌟 OS에서 읽어온 파일 정보(info)에서 ModTime()을 뽑아 함께 넘겨줍니다.
		stat := AnalyzeSession(logEntries, info.ModTime(), startOfToday, startOfWeek)

		// .jsonl에서 branch를 못 찾으면 .json 메타데이터의 cwd로 fallback
		jsonPath := strings.TrimSuffix(fullPath, "l") // .jsonl → .json
		if data, err := os.ReadFile(jsonPath); err == nil {
			var meta struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(data, &meta) == nil && meta.Cwd != "" {
				stat.Cwd = meta.Cwd
				if stat.Branch == "" {
					stat.Branch = getGitBranch(meta.Cwd)
				}
			}
		}

		sessionStats = append(sessionStats, stat)
	}

	// 6. 뷰 모델(Summary)로 최종 집계하여 반환
	return CalculateSummary(sessionStats, now), sessionStats, nil
}
