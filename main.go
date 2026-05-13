package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/flanoer/kiromon/internal/session"
	"github.com/flanoer/kiromon/internal/usage"

	"fyne.io/systray"
	"github.com/fsnotify/fsnotify"
)

// 🌟 아키텍트의 팁: 상태를 유지해야 하는 변수들은 패키지 레벨에 모아둡니다.
var (
	lastSummary      session.Summary
	lastDay          int
	sessionMenuItems []*systray.MenuItem
	logFile          *os.File
)

func main() {
	initLogger()
	if logFile != nil {
		defer logFile.Close()
	}

	// 🔒 단일 인스턴스 보장: 이미 실행 중이면 즉시 종료
	home, _ := os.UserHomeDir()
	lockPath := filepath.Join(home, ".kiromon.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		slog.Error("Failed to open lock file", "error", err)
		os.Exit(1)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		slog.Error("Another Kiromon instance is already running. Exiting.")
		lockFile.Close()
		os.Exit(1)
	}
	defer func() {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		os.Remove(lockPath)
	}()

	// systray.Run은 내부적으로 macOS의 메인 이벤트 루프(UI 스레드)를 점유합니다.
	// 이 함수가 호출되면 앱이 종료될 때까지 블로킹(대기)됩니다.
	systray.Run(onReady, nil)
}

// expandPath는 "~" 경로를 실제 홈 디렉토리 절대 경로로 변환합니다.
// fsnotify는 OS 레벨의 경로를 요구하기 때문에 이 과정이 꼭 필요합니다.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

func onReady() {
	// 1. UI 뼈대 만들기
	systray.SetTitle("🤖 Kiro: Loading...")

	mToday := systray.AddMenuItem("📊 Today", "")
	mToday.Disable() // 단순 헤더 역할을 위해 클릭 비활성화

	mSessions := systray.AddMenuItem("  Sessions: 0", "")
	mMessages := systray.AddMenuItem("  Messages: 0", "")
	mActiveTime := systray.AddMenuItem("  Active Time: 0m", "")

	// 🌟 Usage 메뉴 아이템 추가
	mUsage := systray.AddMenuItem("💳 CLI Usage: Loading...", "")
	mIDEUsage := systray.AddMenuItem("💳 IDE Usage: Loading...", "")

	systray.AddSeparator()
	mThisWeek := systray.AddMenuItem("📈 This Week: 0 msgs", "")

	mSessions.Disable()
	mMessages.Disable()
	mActiveTime.Disable()
	mUsage.Disable()
	mIDEUsage.Disable()
	mThisWeek.Disable()

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "앱 종료")
	mRefresh := systray.AddMenuItem("🔄 Refresh", "")

	// 2. 백그라운드 작업 스케줄러 실행 (fsnotify + Debounce 아키텍처)
	go func() {
		targetDir := expandPath("~/.kiro/sessions/cli/")

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			slog.Error("Failed to create fsnotify watcher", "error", err)
			return
		}
		defer watcher.Close()

		if err := watcher.Add(targetDir); err != nil {
			slog.Warn("Directory watch failed", "path", targetDir, "error", err)
			return // 폴더가 아직 없다면 앱이 죽지 않고 대기하도록 에러 로깅만 할 수도 있습니다.
		}

		// 🌟 핵심: 디바운스(Debounce) 타이머 설정
		// 타이머를 생성하고 즉시 멈춰둡니다. (이벤트가 올 때만 활성화)
		debounceTimer := time.NewTimer(0)
		<-debounceTimer.C // 버퍼 비우기

		// 예비용 하트비트 (자정 날짜 변경 등을 캐치하기 위해 1시간마다 조용히 실행)
		heartbeat := time.NewTicker(1 * time.Hour)
		defer heartbeat.Stop()

		// 앱 시작 시 최초 1회 실행
		updateUI(mSessions, mMessages, mActiveTime, mThisWeek, mUsage, mIDEUsage)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// 파일 내용이 쓰여지거나(Write) 새 파일이 생성(Create)될 때만 반응
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					// 0.5초짜리 타이머를 계속 리셋합니다.
					// 즉, Kiro CLI가 0.1초 간격으로 파일을 계속 쓰면 이 타이머는 절대 터지지 않습니다.
					debounceTimer.Reset(500 * time.Millisecond)
				}

			case <-debounceTimer.C:
				// 파일 쓰기가 멈추고 0.5초가 무사히 지나면 비로소 UI를 갱신합니다. (Race Condition 확률 극도로 저하)
				slog.Debug("File change detected -> updating UI")
				updateUI(mSessions, mMessages, mActiveTime, mThisWeek, mUsage, mIDEUsage)

			case <-heartbeat.C:
				slog.Debug("Heartbeat -> updating UI")
				updateUI(mSessions, mMessages, mActiveTime, mThisWeek, mUsage, mIDEUsage)

			case <-mRefresh.ClickedCh:
				slog.Info("Manual refresh triggered")
				updateUI(mSessions, mMessages, mActiveTime, mThisWeek, mUsage, mIDEUsage)
			}
		}
	}()

	// 3. 종료 버튼 이벤트 리스너 (고루틴)
	go func() {
		<-mQuit.ClickedCh // Quit 버튼이 클릭될 때까지 대기
		systray.Quit()    // 앱 정상 종료
	}()
}

// onExit는 앱이 종료되기 직전에 실행됩니다. (리소스 정리 등)
func onExit() {
	slog.Info("Kiro Menubar stopped")
}

// updateUI는 1단계~3단계에서 만든 로직을 호출하여 메뉴바 텍스트를 갱신합니다.
func updateUI(mSessions, mMessages, mActiveTime, mThisWeek, mUsage, mIDEUsage *systray.MenuItem) {
	// 🌟 1. 함수 시작과 동시에 현재 시간(now) 캡처
	now := time.Now()

	summary, sessions, err := session.ScanSessions("~/.kiro/sessions/cli/", now)
	if err != nil {
		// 디버그 2: 폴더를 못 읽었을 때 에러 출력
		slog.Error("Session scan failed", "error", err)
		systray.SetTitle("🤖 Kiro: Error")
		return
	}

	// 🌟 1. 자정이 지나면 캐시 초기화
	currentDay := now.Day()
	if currentDay != lastDay {
		lastSummary = session.Summary{}
		lastDay = currentDay
	}

	// 🌟 2. 2차 방어벽: 데이터가 이전보다 줄어들었다면(Race Condition) 무시하고 이전 캐시 유지
	if summary.TodayMessages < lastSummary.TodayMessages || summary.TodaySessions < lastSummary.TodaySessions {
		slog.Warn("Race condition detected! Keeping cache.",
			"new_msgs", summary.TodayMessages,
			"old_msgs", lastSummary.TodayMessages)
		summary = lastSummary
	} else {
		lastSummary = summary // 정상적이면 캐시 업데이트
	}

	// 기존 개별 세션 메뉴 아이템들 제거 (Hide)
	for _, item := range sessionMenuItems {
		item.Hide()
	}
	sessionMenuItems = nil

	// 🌟 최근 세션의 프로젝트 + 브랜치 정보 표시 (EndTime 기준 최신순 정렬)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].EndTime.After(sessions[j].EndTime)
	})
	if len(sessions) > 0 {
		systray.AddSeparator()
		header := systray.AddMenuItem("📂 Active Projects:", "")
		header.Disable()
		sessionMenuItems = append(sessionMenuItems, header)

		// 프로젝트별로 중복 없이 최근 순으로 나열
		projectMap := make(map[string]bool)
		for i := 0; i < len(sessions) && len(projectMap) < 10; i++ {
			s := sessions[i]
			if s.Cwd == "" {
				continue
			}
			projectName := filepath.Base(s.Cwd)
			if projectMap[projectName] {
				continue
			}
			projectMap[projectName] = true

			var label string
			if s.Branch != "" {
				label = fmt.Sprintf("  ├─ %s → %s", projectName, s.Branch)
			} else {
				label = fmt.Sprintf("  ├─ %s (no git)", projectName)
			}
			item := systray.AddMenuItem(label, "")
			item.Disable()
			sessionMenuItems = append(sessionMenuItems, item)
		}
	}

	// ActiveTime 포맷팅 (예: 2h 14m)
	activeStr := formatDuration(summary.TodayActiveTime)

	// 🌟 Usage 데이터 가져오기 (Graceful Fallback)
	cliPct, cliErr := usage.GetUsagePercentage()
	idePct, ideErr := usage.GetIDEUsagePercentage()

	var cliStr, ideStr, usageStr string
	if cliErr != nil {
		cliStr = "N/A"
		mUsage.SetTitle("💳 CLI Usage: N/A")
		slog.Error("CLI Usage API failed", "error", cliErr)
	} else {
		cliStr = fmt.Sprintf("%.1f%%", cliPct)
		mUsage.SetTitle(fmt.Sprintf("💳 CLI Usage: %.1f%%", cliPct))
	}
	if ideErr != nil {
		ideStr = "N/A"
		mIDEUsage.SetTitle("💳 IDE Usage: N/A")
		slog.Error("IDE Usage read failed", "error", ideErr)
	} else {
		ideStr = fmt.Sprintf("%.1f%%", idePct)
		mIDEUsage.SetTitle(fmt.Sprintf("💳 IDE Usage: %.1f%%", idePct))
	}

	// 타이틀: 🤖 2h 14m 42 3 | 💳 CLI 9.9% IDE 9.9%
	usageStr = fmt.Sprintf(" | 💳 CLI %s IDE %s", cliStr, ideStr)

	// 메뉴바 타이틀에 Usage 결합
	title := fmt.Sprintf("🤖 %s %d %d%s", activeStr, summary.TodayMessages, summary.TodaySessions, usageStr)
	systray.SetTitle(title)

	// 드롭다운 메뉴 아이템 업데이트
	mSessions.SetTitle(fmt.Sprintf("  Sessions: %d", summary.TodaySessions))
	mMessages.SetTitle(fmt.Sprintf("  Messages: %d", summary.TodayMessages))
	mActiveTime.SetTitle(fmt.Sprintf("  Active Time: %s", activeStr))
	mThisWeek.SetTitle(fmt.Sprintf("📈 This Week: %d msgs", summary.ThisWeekMessages))
}

// formatDuration은 time.Duration을 UI 요구사항에 맞게 예쁘게 변환합니다.
func formatDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	} else if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func initLogger() {
	home, err := os.UserHomeDir()
	if err != nil {
		// 홈 디렉토리를 못 찾으면 기본 터미널 출력으로 폴백
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
		return
	}

	// macOS 표준 로그 폴더 지정
	logDir := filepath.Join(home, "Library", "Logs")
	os.MkdirAll(logDir, 0755) // 폴더가 없으면 생성

	logPath := filepath.Join(logDir, "kiro-menubar.log")

	// 파일 열기 (추가 모드, 없으면 생성)
	logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
		return
	}

	// 개발 중에는 LevelDebug, 운영 시에는 LevelInfo로 설정
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// TextHandler (또는 JSONHandler) 적용 후 글로벌 기본 로거로 세팅
	logger := slog.New(slog.NewTextHandler(logFile, opts))
	slog.SetDefault(logger)

	slog.Info("Kiro Menubar started")
}
