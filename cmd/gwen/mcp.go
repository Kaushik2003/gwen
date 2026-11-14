package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/kzark/gwen/internal/wire"
)

// `gwen mcp` serves Gwen's tools to the Claude Code process behind one of her
// chats (plan.md 3.16): a stdio MCP server that lists the tools gwend offers
// on agent.sock and passes each call there with the chat's token. Gwend's
// policy checks every call; this command only carries it.

const agentTokenHeader = "X-Gwen-Agent-Token"

func (c *cli) mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "mcp",
		Short:  "Serve Gwen's tools to her agent's Claude Code (Gwen starts this)",
		Hidden: true,
		Args:   cobra.NoArgs,
		// Nothing of the root's setup is needed, and stdout belongs to MCP.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd.Context(), os.Getenv("GWEN_AGENT_SOCKET"), os.Getenv("GWEN_AGENT_TOKEN"))
		},
	}
}

func runMCP(ctx context.Context, socket, token string) error {
	if socket == "" || token == "" {
		return errors.New("gwen mcp runs only inside one of Gwen's chats")
	}
	hc := &http.Client{Timeout: 30 * time.Minute, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}}}
	var list wire.AgentToolList
	if err := agentCall(ctx, hc, http.MethodGet, "/v1/agent/tools", token, nil, &list); err != nil {
		return fmt.Errorf("list the tools: %w", err)
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "gwen", Version: version}, nil)
	for _, t := range list.Tools {
		var schema any
		if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
			continue
		}
		name := t.Name
		srv.AddTool(&mcp.Tool{Name: name, Description: t.Description, InputSchema: schema},
			func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args := req.Params.Arguments
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				var res wire.AgentToolResult
				if err := agentCall(ctx, hc, http.MethodPost, "/v1/agent/tools/"+name, token, args, &res); err != nil {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
				}
				text, _ := json.Marshal(res)
				return &mcp.CallToolResult{IsError: !res.OK, Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, nil
			})
	}
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// agentCall calls agent.sock with the chat's token.
func agentCall(ctx context.Context, hc *http.Client, method, path, token string, body json.RawMessage, out any) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://gwend"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set(agentTokenHeader, token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("gwend isn't reachable: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		var e wire.ErrorResponse
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			return errors.New(e.Error.Message)
		}
		return fmt.Errorf("gwend answered %s", resp.Status)
	}
	return json.Unmarshal(data, out)
}
