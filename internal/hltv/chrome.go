package hltv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-rod/rod/lib/launcher"

	"psr/internal/config"
)

// FindChromePath returns Google Chrome executable path on Windows, or any Chromium fallback.
func FindChromePath() (string, error) {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	}
	for _, c := range candidates {
		if c == "" || c == `\Google\Chrome\Application\chrome.exe` {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}

	if p, ok := launcher.LookPath(); ok {
		return p, nil
	}
	return "", fmt.Errorf("Google Chrome не найден — установите Chrome")
}

func newChromeLauncher(cfg config.Config, headless bool) (*launcher.Launcher, string, error) {
	bin, err := FindChromePath()
	if err != nil {
		return nil, "", err
	}
	l := launcher.New().
		Bin(bin).
		Headless(headless).
		Leakless(false).
		Set("no-first-run").
		Set("no-default-browser-check").
		Set("window-size", "1280,900").
		Set("lang", "en-US,en").
		Set("disable-infobars")

	profile := cfg.ChromeProfileDir
	if profile != "" {
		if err := os.MkdirAll(profile, 0o755); err == nil {
			l = l.UserDataDir(profile)
		}
	}
	return l, bin, nil
}
