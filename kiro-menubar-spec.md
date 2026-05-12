# kiro-menubar — macOS 메뉴바 Kiro 사용량 모니터

## 목적
macOS 메뉴바에서 kiro-cli / kiro IDE 사용량을 한눈에 확인하는 경량 앱

## 기술 스택
- **언어**: Go (학습 목적)
- **메뉴바**: systray 또는 동등한 Go 라이브러리
- **데이터 소스**: `~/.kiro/sessions/cli/*.jsonl`

## 기능 (MVP)

### 메뉴바 타이틀
오늘의 요약을 한 줄로 표시 (예: `🤖 3s 42m 2h`)

### 드롭다운 메뉴
```
┌─────────────────────────┐
│ 📊 Today                │
│   Sessions: 3           │
│   Messages: 42          │
│   Active Time: 2h 14m   │
│ ─────────────────────── │
│ 📈 This Week: 156 msgs  │
│ ─────────────────────── │
│ Quit                    │
└─────────────────────────┘
```

## 메트릭 정의

| 메트릭 | 설명 | 계산 방법 |
|--------|------|-----------|
| Sessions | 오늘 시작된 세션 수 | jsonl 파일의 첫 번째 엔트리 timestamp가 오늘인 파일 수 |
| Messages | 오늘 주고받은 메시지 수 | `kind: Prompt` + `kind: AssistantMessage` 카운트 |
| Active Time | 오늘 활성 시간 | 세션별 (마지막 메시지 시간 - 첫 메시지 시간) 합산 |
| This Week | 이번 주 메시지 수 | 월요일~오늘까지의 Messages 합산 |

## 세션 JSONL 구조 (참고)

```jsonl
{"version":"v1","kind":"Prompt","data":{"message_id":"...","content":[{"kind":"text","data":"..."}],"meta":{"timestamp":1778034161}}}
{"version":"v1","kind":"AssistantMessage","data":{"message_id":"...","content":[{"kind":"text","data":"..."}]}}
{"version":"v1","kind":"ToolUse","data":{...}}
{"version":"v1","kind":"ToolResults","data":{...}}
```

- timestamp: Unix epoch (초 단위), `Prompt`의 `meta.timestamp`에 존재
- AssistantMessage에는 timestamp가 없을 수 있음 → 파일 mtime 또는 이전 Prompt 시간 기준 추정

## 동작 방식
1. 앱 시작 시 `~/.kiro/sessions/cli/*.jsonl` 스캔
2. 오늘/이번주 해당 파일만 파싱 (파일 mtime으로 1차 필터링)
3. 주기적 갱신 (60초 간격)
4. 메뉴바 타이틀 + 드롭다운 업데이트

## 개발 진행 방식
- 페어 프로그래밍 스타일 (사용자가 직접 코딩, AI가 가이드)
- 단계별 진행:
  1. Go 프로젝트 초기화 + systray 라이브러리 설정
  2. JSONL 파서 구현
  3. 메트릭 계산 로직
  4. 메뉴바 UI 연결
  5. 주기적 갱신 + 파일 감시

## 참고
- systray 라이브러리: `github.com/getlantern/systray` 또는 `fyne.io/systray`
- 세션 파일 위치: `~/.kiro/sessions/cli/`
