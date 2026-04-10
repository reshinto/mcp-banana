// Package policy provides model recommendation logic for selecting the
// appropriate image generation model based on task description and priority.
package policy

import "strings"

const (
	modelPro      = "nano-banana-pro"
	modelBalanced = "nano-banana-2"
	modelOriginal = "nano-banana-original"

	prioritySpeed    = "speed"
	priorityQuality  = "quality"
	priorityBalanced = "balanced"
)

var proKeywords = []string{
	"professional", "photorealistic", "detailed", "complex", "final",
}

var speedKeywords = []string{
	"quick", "draft", "sketch", "iterate", "batch", "preview",
}

// Alternative represents a model alternative with a tradeoff explanation.
type Alternative struct {
	Model    string `json:"model"`
	Tradeoff string `json:"tradeoff"`
}

// Recommendation is the result of a model selection decision.
type Recommendation struct {
	RecommendedModel string        `json:"recommended_model"`
	Reason           string        `json:"reason"`
	Alternatives     []Alternative `json:"alternatives"`
}

// alternativesFor returns the ordered list of alternatives for a given recommended model.
// The recommended model is never included in the alternatives list.
func alternativesFor(recommendedModel string) []Alternative {
	switch recommendedModel {
	case modelPro:
		return []Alternative{
			{Model: modelBalanced, Tradeoff: "faster, lower cost"},
			{Model: modelOriginal, Tradeoff: "fastest, basic quality"},
		}
	case modelOriginal:
		return []Alternative{
			{Model: modelBalanced, Tradeoff: "better quality, moderate speed"},
			{Model: modelPro, Tradeoff: "best quality, slowest"},
		}
	default: // modelBalanced
		return []Alternative{
			{Model: modelPro, Tradeoff: "higher quality, slower"},
			{Model: modelOriginal, Tradeoff: "faster, lower quality"},
		}
	}
}

// Recommend selects the most appropriate image generation model based on the
// task description and priority. The priority parameter accepts "speed",
// "quality", "balanced", or "" (treated as "balanced"). Any unrecognized
// priority value is silently normalized to "balanced". Keyword matching in
// taskDescription is case-insensitive.
func Recommend(taskDescription string, priority string) Recommendation {
	normalizedPriority := strings.ToLower(strings.TrimSpace(priority))
	if normalizedPriority != prioritySpeed && normalizedPriority != priorityQuality {
		normalizedPriority = priorityBalanced
	}

	switch normalizedPriority {
	case prioritySpeed:
		return Recommendation{
			RecommendedModel: modelOriginal,
			Reason:           "speed priority selected: nano-banana-original provides the fastest generation time",
			Alternatives:     alternativesFor(modelOriginal),
		}
	case priorityQuality:
		return Recommendation{
			RecommendedModel: modelPro,
			Reason:           "quality priority selected: nano-banana-pro provides the highest output quality",
			Alternatives:     alternativesFor(modelPro),
		}
	default:
		return recommendBalanced(taskDescription)
	}
}

// recommendBalanced applies keyword-based selection for the balanced priority path.
// Pro keywords are checked before speed keywords; first match wins.
func recommendBalanced(taskDescription string) Recommendation {
	lowerDesc := strings.ToLower(taskDescription)

	for _, keyword := range proKeywords {
		if strings.Contains(lowerDesc, keyword) {
			return Recommendation{
				RecommendedModel: modelPro,
				Reason:           "balanced priority with quality keyword \"" + keyword + "\": nano-banana-pro selected for high-fidelity output",
				Alternatives:     alternativesFor(modelPro),
			}
		}
	}

	for _, keyword := range speedKeywords {
		if strings.Contains(lowerDesc, keyword) {
			return Recommendation{
				RecommendedModel: modelOriginal,
				Reason:           "balanced priority with speed keyword \"" + keyword + "\": nano-banana-original selected for fast generation",
				Alternatives:     alternativesFor(modelOriginal),
			}
		}
	}

	return Recommendation{
		RecommendedModel: modelBalanced,
		Reason:           "balanced priority with no specific keywords: nano-banana-2 selected as the default balanced model",
		Alternatives:     alternativesFor(modelBalanced),
	}
}
