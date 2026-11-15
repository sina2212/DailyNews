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

var labels = map[string]string{
	"politics":   "سیاسی",
	"economy":    "اقتصادی",
	"sports":     "ورزشی",
	"technology": "فناوری",
	"other":      "سایر",
}

func Build(date time.Time, items []Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "📰 <b>خلاصه اخبار روز</b>\n📅 %s\n\n", date.Format("2006/01/02"))
	if len(items) == 0 {
		return "📰 <b>خلاصه اخبار روز</b>\nبرای روز موردنظر خبر قابل‌پردازشی پیدا نشد."
	}

	groups := map[string][]Item{}
	order := []string{"politics", "economy", "sports", "technology", "other"}
	for _, it := range items {
		cat := it.AI.Category
		if _, ok := labels[cat]; !ok {
			cat = "other"
		}
		groups[cat] = append(groups[cat], it)
	}

	n := 0
	for _, cat := range order {
		list := groups[cat]
		if len(list) == 0 {
			continue
		}
		fmt.Fprintf(&b, "<b>━━ %s ━━</b>\n\n", labels[cat])
		for _, it := range list {
			n++
			// قالب درخواستی کاربر:
			// ۱) متن خلاصه (حداکثر ۷۰ کلمه)
			// ۲) دسته‌بندی
			// ۳) منبع
			fmt.Fprintf(&b, "%d) %s\n", n, escape(it.AI.SummaryFA))
			fmt.Fprintf(&b, "دسته: %s\n", labels[cat])
			fmt.Fprintf(&b, "منبع: <a href=\"%s\">%s</a>\n\n", escapeAttr(it.URL), escape(it.Source))
		}
	}

	b.WriteString("<i>خلاصه‌سازی خودکار است. برای جزئیات و راستی‌آزمایی منبع اصلی را بخوانید.</i>")
	return b.String()
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func escapeAttr(s string) string {
	return strings.ReplaceAll(escape(s), "\"", "&quot;")
}
