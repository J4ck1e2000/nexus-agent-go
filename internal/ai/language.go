package ai

import (
	"strings"
	"unicode"
)

type responseLanguage string

const (
	responseLanguageZH responseLanguage = "zh"
	responseLanguageEN responseLanguage = "en"
)

func detectResponseLanguage(text string) responseLanguage {
	for _, r := range strings.TrimSpace(text) {
		if unicode.Is(unicode.Han, r) {
			return responseLanguageZH
		}
	}
	return responseLanguageEN
}

func localizedText(lang responseLanguage, zhText, enText string) string {
	if lang == responseLanguageZH {
		return strings.TrimSpace(zhText)
	}
	return strings.TrimSpace(enText)
}

func localizedStatusText(lang responseLanguage, status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "online":
		return localizedText(lang, "在线", "online")
	case "offline":
		return localizedText(lang, "离线", "offline")
	default:
		return strings.TrimSpace(status)
	}
}

func localizedSeverityText(lang responseLanguage, severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "high":
		return localizedText(lang, "高", "high")
	case "medium":
		return localizedText(lang, "中", "medium")
	case "low":
		return localizedText(lang, "低", "low")
	default:
		return strings.TrimSpace(severity)
	}
}
