package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yourname/ai-daily-news/internal/ai"
	"github.com/yourname/ai-daily-news/internal/collector"
	"github.com/yourname/ai-daily-news/internal/config"
	"github.com/yourname/ai-daily-news/internal/digest"
	"github.com/yourname/ai-daily-news/internal/telegram"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	// Load .env file if present
	loaded := false
	if loadDotEnv("../.env") {
		fmt.Println("✅ .env loaded from current directory")
		loaded = true
	}
	if loadDotEnv(".env") {
		fmt.Println("✅ .env loaded from parent directory")
		loaded = true
	}
	if !loaded {
		fmt.Println("⚠️  No .env file found in current or parent directory")
		fmt.Println("   Current working directory:", mustGetwd())
	}
	printEnvStatus("DATABASE_URL")
	printEnvStatus("TELEGRAM_BOT_TOKEN")
	printEnvStatus("AI_API_KEY")
	printEnvStatus("TELEGRAM_PROXY")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	loc, _ := time.LoadLocation(cfg.Timezone)
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(5)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		fatal(err)
	}
	if err = runMigrations(ctx, db); err != nil {
		fatal(err)
	}

	feeds := []collector.Feed{
		// Politics / Iran and world
		{Name: "DW Persian", Category: "politics", URL: "https://rss.dw.com/xml/rss-per-all_volltext", Region: "iran_world"},
		{Name: "Radio Farda", Category: "politics", URL: "https://www.radiofarda.com/api/z-pqpiev-qpp", Region: "iran_world"},
		{Name: "Iran International", Category: "politics", URL: "https://www.iranintl.com/fa/rss", Region: "iran_world"},
		{Name: "ISNA", Category: "politics", URL: "https://www.isna.ir/rss", Region: "iran"},
		{Name: "IRNA", Category: "politics", URL: "https://www.irna.ir/rss", Region: "iran"},
		{Name: "Mehr News", Category: "politics", URL: "https://www.mehrnews.com/rss", Region: "iran"},

		// Economy / Iran and international
		{Name: "ISNA Economy", Category: "economy", URL: "https://www.isna.ir/rss/tp/20", Region: "iran"},
		{Name: "CNBC Business", Category: "economy", URL: "https://www.cnbc.com/id/10001147/device/rss/rss.html", Region: "world"},

		// Football / Iran and international
		{Name: "Varzesh3 Football", Category: "football", URL: "https://www.varzesh3.com/rss/all", Region: "iran"},
		{Name: "Sky Sports Football", Category: "football", URL: "https://www.skysports.com/rss/11095", Region: "world"},
		{Name: "The Guardian Football", Category: "football", URL: "https://www.theguardian.com/football/rss", Region: "world"},

		// Basketball / international
		{Name: "CBS Sports NBA", Category: "basketball", URL: "https://www.cbssports.com/rss/headlines/nba/", Region: "world"},
		{Name: "The Athletic NBA", Category: "basketball", URL: "https://www.nytimes.com/athletic/rss/nba/", Region: "world"},
		{Name: "Eurohoops", Category: "basketball", URL: "https://www.eurohoops.net/en/feed/", Region: "world"},

		// Formula 1 / international
		{Name: "Formula 1 Official", Category: "formula1", URL: "https://www.formula1.com/en/latest/all.xml", Region: "world"},
		{Name: "Sky Sports F1", Category: "formula1", URL: "https://www.skysports.com/rss/12433", Region: "world"},
		{Name: "Autosport", Category: "formula1", URL: "https://www.autosport.com/rss/feed/f1", Region: "world"},

		// Technology
		{Name: "The Verge", Category: "technology", URL: "https://www.theverge.com/rss/index.xml", Region: "world"},
		{Name: "Digiato", Category: "technology", URL: "https://digiato.com/feed", Region: "iran"},
	}
	aiClient := &ai.Client{BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey, Model: cfg.AIModel}
	sender := &telegram.Sender{Token: cfg.TelegramToken, ChatID: cfg.TelegramChatID, ProxyURL: cfg.TelegramProxy}

	// ----- One-shot mode: go run ./cmd/server now  -----
	if len(os.Args) > 1 && (os.Args[1] == "now" || os.Args[1] == "today") {
		slog.Info("one-shot mode: collecting + summarizing TODAY and sending")
		collect(ctx, db, feeds)
		slog.Info("collection finished, starting summarization")
		today := time.Now().In(loc)
		if err := sendDigest(ctx, db, aiClient, sender, today, cfg.MaxArticles); err != nil {
			slog.Error("digest failed", "error", err)
			os.Exit(1)
		}
		slog.Info("today digest sent successfully")
		return
	}

	// ----- Normal long-running mode -----
	interval, err := time.ParseDuration(cfg.CollectInterval)
	if err != nil {
		fatal(err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	slog.Info("AI Daily News started", "timezone", cfg.Timezone, "digest_time", cfg.DigestTime, "telegram_proxy", cfg.TelegramProxy != "")
	collect(ctx, db, feeds)
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()
	lastDigest := ""
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutdown")
			return
		case <-ticker.C:
			collect(ctx, db, feeds)
		case now := <-minute.C:
			local := now.In(loc)
			if local.Format("15:04") == cfg.DigestTime && local.Format("2006-01-02") != lastDigest {
				// still send YESTERDAY at the morning time
				if err := sendDigest(ctx, db, aiClient, sender, local.AddDate(0, 0, -1), cfg.MaxArticles); err != nil {
					slog.Error("digest failed", "error", err)
				} else {
					lastDigest = local.Format("2006-01-02")
				}
			}
		}
	}
}

func collect(ctx context.Context, db *sql.DB, feeds []collector.Feed) {
	for _, f := range feeds {
		slog.Info("fetching feed", "source", f.Name)
		items, err := collector.Fetch(f)
		if err != nil {
			slog.Warn("feed fetch failed", "source", f.Name, "error", err)
			continue
		}
		slog.Info("feed fetched", "source", f.Name, "items", len(items))
		for _, it := range items {
			var sourceID int64
			_ = db.QueryRowContext(ctx, `INSERT INTO news_sources(name,feed_url,category,region) VALUES($1,$2,$3,$4) ON CONFLICT(feed_url) DO UPDATE SET name=EXCLUDED.name RETURNING id`, f.Name, f.URL, f.Category, f.Region).Scan(&sourceID)
			pub := any(nil)
			if !it.Published.IsZero() {
				pub = it.Published
			}
			_, err = db.ExecContext(ctx, `INSERT INTO news_articles(source_id,external_id,url,title,raw_text,category,region,published_at,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(url) DO NOTHING`, sourceID, it.URL, it.URL, it.Title, it.Text, it.Category, it.Region, pub, it.Hash)
			if err != nil {
				slog.Warn("article insert failed", "url", it.URL, "error", err)
			}
		}
	}
}

func sendDigest(ctx context.Context, db *sql.DB, client *ai.Client, sender *telegram.Sender, date time.Time, limit int) error {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)
	// First process new articles. Existing summaries are retained and reused on retries.
	rows, err := db.QueryContext(ctx, `WITH pending AS (
			SELECT a.id,a.title,a.raw_text,a.category,a.url,COALESCE(s.name,'منبع') AS source,
				ROW_NUMBER() OVER (PARTITION BY a.category ORDER BY COALESCE(a.published_at,a.fetched_at) DESC) AS rn
			FROM news_articles a LEFT JOIN news_sources s ON s.id=a.source_id
			WHERE COALESCE(a.published_at,a.fetched_at)>=$1 AND COALESCE(a.published_at,a.fetched_at)<$2
			AND NOT EXISTS(SELECT 1 FROM news_summaries ns WHERE ns.article_id=a.id)
		)
		SELECT id,title,raw_text,category,url,source FROM pending WHERE rn <= $3
		ORDER BY category,rn`, start, end, limit*3)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var title, body, cat, url, source string
		if err = rows.Scan(&id, &title, &body, &cat, &url, &source); err != nil {
			rows.Close()
			return err
		}
		slog.Info("summarizing", "title", title)
		res, e := client.Summarize(ctx, ai.Article{Title: title, Text: body, Category: cat, URL: url})
		if e != nil {
			slog.Warn("summarize failed", "url", url, "error", e)
			continue
		}
		slog.Info("summarized", "title_fa", res.TitleFA, "category", res.Category)
		keyPoints, _ := json.Marshal(res.KeyPoints)
		_, e = db.ExecContext(ctx, `INSERT INTO news_summaries(article_id,title_fa,summary_fa,importance,category,key_points,model)
			VALUES($1,$2,$3,$4,$5,$6::jsonb,$7) ON CONFLICT(article_id) DO NOTHING`, id, res.TitleFA, res.SummaryFA, res.Importance, res.Category, string(keyPoints), client.Model)
		if e != nil {
			slog.Warn("summary save failed", "error", e)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// Build from all summaries for the date, including summaries created by an earlier attempt.
	resultRows, err := db.QueryContext(ctx, `WITH ranked AS (
			SELECT ns.title_fa,ns.summary_fa,ns.importance,ns.category,a.url,COALESCE(s.name,'منبع') AS source,
				ROW_NUMBER() OVER (PARTITION BY ns.category ORDER BY ns.importance DESC, COALESCE(a.published_at,a.fetched_at) DESC) AS rn
			FROM news_summaries ns JOIN news_articles a ON a.id=ns.article_id
			LEFT JOIN news_sources s ON s.id=a.source_id
			WHERE COALESCE(a.published_at,a.fetched_at)>=$1 AND COALESCE(a.published_at,a.fetched_at)<$2
		)
		SELECT title_fa,summary_fa,importance,category,url,source FROM ranked
		WHERE rn <= $3 ORDER BY category, importance DESC`, start, end, limit)
	if err != nil {
		return err
	}
	defer resultRows.Close()
	items := []digest.Item{}
	for resultRows.Next() {
		var title, summary, category, url, source string
		var importance int
		if err = resultRows.Scan(&title, &summary, &importance, &category, &url, &source); err != nil {
			return err
		}
		items = append(items, digest.Item{AI: ai.Result{TitleFA: title, SummaryFA: summary, Importance: importance, Category: category}, URL: url, Source: source})
	}
	if err = resultRows.Err(); err != nil {
		return err
	}
	messages := digest.BuildMessages(date, items, 3900)
	bodies := make([]string, 0, len(messages))
	for _, message := range messages {
		bodies = append(bodies, message.Text)
	}
	body := strings.Join(bodies, "\n\n---\n\n")
	_, err = db.ExecContext(ctx, `INSERT INTO daily_digests(digest_date,timezone,status,body)
		VALUES($1,$2,'prepared',$3) ON CONFLICT(digest_date) DO UPDATE SET body=EXCLUDED.body,status='prepared'`, date.Format("2006-01-02"), os.Getenv("APP_TIMEZONE"), body)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		slog.Warn("no news summaries available for digest", "date", date.Format("2006-01-02"))
		return nil
	}
	slog.Info("sending categorized Telegram messages", "messages", len(messages), "articles", len(items))
	if err = sender.SendBatch(bodies); err != nil {
		return err
	}
	slog.Info("all categorized Telegram messages sent")
	_, err = db.ExecContext(ctx, `UPDATE daily_digests SET status='sent',sent_at=NOW() WHERE digest_date=$1`, date.Format("2006-01-02"))
	return err
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	b, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		b, err = os.ReadFile("migrations/001_init.sql")
	}
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(b))
	return err
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "?"
	}
	return wd
}

func printEnvStatus(key string) {
	v := os.Getenv(key)
	if v == "" {
		fmt.Printf("  ❌ %s = (empty)\n", key)
	} else {
		masked := v
		if len(v) > 10 {
			masked = v[:4] + "****" + v[len(v)-4:]
		}
		fmt.Printf("  ✅ %s = %s (len=%d)\n", key, masked, len(v))
	}
}

// loadDotEnv reads a .env file and sets environment variables (does not override existing ones).
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
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return true
}
