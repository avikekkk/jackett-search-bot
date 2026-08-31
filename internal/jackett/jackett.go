// Package jackett queries a Jackett instance over its torznab API and turns the
// feed into the releases the bot shows.
package jackett

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// searchTimeout bounds one torznab call. Jackett fans a query out to every
// indexer, so this is generous compared with a normal HTTP call.
const searchTimeout = 15 * time.Second

// maxBodyBytes caps how much of a feed is read, so a misbehaving indexer cannot
// exhaust memory.
const maxBodyBytes = 32 << 20

// Error is a failure the bot reports to the user rather than logging as a bug.
type Error struct {
	msg string
}

func (e *Error) Error() string { return e.msg }

func errorf(format string, args ...any) error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

// Release is one item of a torznab feed, already formatted for display.
type Release struct {
	Title     string
	Size      string
	SizeBytes int64
	// Indexer is the tracker the release came from, nil when Jackett reports an
	// indexer this package does not know about.
	Indexer *Indexer
}

// SplitTitle separates a release name from the tag block its tracker appends,
// delegating to whatever that indexer does. A tracker with no such quirk, or an
// unknown one, yields the whole title as the name.
func (r *Release) SplitTitle() (name, tags string) {
	if r.Indexer == nil || r.Indexer.SplitTitle == nil {
		return strings.TrimSpace(r.Title), ""
	}
	return r.Indexer.SplitTitle(r.Title)
}

// Client talks to one Jackett instance.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New returns a client for the configured Jackett instance.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: searchTimeout},
	}
}

// imdbIDRe matches the IMDb IDs Jackett can look up directly.
var imdbIDRe = regexp.MustCompile(`^tt\d+$`)

// imdbLinkRe pulls the ID out of a pasted IMDb URL.
var imdbLinkRe = regexp.MustCompile(`imdb\.com/title/(tt\d+)`)

// SearchURL builds the torznab request for a query. An IMDb ID or link is sent
// as an imdbid lookup; anything else is a free-text search.
func (c *Client) SearchURL(query string, opts Options) string {
	endpoint := c.baseURL + "/api/v2.0/indexers/" + opts.endpointIndexer() + "/results/torznab/api"
	params := url.Values{"apikey": {c.apiKey}}

	if imdbID := ParseIMDbID(query); imdbID != "" {
		params.Set("imdbid", imdbID)
	} else {
		params.Set("t", "search")
		params.Set("q", query)
	}
	return endpoint + "?" + params.Encode()
}

// ParseIMDbID returns the IMDb ID in a query, or "" when it is free text.
func ParseIMDbID(query string) string {
	query = strings.TrimSpace(query)
	if imdbIDRe.MatchString(query) {
		return query
	}
	if match := imdbLinkRe.FindStringSubmatch(query); match != nil {
		return match[1]
	}
	return ""
}

// Search returns every release matching the query, newest feed order preserved.
func (c *Client) Search(ctx context.Context, query string, opts Options) ([]*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.SearchURL(query, opts), nil)
	if err != nil {
		return nil, errorf("Invalid Jackett request")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errorf("Jackett unreachable")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errorf("Jackett rejected the API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errorf("Jackett error %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, errorf("Jackett response could not be read")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}

	return ParseFeed(body, opts)
}

// feed is the subset of a torznab RSS document the bot reads.
type feed struct {
	Items []struct {
		Title string `xml:"title"`
		Size  string `xml:"size"`
		// Indexer is the jackettindexer element Jackett stamps on every item,
		// naming the tracker the item came from.
		Indexer struct {
			ID string `xml:"id,attr"`
		} `xml:"jackettindexer"`
	} `xml:"channel>item"`
}

// ParseFeed turns a torznab document into releases, skipping items that are
// missing any field the result list shows.
func ParseFeed(document []byte, opts Options) ([]*Release, error) {
	var parsed feed
	if err := xml.Unmarshal(document, &parsed); err != nil {
		return nil, errorf("Jackett returned an unreadable response")
	}

	var releases []*Release
	for _, item := range parsed.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		if !opts.wantsIndexer(item.Indexer.ID) || !opts.wantsTitle(title) {
			continue
		}

		sizeBytes, err := strconv.ParseInt(strings.TrimSpace(item.Size), 10, 64)
		if err != nil {
			continue
		}

		indexer, _ := IndexerByID(item.Indexer.ID)
		releases = append(releases, &Release{
			Title:     title,
			Size:      FormatBytes(float64(sizeBytes)),
			SizeBytes: sizeBytes,
			Indexer:   indexer,
		})
	}
	return releases, nil
}

// FormatBytes renders a byte count the way the result list shows sizes.
func FormatBytes(numBytes float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := numBytes
	if value < 0 {
		value = 0
	}
	for i, unit := range units {
		if value < 1024 || i == len(units)-1 {
			if unit == "B" {
				return fmt.Sprintf("%d %s", int64(value), unit)
			}
			return trimZeros(value) + " " + unit
		}
		value /= 1024
	}
	return trimZeros(value) + " TB"
}

// trimZeros rounds to two decimals and drops trailing zeros, keeping at least
// one decimal place so sizes still read as measurements.
func trimZeros(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	if strings.HasSuffix(text, ".") {
		text += "0"
	}
	return text
}
