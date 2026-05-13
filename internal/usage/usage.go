package usage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite 드라이버
)

// 🌟 데이터베이스 스키마 매핑
type TokenData struct {
	AccessToken  string `json:"access_token"`
	ExpiresAt    string `json:"expires_at"`
	RefreshToken string `json:"refresh_token"`
	Region       string `json:"region"`
}

type ClientData struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// OIDC 토큰 갱신 응답 구조
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type APIResponse struct {
	UsageBreakdownList []struct {
		// 🌟 실제 JSON 키에 맞게 카멜 케이스로 변경!
		CurrentUsage float64 `json:"currentUsageWithPrecision"`
		UsageLimit   float64 `json:"usageLimitWithPrecision"`
	} `json:"usageBreakdownList"` // 🌟 여기도 카멜 케이스로 변경!
}

var (
	// Usage API 결과 캐시 (5분 유지)
	usageMutex    sync.Mutex
	cachedPercent float64
	usageExpiry   time.Time

	// IDE Usage 캐시 (5분 유지)
	ideMutex      sync.Mutex
	idePercent    float64
	ideExpiry     time.Time

	// 🌟 새롭게 추가: 토큰 메모리 캐시 (만료 시점까지 유지)
	tokenMutex      sync.Mutex
	cachedToken     string
	tokenExpiryTime time.Time
)

func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}

// getDBValue 는 SQLite의 auth_kv 테이블에서 특정 키의 값을 가져옵니다.
func getDBValue(dbPath, key string) (string, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var value string
	err = db.QueryRow("SELECT value FROM auth_kv WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("key not found in db: %s", key)
		}
		return "", err
	}
	return value, nil
}

// refreshOIDCToken 은 만료된 토큰을 AWS OIDC 엔드포인트를 통해 갱신합니다.
func refreshOIDCToken(tokenData TokenData, clientData ClientData) (string, int, error) {
	payload := map[string]string{
		"clientId":     clientData.ClientId,
		"clientSecret": clientData.ClientSecret,
		"grantType":    "refresh_token",
		"refreshToken": tokenData.RefreshToken,
	}
	reqBody, _ := json.Marshal(payload)

	// OIDC Endpoint 구성 (보통 https://oidc.{region}.amazonaws.com/token)
	endpoint := fmt.Sprintf("https://oidc.%s.amazonaws.com/token", tokenData.Region)
	if tokenData.Region == "" {
		endpoint = "https://oidc.us-east-1.amazonaws.com/token" // Fallback
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("OIDC refresh failed: %d, %s", resp.StatusCode, string(bodyBytes))
	}

	var refreshResp RefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return "", 0, err
	}

	// 참고: 엄밀히 말하면 갱신된 토큰을 다시 SQLite에 UPDATE 해주는 것이 좋으나,
	// 메뉴바 앱의 읽기 전용(Read-only) 특성과 Race Condition 방지를 위해 메모리에서 1회성으로 사용합니다.
	// kiro-cli가 나중에 알아서 DB를 업데이트할 것입니다.
	return refreshResp.AccessToken, refreshResp.ExpiresIn, nil
}

func getValidAccessToken() (string, error) {
	// 1. 메모리 캐시 확인 (만료 1분 전까지 안전하게 사용)
	tokenMutex.Lock()
	if cachedToken != "" && time.Now().Add(1*time.Minute).Before(tokenExpiryTime) {
		token := cachedToken
		tokenMutex.Unlock()
		return token, nil
	}
	tokenMutex.Unlock()

	// (캐시가 없거나 만료되었다면 SQLite DB 접근 시작)
	dbPath := expandPath("~/Library/Application Support/kiro-cli/data.sqlite3")

	tokenVal, err := getDBValue(dbPath, "kirocli:odic:token")
	if err != nil {
		return "", err
	}

	var t TokenData
	if err := json.Unmarshal([]byte(tokenVal), &t); err != nil {
		return "", err
	}

	// 2. DB에서 읽어온 토큰의 만료 시간 확인
	expiresAt, err := time.Parse(time.RFC3339, t.ExpiresAt)
	if err == nil && expiresAt.After(time.Now().Add(1*time.Minute)) {
		// 🌟 메모리에 캐시 저장 후 반환
		tokenMutex.Lock()
		cachedToken = t.AccessToken
		tokenExpiryTime = expiresAt
		tokenMutex.Unlock()
		return t.AccessToken, nil
	}

	// 3. 만료되었다면 Refresh 시도
	clientVal, err := getDBValue(dbPath, "kirocli:odic:device-registration")
	if err != nil {
		return "", fmt.Errorf("token expired, but failed to load client info: %w", err)
	}

	var c ClientData
	if err := json.Unmarshal([]byte(clientVal), &c); err != nil {
		return "", err
	}

	// Refresh 실행 (refreshOIDCToken 함수는 기존과 동일하게 유지하되, 리턴값을 캐싱합니다)
	newToken, expiresIn, err := refreshOIDCToken(t, c) // 반환값 수정 필요 (아래 참고)
	if err != nil {
		return "", err
	}

	// 🌟 Refresh된 새 토큰을 메모리에 캐시 저장
	tokenMutex.Lock()
	cachedToken = newToken
	tokenExpiryTime = time.Now().Add(time.Duration(expiresIn) * time.Second)
	tokenMutex.Unlock()

	return newToken, nil
}

func GetUsagePercentage() (float64, error) {
	usageMutex.Lock()         // 🌟 변수명 수정
	defer usageMutex.Unlock() // 🌟 변수명 수정

	if time.Now().Before(usageExpiry) { // 🌟 변수명 수정
		return cachedPercent, nil
	}

	percent, err := fetchUsageFromAPI()
	if err != nil {
		// 🌟 핵심 방어 로직 (Circuit Breaker 역할)
		// 실패하더라도 무한 재시도를 막기 위해 1분간의 쿨타임(Backoff)을 강제로 부여합니다.
		usageExpiry = time.Now().Add(1 * time.Minute)
		return 0, err
	}

	cachedPercent = percent
	usageExpiry = time.Now().Add(5 * time.Minute) // 🌟 변수명 수정

	return percent, nil
}

func fetchUsageFromAPI() (float64, error) {
	// 🌟 SQLite에서 활성 토큰 가져오기 (자동 갱신 포함)
	accessToken, err := getValidAccessToken()
	if err != nil {
		return 0, err
	}

	profileArn := "arn:aws:codewhisperer:us-east-1:940850250778:profile/9E3XN7U3UQRY"
	reqBody, _ := json.Marshal(map[string]string{"profileArn": profileArn})

	req, err := http.NewRequest("POST", "https://codewhisperer.us-east-1.amazonaws.com/", bytes.NewBuffer(reqBody))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("x-amz-target", "AmazonCodeWhispererService.GetUsageLimits")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "AWS-CLI/2.0 Python/3.9")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)

		// 🌟 자기 치유(Self-Healing) 로직: 401이나 403이 뜨면 메모리 캐시가 오염된 것으로 간주하고 즉시 폐기
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			tokenMutex.Lock()
			cachedToken = ""
			tokenExpiryTime = time.Time{} // 초기화
			tokenMutex.Unlock()
		}

		return 0, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyData, _ := io.ReadAll(resp.Body)

	slog.Debug("Usage API successful", "rawResponse", string(bodyData))

	var apiResp APIResponse
	if err := json.Unmarshal(bodyData, &apiResp); err != nil {
		return 0, err
	}

	if len(apiResp.UsageBreakdownList) == 0 {
		return 0, fmt.Errorf("empty usage breakdown list")
	}

	usage := apiResp.UsageBreakdownList[0]
	if usage.UsageLimit == 0 {
		return 0, nil
	}

	return (usage.CurrentUsage / usage.UsageLimit) * 100, nil
}

// GetIDEUsagePercentage는 Kiro IDE의 로컬 캐시 DB에서 사용량(%)을 읽어옵니다. (API 호출 없음)
func GetIDEUsagePercentage() (float64, error) {
	ideMutex.Lock()
	defer ideMutex.Unlock()

	if time.Now().Before(ideExpiry) {
		return idePercent, nil
	}

	dbPath := expandPath("~/Library/Application Support/Kiro/User/globalStorage/state.vscdb")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("IDE state DB not found: %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var raw string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "kiro.kiroAgent").Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("kiro.kiroAgent key not found in IDE DB")
		}
		return 0, err
	}

	// JSON 파싱: top-level key "kiro.resourceNotifications.usageState" → usageBreakdowns[0].percentageUsed
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return 0, fmt.Errorf("IDE JSON parse error: %w", err)
	}
	us, ok := top["kiro.resourceNotifications.usageState"]
	if !ok {
		return 0, fmt.Errorf("kiro.resourceNotifications.usageState not found in IDE JSON")
	}
	var usageState struct {
		UsageBreakdowns []struct {
			PercentageUsed float64 `json:"percentageUsed"`
		} `json:"usageBreakdowns"`
	}
	if err := json.Unmarshal(us, &usageState); err != nil {
		return 0, err
	}
	if len(usageState.UsageBreakdowns) == 0 {
		return 0, fmt.Errorf("usageBreakdowns is empty in IDE JSON")
	}

	pct := usageState.UsageBreakdowns[0].PercentageUsed
	idePercent = pct
	ideExpiry = time.Now().Add(5 * time.Minute)
	slog.Debug("IDE usage loaded", "percent", pct)
	return pct, nil
}
