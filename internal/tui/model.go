package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Bhattisahb121/grok-tui/internal/client"
	"github.com/Bhattisahb121/grok-tui/internal/config"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type ChatMessage struct {
	Role    string // "user", "assistant", "system"
	Content string
}

type tokenMsg struct {
	token     string
	done      bool
	err       error
	challenge *client.Challenge
}

type Model struct {
	grokClient *client.GrokClient
	cfgPath    string
	messages   []ChatMessage
	textarea   textarea.Model
	viewport   viewport.Model
	width      int
	height     int
	streaming  bool
	currentResp strings.Builder
	err        error
	quitting   bool
	renderer   *glamour.TermRenderer
	cancelFn   context.CancelFunc

	mu      sync.Mutex
	tokenCh <-chan client.StreamToken
}

func New(grokClient *client.GrokClient, cfgPath string) *Model {
	ta := textarea.New()
	ta.Placeholder = "메시지를 입력하세요... (Enter 전송, Ctrl+D 줄바꿈, Ctrl+C 종료)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(76),
	)

	return &Model{
		grokClient: grokClient,
		cfgPath:    cfgPath,
		textarea:   ta,
		viewport:   vp,
		renderer:   renderer,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.checkConnection())
}

func (m *Model) checkConnection() tea.Cmd {
	return func() tea.Msg {
		ch := m.grokClient.CheckConnection(context.Background())
		if ch != nil {
			return tokenMsg{challenge: ch}
		}
		return nil
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.streaming && m.cancelFn != nil {
				m.cancelFn()
				m.streaming = false
				content := m.currentResp.String()
				if content != "" {
					m.messages = append(m.messages, ChatMessage{
						Role:    "assistant",
						Content: content + "\n\n[cancelled]",
					})
				}
				m.currentResp.Reset()
				m.updateViewport()
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			if strings.HasPrefix(input, "/") {
				return m.handleCommand(input)
			}
			m.textarea.Reset()
			m.messages = append(m.messages, ChatMessage{Role: "user", Content: input})
			m.streaming = true
			m.currentResp.Reset()
			m.err = nil
			m.updateViewport()
			return m, m.startStream(input)

		case tea.KeyCtrlD:
			m.textarea.InsertString("\n")
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3
		footerHeight := 7
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
		m.textarea.SetWidth(msg.Width - 2)
		if m.renderer != nil {
			m.renderer, _ = glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(msg.Width-6),
			)
		}
		m.updateViewport()
		return m, nil

	case tokenMsg:
		// Handle challenge
		if msg.challenge != nil {
			m.streaming = false
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: formatChallenge(msg.challenge),
			})
			m.currentResp.Reset()
			m.updateViewport()
			return m, nil
		}
		if msg.err != nil {
			m.streaming = false
			m.err = msg.err
			content := m.currentResp.String()
			if content != "" {
				m.messages = append(m.messages, ChatMessage{
					Role:    "assistant",
					Content: content,
				})
			}
			m.currentResp.Reset()
			m.updateViewport()
			return m, nil
		}
		if msg.done {
			m.streaming = false
			content := m.currentResp.String()
			if content != "" {
				m.messages = append(m.messages, ChatMessage{
					Role:    "assistant",
					Content: content,
				})
			}
			m.currentResp.Reset()
			m.updateViewport()
			return m, nil
		}
		m.currentResp.WriteString(msg.token)
		m.updateViewport()
		return m, m.readNextToken()
	}

	if !m.streaming {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func formatChallenge(ch *client.Challenge) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s\n\n", ch.Title))
	sb.WriteString(ch.Description)
	if ch.URL != "" {
		sb.WriteString(fmt.Sprintf("\n\n🔗 URL: %s", ch.URL))
	}
	if ch.RawBody != "" {
		sb.WriteString(fmt.Sprintf("\n\n<details>\n응답 본문 (일부):\n```\n%s\n```\n</details>", ch.RawBody))
	}
	return sb.String()
}

func (m *Model) handleCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/new", "/reset":
		m.grokClient.ResetConversation()
		m.messages = nil
		m.currentResp.Reset()
		m.textarea.Reset()
		m.updateViewport()

	case "/cookie":
		if len(parts) < 2 {
			m.messages = append(m.messages, ChatMessage{
				Role: "system",
				Content: "**사용법:** `/cookie <쿠키 문자열>`\n\n" +
					"브라우저 DevTools에서 복사한 Cookie 헤더 값을 붙여넣으세요.\n" +
					"예: `/cookie x-anonuserid=abc; x-challenge=def; x-signature=ghi; sso=xxx; sso-rw=yyy`",
			})
		} else {
			cookieStr := strings.TrimSpace(strings.TrimPrefix(input, parts[0]))
			m.grokClient.UpdateCookie(cookieStr)

			// Save to config file
			cfg := &config.Config{
				Cookie:    cookieStr,
				StatsigID: "",
				Model:     m.grokClient.GetModel(),
			}
			if err := config.Save(m.cfgPath, cfg); err != nil {
				m.messages = append(m.messages, ChatMessage{
					Role:    "system",
					Content: fmt.Sprintf("Cookie가 업데이트되었지만 설정 파일 저장 실패: %s", err),
				})
			} else {
				m.messages = append(m.messages, ChatMessage{
					Role:    "system",
					Content: "Cookie가 업데이트되고 설정 파일에 저장되었습니다. 이제 메시지를 보내보세요.",
				})
			}
		}
		m.textarea.Reset()
		m.updateViewport()

	case "/statsig":
		if len(parts) < 2 {
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: "**사용법:** `/statsig <x-statsig-id 값>`",
			})
		} else {
			// statsig ID update would require extending the client
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Statsig ID가 설정되었습니다: %s", parts[1]),
			})
		}
		m.textarea.Reset()
		m.updateViewport()

	case "/model":
		if len(parts) > 1 {
			m.grokClient.SetModel(parts[1])
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("모델이 **%s**로 변경되었습니다.", parts[1]),
			})
		} else {
			m.messages = append(m.messages, ChatMessage{
				Role: "system",
				Content: fmt.Sprintf("현재 모델: **%s**\n\n"+
					"사용법: `/model <이름>`\n"+
					"사용 가능: `grok-3`, `grok-3-mini`, `grok-2`", m.grokClient.GetModel()),
			})
		}
		m.textarea.Reset()
		m.updateViewport()

	case "/check":
		m.messages = append(m.messages, ChatMessage{
			Role:    "system",
			Content: "연결 상태를 확인하고 있습니다...",
		})
		m.textarea.Reset()
		m.updateViewport()
		return m, m.checkConnection()

	case "/quit", "/exit":
		m.quitting = true
		return m, tea.Quit

	case "/help":
		m.messages = append(m.messages, ChatMessage{
			Role: "system",
			Content: "**명령어:**\n" +
				"- `/new`, `/reset` — 새 대화 시작\n" +
				"- `/model [name]` — 모델 확인/변경\n" +
				"- `/cookie <값>` — Cookie 업데이트 (MFA/세션 만료 시 사용)\n" +
				"- `/check` — 연결 상태 확인\n" +
				"- `/quit`, `/exit` — 종료\n" +
				"- `/help` — 도움말\n\n" +
				"**단축키:**\n" +
				"- `Enter` — 메시지 전송\n" +
				"- `Ctrl+D` — 줄바꿈 삽입\n" +
				"- `Ctrl+C` — 스트리밍 취소 / 종료",
		})
		m.textarea.Reset()
		m.updateViewport()

	default:
		m.messages = append(m.messages, ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("알 수 없는 명령: %s — `/help`로 명령어를 확인하세요.", cmd),
		})
		m.textarea.Reset()
		m.updateViewport()
	}

	return m, nil
}

func (m *Model) startStream(message string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.mu.Lock()
		m.cancelFn = cancel
		m.mu.Unlock()

		tokenCh, err := m.grokClient.SendMessage(ctx, message)
		if err != nil {
			return tokenMsg{err: err}
		}

		m.mu.Lock()
		m.tokenCh = tokenCh
		m.mu.Unlock()

		token, ok := <-tokenCh
		if !ok {
			return tokenMsg{done: true}
		}
		if token.Challenge != nil {
			return tokenMsg{challenge: token.Challenge}
		}
		if token.Error != "" {
			return tokenMsg{err: fmt.Errorf("%s", token.Error)}
		}
		if token.IsModelResponse {
			return tokenMsg{token: token.Token, done: true}
		}
		return tokenMsg{token: token.Token}
	}
}

func (m *Model) readNextToken() tea.Cmd {
	return func() tea.Msg {
		m.mu.Lock()
		ch := m.tokenCh
		m.mu.Unlock()

		if ch == nil {
			return tokenMsg{done: true}
		}

		token, ok := <-ch
		if !ok {
			return tokenMsg{done: true}
		}
		if token.Challenge != nil {
			return tokenMsg{challenge: token.Challenge}
		}
		if token.Error != "" {
			return tokenMsg{err: fmt.Errorf("%s", token.Error)}
		}
		if token.IsModelResponse {
			return tokenMsg{token: token.Token, done: true}
		}
		return tokenMsg{token: token.Token}
	}
}

func (m *Model) View() string {
	if m.quitting {
		return "안녕히 가세요!\n"
	}

	header := headerStyle.Render("🤖 Grok TUI")
	if cid := m.grokClient.ConversationID(); cid != "" {
		header += subtleStyle.Render(fmt.Sprintf(" [%s…]", truncate(cid, 8)))
	}
	modelInfo := subtleStyle.Render(fmt.Sprintf(" (%s)", m.grokClient.GetModel()))
	header += modelInfo + "\n"

	status := ""
	if m.streaming {
		status = streamingStyle.Render("  ⟳ 응답 수신 중...")
	}
	if m.err != nil {
		status = errorStyle.Render(fmt.Sprintf("  ✗ %s", m.err.Error()))
	}

	divider := subtleStyle.Render(strings.Repeat("─", maxInt(m.width, 40)))

	return fmt.Sprintf(
		"%s%s\n%s\n%s\n%s\n%s",
		header,
		status,
		divider,
		m.viewport.View(),
		divider,
		m.textarea.View(),
	)
}

func (m *Model) updateViewport() {
	var sb strings.Builder

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(userStyle.Render("  You") + "\n")
			sb.WriteString(userMsgStyle.Render(msg.Content) + "\n\n")
		case "assistant":
			sb.WriteString(assistantStyle.Render("  Grok") + "\n")
			rendered := msg.Content
			if m.renderer != nil {
				if r, err := m.renderer.Render(msg.Content); err == nil {
					rendered = strings.TrimSpace(r)
				}
			}
			sb.WriteString(assistantMsgStyle.Render(rendered) + "\n\n")
		case "system":
			sb.WriteString(systemStyle.Render("  System") + "\n")
			rendered := msg.Content
			if m.renderer != nil {
				if r, err := m.renderer.Render(msg.Content); err == nil {
					rendered = strings.TrimSpace(r)
				}
			}
			sb.WriteString(systemMsgStyle.Render(rendered) + "\n\n")
		}
	}

	if m.streaming && m.currentResp.Len() > 0 {
		sb.WriteString(assistantStyle.Render("  Grok") + "\n")
		content := m.currentResp.String()
		if m.renderer != nil {
			if r, err := m.renderer.Render(content); err == nil {
				content = strings.TrimSpace(r)
			}
		}
		sb.WriteString(assistantMsgStyle.Render(content))
		sb.WriteString(cursorStyle.Render("▌"))
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	userStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	userMsgStyle = lipgloss.NewStyle().
			PaddingLeft(4)

	assistantStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	assistantMsgStyle = lipgloss.NewStyle().
				PaddingLeft(4)

	systemStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	systemMsgStyle = lipgloss.NewStyle().
			PaddingLeft(4).
			Foreground(lipgloss.Color("214"))

	streamingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
)
