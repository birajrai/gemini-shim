package multimodal

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/birajrai/gemini-shim/internal/gemini"
)

var (
	pushIDRegex = regexp.MustCompile(`"qKIAYe":"([^"]+)"`)
	pctxRegex   = regexp.MustCompile(`"Ylro7b":"([^"]+)"`)
	atRegex     = regexp.MustCompile(`"thykhd":"([^"]+)"`)

	pageTokensCache struct {
		sync.RWMutex
		tokens map[string]string
		ts     int64
	}
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

// DetectImageMIME infers image MIME type from magic headers.
func DetectImageMIME(data []byte, fallback string) string {
	if fallback == "" {
		fallback = "image/png"
	}
	if len(data) == 0 {
		return fallback
	}

	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte("\xff\xd8\xff")) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return "image/gif"
	}
	if len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if bytes.HasPrefix(data, []byte("BM")) {
		return "image/bmp"
	}
	if bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")) {
		return "image/tiff"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		brand := string(data[8:12])
		switch brand {
		case "avif", "avis":
			return "image/avif"
		case "heic", "heix", "hevc", "hevx":
			return "image/heic"
		}
	}
	return fallback
}

func getPageTokens() map[string]string {
	tokens := make(map[string]string)
	client := newHTTPClient(30 * time.Second)

	req, err := http.NewRequest("GET", "https://gemini.google.com/app", nil)
	if err != nil {
		return tokens
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	cookieStr, sapisid := gemini.LoadCookie()
	if cookieStr != "" {
		req.Header.Set("Cookie", cookieStr)
	}
	if sapisid != "" {
		req.Header.Set("Authorization", gemini.MakeSAPISIDHash(sapisid))
	}

	resp, err := client.Do(req)
	if err != nil {
		config.Log("Page token fetch failed: %v", err)
		return tokens
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return tokens
	}
	html := string(bodyBytes)

	if m := pushIDRegex.FindStringSubmatch(html); len(m) > 1 {
		tokens["push_id"] = m[1]
	}
	if m := pctxRegex.FindStringSubmatch(html); len(m) > 1 {
		tokens["pctx"] = m[1]
	}
	if m := atRegex.FindStringSubmatch(html); len(m) > 1 {
		tokens["at"] = m[1]
	}

	return tokens
}

func cachedPageTokens() map[string]string {
	now := time.Now().Unix()
	pageTokensCache.RLock()
	if now-pageTokensCache.ts < 600 && len(pageTokensCache.tokens) > 0 {
		t := pageTokensCache.tokens
		pageTokensCache.RUnlock()
		return t
	}
	pageTokensCache.RUnlock()

	tokens := getPageTokens()

	pageTokensCache.Lock()
	pageTokensCache.tokens = tokens
	pageTokensCache.ts = now
	pageTokensCache.Unlock()

	return tokens
}

// UploadImage performs Google Scotty resumable upload to store image input.
func UploadImage(imageBytes []byte, filename string, mimeType string) (string, error) {
	tokens := cachedPageTokens()
	pushID := tokens["push_id"]
	if pushID == "" {
		pushID = "feeds/mcudyrk2a4khkz"
	}
	pctx := tokens["pctx"]
	if pctx == "" {
		pctx = "CgcSBWjK7pYx"
	}

	cookieStr, sapisid := gemini.LoadCookie()
	client := newHTTPClient(30 * time.Second)

	// Step 1: Initiate resumable upload session
	startURL := "https://content-push.googleapis.com/upload/"
	req1, err := http.NewRequest("POST", startURL, bytes.NewReader(nil))
	if err != nil {
		return "", fmt.Errorf("failed to create upload session request: %w", err)
	}

	req1.Header.Set("Push-ID", pushID)
	req1.Header.Set("X-Tenant-Id", "bard-storage")
	req1.Header.Set("X-Client-Pctx", pctx)
	req1.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.Itoa(len(imageBytes)))
	req1.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)
	req1.Header.Set("X-Goog-Upload-Protocol", "resumable")
	req1.Header.Set("X-Goog-Upload-Command", "start")
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req1.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	if cookieStr != "" {
		req1.Header.Set("Cookie", cookieStr)
	}
	if sapisid != "" {
		req1.Header.Set("Authorization", gemini.MakeSAPISIDHash(sapisid))
	}

	resp1, err := client.Do(req1)
	if err != nil {
		return "", fmt.Errorf("resumable upload start request failed: %w", err)
	}
	defer resp1.Body.Close()

	uploadURL := resp1.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		uploadURL = resp1.Header.Get("x-goog-upload-url")
	}
	if uploadURL == "" {
		return "", fmt.Errorf("no upload URL in response headers (status %d)", resp1.StatusCode)
	}

	config.Log("Upload session started: %.80s...", uploadURL)

	// Step 2: Upload raw file bytes and finalize
	uploadClient := newHTTPClient(60 * time.Second)
	req2, err := http.NewRequest("POST", uploadURL, bytes.NewReader(imageBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create upload finalize request: %w", err)
	}

	req2.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	req2.Header.Set("X-Goog-Upload-Offset", "0")
	req2.Header.Set("Content-Type", "application/octet-stream")
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp2, err := uploadClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("resumable upload data failed: %w", err)
	}
	defer resp2.Body.Close()

	refBytes, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response: %w", err)
	}

	fileRef := strings.TrimSpace(string(refBytes))
	if fileRef == "" || !strings.HasPrefix(fileRef, "/") {
		return "", fmt.Errorf("invalid file reference: %.100s", fileRef)
	}

	config.Log("Image uploaded: %s -> %.50s...", filename, fileRef)
	return fileRef, nil
}

// FetchImageBytes retrieves image binary data from a remote HTTP/HTTPS URL.
func FetchImageBytes(rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("unsupported image URL scheme: %s", rawURL)
	}

	client := newHTTPClient(30 * time.Second)
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote image fetch failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
