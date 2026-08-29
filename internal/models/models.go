package models

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/birajrai/gemini-shim/internal/config"
)

// ModelInfo defines backend execution parameters for a given model.
type ModelInfo struct {
	Mode  int            `json:"mode"`
	Think int            `json:"think"`
	Desc  string         `json:"desc"`
	Extra map[int]any    `json:"extra,omitempty"`
}

// Models contains known Gemini Web models and their frontend MODE_CATEGORY parameters.
var Models = map[string]ModelInfo{
	"gemini-3.7-flash": {
		Mode:  1,
		Think: 4,
		Desc:  "Latest all-around model (Gemini 3.7 Flash)",
	},
	"gemini-3.6-flash": {
		Mode:  1,
		Think: 4,
		Desc:  "All-around model (Gemini 3.6 Flash)",
	},
	"gemini-3.5-flash": {
		Mode:  1,
		Think: 4,
		Desc:  "Alias for gemini-3.6-flash (backend upgraded)",
	},
	"gemini-3.5-flash-thinking": {
		Mode:  2,
		Think: 0,
		Desc:  "Deep thinking mode, longest output (~20k chars)",
	},
	"gemini-3.1-pro": {
		Mode:  3,
		Think: 4,
		Desc:  "Pro model (requires cookie for real routing)",
	},
	"gemini-3.1-pro-enhanced": {
		Mode:  3,
		Think: 4,
		Desc:  "Pro with enhanced output (experimental)",
		Extra: map[int]any{31: 2, 80: 3},
	},
	"gemini-auto": {
		Mode:  4,
		Think: 4,
		Desc:  "Auto model selection",
	},
	"gemini-3.5-flash-thinking-lite": {
		Mode:  5,
		Think: 0,
		Desc:  "Dynamic thinking with adaptive depth",
	},
	"gemini-flash-lite": {
		Mode:  6,
		Think: 4,
		Desc:  "Lightweight fast model",
	},
}

// ResolveModel maps a requested model name (potentially with @think=N override)
// to its backend parameters. If unknown, it falls back to the default model.
func ResolveModel(modelName string, defaultModel string) (resolvedName string, modeID int, thinkMode int, extra map[int]any, err error) {
	if defaultModel == "" {
		defaultModel = "gemini-3.6-flash"
	}

	var thinkOverride *int
	if idx := strings.LastIndex(modelName, "@think="); idx != -1 {
		thinkStr := modelName[idx+len("@think="):]
		val, err := strconv.Atoi(thinkStr)
		if err != nil {
			return "", 0, 0, nil, fmt.Errorf("invalid think level: %s", thinkStr)
		}
		thinkOverride = &val
		modelName = modelName[:idx]
	}

	cfg, ok := Models[modelName]
	if !ok {
		config.Log("Unknown model '%s', falling back to '%s'", modelName, defaultModel)
		modelName = defaultModel
		cfg = Models[defaultModel]
	}

	resolvedName = modelName
	modeID = cfg.Mode
	if thinkOverride != nil {
		thinkMode = *thinkOverride
	} else {
		thinkMode = cfg.Think
	}
	extra = cfg.Extra

	return resolvedName, modeID, thinkMode, extra, nil
}
