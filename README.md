# Kiromon

[English](#english) | [한국어](#한국어)

![macOS](https://img.shields.io/badge/platform-macOS-lightgrey)
![Go](https://img.shields.io/badge/language-Go-00ADD8)

---

## English

macOS menubar app that monitors your [Kiro](https://kiro.dev) CLI/IDE usage at a glance.

### Features

- 🤖 Menubar title showing today's active time, messages, and sessions
- 💳 Kiro usage percentage display
- 📂 Active projects with git branch info
- 🔄 Real-time updates via filesystem watching (fsnotify + debounce)
- 📈 Weekly message summary
- 📊 Companion **Dashboard** web UI (open from the menubar) with daily charts, forecasts, aggregates, and per-project breakdown

### Install (Download)

```bash
curl -sL https://github.com/flanoer/kiromon/releases/latest/download/Kiromon.app.zip -o /tmp/Kiromon.app.zip \
  && unzip -o /tmp/Kiromon.app.zip -d ~/Applications \
  && xattr -cr ~/Applications/Kiromon.app \
  && rm /tmp/Kiromon.app.zip
```

### Install (Build from source)

Requires [Go](https://go.dev/dl/) 1.26+

```bash
git clone https://github.com/flanoer/kiromon.git
cd kiromon
make install
```

This builds the binary, packages it as `Kiromon.app`, and copies it to `~/Applications`.

### Uninstall

```bash
make uninstall
```

### How it works

Kiromon reads Kiro session files from `~/.kiro/sessions/cli/*.jsonl` and calculates:

| Metric | Description |
|--------|-------------|
| Sessions | Number of sessions started today |
| Messages | Prompt + AssistantMessage count for today |
| Active Time | Sum of (last - first message time) per session |
| This Week | Total messages from Monday to today |

### Dashboard

Click **📊 Open Dashboard** in the menubar to launch a local web dashboard at
`http://127.0.0.1:17891/` with:

- Today's snapshot (sessions, messages, active time, CLI/IDE usage %)
- Forecast — days left in the current billing cycle for CLI and IDE
- Daily activity bar chart (toggle 30 / 90 / 365 days)
- Weekly / monthly / yearly aggregates (averages and peaks)
- Top projects by message count

The dashboard runs as a separate `kiromon-dashboard` binary that is bundled
inside `Kiromon.app`. Kiromon spawns it on demand and shuts it down when you
quit. You can also run it standalone for development:

```bash
make run-dashboard               # go run ./cmd/dashboard
# or after `make dashboard`:
./kiromon-dashboard --port 8080  # custom port
./kiromon-dashboard --port 0     # OS-assigned random port
```

Persistent settings live in `~/.kiromon/dashboard.json`:

```json
{ "port": 17891 }
```

CLI flags override the config file. The bound port is written to
`~/.kiromon/dashboard.port` so the menubar can rediscover an already-running
instance.

### Development

```bash
make build    # Build binary
make app      # Package as .app bundle
make clean    # Remove build artifacts
go test ./... # Run tests
```

### License

MIT

---

## 한국어

macOS 메뉴바에서 [Kiro](https://kiro.dev) CLI/IDE 사용량을 한눈에 확인하는 경량 앱입니다.

### 기능

- 🤖 메뉴바에 오늘의 활성 시간, 메시지 수, 세션 수 표시
- 💳 Kiro 사용량 퍼센트 표시
- 📂 활성 프로젝트 및 git 브랜치 정보
- 🔄 파일시스템 감시를 통한 실시간 업데이트 (fsnotify + debounce)
- 📈 주간 메시지 요약
- 📊 동반 **대시보드** 웹 UI (메뉴바에서 열기) — 일별 차트, forecast, 집계, 프로젝트별 분포 제공

### 설치 (다운로드)

```bash
curl -sL https://github.com/flanoer/kiromon/releases/latest/download/Kiromon.app.zip -o /tmp/Kiromon.app.zip \
  && unzip -o /tmp/Kiromon.app.zip -d ~/Applications \
  && xattr -cr ~/Applications/Kiromon.app \
  && rm /tmp/Kiromon.app.zip
```

### 설치 (소스 빌드)

[Go](https://go.dev/dl/) 1.26+ 필요

```bash
git clone https://github.com/flanoer/kiromon.git
cd kiromon
make install
```

바이너리를 빌드하고 `Kiromon.app`으로 패키징한 뒤 `~/Applications`에 복사합니다.

### 제거

```bash
make uninstall
```

### 동작 방식

Kiromon은 `~/.kiro/sessions/cli/*.jsonl` 세션 파일을 읽어 다음을 계산합니다:

| 메트릭 | 설명 |
|--------|------|
| Sessions | 오늘 시작된 세션 수 |
| Messages | 오늘의 Prompt + AssistantMessage 수 |
| Active Time | 세션별 (마지막 메시지 - 첫 메시지 시간) 합산 |
| This Week | 월요일~오늘까지 총 메시지 수 |

### 대시보드

메뉴바에서 **📊 Open Dashboard** 클릭 → 로컬 웹 대시보드(`http://127.0.0.1:17891/`)
가 브라우저에 열립니다. 표시 내용:

- 오늘 요약 (세션, 메시지, 활성 시간, CLI/IDE Usage %)
- Forecast — 현재 청구 사이클 기준 CLI/IDE 잔여 일수
- 일별 활동 막대 차트 (30 / 90 / 365일 토글)
- 주간 / 월간 / 연간 집계 (평균·최고치)
- Top 프로젝트 (메시지 수 기준)

대시보드는 `Kiromon.app` 번들 안에 함께 들어있는 별도 `kiromon-dashboard`
바이너리로 동작합니다. 메뉴 클릭 시 Kiromon이 자식 프로세스로 띄우고, Kiromon
종료 시 함께 정리됩니다. 개발/단독 실행도 지원합니다:

```bash
make run-dashboard               # go run ./cmd/dashboard
# 또는 `make dashboard` 후:
./kiromon-dashboard --port 8080  # 포트 지정
./kiromon-dashboard --port 0     # OS 랜덤 포트
```

영속 설정은 `~/.kiromon/dashboard.json`에 저장합니다:

```json
{ "port": 17891 }
```

CLI 플래그가 설정 파일보다 우선합니다. 실제로 바인딩된 포트는
`~/.kiromon/dashboard.port` 파일에 기록되어, 메뉴바가 이미 떠 있는 인스턴스를
재발견할 수 있게 합니다.

### 개발

```bash
make build    # 바이너리 빌드
make app      # .app 번들 패키징
make clean    # 빌드 산출물 정리
go test ./... # 테스트 실행
```

### 라이선스

MIT
