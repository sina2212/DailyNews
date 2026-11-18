package digest

import (
	"fmt"
	"strings"
	"time"

	"github.com/yourname/ai-daily-news/internal/ai"
)

type Item struct {
	AI          ai.Result
	URL, Source string
}

type Message struct {
	Category string
	Text     string
}

var labels = map[string]string{
	"politics":   "سیاسی",
	"economy":    "اقتصادی",
	"football":   "فوتبال",
	"basketball": "بسکتبال",
	"formula1":   "فرمول یک",
	"technology": "فناوری",
	"other":      "سایر",
}

var categoryOrder = []string{"politics", "economy", "football", "basketball", "formula1", "technology", "other"}

// BuildMessages creates a separate Telegram message for every populated category.
// It chunks at article boundaries to stay safely below Telegram's 4096-character limit.
func BuildMessages(date time.Time, items []Item, maxChars int) []Message {
	if maxChars <= 0 || maxChars > 3900 {
		maxChars = 3900
	}
	groups := make(map[string][]Item)
	for _, it := range items {
		cat := it.AI.Category
		if _, ok := labels[cat]; !ok {
			cat = "other"
		}
		groups[cat] = append(groups[cat], it)
	}

	var messages []Message
	for _, cat := range categoryOrder {
		list := groups[cat]
		if len(list) == 0 {
			continue
		}
		header := fmt.Sprintf("📰 <b>خلاصه اخبار %s</b>\n📅 %s\n\n", labels[cat], date.Format("2006/01/02"))
		footer := "\n<i>خلاصه‌سازی خودکار است. برای جزئیات و راستی‌آزمایی، منبع اصلی را بخوانید.</i>"
		var body strings.Builder
		body.WriteString(header)
		n := 0

		flush := func() {
			if body.Len() == 0 || body.String() == header {
				return
			}
			body.WriteString(footer)
			messages = append(messages, Message{Category: cat, Text: body.String()})
			body.Reset()
			body.WriteString(header)
			n = 0
		}

		for _, it := range list {
			n++
			block := formatBlock(n, it, maxChars-len(header)-len(footer)-20)
			if body.Len()+len(block)+len(footer) > maxChars && body.Len() > len(header) {
				flush()
				n = 1
				block = formatBlock(n, it, maxChars-len(header)-len(footer)-20)
			}
			body.WriteString(block)
		}
		flush()
	}
	return messages
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func escapeAttr(s string) string {
	return strings.ReplaceAll(escape(s), "\"", "&quot;")
}

func formatBlock(number int, it Item, maxBlockBytes int) string {
	if maxBlockBytes < 64 {
		maxBlockBytes = 64
	}
	title := escapeTruncated(it.AI.TitleFA, 120)
	source := escapeTruncated(it.Source, 60)
	url := escapeAttr(truncateUTF8(it.URL, 240))
	prefix := fmt.Sprintf("%d) <b>%s</b>\n", number, title)
	suffix := fmt.Sprintf("\nمنبع: <a href=\"%s\">%s</a>\n\n", url, source)
	available := maxBlockBytes - len(prefix) - len(suffix)
	if available < 0 {
		available = 0
	}
	summary := escapeTruncated(it.AI.SummaryFA, available)
	return prefix + summary + suffix
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= len("…") {
		return ""
	}
	cut := maxBytes - len("…")
	for cut > 0 && cut < len(value) && (value[cut]&0xC0) == 0x80 {
		cut--
	}
	return value[:cut] + "…"
}

func escapeTruncated(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		piece := escape(string(r))
		if b.Len()+len(piece)+len("…") > maxBytes {
			if b.Len()+len("…") <= maxBytes {
				b.WriteString("…")
			}
			break
		}
		b.WriteString(piece)
	}
	return b.String()
}
