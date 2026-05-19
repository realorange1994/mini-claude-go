# Web Tools

> Web fetch, web search, URL expansion

## Sections Included
- [###] Line 11497-11527 -- ### 54.7 WebFetch Tool

---

## Content

### 54.7 WebFetch Tool

**Go**: `tools/web_fetch.go` (395 lines) · **Upstream**: `WebFetchTool/WebFetchTool.ts` (319 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Tool name** | `"web_fetch"` (line 20) | `WEB_FETCH_TOOL_NAME` | Go适配 |
| 2 | **Input schema** | `url` + `extractMode` (text/markdown/json) (line 25-39) | `url` + `prompt` (required prompt for LLM processing) (line 24-29) | 修正 |
| 3 | **Content processing** | HTML stripping (custom parser) or text mode (line 136-142) | LLM-based: `applyPromptToMarkdown(prompt, content)` (line 264-278) | 修正 |
| 4 | **Prompt-based extraction** | Not present — `extractMode` only | `prompt` parameter — sends content to LLM for extraction (line 271-278) | 缺失 |
| 5 | **Redirect handling** | Not present — follows redirects automatically | Detects cross-host redirects, returns redirect message asking to re-fetch (line 217-249) | 缺失 |
| 6 | **Preapproved hosts** | Not present | `isPreapprovedHost()` — auto-allow certain domains (line 111-118) | 缺失 |
| 7 | **Permission rule by domain** | Not present | `webFetchToolInputToPermissionRuleContent()` — `domain:hostname` (line 50-64) | 缺失 |
| 8 | **file:// URL blocking** | `CheckPermissions` blocks file:// (line 44-46) | Not checked (not needed — zod `z.string().url()` rejects file://) | Go适配 |
| 9 | **Internal URL blocking** | `containsInternalURL()` in CheckPermissions (line 47-49) | Not checked in WebFetch | Go增强 |
| 10 | **Max body size** | 1 MB `maxBodySize` (line 15) | No explicit limit — handled by `getURLMarkdownContent()` | Go适配 |
| 11 | **Compression support** | gzip + deflate decompression (line 110-126) | Handled by fetch utility | ✅ Match |
| 12 | **Proxy support** | `HTTP_PROXY` env var (line 76-79) | Not explicit in tool | Go增强 |
| 13 | **Custom User-Agent** | Full Chrome-like UA string (line 91) | Not visible in tool code — likely set in fetch utility | Go适配 |
| 14 | **JSON output mode** | `extractMode: "json"` — wraps as JSON object (line 155-156) | Not present — output always processed via LLM prompt | Go增强 |
| 15 | **Title/description extraction** | `extractHTMLTitle()` + `extractHTMLMeta()` (line 330-394) | Handled by markdown conversion utility | Go适配 |
| 16 | **Binary content persistence** | Not present | Saves binary content (PDFs, etc.) to disk, notes path in result (line 282-284) | 缺失 |
| 17 | **Markdown passthrough** | Not present | Preapproved markdown URLs < `MAX_MARKDOWN_LENGTH` pass through without LLM (line 265-269) | 缺失 |
| 18 | **Abort/cancellation** | Not present | `abortController.signal` passed through (line 214) | 缺失 |
| 19 | **isConcurrencySafe** | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 20 | **isReadOnly** | Not marked | `isReadOnly() { return true }` | 缺失 |
| 21 | **shouldDefer** | Not marked | `shouldDefer: true` (line 71) | 缺失 |
| 22 | **URL validation** | `url.Parse()` + `IsAbs()` (line 59-62) | Zod `z.string().url()` + `new URL()` check (line 193-200) | ✅ Match |

---


---

