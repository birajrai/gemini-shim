package models_test

import (
	"testing"

	"github.com/birajrai/gemini-shim/internal/models"
)

func TestResolveModel(t *testing.T) {
	// Standard model
	name, modeID, thinkMode, extra, err := models.ResolveModel("gemini-3.7-flash", "gemini-3.6-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gemini-3.7-flash" || modeID != 1 || thinkMode != 4 || extra != nil {
		t.Errorf("unexpected resolve result: name=%s modeID=%d thinkMode=%d extra=%v", name, modeID, thinkMode, extra)
	}

	// Thinking model
	name, modeID, thinkMode, _, err = models.ResolveModel("gemini-3.5-flash-thinking", "gemini-3.6-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gemini-3.5-flash-thinking" || modeID != 2 || thinkMode != 0 {
		t.Errorf("unexpected resolve result: name=%s modeID=%d thinkMode=%d", name, modeID, thinkMode)
	}

	// Think override suffix
	name, modeID, thinkMode, _, err = models.ResolveModel("gemini-3.7-flash@think=0", "gemini-3.6-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gemini-3.7-flash" || modeID != 1 || thinkMode != 0 {
		t.Errorf("unexpected think override result: name=%s modeID=%d thinkMode=%d", name, modeID, thinkMode)
	}

	// Unknown model fallback
	name, modeID, thinkMode, _, err = models.ResolveModel("unknown-custom-model", "gemini-3.6-flash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gemini-3.6-flash" || modeID != 1 || thinkMode != 4 {
		t.Errorf("unexpected fallback result: name=%s modeID=%d thinkMode=%d", name, modeID, thinkMode)
	}

	// Invalid think level
	_, _, _, _, err = models.ResolveModel("gemini-3.7-flash@think=invalid", "gemini-3.6-flash")
	if err == nil {
		t.Errorf("expected error for invalid think level, got nil")
	}
}
