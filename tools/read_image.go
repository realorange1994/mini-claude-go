package tools

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// ImageReadTool reads and encodes images for Vision input.
type ImageReadTool struct{}

func (*ImageReadTool) Name() string        { return "read_image" }
func (*ImageReadTool) Description() string { return "Read an image file and encode it as base64 for Vision input. Use this when you need to analyze or describe an image. Returns the base64-encoded image data, media type, and file size." }

func (*ImageReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the image file (jpg, png, gif, webp).",
			},
			"detail": map[string]any{
				"type":        "string",
				"description": "Vision detail level: 'low', 'high', or 'auto' (default: auto).",
				"enum":        []string{"low", "high", "auto"},
			},
		},
		"required": []string{"path"},
	}
}

func (*ImageReadTool) CheckPermissions(params map[string]any) string { return "" }

func (*ImageReadTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: path is required", IsError: true}
	}

	detail, _ := params["detail"].(string)
	if detail == "" {
		detail = "auto"
	}

	fp := expandPath(pathStr)

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading image: %v", err), IsError: true}
	}

	mediaType := detectImageType(fp, data)
	if mediaType == "" {
		return ToolResult{Output: "Error: unsupported image format", IsError: true}
	}

	// 5MB limit
	if len(data) > 5*1024*1024 {
		return ToolResult{Output: fmt.Sprintf("Error: image too large: %d bytes (max 5MB)", len(data)), IsError: true}
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	tokenEst := estimateImageTokens(b64, detail)

	output := fmt.Sprintf("Image: %s\nType: %s\nSize: %d bytes\nEstimated tokens: %d\nBase64 length: %d chars",
		fp, mediaType, len(data), tokenEst, len(b64))

	return ToolResult{
		Output: output,
	}
}

func detectImageType(path string, data []byte) string {
	pathLower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(pathLower, ".jpg"), strings.HasSuffix(pathLower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(pathLower, ".png"):
		return "image/png"
	case strings.HasSuffix(pathLower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(pathLower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(pathLower, ".bmp"):
		return "image/bmp"
	case strings.HasSuffix(pathLower, ".svg"):
		return "image/svg+xml"
	}

	// Magic bytes
	if len(data) >= 4 {
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image/jpeg"
		}
		if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image/png"
		}
		if len(data) >= 6 && string(data[:3]) == "GIF" {
			return "image/gif"
		}
		if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
			data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return "image/webp"
		}
	}
	return ""
}

func estimateImageTokens(b64 string, detail string) int {
	if detail == "low" {
		return 85 // Fixed cost for low detail
	}
	// High/auto: estimate based on base64 length
	// Rough heuristic: ~1 token per ~4 base64 chars for high detail
	chars := len(b64)
	tokens := chars / 4
	if tokens < 85 {
		tokens = 85
	}
	if tokens > 6000 {
		tokens = 6000 // Cap at reasonable max
	}
	return tokens
}

// GetImageBase64 extracts base64 data and media type from an image read result.
// NOTE: This function is not fully implemented - it only extracts the media type.
// To get base64 data, re-read the file and encode it directly.
func GetImageBase64(result ToolResult) (b64, mediaType string) {
	if result.IsError {
		return "", ""
	}
	lines := strings.Split(result.Output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Type: ") {
			mediaType = strings.TrimPrefix(line, "Type: ")
		}
	}
	return "", mediaType
}
