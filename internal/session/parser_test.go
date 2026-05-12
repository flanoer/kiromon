package session

import (
	"os"
	"testing"
)

func TestParseFile(t *testing.T) {
	// 1. 테스트용 가짜 JSONL 데이터 준비 (스펙 문서의 형태)
	mockData := `{"version":"v1","kind":"Prompt","data":{"message_id":"1","content":[{"kind":"text","data":"hi"}],"meta":{"timestamp":1778034161}}}
{"version":"v1","kind":"AssistantMessage","data":{"message_id":"2","content":[{"kind":"text","data":"hello"}]}}
{"version":"v1","kind":"ToolUse","data":{}}`

	// 2. 임시 파일 생성 (테스트가 끝나면 OS가 자동으로 삭제함)
	tmpFile, err := os.CreateTemp("", "kiro-test-*.jsonl")
	if err != nil {
		t.Fatalf("임시 파일 생성 실패: %v", err) // Fail-fast 원칙
	}
	defer os.Remove(tmpFile.Name()) // 테스트 종료 시 파일 정리 보장

	// 3. 임시 파일에 데이터 쓰기
	if _, err := tmpFile.Write([]byte(mockData)); err != nil {
		t.Fatalf("데이터 쓰기 실패: %v", err)
	}
	tmpFile.Close() // 쓰기 완료 후 닫기 (읽기를 위해)

	// 4. 우리가 만든 ParseFile 함수 테스트
	entries, err := ParseFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseFile 실행 중 에러 발생: %v", err)
	}

	// 5. 검증 (Assertion) - 우리가 원하는 데이터만 잘 파싱되었는가?
	if len(entries) != 3 {
		t.Errorf("기대값: 3줄, 실제값: %d줄", len(entries))
	}

	// 첫 번째 줄(Prompt) 검증
	if entries[0].Kind != "Prompt" {
		t.Errorf("기대값: Prompt, 실제값: %s", entries[0].Kind)
	}
	if entries[0].Data.Meta.Timestamp != 1778034161 {
		t.Errorf("기대값: 1778034161, 실제값: %d", entries[0].Data.Meta.Timestamp)
	}

	// 두 번째 줄(AssistantMessage - 타임스탬프 없음) 검증
	if entries[1].Data.Meta.Timestamp != 0 {
		t.Errorf("타임스탬프가 없는 경우 0이어야 함, 실제값: %d", entries[1].Data.Meta.Timestamp)
	}
}