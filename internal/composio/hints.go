package composio

import (
	"strings"
	"time"
)

// hint is what a search already returned about one item, kept briefly so attaching it
// does not ask the app for the same thing again.
type hint struct {
	at     time.Time
	github *ghIssue
	notion *notionPage
}

const (
	hintTTL = 90 * time.Second
	hintMax = 100
)

func (c *Client) hintNow() time.Time {
	if c.clock != nil {
		return c.clock()
	}
	return time.Now()
}

func (c *Client) putHint(key string, h hint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h.at = c.hintNow()
	if c.hints == nil {
		c.hints = make(map[string]hint)
	}
	for k, old := range c.hints {
		if h.at.Sub(old.at) > hintTTL {
			delete(c.hints, k)
		}
	}
	delete(c.hints, key)
	if len(c.hints) >= hintMax {
		var oldest string
		var at time.Time
		for k, old := range c.hints {
			if oldest == "" || old.at.Before(at) {
				oldest, at = k, old.at
			}
		}
		delete(c.hints, oldest)
	}
	c.hints[key] = h
}

func (c *Client) takeHint(key string) (hint, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.hints[key]
	delete(c.hints, key)
	if !ok || c.hintNow().Sub(h.at) > hintTTL {
		return hint{}, false
	}
	return h, true
}

func notionHintKey(id string) string {
	return "notion\x00" + strings.ToLower(strings.ReplaceAll(id, "-", ""))
}
