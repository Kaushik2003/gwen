package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/kzark/gwen/internal/wire"
)

// The memory endpoints (/v1/memory/...). Each answers unavailable while the
// daemon runs no memory.

// RememberCLI is the channel of words typed in the terminal to be remembered;
// "" is the dashboard.
const RememberCLI = "cli"

// AssistantSelfHistory lists how the assistant chose to treat the user over
// time, newest first; limit 0 means the daemon's default.
func (c *Client) AssistantSelfHistory(ctx context.Context, limit int) (*wire.SelfHistory, error) {
	return call[wire.SelfHistory](ctx, c, http.MethodGet, "/v1/assistant/self/history", query("limit", count(limit)), nil)
}

func (c *Client) MemoryStatus(ctx context.Context) (*wire.MemoryStatus, error) {
	return call[wire.MemoryStatus](ctx, c, http.MethodGet, "/v1/memory/status", nil, nil)
}

// ListClaims lists claims; the query's zero values are the daemon's defaults
// (active claims, not hidden, the first 200).
func (c *Client) ListClaims(ctx context.Context, q wire.MemoryClaimQuery) (*wire.MemoryClaimList, error) {
	hidden := ""
	if q.Hidden {
		hidden = "true"
	}
	return call[wire.MemoryClaimList](ctx, c, http.MethodGet, "/v1/memory/claims", query("status", q.Status,
		"key", q.Key, "subject", q.Subject, "q", q.Q, "hidden", hidden, "limit", count(q.Limit),
		"offset", count(q.Offset)), nil)
}

func (c *Client) GetClaim(ctx context.Context, id string) (*wire.MemoryClaim, error) {
	return call[wire.MemoryClaim](ctx, c, http.MethodGet, claimPath(id), nil, nil)
}

func (c *Client) EditClaim(ctx context.Context, id string, req wire.EditClaimRequest) (*wire.MemoryClaimsResult, error) {
	return call[wire.MemoryClaimsResult](ctx, c, http.MethodPatch, claimPath(id), nil, req)
}

func (c *Client) ConfirmClaim(ctx context.Context, id string) (*wire.MemoryClaim, error) {
	return call[wire.MemoryClaim](ctx, c, http.MethodPost, claimPath(id)+"/confirm", nil, empty)
}

func (c *Client) RejectClaim(ctx context.Context, id string) (*wire.MemoryClaim, error) {
	return call[wire.MemoryClaim](ctx, c, http.MethodPost, claimPath(id)+"/reject", nil, empty)
}

func (c *Client) HideClaim(ctx context.Context, id string, req wire.HideClaimRequest) (*wire.MemoryClaim, error) {
	return call[wire.MemoryClaim](ctx, c, http.MethodPost, claimPath(id)+"/hide", nil, req)
}

// ForgetClaim erases a claim everywhere; the receipt says where.
func (c *Client) ForgetClaim(ctx context.Context, id string) (*wire.ForgetReceipt, error) {
	return call[wire.ForgetReceipt](ctx, c, http.MethodDelete, claimPath(id), nil, nil)
}

// ExplainMemory says why Gwen holds what ref names (claim:<id>, msg:<id>, ...).
func (c *Client) ExplainMemory(ctx context.Context, ref string) (*wire.MemoryExplanation, error) {
	return call[wire.MemoryExplanation](ctx, c, http.MethodGet, "/v1/memory/explain", query("ref", ref), nil)
}

// Remember has Gwen remember words the user typed; channel is RememberCLI
// from the terminal, "" from the dashboard.
func (c *Client) Remember(ctx context.Context, req wire.RememberRequest, channel string) (*wire.MemoryClaimsResult, error) {
	return call[wire.MemoryClaimsResult](ctx, c, http.MethodPost, "/v1/memory/remember", query("channel", channel), req)
}

func (c *Client) RecallMemory(ctx context.Context, req wire.RecallRequest) (*wire.RecallResult, error) {
	return call[wire.RecallResult](ctx, c, http.MethodPost, "/v1/memory/recall", nil, req)
}

// ListConflicts lists conflicts in status; "" means open, "all" every status.
func (c *Client) ListConflicts(ctx context.Context, status string) (*wire.MemoryConflictList, error) {
	return call[wire.MemoryConflictList](ctx, c, http.MethodGet, "/v1/memory/conflicts", query("status", status), nil)
}

func (c *Client) ResolveConflict(ctx context.Context, id string, req wire.ResolveConflictRequest) (*wire.MemoryConflict, error) {
	path := "/v1/memory/conflicts/" + url.PathEscape(id) + "/resolve"
	return call[wire.MemoryConflict](ctx, c, http.MethodPost, path, nil, req)
}

func (c *Client) ListEntities(ctx context.Context) (*wire.MemoryEntityList, error) {
	return call[wire.MemoryEntityList](ctx, c, http.MethodGet, "/v1/memory/entities", nil, nil)
}

// MemoryProfile is today's profile card.
func (c *Client) MemoryProfile(ctx context.Context) (*wire.ProfileCard, error) {
	return call[wire.ProfileCard](ctx, c, http.MethodGet, "/v1/memory/profile", nil, nil)
}

func (c *Client) ListResources(ctx context.Context) (*wire.MemoryResourceList, error) {
	return call[wire.MemoryResourceList](ctx, c, http.MethodGet, "/v1/memory/resources", nil, nil)
}

// AddResource has Gwen read a document on the daemon's device.
func (c *Client) AddResource(ctx context.Context, req wire.AddResourceRequest) (*wire.MemoryResource, error) {
	return call[wire.MemoryResource](ctx, c, http.MethodPost, "/v1/memory/resources", nil, req)
}

func (c *Client) RemoveResource(ctx context.Context, id string) (*wire.ForgetReceipt, error) {
	return call[wire.ForgetReceipt](ctx, c, http.MethodDelete, "/v1/memory/resources/"+url.PathEscape(id), nil, nil)
}

// RebuildMemory rebuilds a derived lane: vec, files, mem0, or supermemory.
func (c *Client) RebuildMemory(ctx context.Context, req wire.RebuildRequest) error {
	return c.do(ctx, http.MethodPost, "/v1/memory/rebuild", nil, req, nil)
}

// SleepMemory runs consolidation now.
func (c *Client) SleepMemory(ctx context.Context) (*wire.SleepReport, error) {
	return call[wire.SleepReport](ctx, c, http.MethodPost, "/v1/memory/sleep", nil, empty)
}

func (c *Client) ExportMemory(ctx context.Context) (*wire.MemoryExport, error) {
	return call[wire.MemoryExport](ctx, c, http.MethodGet, "/v1/memory/export", nil, nil)
}

// ForgetAllConfirm is what ForgetAllMemory's confirm must be.
const ForgetAllConfirm = "forget everything"

// ForgetAllMemory erases everything memory holds; chats are kept. The daemon
// refuses unless confirm is ForgetAllConfirm.
func (c *Client) ForgetAllMemory(ctx context.Context, confirm string) (*wire.ForgetReceipt, error) {
	body := struct {
		Confirm string `json:"confirm"`
	}{confirm}
	return call[wire.ForgetReceipt](ctx, c, http.MethodPost, "/v1/memory/forget-all", nil, body)
}

func claimPath(id string) string { return "/v1/memory/claims/" + url.PathEscape(id) }
