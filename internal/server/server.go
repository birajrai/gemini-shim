package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/birajrai/gemini-shim/internal/gemini"
	"github.com/birajrai/gemini-shim/internal/models"
	"github.com/birajrai/gemini-shim/internal/multimodal"
	"github.com/birajrai/gemini-shim/internal/tools"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const Version = "1.0.0"

var googleModelURLRegex = regexp.MustCompile(`^/v1beta/models/([^:?]+)(?::([a-zA-Z0-9_-]+))?`)

func uploadImages(imageInputs []tools.ImageInput) ([]string, error) {
	if len(imageInputs) == 0 {
		return nil, nil
	}

	var fileRefs []string
	for _, img := range imageInputs {
		data := img.Data
		mime := img.MIME

		if len(data) == 0 && img.URL != "" {
			var err error
			data, err = multimodal.FetchImageBytes(img.URL)
			if err != nil {
				return nil, fmt.Errorf("image fetch failed: %w", err)
			}
		}

		if len(data) == 0 {
			return nil, fmt.Errorf("empty image data")
		}

		mime = multimodal.DetectImageMIME(data, mime)
		ref, err := multimodal.UploadImage(data, "image.png", mime)
		if err != nil {
			return nil, fmt.Errorf("image upload failed: %w", err)
		}
		fileRefs = append(fileRefs, ref)
	}

	return fileRefs, nil
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "*")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		if cfg == nil || len(cfg.APIKeys) == 0 {
			c.Next()
			return
		}

		// Only protect /v1 and /v1beta paths
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/v1") && !strings.HasPrefix(path, "/v1beta") {
			c.Next()
			return
		}

		authorized := false
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			for _, k := range cfg.APIKeys {
				if token == k {
					authorized = true
					break
				}
			}
		}

		if !authorized {
			for _, h := range []string{"x-api-key", "x-goog-api-key"} {
				val := c.GetHeader(h)
				if val != "" {
					for _, k := range cfg.APIKeys {
						if val == k {
							authorized = true
							break
						}
					}
				}
			}
		}

		if !authorized {
			keyQuery := c.Query("key")
			if keyQuery != "" {
				for _, k := range cfg.APIKeys {
					if keyQuery == k {
						authorized = true
						break
					}
				}
			}
		}

		if !authorized {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "invalid api key",
				},
			})
			return
		}

		c.Next()
	}
}

func logMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		clientIP := c.ClientIP()
		status := c.Writer.Status()
		config.Log("%s \"%s %s\" %d %v", clientIP, c.Request.Method, c.Request.URL.RequestURI(), status, latency)
	}
}

// SetupRouter configures and returns the Gin engine with all API routes.
func SetupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(authMiddleware())
	r.Use(logMiddleware())

	r.GET("/", handleIndex)
	r.GET("/v1/models", handleOpenAIModels)
	r.GET("/v1beta/models", handleGoogleModels)

	r.POST("/v1/chat/completions", handleChatCompletions)
	r.POST("/v1/responses", handleResponses)

	// Google Gemini native generateContent endpoints
	r.POST("/v1beta/models/*action", handleGoogleGenerate)

	return r
}

func handleIndex(c *gin.Context) {
	modelNames := make([]string, 0, len(models.Models))
	for name := range models.Models {
		modelNames = append(modelNames, name)
	}
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": Version,
		"models":  modelNames,
	})
}

func handleOpenAIModels(c *gin.Context) {
	var data []gin.H
	for name, m := range models.Models {
		data = append(data, gin.H{
			"id":          name,
			"object":      "model",
			"created":     1700000000,
			"owned_by":    "google",
			"description": m.Desc,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func handleGoogleModels(c *gin.Context) {
	var modelList []gin.H
	for name, m := range models.Models {
		modelList = append(modelList, gin.H{
			"name":                       fmt.Sprintf("models/%s", name),
			"displayName":                name,
			"description":                m.Desc,
			"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent"},
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"models": modelList,
	})
}

func handleChatCompletions(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid JSON"}})
		return
	}

	cfg := config.Get()
	reqModel, _ := req["model"].(string)
	if reqModel == "" && cfg != nil {
		reqModel = cfg.DefaultModel
	}

	modelName, modelID, thinkMode, extra, err := models.ResolveModel(reqModel, cfg.DefaultModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	var rawMessages []map[string]any
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			if mMap, ok := m.(map[string]any); ok {
				rawMessages = append(rawMessages, mMap)
			}
		}
	}

	var rawTools []map[string]any
	if tls, ok := req["tools"].([]any); ok {
		for _, t := range tls {
			if tMap, ok := t.(map[string]any); ok {
				rawTools = append(rawTools, tMap)
			}
		}
	}

	toolChoice := req["tool_choice"]
	if toolChoice == nil {
		toolChoice = "auto"
	}

	prompt, imageInputs := tools.MessagesToPrompt(rawMessages, rawTools, toolChoice)
	if strings.TrimSpace(prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "empty prompt"}})
		return
	}

	fileRefs, err := uploadImages(imageInputs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	stream, _ := req["stream"].(bool)
	cid := fmt.Sprintf("chatcmpl-%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:12])

	toolChoiceStr, _ := toolChoice.(string)
	hasNoTools := len(rawTools) == 0 || toolChoiceStr == "none"

	if stream && hasNoTools {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		w := c.Writer
		flusher, ok := w.(http.Flusher)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "streaming unsupported"}})
			return
		}

		// Initial assistant chunk
		firstChunk := gin.H{
			"id":      cid,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []gin.H{
				{
					"index":         0,
					"delta":         gin.H{"role": "assistant"},
					"finish_reason": nil,
				},
			},
		}
		firstBytes, _ := json.Marshal(firstChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(firstBytes))
		flusher.Flush()

		chunkChan := make(chan string, 50)
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		errChan := make(chan error, 1)
		go func() {
			errChan <- gemini.GenerateStream(ctx, prompt, modelID, thinkMode, fileRefs, extra, chunkChan)
			close(chunkChan)
		}()

		for delta := range chunkChan {
			chunk := gin.H{
				"id":      cid,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   modelName,
				"choices": []gin.H{
					{
						"index":         0,
						"delta":         gin.H{"content": delta},
						"finish_reason": nil,
					},
				},
			}
			chunkBytes, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
			flusher.Flush()
		}

		if streamErr := <-errChan; streamErr != nil {
			config.Log("Stream error: %v", streamErr)
		}

		endChunk := gin.H{
			"id":      cid,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []gin.H{
				{
					"index":         0,
					"delta":         gin.H{},
					"finish_reason": "stop",
				},
			},
		}
		endBytes, _ := json.Marshal(endChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(endBytes))
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	text, err := gemini.Generate(prompt, modelID, thinkMode, fileRefs, extra)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	var toolCalls []tools.ToolCall
	if len(rawTools) > 0 && text != "" && toolChoiceStr != "none" {
		text, toolCalls = tools.ParseToolCalls(text)
	}

	msg := gin.H{
		"role":    "assistant",
		"content": text,
	}
	if text == "" {
		msg["content"] = nil
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		w := c.Writer
		flusher := w.(http.Flusher)

		chunk := gin.H{
			"id":      cid,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []gin.H{
				{
					"index":         0,
					"delta":         msg,
					"finish_reason": finishReason,
				},
			},
		}
		chunkBytes, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	promptTokens := len(prompt) / 4
	completionTokens := len(text) / 4
	c.JSON(http.StatusOK, gin.H{
		"id":      cid,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelName,
		"choices": []gin.H{
			{
				"index":         0,
				"message":       msg,
				"finish_reason": finishReason,
			},
		},
		"usage": gin.H{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	})
}

func handleResponses(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid JSON"}})
		return
	}

	cfg := config.Get()
	reqModel, _ := req["model"].(string)
	if reqModel == "" && cfg != nil {
		reqModel = cfg.DefaultModel
	}

	modelName, modelID, thinkMode, extra, err := models.ResolveModel(reqModel, cfg.DefaultModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	var messages []map[string]any
	if instructions, ok := req["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": instructions,
		})
	}

	if inputItems, ok := req["input"].([]any); ok {
		for _, item := range inputItems {
			if str, ok := item.(string); ok {
				messages = append(messages, map[string]any{"role": "user", "content": str})
			} else if itemMap, ok := item.(map[string]any); ok {
				itemType, _ := itemMap["type"].(string)
				if itemType == "function_call_output" {
					messages = append(messages, map[string]any{
						"role":         "tool",
						"tool_call_id": itemMap["call_id"],
						"name":         itemMap["name"],
						"content":      itemMap["output"],
					})
				} else if itemType == "input_text" || itemType == "input_image" || itemType == "image" {
					messages = append(messages, map[string]any{
						"role":    "user",
						"content": []any{itemMap},
					})
				} else if itemMap["role"] == "assistant" || (itemType == "message" && itemMap["role"] == "assistant") {
					var textAcc string
					var tcList []map[string]any
					if cp, ok := itemMap["content"].([]any); ok {
						for _, cItem := range cp {
							if cMap, ok := cItem.(map[string]any); ok {
								if cMap["type"] == "output_text" {
									t, _ := cMap["text"].(string)
									textAcc += t
								} else if cMap["type"] == "function_call" {
									tcList = append(tcList, cMap)
								}
							}
						}
					} else if cpStr, ok := itemMap["content"].(string); ok {
						textAcc = cpStr
					}

					m := map[string]any{"role": "assistant", "content": textAcc}
					if len(tcList) > 0 {
						var formattedTCs []map[string]any
						for i, tc := range tcList {
							callID, _ := tc["call_id"].(string)
							if callID == "" {
								callID = fmt.Sprintf("call_%d", i)
							}
							name, _ := tc["name"].(string)
							args, _ := tc["arguments"].(string)
							if args == "" {
								args = "{}"
							}
							formattedTCs = append(formattedTCs, map[string]any{
								"id":   callID,
								"type": "function",
								"function": map[string]any{
									"name":      name,
									"arguments": args,
								},
							})
						}
						m["tool_calls"] = formattedTCs
					}
					messages = append(messages, m)
				} else {
					role, _ := itemMap["role"].(string)
					if role == "" {
						role = "user"
					}
					messages = append(messages, map[string]any{"role": role, "content": itemMap["content"]})
				}
			}
		}
	} else if inputStr, ok := req["input"].(string); ok {
		messages = append(messages, map[string]any{"role": "user", "content": inputStr})
	}

	var rawTools []map[string]any
	if tls, ok := req["tools"].([]any); ok {
		for _, t := range tls {
			if tMap, ok := t.(map[string]any); ok {
				if tMap["type"] == "function" && tMap["function"] == nil {
					name, _ := tMap["name"].(string)
					desc, _ := tMap["description"].(string)
					params := tMap["parameters"]
					tMap = map[string]any{
						"type": "function",
						"function": map[string]any{
							"name":        name,
							"description": desc,
							"parameters":  params,
						},
					}
				}
				rawTools = append(rawTools, tMap)
			}
		}
	}

	toolChoice := req["tool_choice"]
	if toolChoice == nil {
		toolChoice = "auto"
	}

	prompt, imageInputs := tools.MessagesToPrompt(messages, rawTools, toolChoice)
	if strings.TrimSpace(prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "empty input"}})
		return
	}

	fileRefs, err := uploadImages(imageInputs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	text, err := gemini.Generate(prompt, modelID, thinkMode, fileRefs, extra)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	toolChoiceStr, _ := toolChoice.(string)
	var toolCalls []tools.ToolCall
	if len(rawTools) > 0 && text != "" && toolChoiceStr != "none" {
		text, toolCalls = tools.ParseToolCalls(text)
	}

	rid := fmt.Sprintf("resp_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	mid := fmt.Sprintf("msg_%s", strings.ReplaceAll(uuid.New().String(), "-", "")[:12])

	var output []gin.H
	if len(toolCalls) > 0 {
		for _, tc := range toolCalls {
			output = append(output, gin.H{
				"type":      "function_call",
				"id":        tc.ID,
				"call_id":   tc.ID,
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
				"status":    "completed",
			})
		}
	}
	if text != "" || len(toolCalls) == 0 {
		output = append(output, gin.H{
			"type":   "message",
			"id":     mid,
			"role":   "assistant",
			"status": "completed",
			"content": []gin.H{
				{
					"type":        "output_text",
					"text":        text,
					"annotations": []any{},
				},
			},
		})
	}

	stream, _ := req["stream"].(bool)
	promptTokens := len(prompt) / 4
	completionTokens := len(text) / 4

	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		w := c.Writer
		flusher := w.(http.Flusher)

		sequenceNumber := 0
		emit := func(eventType string, fields gin.H) {
			sequenceNumber++
			fields["type"] = eventType
			fields["sequence_number"] = sequenceNumber
			eventBytes, _ := json.Marshal(fields)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(eventBytes))
		}

		baseResponse := gin.H{
			"id":         rid,
			"object":     "response",
			"created_at": time.Now().Unix(),
			"model":      modelName,
		}

		emit("response.created", gin.H{
			"response": gin.H{
				"id":         baseResponse["id"],
				"object":     baseResponse["object"],
				"created_at": baseResponse["created_at"],
				"model":      baseResponse["model"],
				"status":     "in_progress",
				"output":     []any{},
				"usage":      nil,
			},
		})

		emit("response.in_progress", gin.H{
			"response": gin.H{
				"id":         baseResponse["id"],
				"object":     baseResponse["object"],
				"created_at": baseResponse["created_at"],
				"model":      baseResponse["model"],
				"status":     "in_progress",
				"output":     []any{},
				"usage":      nil,
			},
		})

		for outputIndex, item := range output {
			itemType, _ := item["type"].(string)
			if itemType == "function_call" {
				pendingItem := gin.H{
					"type":      "function_call",
					"id":        item["id"],
					"call_id":   item["call_id"],
					"name":      item["name"],
					"arguments": "",
					"status":    "in_progress",
				}
				emit("response.output_item.added", gin.H{
					"output_index": outputIndex,
					"item":         pendingItem,
				})
				emit("response.function_call_arguments.delta", gin.H{
					"item_id":      item["id"],
					"output_index": outputIndex,
					"delta":        item["arguments"],
				})
				emit("response.function_call_arguments.done", gin.H{
					"item_id":      item["id"],
					"output_index": outputIndex,
					"arguments":    item["arguments"],
				})
				emit("response.output_item.done", gin.H{
					"output_index": outputIndex,
					"item":         item,
				})
			} else if itemType == "message" {
				pendingItem := gin.H{
					"type":    "message",
					"id":      item["id"],
					"role":    "assistant",
					"status":  "in_progress",
					"content": []any{},
				}
				emit("response.output_item.added", gin.H{
					"output_index": outputIndex,
					"item":         pendingItem,
				})
				if contentList, ok := item["content"].([]gin.H); ok {
					for contentIndex, contentPart := range contentList {
						eventFields := gin.H{
							"item_id":       item["id"],
							"output_index":  outputIndex,
							"content_index": contentIndex,
						}
						emit("response.content_part.added", gin.H{
							"item_id":       eventFields["item_id"],
							"output_index":  eventFields["output_index"],
							"content_index": eventFields["content_index"],
							"part": gin.H{
								"type":        "output_text",
								"text":        "",
								"annotations": []any{},
							},
						})
						emit("response.output_text.delta", gin.H{
							"item_id":       eventFields["item_id"],
							"output_index":  eventFields["output_index"],
							"content_index": eventFields["content_index"],
							"delta":         contentPart["text"],
						})
						emit("response.output_text.done", gin.H{
							"item_id":       eventFields["item_id"],
							"output_index":  eventFields["output_index"],
							"content_index": eventFields["content_index"],
							"text":          contentPart["text"],
						})
						emit("response.content_part.done", gin.H{
							"item_id":       eventFields["item_id"],
							"output_index":  eventFields["output_index"],
							"content_index": eventFields["content_index"],
							"part":          contentPart,
						})
					}
				}
				emit("response.output_item.done", gin.H{
					"output_index": outputIndex,
					"item":         item,
				})
			}
		}

		emit("response.completed", gin.H{
			"response": gin.H{
				"id":         baseResponse["id"],
				"object":     baseResponse["object"],
				"created_at": baseResponse["created_at"],
				"model":      baseResponse["model"],
				"status":     "completed",
				"output":     output,
				"usage": gin.H{
					"input_tokens":  promptTokens,
					"output_tokens": completionTokens,
					"total_tokens":  promptTokens + completionTokens,
				},
			},
		})
		flusher.Flush()
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         rid,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      modelName,
		"output":     output,
		"usage": gin.H{
			"input_tokens":  promptTokens,
			"output_tokens": completionTokens,
			"total_tokens":  promptTokens + completionTokens,
		},
	})
}

func handleGoogleGenerate(c *gin.Context) {
	path := c.Request.URL.Path
	isStream := strings.Contains(path, ":streamGenerateContent")
	isGenerate := strings.Contains(path, ":generateContent")

	if !isStream && !isGenerate {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	m := googleModelURLRegex.FindStringSubmatch(path)
	cfg := config.Get()
	targetModel := ""
	if len(m) > 1 && m[1] != "" {
		targetModel = m[1]
	} else if cfg != nil {
		targetModel = cfg.DefaultModel
	}

	modelName, modelID, thinkMode, extra, err := models.ResolveModel(targetModel, cfg.DefaultModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "failed to read body"}})
		return
	}

	var req map[string]any
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "invalid JSON"}})
		return
	}

	toolConfig, _ := req["toolConfig"].(map[string]any)
	fcConfig, _ := toolConfig["functionCallingConfig"].(map[string]any)
	fcMode, _ := fcConfig["mode"].(string)
	if fcMode == "" {
		fcMode = "AUTO"
	}
	toolsList, _ := req["tools"].([]any)
	hasTools := len(toolsList) > 0 && fcMode != "NONE"

	prompt, imageInputs := tools.GoogleContentsToPrompt(req)
	if strings.TrimSpace(prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "empty content"}})
		return
	}

	fileRefs, err := uploadImages(imageInputs)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	config.Log("Google API: model=%s stream=%v tools=%v prompt_len=%d", modelName, isStream, hasTools, len(prompt))

	if isStream && !hasTools {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		w := c.Writer
		flusher := w.(http.Flusher)

		chunkChan := make(chan string, 50)
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		errChan := make(chan error, 1)
		go func() {
			errChan <- gemini.GenerateStream(ctx, prompt, modelID, thinkMode, fileRefs, extra, chunkChan)
			close(chunkChan)
		}()

		var fullText strings.Builder
		for delta := range chunkChan {
			if delta == "" {
				continue
			}
			fullText.WriteString(delta)
			chunkObj := gin.H{
				"candidates": []gin.H{
					{
						"content": gin.H{
							"parts": []gin.H{
								{"text": delta},
							},
							"role": "model",
						},
						"index": 0,
					},
				},
				"modelVersion": modelName,
			}
			chunkBytes, _ := json.Marshal(chunkObj)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
			flusher.Flush()
		}

		if streamErr := <-errChan; streamErr != nil {
			config.Log("Google stream error: %v", streamErr)
		}

		finalChunk := gin.H{
			"candidates": []gin.H{
				{
					"finishReason": "STOP",
					"index":        0,
				},
			},
			"usageMetadata": gin.H{
				"promptTokenCount":     len(prompt) / 4,
				"candidatesTokenCount": fullText.Len() / 4,
				"totalTokenCount":      (len(prompt) + fullText.Len()) / 4,
			},
			"modelVersion": modelName,
		}
		finalBytes, _ := json.Marshal(finalChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(finalBytes))
		flusher.Flush()
		return
	}

	text, err := gemini.Generate(prompt, modelID, thinkMode, fileRefs, extra)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": fmt.Sprintf("upstream error: %v", err)}})
		return
	}

	if text == "" {
		config.Log("Warning: empty response from Gemini")
	}

	var responseParts []gin.H
	if hasTools && text != "" {
		cleanText, functionCalls := tools.ParseGoogleFunctionCalls(text)
		if len(functionCalls) > 0 {
			if cleanText != "" {
				responseParts = append(responseParts, gin.H{"text": cleanText})
			}
			for _, fc := range functionCalls {
				responseParts = append(responseParts, gin.H{
					"functionCall": gin.H{
						"name": fc.Name,
						"args": fc.Args,
					},
				})
			}
		} else {
			responseParts = append(responseParts, gin.H{"text": text})
		}
	} else {
		fallbackText := text
		if fallbackText == "" {
			fallbackText = "I apologize, but I was unable to generate a response. Please try again."
		}
		responseParts = append(responseParts, gin.H{"text": fallbackText})
	}

	candidate := gin.H{
		"content": gin.H{
			"parts": responseParts,
			"role":  "model",
		},
		"finishReason": "STOP",
		"index":        0,
	}

	usage := gin.H{
		"promptTokenCount":     len(prompt) / 4,
		"candidatesTokenCount": len(text) / 4,
		"totalTokenCount":      (len(prompt) + len(text)) / 4,
	}

	responseObj := gin.H{
		"candidates":    []gin.H{candidate},
		"usageMetadata": usage,
		"modelVersion":  modelName,
	}

	if isStream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("Access-Control-Allow-Origin", "*")

		w := c.Writer
		flusher := w.(http.Flusher)
		b, _ := json.Marshal(responseObj)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
		return
	}

	c.JSON(http.StatusOK, responseObj)
}
