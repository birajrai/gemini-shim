package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/birajrai/gemini-shim/internal/config"
	"github.com/birajrai/gemini-shim/internal/models"
	"github.com/birajrai/gemini-shim/internal/server"
)

func main() {
	var (
		portFlag       int
		configFlag     string
		cookieFileFlag string
		proxyFlag      string
		versionFlag    bool
	)

	flag.IntVar(&portFlag, "port", 0, "Port to listen on (e.g. 8081)")
	flag.StringVar(&configFlag, "config", "", "Path to configuration file")
	flag.StringVar(&cookieFileFlag, "cookie-file", "", "Path to cookie file")
	flag.StringVar(&proxyFlag, "proxy", "", "HTTP proxy, e.g. http://127.0.0.1:7890")
	flag.BoolVar(&versionFlag, "version", false, "Print version and exit")
	flag.Parse()

	if versionFlag {
		fmt.Printf("gemini-shim %s\n", server.Version)
		return
	}

	configPath := configFlag
	if configPath == "" {
		configPath = os.Getenv("GEMINI_SHIM_CONFIG")
	}
	if configPath == "" {
		configPath = config.FindConfig()
	}

	if configPath != "" {
		if _, err := config.LoadConfig(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config from %s: %v\n", configPath, err)
		}
	}

	cfg := config.Get()
	if portFlag != 0 {
		cfg.Port = portFlag
	}
	if cookieFileFlag != "" {
		cfg.CookieFile = cookieFileFlag
	}
	if proxyFlag != "" {
		cfg.Proxy = proxyFlag
	}

	router := server.SetupRouter()

	modelKeys := make([]string, 0, len(models.Models))
	for k := range models.Models {
		modelKeys = append(modelKeys, k)
	}
	sort.Strings(modelKeys)

	cookieStatus := "none (anonymous)"
	if cfg.CookieFile != "" {
		cookieStatus = "yes"
	}

	proxyStatus := "system env"
	if cfg.Proxy != "" {
		proxyStatus = cfg.Proxy
	}

	tempStatus := "no"
	if cfg.TemporaryChats {
		tempStatus = "yes"
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	fmt.Printf("gemini-shim v%s\n", server.Version)
	fmt.Printf("  Listening: http://%s:%d\n", cfg.Host, cfg.Port)
	fmt.Printf("  Base URL:  http://localhost:%d/v1\n", cfg.Port)
	fmt.Printf("  Models:    %s\n", strings.Join(modelKeys, ", "))
	fmt.Printf("  Cookie:    %s\n", cookieStatus)
	fmt.Printf("  Proxy:     %s\n", proxyStatus)
	fmt.Printf("  Streaming: true SSE streaming\n")
	fmt.Printf("  Temporary: %s\n", tempStatus)
	fmt.Println()

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nStopped.")
}
