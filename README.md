# Kiromon

[한국어](README.ko.md)

macOS menubar app that monitors your [Kiro](https://kiro.dev) CLI/IDE usage at a glance.

![macOS](https://img.shields.io/badge/platform-macOS-lightgrey)
![Go](https://img.shields.io/badge/language-Go-00ADD8)

## Features

- 🤖 Menubar title showing today's active time, messages, and sessions
- 💳 Kiro usage percentage display
- 📂 Active projects with git branch info
- 🔄 Real-time updates via filesystem watching (fsnotify + debounce)
- 📈 Weekly message summary

## Install

### From source

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

## How it works

Kiromon reads Kiro session files from `~/.kiro/sessions/cli/*.jsonl` and calculates:

| Metric | Description |
|--------|-------------|
| Sessions | Number of sessions started today |
| Messages | Prompt + AssistantMessage count for today |
| Active Time | Sum of (last - first message time) per session |
| This Week | Total messages from Monday to today |

## Development

```bash
make build    # Build binary
make app      # Package as .app bundle
make clean    # Remove build artifacts
go test ./... # Run tests
```

## License

MIT
