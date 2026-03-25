package weixin

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func isMediaItemType(t int) bool {
	switch t {
	case messageItemImage, messageItemVoice, messageItemFile, messageItemVideo:
		return true
	default:
		return false
	}
}

func bodyFromItemList(items []messageItem) string {
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items)*2)
	for _, item := range items {
		text := itemText(item)
		ref := quotedRef(item.RefMsg)
		if ref != "" {
			lines = append(lines, ref)
		}
		if text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func itemText(item messageItem) string {
	switch item.Type {
	case messageItemText:
		if item.TextItem == nil {
			return ""
		}
		return strings.TrimSpace(item.TextItem.Text)
	case messageItemVoice:
		if item.VoiceItem == nil {
			return ""
		}
		return strings.TrimSpace(item.VoiceItem.Text)
	default:
		return ""
	}
}

func quotedRef(ref *refMessage) string {
	if ref == nil {
		return ""
	}
	var parts []string
	if title := strings.TrimSpace(ref.Title); title != "" {
		parts = append(parts, title)
	}
	if ref.MessageItem != nil {
		if refBody := strings.TrimSpace(itemText(*ref.MessageItem)); refBody != "" {
			parts = append(parts, refBody)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("[引用: %s]", strings.Join(parts, " | "))
}

func mediaOnlyItems(items []messageItem) bool {
	hasMedia := false
	for _, item := range items {
		switch item.Type {
		case messageItemText:
			if strings.TrimSpace(itemText(item)) != "" {
				return false
			}
		case messageItemVoice:
			if strings.TrimSpace(itemText(item)) != "" {
				return false
			}
			hasMedia = true
		case messageItemImage, messageItemFile, messageItemVideo:
			hasMedia = true
		}
	}
	return hasMedia
}

func splitUTF8(s string, maxRunes int) []string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return []string{s}
	}
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		n := maxRunes
		if len(runes) < n {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}
