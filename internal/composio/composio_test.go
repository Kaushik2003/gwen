package composio

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPageID = "abcdef01-2345-6789-abcd-ef0123456789"

const testIssue = `{"number":1,"title":"First issue","body":"Body from search","state":"open","repository_url":"https://api.github.com/repos/ada/gwen","user":{"login":"ada"},"labels":[{"name":"bug"}]}`
const testPage = `{"id":"abcdef01-2345-6789-abcd-ef0123456789","properties":{"Name":{"type":"title","title":[{"plain_text":"Title from search"}]}}}`

type fakeCalls struct {
	mu    sync.Mutex
	calls map[string]int
	data  map[string]string
}

func fakeClient() (*Client, *fakeCalls) {
	f := &fakeCalls{calls: make(map[string]int), data: map[string]string{
		toolGitHubSearch:   `{"items":[` + testIssue + `,{"number":2,"title":"Second issue","body":"Other body","repository_url":"https://api.github.com/repos/ada/gwen"}]}`,
		toolGitHubIssue:    testIssue,
		toolGitHubComments: `{"comments":[{"body":"A comment","user":{"login":"grace"}}]}`,
		toolNotionSearch:   `{"results":[` + testPage + `,{"id":"other","properties":{"Name":{"type":"title","title":[{"plain_text":"Other page"}]}}}]}`,
		toolNotionPage:     testPage,
		toolNotionMarkdown: `{"markdown":"Page text<br>Next line"}`,
		toolGmailSearch:    `{"messages":[{"messageId":"mail1","subject":"Mail"}]}`,
		toolGmailMessage:   `{"messageId":"mail1","subject":"Mail","messageText":"Full mail"}`,
		toolCalendarList:   `{"items":[{"id":"event1","summary":"Meeting"}]}`,
		toolCalendarEvent:  `{"id":"event1","summary":"Meeting","description":"Full description"}`,
	}}
	return &Client{Bin: "fake-composio", run: f.run}, f
}

func (f *fakeCalls) run(_ context.Context, _ []string, _ string, args ...string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := toolOf(args)
	if key == "" {
		key = strings.Join(args, " ")
	}
	f.calls[key]++
	data, ok := f.data[key]
	if !ok {
		return nil, nil, fmt.Errorf("unexpected fake call")
	}
	return []byte(`{"successful":true,"data":` + data + `}`), nil, nil
}

func (f *fakeCalls) want(t *testing.T, calls map[string]int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !reflect.DeepEqual(f.calls, calls) {
		t.Fatalf("calls = %v, want %v", f.calls, calls)
	}
}

func searchForTest(t *testing.T, c *Client, app, query string) []Item {
	t.Helper()
	items, err := c.Search(context.Background(), app, query, 10)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func fetchForTest(t *testing.T, c *Client, app, ref string) *Snapshot {
	t.Helper()
	s, err := c.Fetch(context.Background(), app, ref)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGitHubSearchHint(t *testing.T) {
	c, calls := fakeClient()
	items := searchForTest(t, c, "github", "some words")
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	s := fetchForTest(t, c, "github", items[0].Ref)
	calls.want(t, map[string]int{toolGitHubSearch: 1, toolGitHubComments: 1})
	want := "ada/gwen #1: First issue\nOpen issue by ada\nLabels: bug\n\nBody from search\n\nComments:\n\ngrace:\nA comment"
	if s.Body != want {
		t.Fatalf("body = %q, want %q", s.Body, want)
	}
	again := fetchForTest(t, c, "github", items[0].Ref)
	calls.want(t, map[string]int{toolGitHubSearch: 1, toolGitHubComments: 2, toolGitHubIssue: 1})
	if !reflect.DeepEqual(s, again) {
		t.Fatal("hint and full fetch snapshots differ")
	}
	other := fetchForTest(t, c, "github", items[1].Ref)
	if other.Title != "Second issue" || !strings.Contains(other.Body, "Other body") {
		t.Fatal("second issue did not keep its own copy")
	}
	calls.want(t, map[string]int{toolGitHubSearch: 1, toolGitHubComments: 3, toolGitHubIssue: 1})
}

func TestNotionSearchHint(t *testing.T) {
	c, calls := fakeClient()
	items := searchForTest(t, c, "notion", "some words")
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	s := fetchForTest(t, c, "notion", strings.ToUpper(strings.ReplaceAll(items[0].Ref, "-", "")))
	calls.want(t, map[string]int{toolNotionSearch: 1, toolNotionMarkdown: 1})
	if s.Title != "Title from search" || s.Body != "# Title from search\n\nPage text\nNext line" {
		t.Fatalf("snapshot = %+v", s)
	}
	again := fetchForTest(t, c, "notion", items[0].Ref)
	calls.want(t, map[string]int{toolNotionSearch: 1, toolNotionMarkdown: 2, toolNotionPage: 1})
	if !reflect.DeepEqual(s, again) {
		t.Fatal("hint and full fetch snapshots differ")
	}
	other := fetchForTest(t, c, "notion", items[1].Ref)
	if other.Title != "Other page" {
		t.Fatal("second page did not keep its own copy")
	}
	calls.want(t, map[string]int{toolNotionSearch: 1, toolNotionMarkdown: 3, toolNotionPage: 1})
}

func TestFetchWithoutHintAndExpiredHint(t *testing.T) {
	for _, app := range []string{"github", "notion"} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/expired=%t", app, expired), func(t *testing.T) {
				c, calls := fakeClient()
				now := time.Unix(1000, 0)
				c.clock = func() time.Time { return now }
				ref := "ada/gwen#1"
				want := map[string]int{toolGitHubIssue: 1, toolGitHubComments: 1}
				search := toolGitHubSearch
				if app == "notion" {
					ref, search = testPageID, toolNotionSearch
					want = map[string]int{toolNotionPage: 1, toolNotionMarkdown: 1}
				}
				if expired {
					searchForTest(t, c, app, "some words")
					now = now.Add(hintTTL + time.Nanosecond)
					want[search] = 1
				}
				fetchForTest(t, c, app, ref)
				calls.want(t, want)
			})
		}
	}
}

func TestPastedLinkHints(t *testing.T) {
	for _, app := range []string{"github", "notion"} {
		t.Run(app, func(t *testing.T) {
			c, calls := fakeClient()
			link := "https://github.com/ada/gwen/issues/1"
			want := map[string]int{toolGitHubIssue: 1, toolGitHubComments: 1}
			if app == "notion" {
				link = "https://www.notion.so/Plan-" + strings.ReplaceAll(testPageID, "-", "")
				want = map[string]int{toolNotionPage: 1, toolNotionMarkdown: 1}
			}
			items := searchForTest(t, c, app, link)
			if len(items) != 1 {
				t.Fatalf("items = %d, want 1", len(items))
			}
			fetchForTest(t, c, app, items[0].Ref)
			calls.want(t, want)
		})
	}
}

func TestGmailAndCalendarAlwaysFetch(t *testing.T) {
	for _, app := range []string{"gmail", "googlecalendar"} {
		t.Run(app, func(t *testing.T) {
			c, calls := fakeClient()
			items := searchForTest(t, c, app, "some words")
			if len(items) != 1 {
				t.Fatalf("items = %d, want 1", len(items))
			}
			for range 2 {
				s := fetchForTest(t, c, app, items[0].Ref)
				body := "Full mail"
				if app == "googlecalendar" {
					body = "Full description"
				}
				if !strings.Contains(s.Body, body) {
					t.Fatalf("body missing full content: %q", s.Body)
				}
			}
			want := map[string]int{toolGmailSearch: 1, toolGmailMessage: 2}
			if app == "googlecalendar" {
				want = map[string]int{toolCalendarList: 1, toolCalendarEvent: 2}
			}
			calls.want(t, want)
		})
	}
}

func TestHintMaxAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	c := &Client{clock: func() time.Time { return now }}
	for i := range hintMax + 5 {
		c.putHint(fmt.Sprint(i), hint{})
		now = now.Add(time.Millisecond)
	}
	if len(c.hints) != hintMax {
		t.Fatalf("hints = %d, want %d", len(c.hints), hintMax)
	}
	for i := range 5 {
		if _, ok := c.takeHint(fmt.Sprint(i)); ok {
			t.Fatalf("oldest hint %d was kept", i)
		}
	}
	now = now.Add(hintTTL + time.Nanosecond)
	c.putHint("new", hint{})
	if len(c.hints) != 1 {
		t.Fatalf("expired hints were kept: %d", len(c.hints))
	}
	if _, ok := c.takeHint("new"); !ok {
		t.Fatal("new hint missing")
	}
	if _, ok := c.takeHint("new"); ok {
		t.Fatal("hint was not consumed")
	}
}

func TestHintsConcurrent(t *testing.T) {
	c := &Client{}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 20 {
				key := fmt.Sprintf("%d/%d", i, j)
				c.putHint(key, hint{})
				c.takeHint(key)
			}
		}()
	}
	wg.Wait()
}

func TestToolOf(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"execute"}, ""},
		{[]string{"execute", toolGitHubIssue, "-d", "private data"}, toolGitHubIssue},
		{[]string{"search", "private query"}, ""},
	} {
		if got := toolOf(tc.args); got != tc.want {
			t.Fatalf("toolOf = %q, want %q", got, tc.want)
		}
	}
}
