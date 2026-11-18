package digest

import (
	"strings"
	"testing"
	"time"

	"github.com/yourname/ai-daily-news/internal/ai"
)

func TestBuildMessagesSeparatesSportsCategories(t *testing.T) {
	items := []Item{
		{AI: ai.Result{TitleFA: "فوتبال", SummaryFA: "خبر فوتبال", Category: "football"}, URL: "https://example.com/f", Source: "Football"},
		{AI: ai.Result{TitleFA: "بسکتبال", SummaryFA: "خبر بسکتبال", Category: "basketball"}, URL: "https://example.com/b", Source: "Basketball"},
		{AI: ai.Result{TitleFA: "فرمول یک", SummaryFA: "خبر فرمول یک", Category: "formula1"}, URL: "https://example.com/f1", Source: "F1"},
	}
	messages := BuildMessages(time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC), items, 3900)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	want := []string{"football", "basketball", "formula1"}
	for i, category := range want {
		if messages[i].Category != category {
			t.Fatalf("message %d: expected %s, got %s", i, category, messages[i].Category)
		}
	}
}

func TestBuildMessagesChunksLongCategory(t *testing.T) {
	items := make([]Item, 0, 10)
	for i := 0; i < 10; i++ {
		items = append(items, Item{
			AI:  ai.Result{TitleFA: "عنوان خبر", SummaryFA: strings.Repeat("متن خبر ", 30), Category: "football"},
			URL: "https://example.com/article", Source: "Publisher",
		})
	}
	messages := BuildMessages(time.Now(), items, 700)
	if len(messages) < 2 {
		t.Fatalf("expected category to be split into multiple messages, got %d", len(messages))
	}
	for _, message := range messages {
		if len(message.Text) > 700 {
			t.Fatalf("message exceeded configured byte safety limit: %d", len(message.Text))
		}
	}
}
