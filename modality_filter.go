package main

import (
	"fmt"
	"strings"
)

// ─── Image/Modality Filter (MiMo-Code 5) ───────────────────────────────────
//
// Model-aware media filtering that checks modality capabilities.
// Replaces unsupported attachments with descriptive error text.
//
// MiMo-Code source: provider/transform.ts (331-413 lines)

const (
	MaxPromptImages    = 20
	MaxPromptImageSize = 10 * 1024 * 1024 // 10MB
)

// Modality represents an input modality.
type Modality string

const (
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

// ModelCapabilities defines what a model can accept.
type ModelCapabilitiesInput struct {
	SupportsImages  bool
	SupportsAudio   bool
	SupportsVideo   bool
	SupportsPDF     bool
	MaxImages       int
	MaxImageSize    int
}

// GetDefaultCapabilities returns default capabilities for a model.
func GetDefaultCapabilities(modelID string) ModelCapabilitiesInput {
	// Claude models support images
	if strings.Contains(modelID, "claude") {
		return ModelCapabilitiesInput{
			SupportsImages: true,
			SupportsAudio:  false,
			SupportsVideo:  false,
			SupportsPDF:    true,
			MaxImages:      MaxPromptImages,
			MaxImageSize:   MaxPromptImageSize,
		}
	}

	// GPT models support images
	if strings.Contains(modelID, "gpt-4") || strings.Contains(modelID, "gpt-4o") {
		return ModelCapabilitiesInput{
			SupportsImages: true,
			SupportsAudio:  false,
			SupportsVideo:  false,
			MaxImages:      MaxPromptImages,
			MaxImageSize:   MaxPromptImageSize,
		}
	}

	// Default: text only
	return ModelCapabilitiesInput{
		SupportsImages: false,
		SupportsAudio:  false,
		SupportsVideo:  false,
		MaxImages:      0,
		MaxImageSize:   0,
	}
}

// FilterUnsupportedParts replaces unsupported media with error text.
func FilterUnsupportedParts(parts []ContentPart, caps ModelCapabilitiesInput) []ContentPart {
	var result []ContentPart
	for _, part := range parts {
		switch {
		case part.Type == "image" && !caps.SupportsImages:
			result = append(result, ContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Unsupported: image attachment (%s)]", part.MimeType),
			})
		case part.Type == "audio" && !caps.SupportsAudio:
			result = append(result, ContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Unsupported: audio attachment (%s)]", part.MimeType),
			})
		case part.Type == "video" && !caps.SupportsVideo:
			result = append(result, ContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Unsupported: video attachment (%s)]", part.MimeType),
			})
		case part.Type == "pdf" && !caps.SupportsPDF:
			result = append(result, ContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Unsupported: PDF attachment (%s)]", part.MimeType),
			})
		default:
			result = append(result, part)
		}
	}
	return result
}

// LimitImages enforces image count and size limits.
func LimitImages(parts []ContentPart, maxImages int, maxSize int) []ContentPart {
	if maxImages <= 0 {
		maxImages = MaxPromptImages
	}
	if maxSize <= 0 {
		maxSize = MaxPromptImageSize
	}

	var images []ContentPart
	var other []ContentPart

	for _, part := range parts {
		if part.Type == "image" {
			images = append(images, part)
		} else {
			other = append(other, part)
		}
	}

	// Drop oldest excess images
	if len(images) > maxImages {
		dropped := len(images) - maxImages
		images = images[dropped:]
		other = append([]ContentPart{{
			Type: "text",
			Text: fmt.Sprintf("[%d older images dropped to fit limit]", dropped),
		}}, other...)
	}

	// Check image sizes
	var filtered []ContentPart
	for _, img := range images {
		if img.Size > maxSize {
			filtered = append(filtered, ContentPart{
				Type: "text",
				Text: fmt.Sprintf("[Image too large: %d bytes, max %d bytes]", img.Size, maxSize),
			})
		} else {
			filtered = append(filtered, img)
		}
	}

	return append(filtered, other...)
}

// ContentPart represents a content part for filtering.
type ContentPart struct {
	Type     string
	Text     string
	MimeType string
	Size     int
}

// FormatModalityError formats a modality error for display.
func FormatModalityError(modality Modality, model string) string {
	return fmt.Sprintf("Model %s does not support %s input.", model, modality)
}
