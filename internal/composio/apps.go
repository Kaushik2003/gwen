package composio

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tools used per app. Each was checked against `composio execute <slug>
// --get-schema`; their inputs are validated strictly, so only listed keys go.
const (
	toolGmailSearch    = "GMAIL_FETCH_EMAILS"
	toolGmailMessage   = "GMAIL_FETCH_MESSAGE_BY_MESSAGE_ID"
	toolCalendarList   = "GOOGLECALENDAR_EVENTS_LIST"
	toolCalendarEvent  = "GOOGLECALENDAR_EVENTS_GET"
	toolGitHubSearch   = "GITHUB_SEARCH_ISSUES_AND_PULL_REQUESTS"
	toolGitHubIssue    = "GITHUB_GET_AN_ISSUE"
	toolGitHubComments = "GITHUB_LIST_ISSUE_COMMENTS"
	toolNotionSearch   = "NOTION_SEARCH_NOTION_PAGE"
	toolNotionPage     = "NOTION_RETRIEVE_PAGE"
	toolNotionMarkdown = "NOTION_GET_PAGE_MARKDOWN"
)

// --- Gmail ---

type gmailMessage struct {
	MessageID        string `json:"messageId"`
	ThreadID         string `json:"threadId"`
	Subject          string `json:"subject"`
	Sender           string `json:"sender"`
	To               string `json:"to"`
	MessageTimestamp string `json:"messageTimestamp"`
	MessageText      string `json:"messageText"`
	DisplayURL       string `json:"display_url"`
	Preview          struct {
		Body    string `json:"body"`
		Subject string `json:"subject"`
	} `json:"preview"`
	Payload *gmailPart `json:"payload"`
}

type gmailPart struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func (m gmailMessage) item() Item {
	title := strings.TrimSpace(m.Subject)
	if title == "" {
		title = strings.TrimSpace(m.Preview.Subject)
	}
	if title == "" {
		title = "(no subject)"
	}
	link := m.DisplayURL
	if link == "" && m.MessageID != "" {
		link = "https://mail.google.com/mail/u/0/#all/" + m.MessageID
	}
	snippet := senderName(m.Sender)
	if p := oneLine(html.UnescapeString(m.Preview.Body)); p != "" {
		snippet = joinNonEmpty(" — ", snippet, p)
	}
	return Item{App: "gmail", Ref: m.MessageID, Title: title, URL: link, Snippet: cut(snippet, 200), When: parseTime(m.MessageTimestamp)}
}

func (c *Client) searchGmail(ctx context.Context, query string, limit int) ([]Item, error) {
	// Without verbose the tool fetches only headers and a preview, about a second faster.
	args := map[string]any{"max_results": limit, "verbose": false, "include_payload": false}
	if query != "" {
		args["query"] = query
	}
	var data struct {
		Messages []gmailMessage `json:"messages"`
	}
	if err := c.execute(ctx, "gmail", toolGmailSearch, args, &data); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(data.Messages))
	for _, m := range data.Messages {
		if m.MessageID != "" {
			items = append(items, m.item())
		}
	}
	return items, nil
}

func (c *Client) fetchGmail(ctx context.Context, ref string) (*Snapshot, error) {
	var m gmailMessage
	if err := c.execute(ctx, "gmail", toolGmailMessage, map[string]any{"message_id": ref, "format": "full"}, &m); err != nil {
		return nil, err
	}
	if m.MessageID == "" {
		m.MessageID = ref
	}
	it := m.item()
	head := []string{"From: " + m.Sender, "To: " + m.To, "Subject: " + it.Title}
	if it.When != nil {
		head = append(head, "Date: "+it.When.Local().Format("Mon 2 Jan 2006, 15:04"))
	}
	return &Snapshot{Item: it, Body: strings.Join(head, "\n") + "\n\n" + gmailText(m)}, nil
}

// gmailText is a message's body as plain text: the text part when there is
// one, else the tool's own text, else the HTML part with its tags taken out.
func gmailText(m gmailMessage) string {
	var plain, html string
	var walk func(p *gmailPart)
	walk = func(p *gmailPart) {
		if p == nil {
			return
		}
		switch {
		case p.MimeType == "text/plain" && plain == "":
			plain = decodeBase64URL(p.Body.Data)
		case p.MimeType == "text/html" && html == "":
			html = decodeBase64URL(p.Body.Data)
		}
		for i := range p.Parts {
			walk(&p.Parts[i])
		}
	}
	walk(m.Payload)
	for _, s := range []string{plain, m.MessageText} {
		if strings.TrimSpace(s) != "" {
			if looksLikeHTML(s) {
				return htmlToText(s)
			}
			return tidy(s)
		}
	}
	if html != "" {
		return htmlToText(html)
	}
	return oneLine(m.Preview.Body)
}

func decodeBase64URL(s string) string {
	if s == "" {
		return ""
	}
	s = strings.TrimRight(s, "=")
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(s); err != nil {
			return ""
		}
	}
	return string(b)
}

// senderName is "Ada Lovelace" from `"Ada Lovelace" <ada@example.com>`.
func senderName(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "<"); i > 0 {
		if n := strings.Trim(strings.TrimSpace(s[:i]), `"`); n != "" {
			return n
		}
	}
	return strings.Trim(s, "<>")
}

// --- Google Calendar ---

type calTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
}

type calEvent struct {
	ID          string  `json:"id"`
	Summary     string  `json:"summary"`
	Description string  `json:"description"`
	Location    string  `json:"location"`
	HTMLLink    string  `json:"htmlLink"`
	DisplayURL  string  `json:"display_url"`
	HangoutLink string  `json:"hangoutLink"`
	Status      string  `json:"status"`
	Start       calTime `json:"start"`
	End         calTime `json:"end"`
	Organizer   struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	} `json:"organizer"`
	Attendees []struct {
		Email          string `json:"email"`
		DisplayName    string `json:"displayName"`
		ResponseStatus string `json:"responseStatus"`
		Organizer      bool   `json:"organizer"`
	} `json:"attendees"`
}

func (t calTime) parse() (*time.Time, bool) {
	if t.DateTime != "" {
		return parseTime(t.DateTime), false
	}
	if t.Date != "" {
		if d, err := time.ParseInLocation("2006-01-02", t.Date, time.Local); err == nil {
			return &d, true
		}
	}
	return nil, false
}

// span is "Fri 14 Nov, 10:00–11:00", or "Fri 14 Nov, all day".
func (e calEvent) span() string {
	start, allDay := e.Start.parse()
	if start == nil {
		return ""
	}
	day := start.Local().Format("Mon 2 Jan 2006")
	if allDay {
		return day + ", all day"
	}
	s := day + ", " + start.Local().Format("15:04")
	if end, _ := e.End.parse(); end != nil {
		if end.Local().Format("2006-01-02") == start.Local().Format("2006-01-02") {
			s += "–" + end.Local().Format("15:04")
		} else {
			s += " to " + end.Local().Format("Mon 2 Jan, 15:04")
		}
	}
	return s
}

func (e calEvent) item() Item {
	title := strings.TrimSpace(e.Summary)
	if title == "" {
		title = "(no title)"
	}
	link := e.HTMLLink
	if link == "" {
		link = e.DisplayURL
	}
	when, _ := e.Start.parse()
	return Item{App: "googlecalendar", Ref: e.ID, Title: title, URL: link, Snippet: cut(joinNonEmpty(" · ", e.span(), oneLine(e.Location)), 200), When: when}
}

func (c *Client) searchCalendar(ctx context.Context, query string, limit int) ([]Item, error) {
	now := time.Now()
	args := map[string]any{"singleEvents": true, "orderBy": "startTime"}
	if query == "" {
		// What's coming up, from the start of today.
		y, m, d := now.Date()
		args["timeMin"] = time.Date(y, m, d, 0, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)
		args["maxResults"] = limit
	} else {
		// Matches from half a year back to a year ahead; the ones nearest now come first.
		args["query"] = query
		args["timeMin"] = now.AddDate(0, -6, 0).UTC().Format(time.RFC3339)
		args["timeMax"] = now.AddDate(1, 0, 0).UTC().Format(time.RFC3339)
		args["maxResults"] = 100
	}
	var data struct {
		Items []calEvent `json:"items"`
	}
	if err := c.execute(ctx, "googlecalendar", toolCalendarList, args, &data); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(data.Items))
	for _, e := range data.Items {
		if e.ID != "" && e.Status != "cancelled" {
			items = append(items, e.item())
		}
	}
	if query != "" {
		dist := func(it Item) time.Duration {
			if it.When == nil {
				return 1 << 62
			}
			d := it.When.Sub(now)
			if d < 0 {
				d = -d
			}
			return d
		}
		sort.SliceStable(items, func(i, j int) bool { return dist(items[i]) < dist(items[j]) })
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (c *Client) fetchCalendar(ctx context.Context, ref string) (*Snapshot, error) {
	var e calEvent
	if err := c.execute(ctx, "googlecalendar", toolCalendarEvent, map[string]any{"event_id": ref}, &e); err != nil {
		return nil, err
	}
	if e.ID == "" {
		e.ID = ref
	}
	it := e.item()
	lines := []string{"Event: " + it.Title}
	if s := e.span(); s != "" {
		lines = append(lines, "When: "+s)
	}
	if e.Location != "" {
		lines = append(lines, "Where: "+e.Location)
	}
	if e.HangoutLink != "" {
		lines = append(lines, "Meeting link: "+e.HangoutLink)
	}
	if o := joinNonEmpty(" ", e.Organizer.DisplayName, e.Organizer.Email); o != "" {
		lines = append(lines, "Organizer: "+o)
	}
	if len(e.Attendees) > 0 {
		lines = append(lines, "Attendees:")
		for _, a := range e.Attendees {
			who := a.Email
			if a.DisplayName != "" {
				who = a.DisplayName + " <" + a.Email + ">"
			}
			if r := responseWords[a.ResponseStatus]; r != "" {
				who += " (" + r + ")"
			}
			lines = append(lines, "- "+who)
		}
	}
	body := strings.Join(lines, "\n")
	if d := strings.TrimSpace(e.Description); d != "" {
		if looksLikeHTML(d) {
			d = htmlToText(d)
		}
		body += "\n\n" + d
	}
	return &Snapshot{Item: it, Body: body}, nil
}

var responseWords = map[string]string{"accepted": "going", "declined": "not going", "tentative": "maybe", "needsAction": "hasn't answered"}

// --- GitHub ---

type ghIssue struct {
	Number        int             `json:"number"`
	Title         string          `json:"title"`
	Body          string          `json:"body"`
	State         string          `json:"state"`
	HTMLURL       string          `json:"html_url"`
	RepositoryURL string          `json:"repository_url"`
	UpdatedAt     string          `json:"updated_at"`
	PullRequest   *map[string]any `json:"pull_request"`
	Draft         bool            `json:"draft"`
	User          struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type ghRef struct {
	Owner, Repo string
	Number      int
}

func (r ghRef) String() string { return fmt.Sprintf("%s/%s#%d", r.Owner, r.Repo, r.Number) }

var (
	ghShort = regexp.MustCompile(`^([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)#([0-9]+)$`)
	ghLink  = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)/(?:issues|pull)/([0-9]+)`)
)

// parseGHRef reads "owner/repo#12" or an issue or pull request link.
func parseGHRef(s string) (ghRef, bool) {
	s = strings.TrimSpace(s)
	m := ghShort.FindStringSubmatch(s)
	if m == nil {
		m = ghLink.FindStringSubmatch(s)
	}
	if m == nil {
		return ghRef{}, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return ghRef{}, false
	}
	return ghRef{Owner: m[1], Repo: m[2], Number: n}, true
}

// repo is "owner/repo" from the issue's repository API URL.
func (i ghIssue) repo() (string, string) {
	u, err := url.Parse(i.RepositoryURL)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func (i ghIssue) kind() string {
	if i.PullRequest != nil {
		if i.Draft {
			return "draft pull request"
		}
		return "pull request"
	}
	return "issue"
}

func (i ghIssue) item(ref ghRef) Item {
	if ref.Owner == "" {
		ref.Owner, ref.Repo = i.repo()
		ref.Number = i.Number
	}
	when := parseTime(i.UpdatedAt)
	snippet := fmt.Sprintf("%s/%s #%d · %s %s", ref.Owner, ref.Repo, ref.Number, i.State, i.kind())
	if i.User.Login != "" {
		snippet += " by " + i.User.Login
	}
	link := i.HTMLURL
	if link == "" {
		link = fmt.Sprintf("https://github.com/%s/%s/issues/%d", ref.Owner, ref.Repo, ref.Number)
	}
	return Item{App: "github", Ref: ref.String(), Title: strings.TrimSpace(i.Title), URL: link, Snippet: snippet, When: when}
}

func (c *Client) searchGitHub(ctx context.Context, query string, limit int) ([]Item, error) {
	// A pasted link or owner/repo#12 finds that one.
	if ref, ok := parseGHRef(query); ok {
		var i ghIssue
		if err := c.execute(ctx, "github", toolGitHubIssue, map[string]any{"owner": ref.Owner, "repo": ref.Repo, "issue_number": ref.Number}, &i); err != nil {
			return nil, err
		}
		c.putHint("github\x00"+ref.String(), hint{github: &i})
		return []Item{i.item(ref)}, nil
	}
	// Plain words search what you're involved in; GitHub's own qualifiers search as written.
	q := "involves:@me"
	if query != "" {
		q = query
		if !strings.Contains(query, ":") {
			q += " involves:@me"
		}
	}
	var data struct {
		Items []ghIssue `json:"items"`
	}
	if err := c.execute(ctx, "github", toolGitHubSearch, map[string]any{"q": q, "sort": "updated", "order": "desc", "per_page": limit}, &data); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(data.Items))
	for _, i := range data.Items {
		if it := i.item(ghRef{}); it.Ref != "" && !strings.HasPrefix(it.Ref, "/") {
			kept := i
			c.putHint("github\x00"+it.Ref, hint{github: &kept})
			items = append(items, it)
		}
	}
	return items, nil
}

type ghComments struct {
	Comments []struct {
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comments"`
}

func (c *Client) fetchGitHub(ctx context.Context, refText string) (*Snapshot, error) {
	ref, ok := parseGHRef(refText)
	if !ok {
		return nil, &ToolError{App: "github", Msg: "that isn't an issue or pull request Gwen can read."}
	}
	base := map[string]any{"owner": ref.Owner, "repo": ref.Repo, "issue_number": ref.Number}
	var (
		issue      ghIssue
		comments   ghComments
		wg         sync.WaitGroup
		errComment error
	)
	if h, ok := c.takeHint("github\x00" + ref.String()); ok && h.github != nil {
		issue = *h.github
		args := map[string]any{"owner": ref.Owner, "repo": ref.Repo, "issue_number": ref.Number, "per_page": 10}
		errComment = c.execute(ctx, "github", toolGitHubComments, args, &comments)
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			args := map[string]any{"owner": ref.Owner, "repo": ref.Repo, "issue_number": ref.Number, "per_page": 10}
			errComment = c.execute(ctx, "github", toolGitHubComments, args, &comments)
		}()
		err := c.execute(ctx, "github", toolGitHubIssue, base, &issue)
		wg.Wait()
		if err != nil {
			return nil, err
		}
	}
	return githubSnapshot(ref, issue, comments, errComment), nil
}

func githubSnapshot(ref ghRef, issue ghIssue, comments ghComments, errComment error) *Snapshot {
	it := issue.item(ref)
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d: %s\n%s %s", fmt.Sprintf("%s/%s", ref.Owner, ref.Repo), ref.Number, it.Title, cap1(issue.State), issue.kind())
	if issue.User.Login != "" {
		fmt.Fprintf(&b, " by %s", issue.User.Login)
	}
	if len(issue.Labels) > 0 {
		names := make([]string, len(issue.Labels))
		for i, l := range issue.Labels {
			names[i] = l.Name
		}
		fmt.Fprintf(&b, "\nLabels: %s", strings.Join(names, ", "))
	}
	if body := strings.TrimSpace(issue.Body); body != "" {
		b.WriteString("\n\n" + tidy(body))
	}
	// Comments are extra: the issue still attaches when they can't be read.
	if errComment == nil && len(comments.Comments) > 0 {
		b.WriteString("\n\nComments:")
		for _, cm := range comments.Comments {
			when := ""
			if t := parseTime(cm.CreatedAt); t != nil {
				when = " (" + t.Local().Format("2 Jan 2006") + ")"
			}
			fmt.Fprintf(&b, "\n\n%s%s:\n%s", cm.User.Login, when, tidy(cm.Body))
		}
	}
	return &Snapshot{Item: it, Body: b.String()}
}

// --- Notion ---

type notionPage struct {
	ID             string                    `json:"id"`
	URL            string                    `json:"url"`
	LastEditedTime string                    `json:"last_edited_time"`
	Archived       bool                      `json:"archived"`
	InTrash        bool                      `json:"in_trash"`
	Properties     map[string]notionProperty `json:"properties"`
	Parent         struct {
		Type string `json:"type"`
	} `json:"parent"`
}

type notionProperty struct {
	Type  string `json:"type"`
	Title []struct {
		PlainText string `json:"plain_text"`
	} `json:"title"`
}

// title is the page's title property, whatever the database calls it.
func (p notionPage) title() string {
	for _, prop := range p.Properties {
		if prop.Type != "title" {
			continue
		}
		var b strings.Builder
		for _, t := range prop.Title {
			b.WriteString(t.PlainText)
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			return s
		}
	}
	return "Untitled"
}

func (p notionPage) item() Item {
	when := parseTime(p.LastEditedTime)
	snippet := ""
	if when != nil {
		snippet = "Edited " + when.Local().Format("2 Jan 2006")
	}
	if p.Parent.Type == "database_id" {
		snippet = joinNonEmpty(" · ", "Database entry", snippet)
	}
	link := p.URL
	if link == "" {
		link = "https://www.notion.so/" + strings.ReplaceAll(p.ID, "-", "")
	}
	return Item{App: "notion", Ref: p.ID, Title: p.title(), URL: link, Snippet: snippet, When: when}
}

func (c *Client) searchNotion(ctx context.Context, query string, limit int) ([]Item, error) {
	// A pasted page link finds that page.
	if strings.Contains(query, "notion.so/") || strings.Contains(query, "notion.site/") || strings.Contains(query, "notion.com/") {
		if id := notionID(query); id != query {
			var p notionPage
			if err := c.execute(ctx, "notion", toolNotionPage, map[string]any{"page_id": id}, &p); err != nil {
				return nil, err
			}
			if p.ID != "" && !p.Archived && !p.InTrash {
				c.putHint(notionHintKey(p.ID), hint{notion: &p})
			}
			return []Item{p.item()}, nil
		}
	}
	args := map[string]any{"query": query, "page_size": limit, "direction": "descending", "timestamp": "last_edited_time", "filter_value": "page"}
	var data struct {
		Results []notionPage `json:"results"`
	}
	if err := c.execute(ctx, "notion", toolNotionSearch, args, &data); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(data.Results))
	for _, p := range data.Results {
		if p.ID != "" && !p.Archived && !p.InTrash {
			kept := p
			c.putHint(notionHintKey(p.ID), hint{notion: &kept})
			items = append(items, p.item())
		}
	}
	return items, nil
}

func (c *Client) fetchNotion(ctx context.Context, ref string) (*Snapshot, error) {
	id := notionID(ref)
	var (
		page    notionPage
		md      struct{ Markdown string }
		wg      sync.WaitGroup
		errPage error
	)
	var err error
	if h, ok := c.takeHint(notionHintKey(id)); ok && h.notion != nil {
		page = *h.notion
		err = c.execute(ctx, "notion", toolNotionMarkdown, map[string]any{"page_id": id}, &md)
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errPage = c.execute(ctx, "notion", toolNotionPage, map[string]any{"page_id": id}, &page)
		}()
		err = c.execute(ctx, "notion", toolNotionMarkdown, map[string]any{"page_id": id}, &md)
		wg.Wait()
	}
	if err != nil {
		return nil, err
	}
	if errPage != nil {
		return nil, errPage
	}
	return notionSnapshot(id, page, md.Markdown), nil
}

func notionSnapshot(id string, page notionPage, markdown string) *Snapshot {
	if page.ID == "" {
		page.ID = id
	}
	it := page.item()
	// The page's Markdown keeps Notion's soft line breaks as <br>.
	return &Snapshot{Item: it, Body: "# " + it.Title + "\n\n" + tidy(mdBreak.ReplaceAllString(markdown, "\n"))}
}

var mdBreak = regexp.MustCompile(`(?i)<br\s*/?>`)

var notionHex = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// notionID takes a page id as is, or the id out of a pasted page link, where
// it is the last 32 hex digits of the path ("…/Plan-1a2b…").
func notionID(ref string) string {
	if !strings.HasPrefix(ref, "http") {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	p := strings.ReplaceAll(u.Path, "-", "")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	if len(p) >= 32 && notionHex.MatchString(p[len(p)-32:]) {
		return p[len(p)-32:]
	}
	return ref
}
