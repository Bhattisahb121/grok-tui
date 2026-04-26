package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/Bhattisahb121/grok-tui/internal/config"
)

const (
	baseURL            = "https://grok.com"
	newConversationURL = baseURL + "/rest/app-chat/conversations/new"
	continueURL        = baseURL + "/rest/app-chat/conversations/%s/responses"
)

// ChallengeType represents the type of challenge encountered.
type ChallengeType int

const (
	ChallengeNone       ChallengeType = iota
	ChallengeCloudflare               // Cloudflare Turnstile / JS challenge
	ChallengeMFA                      // Multi-factor authentication required
	ChallengeLogin                    // Login / session expired
	ChallengeRateLimit                // Rate limited
	ChallengeUnknown                  // Unknown challenge
)

// Challenge holds info about an authentication/security challenge.
type Challenge struct {
	Type        ChallengeType
	Title       string
	Description string
	URL         string // URL to visit if manual intervention needed
	RawBody     string // Raw response body for debugging
	StatusCode  int
}

// ConversationPayload is the request body sent to Grok web.
type ConversationPayload struct {
	Temporary             bool              `json:"temporary"`
	ModelName             string            `json:"modelName"`
	Message               string            `json:"message"`
	FileAttachments       []interface{}     `json:"fileAttachments"`
	ImageAttachments      []interface{}     `json:"imageAttachments"`
	DisableSearch         bool              `json:"disableSearch"`
	EnableImageGeneration bool              `json:"enableImageGeneration"`
	ReturnImageBytes      bool              `json:"returnImageBytes"`
	ReturnRawGrokInXaiReq bool              `json:"returnRawGrokInXaiRequest"`
	EnableImageStreaming   bool              `json:"enableImageStreaming"`
	ImageGenerationCount  int               `json:"imageGenerationCount"`
	ForceConcise          bool              `json:"forceConcise"`
	ToolOverrides         map[string]string `json:"toolOverrides"`
	EnableSideBySide      bool              `json:"enableSideBySide"`
	IsPreset              bool              `json:"isPreset"`
	SendFinalMetadata     bool              `json:"sendFinalMetadata"`
	CustomInstructions    string            `json:"customInstructions"`
	DeepsearchPreset      string            `json:"deepsearchPreset"`
	IsReasoning           bool              `json:"isReasoning"`
	WebpageUrls           []string          `json:"webpageUrls"`
	DisableTextFollowUps  bool              `json:"disableTextFollowUps"`
	DisableMemory         bool              `json:"disableMemory"`
	ForceSideBySide       bool              `json:"forceSideBySide"`
	IsAsyncChat           bool              `json:"isAsyncChat"`
}

// StreamToken represents a single streamed token from the response.
type StreamToken struct {
	Token           string
	IsModelResponse bool
	ConversationID  string
	Error           string
	Challenge       *Challenge // non-nil if a challenge was encountered
}

// GrokClient handles communication with the Grok web API.
type GrokClient struct {
	httpClient     *http.Client
	cookie         string
	statsigID      string
	model          string
	conversationID string
}

// New creates a new GrokClient from config.
func New(cfg *config.Config) *GrokClient {
	jar, _ := cookiejar.New(nil)

	// Pre-populate jar with cookies from config
	if cfg.Cookie != "" {
		parseCookiesToJar(jar, baseURL, cfg.Cookie)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			},
			CurvePreferences: []tls.CurveID{
				tls.X25519,
				tls.CurveP256,
				tls.CurveP384,
			},
		},
		ForceAttemptHTTP2: true,
	}

	return &GrokClient{
		httpClient: &http.Client{
			Transport: transport,
			Jar:       jar,
			Timeout:   5 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Allow redirects but cap at 10
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		cookie:    cfg.Cookie,
		statsigID: cfg.StatsigID,
		model:     cfg.Model,
	}
}

func parseCookiesToJar(jar *cookiejar.Jar, rawURL string, cookieStr string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	var cookies []*http.Cookie
	pairs := strings.Split(cookieStr, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eqIdx := strings.Index(pair, "=")
		if eqIdx < 0 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(pair[:eqIdx]),
			Value: strings.TrimSpace(pair[eqIdx+1:]),
		})
	}
	jar.SetCookies(u, cookies)
}

// ConversationID returns the current conversation ID.
func (c *GrokClient) ConversationID() string {
	return c.conversationID
}

// ResetConversation starts a new conversation.
func (c *GrokClient) ResetConversation() {
	c.conversationID = ""
}

// SetModel changes the active model.
func (c *GrokClient) SetModel(model string) {
	c.model = model
}

// GetModel returns the current model name.
func (c *GrokClient) GetModel() string {
	return c.model
}

// UpdateCookie updates the cookie string and repopulates the jar.
func (c *GrokClient) UpdateCookie(cookie string) {
	c.cookie = cookie
	if jar, ok := c.httpClient.Jar.(*cookiejar.Jar); ok {
		parseCookiesToJar(jar, baseURL, cookie)
	}
}

func (c *GrokClient) buildHeaders() http.Header {
	h := http.Header{}
	h.Set("Accept", "*/*")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Content-Type", "application/json")
	h.Set("Origin", baseURL)
	h.Set("Referer", baseURL+"/")
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	h.Set("Sec-Ch-Ua", `"Chromium";v="131", "Not_A Brand";v="24"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Cookie", c.cookie)
	if c.statsigID != "" {
		h.Set("X-Statsig-Id", c.statsigID)
	}
	return h
}

func (c *GrokClient) buildPayload(message string) *ConversationPayload {
	return &ConversationPayload{
		Temporary:             false,
		ModelName:             c.model,
		Message:               message,
		FileAttachments:       []interface{}{},
		ImageAttachments:      []interface{}{},
		DisableSearch:         false,
		EnableImageGeneration: true,
		ReturnImageBytes:      false,
		ReturnRawGrokInXaiReq: false,
		EnableImageStreaming:   true,
		ImageGenerationCount:  2,
		ForceConcise:          false,
		ToolOverrides:         map[string]string{},
		EnableSideBySide:      true,
		IsPreset:              false,
		SendFinalMetadata:     true,
		CustomInstructions:    "",
		DeepsearchPreset:      "",
		IsReasoning:           false,
		WebpageUrls:           []string{},
		DisableTextFollowUps:  false,
		DisableMemory:         false,
		ForceSideBySide:       false,
		IsAsyncChat:           false,
	}
}

// detectChallenge inspects an HTTP response to determine if a challenge is present.
func detectChallenge(resp *http.Response, body []byte) *Challenge {
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	// Cloudflare challenge detection
	cfRay := resp.Header.Get("Cf-Ray")
	cfServer := strings.ToLower(resp.Header.Get("Server"))

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		if cfRay != "" || cfServer == "cloudflare" ||
			strings.Contains(bodyLower, "cloudflare") ||
			strings.Contains(bodyLower, "cf-challenge") ||
			strings.Contains(bodyLower, "turnstile") ||
			strings.Contains(bodyLower, "just a moment") ||
			strings.Contains(bodyLower, "challenge-platform") {
			return &Challenge{
				Type:  ChallengeCloudflare,
				Title: "⚠ Cloudflare Challenge",
				Description: "Cloudflare 보안 검증이 필요합니다.\n\n" +
					"해결 방법:\n" +
					"  1. 브라우저에서 https://grok.com 을 열어 챌린지를 통과하세요\n" +
					"  2. 통과 후 새 Cookie 값을 복사하세요\n" +
					"  3. /cookie <새 쿠키 값> 명령으로 업데이트하세요",
				URL:        "https://grok.com",
				RawBody:    truncateBody(bodyStr, 500),
				StatusCode: resp.StatusCode,
			}
		}
	}

	// Login / session expired
	if resp.StatusCode == 401 ||
		(resp.StatusCode == 403 && (strings.Contains(bodyLower, "unauthorized") ||
			strings.Contains(bodyLower, "unauthenticated") ||
			strings.Contains(bodyLower, "login") ||
			strings.Contains(bodyLower, "sign in"))) {
		return &Challenge{
			Type:  ChallengeLogin,
			Title: "⚠ 로그인 필요 / 세션 만료",
			Description: "로그인이 필요하거나 세션이 만료되었습니다.\n\n" +
				"해결 방법:\n" +
				"  1. 브라우저에서 https://grok.com 에 로그인하세요\n" +
				"  2. DevTools (F12) → Network 탭에서 새 Cookie를 복사하세요\n" +
				"  3. /cookie <새 쿠키 값> 명령으로 업데이트하세요",
			URL:        "https://grok.com",
			RawBody:    truncateBody(bodyStr, 500),
			StatusCode: resp.StatusCode,
		}
	}

	// MFA required
	if strings.Contains(bodyLower, "two-factor") ||
		strings.Contains(bodyLower, "2fa") ||
		strings.Contains(bodyLower, "mfa") ||
		strings.Contains(bodyLower, "verification code") ||
		strings.Contains(bodyLower, "multi-factor") ||
		strings.Contains(bodyLower, "authenticat") {
		if resp.StatusCode != 200 || strings.Contains(bodyLower, "required") {
			return &Challenge{
				Type:  ChallengeMFA,
				Title: "⚠ MFA / 2단계 인증 필요",
				Description: "2단계 인증(MFA)이 필요합니다.\n\n" +
					"해결 방법:\n" +
					"  1. 브라우저에서 https://grok.com 에 접속하세요\n" +
					"  2. MFA 인증을 완료하세요\n" +
					"  3. 인증 후 새 Cookie를 복사하세요\n" +
					"  4. /cookie <새 쿠키 값> 명령으로 업데이트하세요",
				URL:        "https://grok.com",
				RawBody:    truncateBody(bodyStr, 500),
				StatusCode: resp.StatusCode,
			}
		}
	}

	// Rate limiting
	if resp.StatusCode == 429 {
		retryAfter := resp.Header.Get("Retry-After")
		desc := "요청이 너무 많습니다. 잠시 후 다시 시도하세요."
		if retryAfter != "" {
			desc += fmt.Sprintf("\n\nRetry-After: %s초", retryAfter)
		}
		return &Challenge{
			Type:        ChallengeRateLimit,
			Title:       "⚠ 요청 제한 (Rate Limit)",
			Description: desc,
			StatusCode:  resp.StatusCode,
		}
	}

	// Generic non-200 error
	if resp.StatusCode != 200 {
		return &Challenge{
			Type:        ChallengeUnknown,
			Title:       fmt.Sprintf("⚠ HTTP %d 오류", resp.StatusCode),
			Description: fmt.Sprintf("예상치 못한 응답입니다 (HTTP %d).\n\n응답 내용:\n%s", resp.StatusCode, truncateBody(bodyStr, 300)),
			RawBody:     truncateBody(bodyStr, 500),
			StatusCode:  resp.StatusCode,
		}
	}

	return nil
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// SendMessage sends a message and streams the response via a channel.
func (c *GrokClient) SendMessage(ctx context.Context, message string) (<-chan StreamToken, error) {
	payload := c.buildPayload(message)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	apiURL := newConversationURL
	if c.conversationID != "" {
		apiURL = fmt.Sprintf(continueURL, c.conversationID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = c.buildHeaders()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check for challenges before streaming
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		challenge := detectChallenge(resp, respBody)
		if challenge != nil {
			ch := make(chan StreamToken, 1)
			ch <- StreamToken{Challenge: challenge}
			close(ch)
			return ch, nil
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamToken, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()
		c.parseStream(resp.Body, ch)
	}()

	return ch, nil
}

func (c *GrokClient) parseStream(body io.Reader, ch chan<- StreamToken) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		result, ok := data["result"].(map[string]interface{})
		if !ok {
			continue
		}

		// Extract conversation ID
		if convID, ok := result["conversationId"].(string); ok && convID != "" {
			c.conversationID = convID
		}

		response, ok := result["response"].(map[string]interface{})
		if !ok {
			continue
		}

		// Check for final model response
		if modelResp, ok := response["modelResponse"].(map[string]interface{}); ok {
			if msg, ok := modelResp["message"].(string); ok {
				ch <- StreamToken{
					Token:           msg,
					IsModelResponse: true,
					ConversationID:  c.conversationID,
				}
			}
			continue
		}

		// Stream tokens
		if token, ok := response["token"].(string); ok && token != "" {
			ch <- StreamToken{
				Token:          token,
				ConversationID: c.conversationID,
			}
		}
	}
}

// CheckConnection performs a quick connectivity check and returns any challenge.
func (c *GrokClient) CheckConnection(ctx context.Context) *Challenge {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return &Challenge{
			Type:        ChallengeUnknown,
			Title:       "⚠ 연결 오류",
			Description: fmt.Sprintf("요청 생성 실패: %s", err),
		}
	}
	req.Header = c.buildHeaders()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Challenge{
			Type:        ChallengeUnknown,
			Title:       "⚠ 연결 실패",
			Description: fmt.Sprintf("grok.com에 연결할 수 없습니다: %s", err),
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	return detectChallenge(resp, respBody)
}
