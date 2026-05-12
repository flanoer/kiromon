package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type LogEntry struct {
	Kind string   `json:"kind"`
	Data DataNode `json:"data"`
}

type DataNode struct {
	Meta    MetaNode      `json:"meta"`
	Content []ContentNode `json:"content,omitempty"`
}

type MetaNode struct {
	Timestamp int64 `json:"timestamp"`
}

type ContentNode struct {
	Kind string          `json:"kind,omitempty"` // 🌟 하위 노드의 타입 식별자 추가
	Data json.RawMessage `json:"data,omitempty"`
}

// 🌟 ToolUse 파싱을 위한 전용 구조체를 밖으로 빼냅니다.
type ToolUseData struct {
	Input ToolUseInput `json:"input"`
}

type ToolUseInput struct {
	WorkingDir string `json:"working_dir"`
}

func ParseFile(filePath string) ([]LogEntry, error) {
	// 1. file open (access to infra layer)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err // 에러는 호출자에게 위임(Fail fast)
	}
	// 함수 종료시 파일 리소스 안전하게 반환(java 의 try with resources 와 유사)
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// 🌟 핵심 방어벽: 대용량 AI 로그(코드 블록 등)를 읽기 위한 버퍼 한계 해제
	// 초기 할당 1MB, 한 줄당 최대 10MB까지 허용 (기본값 64KB에서 대폭 상향)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	var entries []LogEntry
	// 스캐너를 통한 라인단위 읽기
	for scanner.Scan() {
		line := scanner.Bytes() // 텍스트 대신 바이트 배열로 읽어 성능 최적화

		if len(line) == 0 {
			continue // 빈줄 무시
		}

		var entry LogEntry
		// json 언마샬링
		if err := json.Unmarshal(line, &entry); err != nil {
			// 일부 라인이 깨져있어도 전체 프로세스가 죽지 않도록 로깅만 하고 넘어갈 수도 있음
			// MVP 단계이므로 일단 에러를 무시하고 진행하거나, 필요시 로그 남기기
			continue
		}

		entries = append(entries, entry)
	}

	// 스캐너 자체 에러 검증
	if err := scanner.Err(); err != nil {
		// 🌟 이제 "token too long" 에러 대신, 진짜 파일 IO 에러만 잡히게 됩니다.
		return nil, fmt.Errorf("error reading session file %s: %w", filePath, err)
	}
	return entries, nil
}
