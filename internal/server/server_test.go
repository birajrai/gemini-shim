package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/birajrai/gemini-shim/internal/server"
)

func TestIndexEndpoint(t *testing.T) {
	router := server.SetupRouter()

	req, _ := http.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if res["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", res["status"])
	}
	if res["version"] != server.Version {
		t.Errorf("expected version %s, got %v", server.Version, res["version"])
	}
	modelsList, ok := res["models"].([]any)
	if !ok || len(modelsList) == 0 {
		t.Errorf("expected non-empty models array, got %v", res["models"])
	}
}

func TestModelsEndpoints(t *testing.T) {
	router := server.SetupRouter()

	// OpenAI models endpoint
	req1, _ := http.NewRequest("GET", "/v1/models", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w1.Code)
	}

	var res1 map[string]any
	if err := json.Unmarshal(w1.Body.Bytes(), &res1); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if res1["object"] != "list" {
		t.Errorf("expected object 'list', got %v", res1["object"])
	}

	// Google Gemini models endpoint
	req2, _ := http.NewRequest("GET", "/v1beta/models", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w2.Code)
	}

	var res2 map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &res2); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if _, ok := res2["models"].([]any); !ok {
		t.Errorf("expected models array in Google models response, got %v", res2)
	}
}

func TestCORS(t *testing.T) {
	router := server.SetupRouter()

	req, _ := http.NewRequest("OPTIONS", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", w.Code)
	}
	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got %q", origin)
	}
}

func TestAuthMiddleware(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.APIKeys = []string{"secret-token-123"}
	config.Set(cfg)
	defer func() {
		cfg.APIKeys = nil
		config.Set(cfg)
	}()

	router := server.SetupRouter()

	// 1. Unauthorized request
	req1, _ := http.NewRequest("GET", "/v1/models", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for missing auth, got %d", w1.Code)
	}

	// 2. Bearer token
	req2, _ := http.NewRequest("GET", "/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer secret-token-123")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200 with Bearer token, got %d", w2.Code)
	}

	// 3. x-api-key header
	req3, _ := http.NewRequest("GET", "/v1/models", nil)
	req3.Header.Set("x-api-key", "secret-token-123")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected status 200 with x-api-key, got %d", w3.Code)
	}

	// 4. x-goog-api-key header
	req4, _ := http.NewRequest("GET", "/v1/models", nil)
	req4.Header.Set("x-goog-api-key", "secret-token-123")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Errorf("expected status 200 with x-goog-api-key, got %d", w4.Code)
	}

	// 5. Query param ?key=
	req5, _ := http.NewRequest("GET", "/v1/models?key=secret-token-123", nil)
	w5 := httptest.NewRecorder()
	router.ServeHTTP(w5, req5)
	if w5.Code != http.StatusOK {
		t.Errorf("expected status 200 with ?key= param, got %d", w5.Code)
	}

	// 6. Invalid token
	req6, _ := http.NewRequest("GET", "/v1/models", nil)
	req6.Header.Set("Authorization", "Bearer wrong-key")
	w6 := httptest.NewRecorder()
	router.ServeHTTP(w6, req6)
	if w6.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 with wrong key, got %d", w6.Code)
	}
}

func TestChatCompletionsValidation(t *testing.T) {
	router := server.SetupRouter()

	// Empty prompt
	body := []byte(`{"messages": []}`)
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty prompt, got %d", w.Code)
	}

	// Invalid model think level
	body2 := []byte(`{"model": "gemini-3.7-flash@think=invalid", "messages": [{"role": "user", "content": "hi"}]}`)
	req2, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid think level, got %d", w2.Code)
	}
}
