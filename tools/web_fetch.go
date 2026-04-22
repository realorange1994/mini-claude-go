package tools

import (
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxBodySize = 1 << 20 // 1MB

// WebFetchTool fetches and extracts readable content from URLs.
type WebFetchTool struct{}

func (*WebFetchTool) Name() string { return "web_fetch" }
func (*WebFetchTool) Description() string {
	return "Fetch a URL and extract readable text content. Strips HTML, removes scripts/styles, extracts title and meta description."
}

func (*WebFetchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The URL to fetch (must include http:// or https://).",
			},
			"extractMode": map[string]any{
				"type":        "string",
				"description": "Extraction mode: 'text', 'markdown', or 'json' (default: markdown).",
			},
		},
		"required": []string{"url"},
	}
}

func (*WebFetchTool) CheckPermissions(params map[string]any) string {
	rawURL, _ := params["url"].(string)
	if strings.HasPrefix(rawURL, "file://") {
		return "Blocked: file:// URLs are not allowed"
	}
	if containsInternalURL(rawURL) {
		return "Blocked: internal/private URLs are not allowed"
	}
	return ""
}

func (*WebFetchTool) Execute(params map[string]any) ToolResult {
	rawURL, _ := params["url"].(string)
	if rawURL == "" {
		return ToolResult{Output: "Error: url is required", IsError: true}
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() {
		return ToolResult{Output: fmt.Sprintf("Error: invalid URL: %s", rawURL), IsError: true}
	}

	extractMode := "markdown"
	if m, ok := params["extractMode"].(string); ok && m != "" {
		extractMode = m
	}

	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	}

	// Support proxy from environment variable
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if proxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: create request: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			return ToolResult{Output: "Error: request timed out after 30s", IsError: true}
		}
		return ToolResult{Output: fmt.Sprintf("Error: fetch failed: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ToolResult{Output: fmt.Sprintf("Error: HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode)), IsError: true}
	}

	// Handle compressed responses
	reader := resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gr, err := gzip.NewReader(reader)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: gzip reader: %v", err), IsError: true}
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		dr, err := zlib.NewReader(reader)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: zlib reader: %v", err), IsError: true}
		}
		defer dr.Close()
		reader = dr
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxBodySize))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: read body: %v", err), IsError: true}
	}

	contentType := resp.Header.Get("Content-Type")
	content := string(body)

	if strings.Contains(contentType, "html") || strings.Contains(contentType, "text") {
		if extractMode == "text" {
			content = stripHTMLSimple(content)
		} else {
			content = extractTextFromHTML(content)
		}
	}

	title := extractHTMLTitle(string(body))
	description := extractHTMLMeta(string(body), "description")

	var result strings.Builder
	if title != "" {
		result.WriteString("Title: " + title + "\n\n")
	}
	if description != "" {
		result.WriteString("Description: " + description + "\n\n")
	}

	if extractMode == "json" {
		result.WriteString(fmt.Sprintf(`{"url": %q, "content": %q, "content_type": %q}`, rawURL, content, contentType))
	} else {
		result.WriteString("--- Content ---\n")
		result.WriteString(content)
	}

	// Truncate if too large
	if result.Len() > maxBodySize {
		out := result.String()
		out = out[:maxBodySize/2] + fmt.Sprintf("\n\n... (%d chars truncated) ...\n\n", result.Len()-maxBodySize) + out[result.Len()-maxBodySize/2:]
		return ToolResult{Output: out}
	}

	return ToolResult{Output: result.String()}
}

func stripHTMLSimple(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			result.WriteString(" ")
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

func extractTextFromHTML(html string) string {
	var result strings.Builder
	inTag := false
	inScript := false
	inStyle := false
	var tagDepth int

	for i := 0; i < len(html); i++ {
		c := html[i]

		if i+7 < len(html) && strings.ToLower(string(html[i:i+7])) == "<script" {
			inScript = true
			tagDepth++
			i += 6
			continue
		}
		if i+6 < len(html) && strings.ToLower(string(html[i:i+6])) == "</script" {
			inScript = false
			tagDepth--
			i += 5
			continue
		}
		if i+6 < len(html) && strings.ToLower(string(html[i:i+6])) == "<style" {
			inStyle = true
			tagDepth++
			i += 5
			continue
		}
		if i+7 < len(html) && strings.ToLower(string(html[i:i+7])) == "</style" {
			inStyle = false
			tagDepth--
			i += 6
			continue
		}

		if inScript || inStyle || tagDepth > 0 {
			continue
		}

		if c == '<' {
			inTag = true
			result.WriteString(" ")
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			result.WriteByte(c)
		}
	}

	text := result.String()

	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}

func stripTag(html, tag string) string {
	open := "<" + tag
	closeTag := "</" + tag + ">"
	for {
		start := strings.Index(strings.ToLower(html), open)
		if start == -1 {
			break
		}
		tagEnd := strings.Index(html[start:], ">")
		if tagEnd == -1 {
			break
		}
		closePos := strings.Index(strings.ToLower(html[start+tagEnd+1:]), closeTag)
		if closePos == -1 {
			html = html[:start] + html[start+tagEnd+1:]
		} else {
			html = html[:start] + html[start+tagEnd+1+closePos+len(closeTag):]
		}
	}
	return html
}

func stripHTML(html string) string {
	html = stripTag(html, "script")
	html = stripTag(html, "style")
	html = stripTag(html, "nav")
	html = stripTag(html, "header")
	html = stripTag(html, "footer")
	html = stripTag(html, "aside")

	for {
		start := strings.Index(html, "<!--")
		if start == -1 {
			break
		}
		end := strings.Index(html[start+4:], "-->")
		if end == -1 {
			break
		}
		html = html[:start] + html[start+4+end+3:]
	}

	var sb strings.Builder
	inTag := false
	spaceCount := 0
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			if spaceCount > 0 {
				sb.WriteRune(' ')
				spaceCount = 0
			}
			continue
		}
		if inTag {
			continue
		}
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			spaceCount++
			if spaceCount > 1 {
				continue
			}
		} else {
			spaceCount = 0
		}
		sb.WriteRune(r)
	}

	result := strings.TrimSpace(sb.String())
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return result
}

func extractHTMLTitle(html string) string {
	if idx := strings.Index(strings.ToLower(html), "<title"); idx != -1 {
		start := strings.Index(html[idx:], ">")
		if start != -1 {
			content := html[idx+start+1:]
			end := strings.Index(content, "<")
			if end != -1 {
				return strings.TrimSpace(content[:end])
			}
		}
	}
	if idx := strings.Index(strings.ToLower(html), "<h1"); idx != -1 {
		start := strings.Index(html[idx:], ">")
		if start != -1 {
			content := html[idx+start+1:]
			end := strings.Index(content, "<")
			if end != -1 {
				return strings.TrimSpace(content[:end])
			}
		}
	}
	return ""
}

func extractHTMLMeta(html string, name string) string {
	lower := strings.ToLower(html)
	searches := []string{
		fmt.Sprintf(`name="%s"`, name),
		fmt.Sprintf(`property="%s"`, name),
	}
	var attrPos int
	var searchPat string
	for _, pat := range searches {
		p := strings.Index(lower, pat)
		if p != -1 {
			attrPos = p
			searchPat = pat
			break
		}
	}
	if searchPat == "" {
		return ""
	}

	tagClose := strings.Index(html[attrPos:], ">")
	if tagClose == -1 {
		return ""
	}
	rest := html[attrPos : attrPos+tagClose]

	for _, delim := range []string{`"`, `'`} {
		pat := "content=" + delim
		p := strings.Index(rest, pat)
		if p == -1 {
			continue
		}
		p += len(pat)
		closeQuote := strings.Index(rest[p:], string(delim))
		if closeQuote == -1 {
			continue
		}
		return strings.TrimSpace(rest[p : p+closeQuote])
	}
	return ""
}
