# AI Daily News — MVP

Go + PostgreSQL service that collects RSS items, summarizes them in Persian through an OpenAI-compatible API, and sends a daily Telegram digest.

## Current MVP scope
- Persian output; Iran + international coverage intent.
- Politics, economy, technology, plus separate football, basketball and Formula 1 categories.
- Daily delivery at `08:00` in configurable IANA timezone (default `Europe/Berlin`).
- RSS ingestion and OpenAI-compatible Chat Completions API.
- PostgreSQL persistence and Docker Compose deployment.

## News categories and Telegram delivery

- Sports are collected separately as `football`, `basketball`, and `formula1`.
- Football includes an Iran-focused feed (Varzesh3) plus international publishers.
- Basketball and Formula 1 use international publishers and are not dependent on Varzesh3.
- Each populated category is sent as its own Telegram message. Long categories are split at article boundaries to stay below Telegram's message-size limit.
- `MAX_ARTICLES_PER_CATEGORY` controls the maximum number of selected summaries per category (default: `20`). Set it in `.env` to change the count.
- The current runtime feed registry is in `cmd/server/main.go`; `configs/sources.yaml` is maintained as a human-readable mirror. Keep both in sync when editing feeds.

## Important implementation notes
This is a starter MVP, not a production-grade newsroom. The initial feed list is deliberately small and must be reviewed. Some feed URLs may change or reject automated access. Iran coverage is not complete until you add and validate a broader set of Persian and international feeds. RSS descriptions may be short; summaries must not invent missing details. The current scheduler is single-process and should be run as one app replica. For high reliability, add a persisted job/lease mechanism and Telegram chunking before scaling.

## 1. Create Telegram bot
1. Open Telegram and message `@BotFather`.
2. Run `/newbot`, follow the prompts, and copy the token.
3. Send a message to your new bot.
4. Obtain your numeric chat ID using `https://api.telegram.org/bot<TOKEN>/getUpdates` (do not share the token). For a private bot, use your own chat ID.

## 2. Configure
From the project root:

```bash
cp .env.example .env
```

Edit `.env` and set:

```env
POSTGRES_PASSWORD=use-a-long-random-password
DATABASE_URL=postgres://news_user:YOUR_POSTGRES_PASSWORD@postgres:5432/ai_daily_news?sslmode=disable
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_CHAT_ID=your_chat_id
AI_API_KEY=your_provider_api_key
AI_BASE_URL=https://api.openai.com/v1
AI_MODEL=gpt-4.1-mini
APP_TIMEZONE=Europe/Berlin
DIGEST_TIME=08:00
MAX_ARTICLES_PER_CATEGORY=20
```

The database password in `DATABASE_URL` must match `POSTGRES_PASSWORD`. If it contains reserved URL characters, URL-encode it.

## 3. Run with Docker

```bash
docker compose --env-file .env -f deployments/docker-compose.yml up -d --build
```

View logs:

```bash
docker compose --env-file .env -f deployments/docker-compose.yml logs -f app
```

Stop:

```bash
docker compose --env-file .env -f deployments/docker-compose.yml down
```

Database data remains in the named Docker volume. Do not use `down -v` unless you intend to delete it.

## 4. Local development
Requires Go 1.23+ and PostgreSQL 15+.

```bash
go mod tidy
go run ./cmd/server #for your daily news. send on 08:00
go run ./cmd/server now #for any time of a day
```

Run from the repository root so the migration file can be found.

## 5. Test delivery
- Start the app and check logs for `AI Daily News started`.
- Verify rows appear in `news_sources` and `news_articles`.
- For a controlled test, temporarily set `DIGEST_TIME` to the next minute, restart the app, and check the Telegram chat. Restore `08:00` afterward.
- Check `daily_digests` and `news_summaries` in PostgreSQL.

## 6. Security and operations
- Never commit `.env` or share the Telegram token/API key.
- Keep PostgreSQL unexposed to the public internet; the Compose service is internal by default.
- Back up the PostgreSQL volume regularly.
- Configure firewall access for SSH only unless you intentionally expose another service.
- Review source licensing and RSS terms. Link to original publishers; do not republish full copyrighted articles.

## Known next steps before calling it production-ready
1. Periodically validate the Persian/Iranian and sports RSS endpoints; feed availability can vary by network and publisher.
2. Add robust HTML/entity cleanup, language detection, and cross-source semantic deduplication.
3. Add a proper `processed_at`/retry workflow and persistent scheduler lock; currently the simple in-process scheduler is intended for one replica.
4. Add a daily editorial overview and persistent per-category delivery tracking (the current digest table stores one combined daily record).
5. Add tests, health checks, metrics, structured delivery retries, and backup automation.
6. Add bot commands (`/today`, `/weekly`, `/categories`) in the next iteration.
