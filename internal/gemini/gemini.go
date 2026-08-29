package gemini

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/google/uuid"
)

var (
	bardErrorRegex = regexp.MustCompile(`BardErrorInfo\s*\[(\d+)\]`)
	codeBlockRegex = regexp.MustCompile(`(?s)` + "```" + `(?:python|javascript|text)\?code_(?:reference|stdout)&code_event_index=\d+\n.*?` + "```" + `\n?`)
	cardRegex      = regexp.MustCompile(`http://googleusercontent\.com/card_content/\d+\n?`)
)

func newHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}

	cfg := config.Get()
	if cfg != nil && cfg.Proxy != "" {
		if proxyURL, err := url.Parse(cfg.Proxy); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// BuildPayload constructs the Gemini StreamGenerate 102-element inner RPC payload.
func BuildPayload(prompt string, modelID int, thinkMode int, fileRefs []string, extraFields map[int]any) (string, error) {
	cfg := config.Get()
	inner := make([]any, 102)

	if len(fileRefs) > 0 {
		var refs [][]any
		for _, ref := range fileRefs {
			refs = append(refs, []any{nil, nil, ref})
		}
		inner[0] = []any{prompt, 0, nil, refs, nil, nil, 0}
	} else {
		inner[0] = []any{prompt, 0, nil, nil, nil, nil, 0}
	}

	inner[1] = []any{"en"}
	inner[2] = []any{"", "", "", nil, nil, nil, nil, nil, nil, ""}
	inner[6] = []any{0}
	inner[7] = 1
	inner[10] = 1
	inner[11] = 0
	inner[17] = []any{[]any{thinkMode}}
	inner[18] = 0
	inner[27] = 1
	inner[30] = []any{4}

	if cfg != nil && cfg.TemporaryChats {
		inner[41] = []any{1}
		inner[45] = 1
	} else {
		inner[41] = []any{2}
	}

	inner[53] = 0
	inner[59] = uuid.New().String()
	inner[61] = []any{}
	inner[68] = 1
	inner[79] = modelID

	for k, v := range extraFields {
		if k >= 0 && k < len(inner) {
			inner[k] = v
		}
	}

	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inner payload: %w", err)
	}

	outer := []any{nil, string(innerJSON)}
	outerJSON, err := json.Marshal(outer)
	if err != nil {
		return "", fmt.Errorf("failed to marshal outer payload: %w", err)
	}

	params := url.Values{}
	params.Set("f.req", string(outerJSON))
	if cfg != nil && cfg.XSRFToken != nil && *cfg.XSRFToken != "" {
		params.Set("at", *cfg.XSRFToken)
	}

	return params.Encode(), nil
}

var (
	dynamicBLMutex sync.RWMutex
	dynamicBL      = "boq_assistant-bard-web-server_20260827.05_p0"
	blRegex        = regexp.MustCompile(`"cfb2h":"(boq_assistant-bard-web-server_[^"]+)"`)
)

// FetchLatestGeminiBL queries Gemini Web to fetch the active build label dynamically.
func FetchLatestGeminiBL() string {
	client := newHTTPClient(10 * time.Second)
	req, err := http.NewRequest("GET", "https://gemini.google.com/app", nil)
	if err != nil {
		dynamicBLMutex.RLock()
		defer dynamicBLMutex.RUnlock()
		return dynamicBL
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if m := blRegex.FindStringSubmatch(string(body)); len(m) > 1 {
			dynamicBLMutex.Lock()
			dynamicBL = m[1]
			dynamicBLMutex.Unlock()
			config.Log("Auto-refreshed Gemini build label: %s", m[1])
			return m[1]
		}
	}
	dynamicBLMutex.RLock()
	defer dynamicBLMutex.RUnlock()
	return dynamicBL
}

func getURL() string {
	cfg := config.Get()
	reqID := (time.Now().Unix() + int64(rand.Intn(1000))) % 1000000
	accountPrefix := AccountPrefix()
	bl := dynamicBL
	if cfg != nil && cfg.GeminiBL != "" {
		bl = cfg.GeminiBL
	}

	return fmt.Sprintf(
		"https://gemini.google.com%s/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate?bl=%s&hl=en&_reqid=%d&rt=c",
		accountPrefix, bl, reqID,
	)
}

func buildHeaders() http.Header {
	cfg := config.Get()
	accountPrefix := AccountPrefix()
	h := make(http.Header)
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Set("Origin", "https://gemini.google.com")
	h.Set("Referer", fmt.Sprintf("https://gemini.google.com%s/app", accountPrefix))
	h.Set("X-Same-Domain", "1")
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	if cfg != nil && cfg.AuthUser != nil && *cfg.AuthUser != "" {
		h.Set("X-Goog-AuthUser", *cfg.AuthUser)
	}

	cookieStr, sapisid := LoadCookie()
	if cookieStr != "" {
		h.Set("Cookie", cookieStr)
	}
	if sapisid != "" {
		h.Set("Authorization", MakeSAPISIDHash(sapisid))
	}

	return h
}

// CleanText removes internal code snippets and card URL references from output text.
func CleanText(text string, strip bool) string {
	text = codeBlockRegex.ReplaceAllString(text, "")
	text = cardRegex.ReplaceAllString(text, "")
	if strip {
		return strings.TrimSpace(text)
	}
	return text
}

func extractTextsFromLine(line string) []string {
	if !strings.Contains(line, "\"wrb.fr\"") || len(line) < 200 {
		return nil
	}

	var arr []any
	if err := json.Unmarshal([]byte(line), &arr); err != nil || len(arr) == 0 {
		return nil
	}

	first, ok := arr[0].([]any)
	if !ok || len(first) < 3 {
		return nil
	}

	innerStr, ok := first[2].(string)
	if !ok || len(innerStr) < 50 {
		return nil
	}

	var inner []any
	if err := json.Unmarshal([]byte(innerStr), &inner); err != nil || len(inner) <= 4 {
		return nil
	}

	partsList, ok := inner[4].([]any)
	if !ok {
		return nil
	}

	var texts []string
	for _, part := range partsList {
		if partList, ok := part.([]any); ok && len(partList) > 1 {
			if subList, ok := partList[1].([]any); ok {
				for _, item := range subList {
					if t, ok := item.(string); ok && t != "" {
						texts = append(texts, t)
					}
				}
			}
		}
	}

	return texts
}

// ExtractResponseText finds the final complete text from the upstream raw response string.
func ExtractResponseText(raw string) (string, error) {
	if m := bardErrorRegex.FindStringSubmatch(raw); len(m) > 1 {
		return "", fmt.Errorf("Gemini upstream rejected request: BardErrorInfo [%s]", m[1])
	}

	var lastText string
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		for _, t := range extractTextsFromLine(line) {
			if len(t) > len(lastText) {
				lastText = t
			}
		}
	}

	return CleanText(lastText, true), nil
}

// Generate makes a non-streaming call to Gemini's StreamGenerate endpoint with automatic retries.
func Generate(prompt string, modelID int, thinkMode int, fileRefs []string, extraFields map[int]any) (string, error) {
	cfg := config.Get()
	bodyStr, err := BuildPayload(prompt, modelID, thinkMode, fileRefs, extraFields)
	if err != nil {
		return "", err
	}

	urlStr := getURL()
	headers := buildHeaders()
	timeout := time.Duration(cfg.RequestTimeoutSec) * time.Second
	client := newHTTPClient(timeout)

	var lastErr error
	for attempt := 0; attempt < cfg.RetryAttempts; attempt++ {
		req, err := http.NewRequest("POST", urlStr, bytes.NewBufferString(bodyStr))
		if err != nil {
			return "", err
		}
		req.Header = headers.Clone()

		resp, err := client.Do(req)
		if err == nil {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil {
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					text, parseErr := ExtractResponseText(string(bodyBytes))
					if parseErr == nil {
						return text, nil
					}
					lastErr = parseErr
				} else {
					if resp.StatusCode == 405 {
						FetchLatestGeminiBL()
						urlStr = getURL()
					}
					lastErr = fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, string(bodyBytes))
				}
			} else {
				lastErr = readErr
			}
		} else {
			lastErr = err
		}

		if attempt < cfg.RetryAttempts-1 {
			config.Log("Retry %d/%d: %v", attempt+1, cfg.RetryAttempts, lastErr)
			time.Sleep(time.Duration(cfg.RetryDelaySec) * time.Second)
		}
	}

	return "", lastErr
}

// GenerateStream streams delta response tokens through a channel.
func GenerateStream(ctx context.Context, prompt string, modelID int, thinkMode int, fileRefs []string, extraFields map[int]any, chunkChan chan<- string) error {
	cfg := config.Get()
	bodyStr, err := BuildPayload(prompt, modelID, thinkMode, fileRefs, extraFields)
	if err != nil {
		return err
	}

	urlStr := getURL()
	headers := buildHeaders()
	// Create client with zero timeout on HTTP client so stream can stay open for long responses
	client := newHTTPClient(0)

	var lastErr error
	emittedRawText := ""

	for attempt := 0; attempt < cfg.RetryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", urlStr, bytes.NewBufferString(bodyStr))
		if err != nil {
			return err
		}
		req.Header = headers.Clone()

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			reader := bufio.NewReader(resp.Body)
			var buf bytes.Buffer

			streamDone := false
			for {
				select {
				case <-ctx.Done():
					resp.Body.Close()
					return ctx.Err()
				default:
				}

				chunk := make([]byte, 4096)
				n, readErr := reader.Read(chunk)
				if n > 0 {
					buf.Write(chunk[:n])
					bufStr := buf.String()

					if bardErrorRegex.MatchString(bufStr) {
						m := bardErrorRegex.FindStringSubmatch(bufStr)
						resp.Body.Close()
						lastErr = fmt.Errorf("Gemini upstream rejected request: BardErrorInfo [%s]", m[1])
						break
					}

					for {
						idx := bytes.IndexByte(buf.Bytes(), '\n')
						if idx == -1 {
							break
						}
						lineBytes := buf.Next(idx + 1)
						line := strings.TrimRight(string(lineBytes), "\r\n")

						for _, t := range extractTextsFromLine(line) {
							if t == emittedRawText || strings.HasPrefix(emittedRawText, t) {
								continue
							}
							if !strings.HasPrefix(t, emittedRawText) {
								// upstream replaced whole string (retry or alternative path)
								delta := CleanText(t, false)
								emittedRawText = t
								if delta != "" {
									chunkChan <- delta
								}
								continue
							}
							delta := CleanText(t[len(emittedRawText):], false)
							emittedRawText = t
							if delta != "" {
								chunkChan <- delta
							}
						}
					}
				}

				if readErr != nil {
					resp.Body.Close()
					if readErr == io.EOF {
						streamDone = true
					} else {
						lastErr = readErr
					}
					break
				}
			}

			if streamDone {
				return nil
			}
		} else {
			if resp != nil {
				if resp.StatusCode == 405 {
					FetchLatestGeminiBL()
					urlStr = getURL()
				}
				resp.Body.Close()
				lastErr = fmt.Errorf("upstream returned status %d", resp.StatusCode)
			} else {
				lastErr = err
			}
		}

		if attempt < cfg.RetryAttempts-1 {
			config.Log("Stream retry %d/%d: %v", attempt+1, cfg.RetryAttempts, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(cfg.RetryDelaySec) * time.Second):
			}
		}
	}

	return lastErr
}
