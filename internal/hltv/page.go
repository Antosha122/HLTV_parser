package hltv

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// newPage opens a browser tab. Visible mode uses a plain page — stealth triggers Cloudflare loops.
func newPage(browser *rod.Browser, headless bool) (*rod.Page, error) {
	if headless {
		return stealth.Page(browser)
	}
	return browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
}
