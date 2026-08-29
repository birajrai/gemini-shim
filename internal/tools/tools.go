package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ImageInput represents an image supplied in chat messages.
type ImageInput struct {
	Data []byte
	MIME string
	URL  string
}

// ToolCall represents an extracted OpenAI-format tool call.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the tool name and JSON string arguments.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// GoogleFunctionCall represents an extracted Google Gemini function call.
type GoogleFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

var (
	toolCallPattern        = regexp.MustCompile("(?s)```tool_call\\s*\\n(.*?)\\n```")
	googlePattern1         = regexp.MustCompile("(?s)```function_call\\s*\\n(.*?)\\n```")
	googlePattern2         = regexp.MustCompile("(?s)(?:^|\\n)function_call\\s*\\n(\\{[^`]*?\\})")
	dataURLPattern         = regexp.MustCompile(`(?s)^data:([^;,]+)?(;base64)?,(.*)$`)
)

func buildToolChoiceInstruction(toolChoice any) string {
	if toolChoice == nil {
		return ""
	}
	if str, ok := toolChoice.(string); ok {
		switch str {
		case "none":
			return "\n\nIMPORTANT: Do NOT call any tools. Respond with text only."
		case "required":
			return "\n\nIMPORTANT: You MUST call at least one tool. Do not respond with text only."
		}
	} else if m, ok := toolChoice.(map[string]any); ok {
		if fn, ok := m["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				return fmt.Sprintf("\n\nIMPORTANT: You MUST call the tool \"%s\". Do not call other tools.", name)
			}
		}
	}
	return ""
}

func decodeDataURL(raw string) *ImageInput {
	m := dataURLPattern.FindStringSubmatch(raw)
	if len(m) < 4 {
		return nil
	}
	mime := m[1]
	if mime == "" {
		mime = "image/png"
	}
	isBase64 := m[2] != ""
	dataStr := m[3]

	if isBase64 {
		b, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			return nil
		}
		return &ImageInput{Data: b, MIME: mime}
	}

	decoded, err := url.QueryUnescape(dataStr)
	if err != nil {
		return nil
	}
	return &ImageInput{Data: []byte(decoded), MIME: mime}
}

func extractImageFromPart(part map[string]any) *ImageInput {
	pType, _ := part["type"].(string)

	if pType == "image_url" {
		if imgURLObj, ok := part["image_url"].(map[string]any); ok {
			rawURL, _ := imgURLObj["url"].(string)
			mime, _ := imgURLObj["mime_type"].(string)
			if strings.HasPrefix(rawURL, "data:") {
				return decodeDataURL(rawURL)
			}
			return &ImageInput{URL: rawURL, MIME: mime}
		} else if rawURL, ok := part["image_url"].(string); ok {
			if strings.HasPrefix(rawURL, "data:") {
				return decodeDataURL(rawURL)
			}
			return &ImageInput{URL: rawURL}
		}
	}

	if pType == "input_image" || pType == "image" {
		var rawURL, mime string
		if imgURLObj, ok := part["image_url"].(map[string]any); ok {
			rawURL, _ = imgURLObj["url"].(string)
			mime, _ = imgURLObj["mime_type"].(string)
		} else if u, ok := part["image_url"].(string); ok {
			rawURL = u
		} else if u, ok := part["url"].(string); ok {
			rawURL = u
		}

		if rawURL != "" {
			if strings.HasPrefix(rawURL, "data:") {
				return decodeDataURL(rawURL)
			}
			return &ImageInput{URL: rawURL, MIME: mime}
		}

		var rawData string
		if d, ok := part["data"].(string); ok {
			rawData = d
		} else if d, ok := part["base64"].(string); ok {
			rawData = d
		}

		if rawData != "" {
			if m, ok := part["mime_type"].(string); ok {
				mime = m
			} else if m, ok := part["media_type"].(string); ok {
				mime = m
			} else {
				mime = "image/png"
			}
			if strings.HasPrefix(rawData, "data:") {
				return decodeDataURL(rawData)
			}
			b, err := base64.StdEncoding.DecodeString(rawData)
			if err == nil {
				return &ImageInput{Data: b, MIME: mime}
			}
		}
	}

	return nil
}

// MessagesToPrompt converts standard OpenAI messages and tools into an upstream prompt and image inputs.
func MessagesToPrompt(messages []map[string]any, tools []map[string]any, toolChoice any) (string, []ImageInput) {
	var parts []string
	var images []ImageInput

	choiceStr, _ := toolChoice.(string)
	if len(tools) > 0 && choiceStr != "none" {
		var toolDefs []map[string]any
		for _, tool := range tools {
			fn, ok := tool["function"].(map[string]any)
			if !ok || tool["type"] != "function" {
				fn = tool
			}
			name, _ := fn["name"].(string)
			if name == "" {
				name, _ = tool["name"].(string)
			}
			desc, _ := fn["description"].(string)
			if desc == "" {
				desc, _ = tool["description"].(string)
			}
			params := fn["parameters"]
			if params == nil {
				params = tool["parameters"]
			}
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}

			toolDefs = append(toolDefs, map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  params,
			})
		}

		if len(toolDefs) > 0 {
			specJSON, _ := json.MarshalIndent(toolDefs, "", "  ")
			constraint := buildToolChoiceInstruction(toolChoice)
			parts = append(parts, fmt.Sprintf(
				"# Tool Use\n\n"+
					"You can call the following tools. Call format:\n"+
					"```tool_call\n{\"name\": \"func_name\", \"arguments\": {...}}\n```\n"+
					"When calling tools, output ONLY the tool_call block(s).\n\n"+
					"Available tools:\n%s%s",
				string(specJSON), constraint,
			))
		}
	}

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "" {
			role = "user"
		}

		var textContent string
		content := msg["content"]

		if list, ok := content.([]any); ok {
			var textParts []string
			for _, item := range list {
				if part, ok := item.(map[string]any); ok {
					pType, _ := part["type"].(string)
					if pType == "text" || pType == "input_text" {
						t, _ := part["text"].(string)
						textParts = append(textParts, t)
					} else {
						img := extractImageFromPart(part)
						if img != nil {
							images = append(images, *img)
							textParts = append(textParts, "[Image attached]")
						}
					}
				}
			}
			textContent = strings.Join(textParts, " ")
		} else if str, ok := content.(string); ok {
			textContent = str
		}

		switch role {
		case "system":
			parts = append(parts, fmt.Sprintf("[System instruction]: %s", textContent))
		case "assistant":
			if tcs, ok := msg["tool_calls"].([]any); ok && len(tcs) > 0 {
				var tcStrs []string
				for _, tc := range tcs {
					if tcMap, ok := tc.(map[string]any); ok {
						fn, _ := tcMap["function"].(map[string]any)
						name, _ := fn["name"].(string)
						args, _ := fn["arguments"].(string)
						if args == "" {
							args = "{}"
						}
						tcStrs = append(tcStrs, fmt.Sprintf("```tool_call\n{\"name\": \"%s\", \"arguments\": %s}\n```", name, args))
					}
				}
				prefix := ""
				if textContent != "" {
					prefix = fmt.Sprintf("[Assistant]: %s\n", textContent)
				} else {
					prefix = "[Assistant]:\n"
				}
				parts = append(parts, prefix+strings.Join(tcStrs, "\n"))
			} else {
				parts = append(parts, fmt.Sprintf("[Assistant]: %s", textContent))
			}
		case "tool":
			name, _ := msg["name"].(string)
			parts = append(parts, fmt.Sprintf("[Tool result for %s]: %s", name, textContent))
		default:
			if textContent != "" {
				parts = append(parts, textContent)
			}
		}
	}

	return strings.Join(parts, "\n\n"), images
}

// ParseToolCalls extracts OpenAI tool_call blocks from model output.
func ParseToolCalls(text string) (string, []ToolCall) {
	var toolCalls []ToolCall

	clean := toolCallPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := toolCallPattern.FindStringSubmatch(match)
		if len(sub) > 1 {
			var data struct {
				Name      string `json:"name"`
				Arguments any    `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(sub[1])), &data); err == nil && data.Name != "" {
				var argsStr string
				if s, ok := data.Arguments.(string); ok {
					argsStr = s
				} else if data.Arguments != nil {
					b, _ := json.Marshal(data.Arguments)
					argsStr = string(b)
				} else {
					argsStr = "{}"
				}
				toolCalls = append(toolCalls, ToolCall{
					ID:   fmt.Sprintf("call_%s", uuid.New().String()[:8]),
					Type: "function",
					Function: ToolCallFunction{
						Name:      data.Name,
						Arguments: argsStr,
					},
				})
			}
		}
		return ""
	})

	return strings.TrimSpace(clean), toolCalls
}

// GoogleContentsToPrompt converts Google native Gemini contents and tools into an upstream prompt and image inputs.
func GoogleContentsToPrompt(req map[string]any) (string, []ImageInput) {
	var parts []string
	var images []ImageInput

	toolConfig, _ := req["toolConfig"].(map[string]any)
	fcConfig, _ := toolConfig["functionCallingConfig"].(map[string]any)
	fcMode, _ := fcConfig["mode"].(string)
	if fcMode == "" {
		fcMode = "AUTO"
	}

	tools, _ := req["tools"].([]any)
	var toolDefs []map[string]any

	if len(tools) > 0 && fcMode != "NONE" {
		for _, tg := range tools {
			if tgMap, ok := tg.(map[string]any); ok {
				if fns, ok := tgMap["functionDeclarations"].([]any); ok {
					for _, fnItem := range fns {
						if fn, ok := fnItem.(map[string]any); ok {
							name, _ := fn["name"].(string)
							desc, _ := fn["description"].(string)
							td := map[string]any{"name": name, "description": desc}
							if params := fn["parameters"]; params != nil {
								td["parameters"] = params
							} else if params := fn["parametersJsonSchema"]; params != nil {
								td["parameters"] = params
							}
							toolDefs = append(toolDefs, td)
						}
					}
				}
			}
		}
	}

	buildConstraint := func() string {
		if fcMode == "NONE" {
			return "\n\nIMPORTANT: Do NOT call any tools. Respond with text only."
		}
		if fcMode == "ANY" {
			if allowed, ok := fcConfig["allowedFunctionNames"].([]any); ok && len(allowed) > 0 {
				var names []string
				for _, a := range allowed {
					names = append(names, fmt.Sprintf("\"%v\"", a))
				}
				return fmt.Sprintf("\n\nIMPORTANT: You MUST call one of these tools: %s. Do not respond with text only.", strings.Join(names, ", "))
			}
			return "\n\nIMPORTANT: You MUST call at least one tool. Do not respond with text only."
		}
		return ""
	}

	buildToolPrompt := func(defs []map[string]any) string {
		specJSON, _ := json.MarshalIndent(defs, "", "  ")
		return fmt.Sprintf(
			"# Tool Use\n\n"+
				"You can call the following tools to help accomplish tasks. "+
				"These tools connect to the user's local environment and will execute when called.\n\n"+
				"Call format (use this exact format):\n"+
				"```function_call\n"+
				"{\"name\": \"<tool_name>\", \"args\": {<arguments>}}\n"+
				"```\n\n"+
				"When calling tools:\n"+
				"- Output ONLY the function_call block(s), nothing else\n"+
				"- You may call multiple tools with multiple blocks\n"+
				"- After receiving a [Tool result for ...], use that data to answer the user\n\n"+
				"Available tools:\n%s",
			string(specJSON),
		)
	}

	if sysInst, ok := req["systemInstruction"].(map[string]any); ok {
		var sysParts []string
		if sParts, ok := sysInst["parts"].([]any); ok {
			for _, p := range sParts {
				if pMap, ok := p.(map[string]any); ok {
					if t, ok := pMap["text"].(string); ok && t != "" {
						sysParts = append(sysParts, t)
					}
				}
			}
		}
		sysText := strings.Join(sysParts, " ")
		if sysText != "" {
			if len(toolDefs) > 0 {
				parts = append(parts, sysText+"\n\n"+buildToolPrompt(toolDefs)+buildConstraint())
			} else {
				parts = append(parts, sysText)
			}
		}
	} else if len(toolDefs) > 0 {
		parts = append(parts, buildToolPrompt(toolDefs)+buildConstraint())
	}

	if contents, ok := req["contents"].([]any); ok {
		for _, item := range contents {
			content, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := content["role"].(string)
			if role == "" {
				role = "user"
			}

			var msgParts []string
			if cParts, ok := content["parts"].([]any); ok {
				for _, p := range cParts {
					pMap, ok := p.(map[string]any)
					if !ok {
						continue
					}
					if t, ok := pMap["text"].(string); ok && t != "" {
						msgParts = append(msgParts, t)
					} else if inlineData, ok := pMap["inlineData"].(map[string]any); ok {
						dataStr, _ := inlineData["data"].(string)
						mime, _ := inlineData["mimeType"].(string)
						if mime == "" {
							mime = "image/png"
						}
						b, err := base64.StdEncoding.DecodeString(dataStr)
						if err == nil {
							images = append(images, ImageInput{Data: b, MIME: mime})
							msgParts = append(msgParts, "[Image attached]")
						}
					} else if fc, ok := pMap["functionCall"].(map[string]any); ok {
						name, _ := fc["name"].(string)
						args := fc["args"]
						if args == nil {
							args = map[string]any{}
						}
						argsJSON, _ := json.Marshal(args)
						msgParts = append(msgParts, fmt.Sprintf("```function_call\n{\"name\": \"%s\", \"args\": %s}\n```", name, string(argsJSON)))
					} else if fr, ok := pMap["functionResponse"].(map[string]any); ok {
						name, _ := fr["name"].(string)
						respObj := fr["response"]
						if respObj == nil {
							respObj = map[string]any{}
						}
						respJSON, _ := json.Marshal(respObj)
						msgParts = append(msgParts, fmt.Sprintf("[Tool result for %s]: %s", name, string(respJSON)))
					}
				}
			}

			text := strings.Join(msgParts, "\n")
			if role == "model" {
				parts = append(parts, fmt.Sprintf("[Assistant]: %s", text))
			} else {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n\n"), images
}

// ParseGoogleFunctionCalls extracts function_call blocks from Google Gemini responses.
func ParseGoogleFunctionCalls(text string) (string, []GoogleFunctionCall) {
	var functionCalls []GoogleFunctionCall
	clean := text

	for _, pattern := range []*regexp.Regexp{googlePattern1, googlePattern2} {
		clean = pattern.ReplaceAllStringFunc(clean, func(match string) string {
			sub := pattern.FindStringSubmatch(match)
			if len(sub) > 1 {
				var data struct {
					Name      string         `json:"name"`
					Args      map[string]any `json:"args"`
					Arguments map[string]any `json:"arguments"`
				}
				if err := json.Unmarshal([]byte(strings.TrimSpace(sub[1])), &data); err == nil && data.Name != "" {
					args := data.Args
					if args == nil {
						args = data.Arguments
					}
					if args == nil {
						args = map[string]any{}
					}
					functionCalls = append(functionCalls, GoogleFunctionCall{
						Name: data.Name,
						Args: args,
					})
				}
			}
			return ""
		})
	}

	clean = strings.TrimSpace(clean)
	if len(functionCalls) == 0 && strings.HasPrefix(clean, "{") {
		var data struct {
			Name      string         `json:"name"`
			Args      map[string]any `json:"args"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(clean), &data); err == nil && data.Name != "" && (data.Args != nil || data.Arguments != nil) {
			args := data.Args
			if args == nil {
				args = data.Arguments
			}
			if args == nil {
				args = map[string]any{}
			}
			functionCalls = append(functionCalls, GoogleFunctionCall{
				Name: data.Name,
				Args: args,
			})
			clean = ""
		}
	}

	return clean, functionCalls
}
