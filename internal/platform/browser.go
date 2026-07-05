package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenChrome opens URL in Google Chrome (Windows) or default browser elsewhere.
func OpenChrome(url string) error {
	return OpenChromeWithProfile(url, "", 0)
}

const defaultChromeDebugPort = 9222

// OpenChromeWithProfile opens Chrome with PSR profile and remote-debugging port for later attach.
func OpenChromeWithProfile(url, profileDir string, debugPort int) error {
	if runtime.GOOS == "windows" {
		if chrome := findChrome(); chrome != "" {
			args := []string{
				"--new-window",
				"--no-first-run",
				"--disable-infobars",
				"--start-maximized",
			}
			if debugPort <= 0 && profileDir != "" {
				debugPort = defaultChromeDebugPort
			}
			if debugPort > 0 {
				args = append(args, fmt.Sprintf("--remote-debugging-port=%d", debugPort))
			}
			if profileDir != "" {
				abs, err := filepath.Abs(profileDir)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(abs, 0o755); err != nil {
					return err
				}
				args = append([]string{"--user-data-dir=" + abs}, args...)
			}
			args = append(args, url)
			return exec.Command(chrome, args...).Start()
		}
	}
	if profileDir != "" {
		return fmt.Errorf("chrome не найден — установите Google Chrome")
	}
	return OpenBrowser(url)
}

func findChrome() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
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
			return c
		}
	}
	return ""
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if chrome := findChrome(); chrome != "" {
			return exec.Command(chrome, "--new-window", url).Start()
		}
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
