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

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+

### Install

```bash
git clone https://github.com/flanoer/kiromon.git
cd kiromon
make install
```

This builds the binary, packages it as `Kiromon.app`, and copies it to `/Applications`.

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

### 사전 요구사항

- [Go](https://go.dev/dl/) 1.26+

### 설치

```bash
git clone https://github.com/flanoer/kiromon.git
cd kiromon
make install
```

바이너리를 빌드하고 `Kiromon.app`으로 패키징한 뒤 `/Applications`에 복사합니다.

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

### 개발

```bash
make build    # 바이너리 빌드
make app      # .app 번들 패키징
make clean    # 빌드 산출물 정리
go test ./... # 테스트 실행
```

### 라이선스

MIT
