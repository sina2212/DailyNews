package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL, APIKey, Model string
	HTTP                   *http.Client
}
type Article struct{ Title, Text, Category, URL string }
type Result struct {
	TitleFA    string   `json:"title_fa"`
	SummaryFA  string   `json:"summary_fa"`
	Importance int      `json:"importance"`
	Category   string   `json:"category"`
	KeyPoints  []string `json:"key_points"`
}

func (c *Client) Summarize(ctx context.Context, a Article) (Result, error) {
	prompt := `خبر زیر را به فارسی روان، بی‌طرف و دقیق خلاصه کن.
فقط یک JSON معتبر برگردان با فیلدهای:
- title_fa (عنوان کوتاه فارسی)
- summary_fa (خلاصه خبر حداکثر ۷۰ کلمه، ۲ تا ۴ جمله)
- importance (عدد صحیح ۱ تا ۱۰)
- category (دقیقاً یکی از این‌ها: politics, economy, football, basketball, formula1, technology, other)
- key_points (آرایه حداکثر ۳ نکته کلیدی)

قوانین:
- چیزی که در متن خبر نیست را جعل نکن.
- اگر خبر ورزشی است، حتماً بین football (فوتبال)، basketball (بسکتبال) و formula1 (فرمول یک) انتخاب کن؛ sports عمومی استفاده نکن.
- اگر متن ناکافی است، در خلاصه صریح بگو.
- summary_fa حتماً کمتر یا مساوی ۷۰ کلمه باشد.

عنوان: ` + a.Title + `
دسته اولیه: ` + a.Category + `
متن: ` + a.Text + `
منبع: ` + a.URL
	body := map[string]any{"model": c.Model, "temperature": 0.2, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]string{"role": "system", "content": "You are a careful Persian-language news editor. Always reply with valid JSON only."}, map[string]string{"role": "user", "content": prompt}}}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("AI API status: %s", resp.Status)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Result{}, err
	}
	if len(envelope.Choices) == 0 {
		return Result{}, fmt.Errorf("AI returned no choices")
	}
	var result Result
	if err = json.Unmarshal([]byte(envelope.Choices[0].Message.Content), &result); err != nil {
		return Result{}, err
	}
	if result.Importance < 1 {
		result.Importance = 1
	}
	if result.Importance > 10 {
		result.Importance = 10
	}
	switch result.Category {
	case "politics", "economy", "football", "basketball", "formula1", "technology", "other":
		// Keep the model's valid category.
	default:
		result.Category = a.Category
	}
	return result, nil
}
