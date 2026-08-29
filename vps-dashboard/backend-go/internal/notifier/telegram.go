package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"vps-dashboard-api/internal/models"
)

// telegramTimeout is the per-request HTTP timeout for the Bot API.
const telegramTimeout = 8 * time.Second

// TelegramSender posts messages to api.telegram.org via the Bot API. The
// embedded http.Client may be replaced for tests; an httptest.Server can
// be wired in by overriding APIBase.
type TelegramSender struct {
	HTTP    *http.Client
	APIBase string // default https://api.telegram.org
}

// NewTelegramSender returns a sender pre-configured with an 8s timeout.
func NewTelegramSender() *TelegramSender {
	return &TelegramSender{
		HTTP:    &http.Client{Timeout: telegramTimeout},
		APIBase: "https://api.telegram.org",
	}
}

// telegramRequest is the JSON body sent to /sendMessage.
type telegramRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// Send transmits a Message to the Telegram bot configured in ch.Config.
// A non-2xx response is wrapped together with the response body so
// callers can surface useful diagnostics.
func (t *TelegramSender) Send(ctx context.Context, ch *models.Channel, m Message) error {
	if ch == nil {
		return fmt.Errorf("telegram: nil channel")
	}
	token, _ := ch.Config["bot_token"].(string)
	chatID, _ := ch.Config["chat_id"].(string)
	parseMode, _ := ch.Config["parse_mode"].(string)
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("telegram: bot_token missing")
	}
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("telegram: chat_id missing")
	}
	if parseMode == "" {
		parseMode = "HTML"
	}

	body := telegramRequest{
		ChatID:    chatID,
		Text:      formatMessage(m, parseMode),
		ParseMode: parseMode,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("telegram: marshal body: %w", err)
	}

	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", strings.TrimRight(base, "/"), token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("telegram: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := t.HTTP
	if client == nil {
		client = &http.Client{Timeout: telegramTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		snippet := strings.TrimSpace(string(respBody))
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return fmt.Errorf("telegram: status %d: %s", resp.StatusCode, snippet)
	}
	return nil
}

// formatMessage renders the Message into a Telegram-friendly string.
// HTML mode escapes user input via html.EscapeString. Markdown mode is
// rendered verbatim with a leading severity tag.
func formatMessage(m Message, parseMode string) string {
	severity := strings.TrimSpace(m.Severity)
	if severity == "" {
		severity = "info"
	}

	var sb strings.Builder
	if parseMode == "Markdown" {
		if strings.TrimSpace(m.Title) != "" {
			sb.WriteString(fmt.Sprintf("*[%s] %s*\n", strings.ToUpper(severity), m.Title))
		} else {
			sb.WriteString(fmt.Sprintf("*[%s]*\n", strings.ToUpper(severity)))
		}
		if strings.TrimSpace(m.Text) != "" {
			sb.WriteString(m.Text)
			sb.WriteString("\n")
		}
		if strings.TrimSpace(m.ProjectID) != "" {
			sb.WriteString(fmt.Sprintf("project: `%s`\n", m.ProjectID))
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	// Default HTML.
	title := html.EscapeString(strings.TrimSpace(m.Title))
	text := html.EscapeString(strings.TrimSpace(m.Text))
	sev := html.EscapeString(strings.ToUpper(severity))

	if title != "" {
		sb.WriteString(fmt.Sprintf("<b>[%s] %s</b>\n", sev, title))
	} else {
		sb.WriteString(fmt.Sprintf("<b>[%s]</b>\n", sev))
	}
	if text != "" {
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	if strings.TrimSpace(m.ProjectID) != "" {
		sb.WriteString(fmt.Sprintf("project: <code>%s</code>\n", html.EscapeString(m.ProjectID)))
	}
	return strings.TrimRight(sb.String(), "\n")
}
