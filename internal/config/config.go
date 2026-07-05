package config

import (
	"os"
	"strconv"
	"time"
)

const (
	DefaultBaseURL      = "https://www.hltv.org"
	DefaultRequestDelay = 2 * time.Second
	DefaultHumanMin     = 0 * time.Second
	DefaultHumanMax     = 1 * time.Second
	DefaultTimeout      = 90 * time.Second
	DefaultAPIAddr      = ":8080"
	DefaultWeightsPath  = "data/weights.json"
	DefaultCookiePath      = "data/hltv.cookie"
	DefaultChromeProfile   = "data/chrome-profile"
	DefaultChromeDebugPort = 0 // 0 = no CDP, use HTTP+cookie (Cloudflare-safe)
)

type Config struct {
	BaseURL      string
	RequestDelay  time.Duration
	HumanDelayMin time.Duration
	HumanDelayMax time.Duration
	Timeout       time.Duration
	DBPath       string
	APIAddr      string
	WeightsPath  string
	CookiePath       string
	ChromeProfileDir string
	ChromeDebugPort  int
	// Cookie from browser session — helps bypass Cloudflare when set.
	Cookie string
	// Headless runs Chrome without a window. Cloudflare often blocks headless; default is visible.
	Headless bool
	// AutoChrome tries to pass Cloudflare via automated Chrome. Unreliable — use manual cookie instead.
	AutoChrome bool
	// UseBrowser enables headless Chrome via rod when HTTP gets blocked.
	UseBrowser bool
}

func Load() Config {
	cfg := Config{
		BaseURL:      envOr("HLTV_BASE_URL", DefaultBaseURL),
		RequestDelay:  durationEnv("HLTV_REQUEST_DELAY", DefaultRequestDelay),
		HumanDelayMin: durationEnv("HLTV_HUMAN_DELAY_MIN", DefaultHumanMin),
		HumanDelayMax: durationEnv("HLTV_HUMAN_DELAY_MAX", DefaultHumanMax),
		Timeout:       durationEnv("HLTV_TIMEOUT", DefaultTimeout),
		DBPath:       envOr("PSR_DB_PATH", "data/psr.db"),
		APIAddr:      envOr("PSR_API_ADDR", DefaultAPIAddr),
		WeightsPath:  envOr("PSR_WEIGHTS_PATH", DefaultWeightsPath),
		CookiePath:       envOr("PSR_COOKIE_PATH", DefaultCookiePath),
		ChromeProfileDir: envOr("PSR_CHROME_PROFILE", DefaultChromeProfile),
		ChromeDebugPort:  intEnv("PSR_CHROME_DEBUG_PORT", DefaultChromeDebugPort),
		Cookie:           os.Getenv("HLTV_COOKIE"),
		Headless:         envBool("HLTV_HEADLESS", false),
		AutoChrome:       envBool("HLTV_AUTO_CHROME", false),
		UseBrowser:       envBool("HLTV_USE_BROWSER", false),
	}
	// HTTP+cookie: короче паузы (без CDP-режима).
	if cfg.ChromeDebugPort <= 0 {
		if os.Getenv("HLTV_REQUEST_DELAY") == "" {
			cfg.RequestDelay = 1500 * time.Millisecond
		}
		if os.Getenv("HLTV_HUMAN_DELAY_MIN") == "" {
			cfg.HumanDelayMin = 0
		}
		if os.Getenv("HLTV_HUMAN_DELAY_MAX") == "" {
			cfg.HumanDelayMax = 0
		}
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return time.Duration(sec) * time.Second
}

func intEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
