package browserai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

type SelectorCandidate struct {
	Selector string
	Text     string // Substring or text to match
}

type SelectorSet struct {
	PromptInputs    []SelectorCandidate
	SubmitButtons   []SelectorCandidate
	DownloadButtons []SelectorCandidate
	LoginMarkers    []SelectorCandidate
	LoadingMarkers  []SelectorCandidate
	ErrorMarkers    []SelectorCandidate
}

// Registry containing the candidates for each provider
var GeminiSelectors = SelectorSet{
	PromptInputs: []SelectorCandidate{
		{Selector: "div[contenteditable='true'][aria-label*='prompt']"},
		{Selector: "div[contenteditable='true'][aria-label*='Prompt']"},
		{Selector: "div[contenteditable='true'][aria-label*='nhập']"},
		{Selector: "div[contenteditable='true'][placeholder*='prompt']"},
		{Selector: "div[contenteditable='true']"},
		{Selector: "textarea"},
	},
	SubmitButtons: []SelectorCandidate{
		{Selector: "button[aria-label*='Send']"},
		{Selector: "button[aria-label*='Gửi']"},
		{Selector: "button[title*='Send']"},
		{Selector: "button[title*='Gửi']"},
		{Selector: "button.send-button"},
		{Selector: "button[aria-label*='Submit']"},
	},
	DownloadButtons: []SelectorCandidate{
		{Selector: "button[aria-label*='Download']"},
		{Selector: "button[aria-label*='Tải']"},
		{Selector: "button[title*='Download']"},
		{Selector: "button[title*='Tải']"},
		{Selector: "a[download]"},
	},
	LoginMarkers: []SelectorCandidate{
		{Selector: "a[href*='accounts.google.com']"},
		{Selector: "button:contains('Sign in')"},
		{Selector: "a:contains('Sign in')"},
		{Selector: "button:contains('Đăng nhập')"},
		{Selector: "a:contains('Đăng nhập')"},
	},
	LoadingMarkers: []SelectorCandidate{
		{Selector: "mat-progress-bar"},
		{Selector: "div.progress-bar"},
		{Selector: ".generating"},
		{Selector: ".typing"},
		{Selector: "div[aria-label*='loading']"},
	},
	ErrorMarkers: []SelectorCandidate{
		{Selector: "[role='alert']"},
		{Selector: ".error-message"},
	},
}

var FlowSelectors = SelectorSet{
	PromptInputs: []SelectorCandidate{
		{Selector: "//*[@id=\"__next\"]/div[1]/div[5]/div/div/div/div/div[1]/div"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div/div[1]/div"},
		{Selector: "textarea[placeholder*='prompt']"},
		{Selector: "textarea[placeholder*='Prompt']"},
		{Selector: "textarea"},
		{Selector: "div[contenteditable='true']"},
	},
	SubmitButtons: []SelectorCandidate{
		{Selector: "button[aria-label*='Gửi']"},
		{Selector: "button[aria-label*='Send']"},
		{Selector: "button[aria-label*='Submit']"},
		{Selector: "button[title*='Gửi']"},
		{Selector: "button[title*='Send']"},
		{Selector: "button:contains('Generate')"},
		{Selector: "button:contains('Tạo')"},
		{Selector: "button[type='submit']"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[5]/div/div/div/div/div[2]/div[2]/button[3]"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div/div[2]/div[2]/button[3]"},
	},
	DownloadButtons: []SelectorCandidate{
		{Selector: "button:contains('Download')"},
		{Selector: "button:contains('Tải')"},
		{Selector: "a[download]"},
	},
	LoginMarkers: []SelectorCandidate{
		{Selector: "a[href*='accounts.google.com']"},
		{Selector: "button:contains('Sign in')"},
		{Selector: "a:contains('Sign in')"},
	},
	LoadingMarkers: []SelectorCandidate{
		{Selector: ".loading-spinner"},
		{Selector: "div[aria-label*='rendering']"},
		{Selector: "div[aria-label*='processing']"},
	},
	ErrorMarkers: []SelectorCandidate{
		{Selector: ".error-toast"},
		{Selector: ".error-alert"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[1]/div[2]/div[2]/div/div/div[2]"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[1]/div[2]/div[2]/div/div/div[1]"},
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[1]/div[2]/div[2]/div/button"},
	},
}

// FindFirstVisible finds the first visible element matching candidates
func FindFirstVisible(
	ctx context.Context,
	page *rod.Page,
	candidates []SelectorCandidate,
	timeout time.Duration,
) (*rod.Element, error) {
	deadline := time.Now().Add(timeout)
	for {
		for _, cand := range candidates {
			var elements rod.Elements
			var err error
			if strings.HasPrefix(cand.Selector, "/") || strings.HasPrefix(cand.Selector, "(") {
				elements, err = page.ElementsX(cand.Selector)
			} else {
				elements, err = page.Elements(cand.Selector)
			}
			if err != nil {
				continue
			}
			for _, el := range elements {
				if visible, _ := el.Visible(); visible {
					if cand.Text != "" {
						txt, _ := el.Text()
						if !strings.Contains(strings.ToLower(txt), strings.ToLower(cand.Text)) {
							continue
						}
					}
					return el, nil
				}
			}
		}

		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("không tìm thấy phần tử hiển thị phù hợp trong số các selector")
}
