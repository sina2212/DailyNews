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
	var doc rss
	if err = xml.NewDecoder(resp.Body).Decode(&doc); err != nil {
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
		var published time.Time
		for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339} {
			if t, e := time.Parse(layout, strings.TrimSpace(x.PubDate)); e == nil {
				published = t
				break
			}
		}
		text := strings.TrimSpace(stripHTML(x.Description))
		sum := sha256.Sum256([]byte(strings.ToLower(title) + "|" + url))
		out = append(out, Item{Source: f.Name, Category: f.Category, Region: f.Region, URL: url, Title: title, Text: text, Published: published, Hash: hex.EncodeToString(sum[:])})
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
