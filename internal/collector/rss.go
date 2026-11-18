package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"
)

type Feed struct{ Name, Category, URL, Region string }
type Item struct {
	Source, Category, Region, URL, Title, Text string
	Published                                  time.Time
	Hash                                       string
}
type rss struct {
	Channel struct {
		Items []struct {
			GUID        string `xml:"guid"`
			Link        string `xml:"link"`
			Title       string `xml:"title"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomFeed struct {
	Entries []struct {
		Title     string `xml:"title"`
		Summary   string `xml:"summary"`
		Content   string `xml:"content"`
		Updated   string `xml:"updated"`
		Published string `xml:"published"`
		Links     []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func Fetch(f Feed) ([]Item, error) {
	req, _ := http.NewRequest(http.MethodGet, f.URL, nil)
	req.Header.Set("User-Agent", "AI-Daily-News/1.0 (+RSS reader)")
	client := http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, io.ErrUnexpectedEOF
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var doc rss
	if err = xml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(doc.Channel.Items))
	for _, x := range doc.Channel.Items {
		url := strings.TrimSpace(x.Link)
		if url == "" {
			url = strings.TrimSpace(x.GUID)
		}
		title := strings.TrimSpace(x.Title)
		if url == "" || title == "" {
			continue
		}
		published := parseDate(x.PubDate)
		text := strings.TrimSpace(stripHTML(x.Description))
		out = append(out, makeItem(f, url, title, text, published))
	}
	// Some publishers (including some Formula 1 feeds) publish Atom rather than RSS.
	if len(out) == 0 {
		var atom atomFeed
		if err = xml.Unmarshal(raw, &atom); err == nil {
			for _, x := range atom.Entries {
				url := ""
				for _, link := range x.Links {
					if strings.TrimSpace(link.Href) != "" {
						url = strings.TrimSpace(link.Href)
						break
					}
				}
				title := strings.TrimSpace(x.Title)
				if url == "" || title == "" {
					continue
				}
				description := x.Summary
				if strings.TrimSpace(description) == "" {
					description = x.Content
				}
				published := parseDate(x.Published)
				if published.IsZero() {
					published = parseDate(x.Updated)
				}
				out = append(out, makeItem(f, url, title, strings.TrimSpace(stripHTML(description)), published))
			}
		}
	}
	return out, nil
}
func stripHTML(s string) string {
	var b strings.Builder
	inside := false
	for _, r := range s {
		if r == '<' {
			inside = true
			continue
		}
		if r == '>' {
			inside = false
			b.WriteByte(' ')
			continue
		}
		if !inside {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func makeItem(f Feed, url, title, text string, published time.Time) Item {
	sum := sha256.Sum256([]byte(strings.ToLower(title) + "|" + url))
	return Item{Source: f.Name, Category: f.Category, Region: f.Region, URL: url, Title: title, Text: text, Published: published, Hash: hex.EncodeToString(sum[:])}
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339Nano} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
