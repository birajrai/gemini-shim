package gemini

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/birajrai/gemini-shim/internal/config"
)

type cookieCache struct {
	sync.RWMutex
	cookieStr string
	sapisid   string
	mtime     int64
}

var cache = &cookieCache{}

// LoadCookie reads the cookie file if configured, caching based on file modification time.
func LoadCookie() (cookieStr string, sapisid string) {
	cfg := config.Get()
	if cfg == nil || cfg.CookieFile == "" {
		return "", ""
	}

	info, err := os.Stat(cfg.CookieFile)
	if err != nil {
		return "", ""
	}

	mtime := info.ModTime().UnixNano()

	cache.RLock()
	if mtime == cache.mtime && cache.cookieStr != "" {
		c, s := cache.cookieStr, cache.sapisid
		cache.RUnlock()
		return c, s
	}
	cache.RUnlock()

	data, err := os.ReadFile(cfg.CookieFile)
	if err != nil {
		config.Log("Cookie load error: %v", err)
		cache.RLock()
		defer cache.RUnlock()
		return cache.cookieStr, cache.sapisid
	}

	content := strings.TrimSpace(string(data))
	var cStr, sID string

	if strings.HasPrefix(content, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(content), &obj); err == nil {
			if v, ok := obj["cookie"].(string); ok {
				cStr = v
			}
			if v, ok := obj["sapisid"].(string); ok {
				sID = v
			}
		}
	} else {
		cStr = content
		for _, part := range strings.Split(cStr, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "SAPISID=") {
				sID = strings.TrimPrefix(part, "SAPISID=")
				break
			}
		}
	}

	cache.Lock()
	cache.cookieStr = cStr
	cache.sapisid = sID
	cache.mtime = mtime
	cache.Unlock()

	return cStr, sID
}

// MakeSAPISIDHash generates the SAPISIDHASH Authorization header value for Google APIs.
func MakeSAPISIDHash(sapisid string) string {
	if sapisid == "" {
		return ""
	}
	ts := time.Now().Unix()
	msg := fmt.Sprintf("%d %s https://gemini.google.com", ts, sapisid)
	h := sha1.Sum([]byte(msg))
	hashHex := hex.EncodeToString(h[:])
	return fmt.Sprintf("SAPISIDHASH %d_%s", ts, hashHex)
}

// AccountPrefix returns the URL account path segment for multi-login Google accounts (/u/N).
func AccountPrefix() string {
	cfg := config.Get()
	if cfg == nil || cfg.AuthUser == nil || *cfg.AuthUser == "" {
		return ""
	}
	return fmt.Sprintf("/u/%s", *cfg.AuthUser)
}
