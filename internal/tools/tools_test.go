package tools_test

import (
	"strings"
	"testing"

	"github.com/birajrai/gemini-shim/internal/tools"
)

func TestMessagesToPrompt(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "You are a helpful assistant."},
		{"role": "user", "content": "What is the weather in Tokyo?"},
		{"role": "assistant", "content": "Checking..."},
		{"role": "tool", "name": "get_weather", "content": "{\"temp\": 20}"},
	}

	toolDefs := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get current weather",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	prompt, _ := tools.MessagesToPrompt(messages, toolDefs, "auto")

	if !strings.Contains(prompt, "# Tool Use") {
		t.Errorf("expected tool prompt to contain '# Tool Use'")
	}
	if !strings.Contains(prompt, "get_weather") {
		t.Errorf("expected tool prompt to contain 'get_weather'")
	}
	if !strings.Contains(prompt, "[System instruction]: You are a helpful assistant.") {
		t.Errorf("expected system instruction formatting")
	}
	if !strings.Contains(prompt, "[Assistant]: Checking...") {
		t.Errorf("expected assistant formatting")
	}
	if !strings.Contains(prompt, "[Tool result for get_weather]: {\"temp\": 20}") {
		t.Errorf("expected tool result formatting")
	}
}

func TestParseToolCalls(t *testing.T) {
	raw := "Let me check the weather for you.\n```tool_call\n{\"name\": \"get_weather\", \"arguments\": {\"location\": \"Tokyo\"}}\n```\nDone."
	clean, toolCalls := tools.ParseToolCalls(raw)

	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", toolCalls[0].Function.Name)
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "Tokyo") {
		t.Errorf("expected arguments to contain 'Tokyo', got %s", toolCalls[0].Function.Arguments)
	}
	if strings.Contains(clean, "```tool_call") {
		t.Errorf("expected clean text to not contain tool_call block: %q", clean)
	}
}

func TestGoogleContentsToPromptAndParse(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Search for Go 1.27 release notes"},
				},
			},
		},
		"tools": []any{
			map[string]any{
				"functionDeclarations": []any{
					map[string]any{
						"name":        "search",
						"description": "Web search tool",
					},
				},
			},
		},
	}

	prompt, _ := tools.GoogleContentsToPrompt(req)
	if !strings.Contains(prompt, "search") || !strings.Contains(prompt, "```function_call") {
		t.Errorf("expected Google tool prompt to contain search function declaration")
	}

	modelResponse := "```function_call\n{\"name\": \"search\", \"args\": {\"query\": \"Go 1.27\"}}\n```"
	clean, fnCalls := tools.ParseGoogleFunctionCalls(modelResponse)

	if len(fnCalls) != 1 {
		t.Fatalf("expected 1 function call, got %d", len(fnCalls))
	}
	if fnCalls[0].Name != "search" {
		t.Errorf("expected function name search, got %s", fnCalls[0].Name)
	}
	if fnCalls[0].Args["query"] != "Go 1.27" {
		t.Errorf("expected query 'Go 1.27', got %v", fnCalls[0].Args["query"])
	}
	if clean != "" {
		t.Errorf("expected clean text to be empty, got %q", clean)
	}
}
