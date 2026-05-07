package tools

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultMaxSearchResults = 10

// Pre-compiled regex patterns for web scraping.
var (
	reBingListItem   = regexp.MustCompile(`(?s)<li[^>]*class="[^"]*b_algo[^"]*"[^>]*>(.*?)</li>`)
	reBingLink       = regexp.MustCompile(`(?s)<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	reSnippetPara    = regexp.MustCompile(`(?s)<p[^>]*class="b_lineclamp[^"]*"[^>]*>(.*?)</p>`)
	reSnippetCaption = regexp.MustCompile(`(?s)<div[^>]*class="b_caption"[^>]*>(.*?)</div>`)
	reSnippetSnippet = regexp.MustCompile(`(?s)<div[^>]*class="b_snippet"[^>]*>(.*?)</div>`)
	re360ListItem    = regexp.MustCompile(`(?s)<li[^>]*class="[^"]*res-list[^"]*"[^>]*>(.*?)</li>`)
	re360Link        = regexp.MustCompile(`(?s)<[hH][234][^>]*>\s*<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	re360Snippet     = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	reAnyLink        = regexp.MustCompile(`<a[^>]+href="([^"]+)"[^>]*>([^<]*)</a>`)
)

// WebSearchTool searches the web using Bing (with 360 fallback for China).
type WebSearchTool struct{}

func (*WebSearchTool) Name() string { return "web_search_scraper" }
func (*WebSearchTool) Description() string {
	return "Search the web using Bing/360 HTML scraping. Fallback search when web_search fails."
}

func (*WebSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"count": map[string]any{
				"type":        "integer",
				"description": "Number of results (1-10, default: 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (*WebSearchTool) CheckPermissions(params map[string]any) PermissionResult {
	query, _ := params["query"].(string)
	if containsInternalURL(query) {
		return PermissionResultAsk("Search blocked: internal URL detected in query", "tool")
	}
	return PermissionResultPassthrough()
}

func (*WebSearchTool) Execute(params map[string]any) ToolResult {
	query, _ := params["query"].(string)
	if query == "" {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	count := defaultMaxSearchResults
	if c, ok := params["count"]; ok {
		switch v := c.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		}
	}
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := searchBing(ctx, query, count)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Search error: %v", err), IsError: true}
	}

	if len(results) == 0 {
		return ToolResult{Output: fmt.Sprintf("No results found for: %s", query)}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Search results for: %s\n", query))
	for i, r := range results {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, r.Title))
		lines = append(lines, fmt.Sprintf("   URL: %s", r.URL))
		if r.Snippet != "" {
			lines = append(lines, fmt.Sprintf("   %s", r.Snippet))
		}
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchBing(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&setmkt=en-US", url.QueryEscape(query))

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// Support proxy from environment variable
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			client.Transport = &http.Transport{
				Proxy:               http.ProxyURL(proxy),
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     30 * time.Second,
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	// Edge-like headers for better compatibility
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			return nil, fmt.Errorf("search timed out after 30s")
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle compressed responses
	reader := resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		dr, err := zlib.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("zlib reader: %w", err)
		}
		defer dr.Close()
		reader = dr
	}

	body, err := io.ReadAll(io.LimitReader(reader, 200000))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	html := string(body)

	// Parse Bing results
	results := parseBingResults(html, maxResults)

	// If Bing failed, try 360 search with its own HTML
	if len(results) == 0 {
		results360, err := search360(ctx, query, maxResults)
		if err == nil && len(results360) > 0 {
			return results360, nil
		}
	}

	// Last resort: extract any links
	if len(results) == 0 {
		results = extractAnyLinks(html, maxResults)
	}

	return results, nil
}

// parseBingResults extracts search results from Bing HTML.
func parseBingResults(html string, maxResults int) []SearchResult {
	var results []SearchResult

	// Check if b_algo is present at all
	if !strings.Contains(html, "b_algo") {
		return nil
	}

	// Bing structure: <li class="b_algo">...</li> - be flexible with attribute ordering
	matches := reBingListItem.FindAllStringSubmatch(html, -1)


	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}

		block := match[1]

		linkMatch := reBingLink.FindStringSubmatch(block)
		if linkMatch == nil {
			continue
		}

		rawURL := linkMatch[1]
		title := stripTags(linkMatch[2])
		title = strings.TrimSpace(decodeHTMLEntities(title))
		if title == "" {
			continue
		}

		if strings.Contains(rawURL, "bing.com/ck") {
			rawURL = resolveBingURL(rawURL)
		}
		if rawURL == "" || strings.HasPrefix(rawURL, "/") {
			continue
		}

		snippet := extractBingSnippet(block)
		snippet = strings.TrimSpace(decodeHTMLEntities(snippet))

		results = append(results, SearchResult{
			Title:   title,
			URL:     rawURL,
			Snippet: snippet,
		})
	}

	return results
}

// extractBingSnippet extracts the snippet from a Bing result block.
func extractBingSnippet(block string) string {
	for _, re := range []*regexp.Regexp{reSnippetPara, reSnippetCaption, reSnippetSnippet} {
		if m := re.FindStringSubmatch(block); m != nil {
			return stripTags(m[1])
		}
	}
	return ""
}

// search360 fetches and parses results from 360 search (so.com).
func search360(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://www.so.com/s?q=%s", url.QueryEscape(query))

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 80000))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return parse360Results(string(body), maxResults), nil
}

// parse360Results extracts search results from 360 search HTML (fallback).
func parse360Results(html string, maxResults int) []SearchResult {
	var results []SearchResult

	matches := re360ListItem.FindAllStringSubmatch(html, -1)

	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}

		block := match[1]

		linkMatch := re360Link.FindStringSubmatch(block)
		if linkMatch == nil {
			continue
		}

		rawURL := linkMatch[1]
		if strings.Contains(rawURL, "so.com") || strings.HasPrefix(rawURL, "/") {
			continue
		}

		title := stripTags(linkMatch[2])
		title = decodeHTMLEntities(title)
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}

		snippet := ""
		if sm := re360Snippet.FindStringSubmatch(block); sm != nil {
			snippet = stripTags(sm[1])
			snippet = decodeHTMLEntities(snippet)
			snippet = strings.TrimSpace(snippet)
		}

		results = append(results, SearchResult{
			Title:   title,
			URL:     rawURL,
			Snippet: snippet,
		})
	}

	return results
}

// extractAnyLinks extracts any links from HTML as last resort.
func extractAnyLinks(html string, maxResults int) []SearchResult {
	var results []SearchResult

	matches := reAnyLink.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}
		rawURL := match[1]
		title := match[2]

		if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "#") || rawURL == "" {
			continue
		}
		if strings.Contains(rawURL, "bing.com") && !strings.Contains(rawURL, "url=") {
			continue
		}
		if seen[rawURL] {
			continue
		}
		seen[rawURL] = true

		title = strings.TrimSpace(title)
		if title != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     rawURL,
				Snippet: "",
			})
		}
	}

	return results
}

// resolveBingURL converts Bing redirect URL to actual URL.
func resolveBingURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "#") {
		return ""
	}

	if strings.Contains(rawURL, "u=") {
		if idx := strings.Index(rawURL, "u="); idx != -1 {
			encoded := rawURL[idx+2:]
			if ampIdx := strings.IndexAny(encoded, "&?"); ampIdx != -1 {
				encoded = encoded[:ampIdx]
			}
			if len(encoded) >= 3 {
				b64 := encoded[2:]
				b64 = strings.ReplaceAll(b64, "-", "+")
				b64 = strings.ReplaceAll(b64, "_", "/")
				for len(b64)%4 != 0 {
					b64 += "="
				}
				if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
					s := string(decoded)
					if strings.HasPrefix(s, "http") {
						return s
					}
				}
			}
		}
	}

	if !strings.Contains(rawURL, "bing.com") {
		return rawURL
	}

	return ""
}

func stripTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
