package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/yourname/ai-daily-news/internal/ai"
	"github.com/yourname/ai-daily-news/internal/collector"
	"github.com/yourname/ai-daily-news/internal/config"
	"github.com/yourname/ai-daily-news/internal/digest"
	"github.com/yourname/ai-daily-news/internal/telegram"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Load .env from current dir and parent
	loaded := loadDotEnv(".env")
	if !loaded {
		loaded = loadDotEnv("../../.env")
	}
	if loaded {
		fmt.Println("✅ فایل .env خوانده شد")
	} else {
		fmt.Println("⚠️  فایل .env پیدا نشد (در مسیر فعلی یا پوشه بالاتر)")
	}

	// Debug: show which important keys are present (masked)
	printKeyStatus("AI_API_KEY")
	printKeyStatus("TELEGRAM_BOT_TOKEN")
	printKeyStatus("TELEGRAM_CHAT_ID")
	printKeyStatus("DATABASE_URL")

	if _, err := config.Load(); err != nil {
		slog.Warn("config incomplete (expected for dry test)", "error", err)
	}

	dbURL := envOr("DATABASE_URL", "postgres://postgres:Sina2212**@127.0.0.1:5432/ai_daily_news?sslmode=disable")
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = db.PingContext(ctx); err != nil {
		fatal(err)
	}
	fmt.Println("✅ Connected to PostgreSQL")

	// Run migrations
	b, err := os.ReadFile("../../migrations/001_init.sql")
	if err != nil {
		b, err = os.ReadFile("../../migrations/001_init.sql")
	}
	if err != nil {
		fatal(err)
	}
	if _, err = db.ExecContext(ctx, string(b)); err != nil {
		fatal(err)
	}
	fmt.Println("✅ Migrations applied")

	feeds := []collector.Feed{
		{Name: "BBC Persian", Category: "politics", URL: "https://feeds.bbci.co.uk/persian/rss.xml", Region: "iran_world"},
		{Name: "The Verge", Category: "technology", URL: "https://www.theverge.com/rss/index.xml", Region: "world"},
		{Name: "ESPN", Category: "sports", URL: "https://www.espn.com/espn/rss/news", Region: "world"},
	}

	fmt.Println("\n📡 Fetching sample RSS feeds...")
	var sampleItems []collector.Item
	for _, f := range feeds {
		items, err := collector.Fetch(f)
		if err != nil {
			fmt.Printf("  ⚠️  %s: %v\n", f.Name, err)
			continue
		}
		fmt.Printf("  ✅ %s → %d items\n", f.Name, len(items))
		if len(items) > 0 {
			sampleItems = append(sampleItems, items[0])
			var sourceID int64
			_ = db.QueryRowContext(ctx, `INSERT INTO news_sources(name,feed_url,category,region) VALUES($1,$2,$3,$4) ON CONFLICT(feed_url) DO UPDATE SET name=EXCLUDED.name RETURNING id`, f.Name, f.URL, f.Category, f.Region).Scan(&sourceID)
			for i, it := range items {
				if i >= 3 {
					break
				}
				pub := any(nil)
				if !it.Published.IsZero() {
					pub = it.Published
				}
				_, _ = db.ExecContext(ctx, `INSERT INTO news_articles(source_id,external_id,url,title,raw_text,category,region,published_at,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(url) DO NOTHING`, sourceID, it.URL, it.URL, it.Title, it.Text, it.Category, it.Region, pub, it.Hash)
			}
		}
	}

	if len(sampleItems) == 0 {
		fmt.Println("❌ No items fetched. Check network or feed URLs.")
		return
	}

	aiKey := os.Getenv("AI_API_KEY")
	if aiKey == "" {
		fmt.Println("\n⚠️  AI_API_KEY خالی است → فقط تست جمع‌آوری و فرمت انجام می‌شود.")
		items := []digest.Item{
			{AI: ai.Result{TitleFA: "نمونه عنوان سیاسی", SummaryFA: "این یک خلاصه آزمایشی حداکثر هفتاد کلمه‌ای است که برای تست قالب پیام نوشته شده. رویداد مهمی در حوزه سیاست رخ داده و تأثیرات آن بررسی می‌شود.", Importance: 8, Category: "politics"}, URL: sampleItems[0].URL, Source: sampleItems[0].Source},
			{AI: ai.Result{TitleFA: "نمونه فناوری", SummaryFA: "شرکت بزرگی محصول جدیدی معرفی کرد که می‌تواند بازار را تغییر دهد. جزئیات فنی و قیمت هنوز اعلام نشده اما انتظارات بالاست.", Importance: 7, Category: "technology"}, URL: "https://example.com", Source: "The Verge"},
		}
		msg := digest.Build(time.Now(), items)
		fmt.Println("\n========== نمونه پیام تلگرام ==========")
		fmt.Println(msg)
		fmt.Println("======================================")
		return
	}

	fmt.Println("\n🤖 Summarizing with AI...")
	aiClient := &ai.Client{BaseURL: envOr("AI_BASE_URL", "https://api.avalai.ir/v1"), APIKey: aiKey, Model: envOr("AI_MODEL", "gpt-4o-mini")}
	var digestItems []digest.Item
	for i, it := range sampleItems {
		if i >= 2 {
			break
		}
		res, err := aiClient.Summarize(ctx, ai.Article{Title: it.Title, Text: it.Text, Category: it.Category, URL: it.URL})
		if err != nil {
			fmt.Printf("  ⚠️  summarize failed: %v\n", err)
			continue
		}
		fmt.Printf("  ✅ %s → category=%s importance=%d\n", res.TitleFA, res.Category, res.Importance)
		digestItems = append(digestItems, digest.Item{AI: res, URL: it.URL, Source: it.Source})
	}

	if len(digestItems) == 0 {
		fmt.Println("No successful summaries.")
		return
	}

	msg := digest.Build(time.Now(), digestItems)
	fmt.Println("\n========== پیام نهایی ==========")
	fmt.Println(msg)
	fmt.Println("================================")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token != "" && chatID != "" {
		fmt.Println("\n📤 Sending to Telegram...")
		sender := &telegram.Sender{Token: token, ChatID: chatID}
		if err := sender.Send(msg); err != nil {
			fmt.Printf("❌ Telegram error: %v\n", err)
		} else {
			fmt.Println("✅ Message sent successfully!")
		}
	} else {
		fmt.Println("\nℹ️  TELEGRAM_BOT_TOKEN یا CHAT_ID خالی است → فقط چاپ شد، ارسال نشد.")
	}
}

func loadDotEnv(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Remove UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	content := string(data)
	// Normalize Windows line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	count := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Support KEY=VALUE and KEY = VALUE
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		// Only set if not already present in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
			count++
		}
	}
	return count > 0 || true // file existed
}

func printKeyStatus(key string) {
	v := os.Getenv(key)
	if v == "" {
		fmt.Printf("  ❌ %s = (خالی)\n", key)
	} else {
		masked := v
		if len(v) > 8 {
			masked = v[:4] + "****" + v[len(v)-4:]
		} else if len(v) > 2 {
			masked = v[:1] + "****"
		}
		fmt.Printf("  ✅ %s = %s (طول: %d)\n", key, masked, len(v))
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "FATAL:", err)
	os.Exit(1)
}

