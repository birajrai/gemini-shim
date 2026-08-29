package gemini_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/birajrai/gemini-shim/internal/gemini"
)

func decodePayload(t *testing.T, payloadStr string) []any {
	vals, err := url.ParseQuery(payloadStr)
	if err != nil {
		t.Fatalf("failed to parse url query: %v", err)
	}

	freq := vals.Get("f.req")
	if freq == "" {
		t.Fatalf("missing f.req in payload")
	}

	var outer []any
	if err := json.Unmarshal([]byte(freq), &outer); err != nil {
		t.Fatalf("failed to unmarshal outer payload: %v", err)
	}

	innerStr, ok := outer[1].(string)
	if !ok {
		t.Fatalf("outer[1] is not string: %v", outer[1])
	}

	var inner []any
	if err := json.Unmarshal([]byte(innerStr), &inner); err != nil {
		t.Fatalf("failed to unmarshal inner payload: %v", err)
	}

	return inner
}

func TestBuildPayloadPersistence(t *testing.T) {
	cfg := config.DefaultConfig()
	config.Set(cfg)

	// Persistent chats
	cfg.TemporaryChats = false
	payload, err := gemini.BuildPayload("hello", 1, 4, nil, nil)
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	inner := decodePayload(t, payload)
	if p41, ok := inner[41].([]any); !ok || len(p41) != 1 || int(p41[0].(float64)) != 2 {
		t.Errorf("expected inner[41] to be [2], got %v", inner[41])
	}
	if inner[45] != nil {
		t.Errorf("expected inner[45] to be nil, got %v", inner[45])
	}

	// Temporary chats
	cfg.TemporaryChats = true
	payload, err = gemini.BuildPayload("hello", 1, 4, nil, nil)
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	inner = decodePayload(t, payload)
	if p41, ok := inner[41].([]any); !ok || len(p41) != 1 || int(p41[0].(float64)) != 1 {
		t.Errorf("expected inner[41] to be [1], got %v", inner[41])
	}
	if p45, ok := inner[45].(float64); !ok || int(p45) != 1 {
		t.Errorf("expected inner[45] to be 1, got %v", inner[45])
	}
}

func TestBuildPayloadFileRefs(t *testing.T) {
	cfg := config.DefaultConfig()
	config.Set(cfg)

	payload, err := gemini.BuildPayload("describe image", 1, 4, []string{"/blob/123"}, nil)
	if err != nil {
		t.Fatalf("BuildPayload error: %v", err)
	}
	inner := decodePayload(t, payload)

	inner0, ok := inner[0].([]any)
	if !ok || len(inner0) < 4 {
		t.Fatalf("unexpected inner[0]: %v", inner[0])
	}
	if inner0[0] != "describe image" {
		t.Errorf("expected prompt 'describe image', got %v", inner0[0])
	}
	refs, ok := inner0[3].([]any)
	if !ok || len(refs) != 1 {
		t.Fatalf("expected 1 file ref in inner[0][3], got %v", inner0[3])
	}
	ref0, ok := refs[0].([]any)
	if !ok || len(ref0) != 3 || ref0[2] != "/blob/123" {
		t.Errorf("expected ref [nil, nil, '/blob/123'], got %v", refs[0])
	}
}

func TestSAPISIDHash(t *testing.T) {
	hash := gemini.MakeSAPISIDHash("test-sapisid")
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if len(hash) < 20 || hash[:12] != "SAPISIDHASH " {
		t.Errorf("invalid SAPISIDHASH format: %s", hash)
	}
}

func TestCleanText(t *testing.T) {
	raw := "Hello world!\n```python?code_reference&code_event_index=1\nimport os\n```\nhttp://googleusercontent.com/card_content/123\nFinished."
	cleaned := gemini.CleanText(raw, true)
	expected := "Hello world!\nFinished."
	if cleaned != expected {
		t.Errorf("expected %q, got %q", expected, cleaned)
	}
}

func TestExtractResponseTextBardError(t *testing.T) {
	raw := `Some header data\nBardErrorInfo [102]\nSome other data`
	_, err := gemini.ExtractResponseText(raw)
	if err == nil {
		t.Errorf("expected error for BardErrorInfo, got nil")
	}
}
