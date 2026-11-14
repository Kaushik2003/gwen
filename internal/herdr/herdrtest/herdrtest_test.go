package herdrtest_test

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/herdr"
	"github.com/kzark/gwen/internal/herdr/herdrtest"
	"github.com/stretchr/testify/require"
)

// plainConn is a raw client in plain mode, the way socat or any language
// would talk to herdr-core.
type plainConn struct {
	t  *testing.T
	nc net.Conn
	r  *bufio.Reader
}

func dialPlain(t *testing.T, core *herdrtest.Core) *plainConn {
	t.Helper()
	nc, err := net.Dial("unix", core.Socket())
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })
	// A deadline only guards against a hang; every read is expected to
	// succeed at once.
	require.NoError(t, nc.SetDeadline(time.Now().Add(10*time.Second)))
	return &plainConn{t: t, nc: nc, r: bufio.NewReader(nc)}
}

func (p *plainConn) send(line string) {
	p.t.Helper()
	_, err := p.nc.Write([]byte(line + "\n"))
	require.NoError(p.t, err)
}

func (p *plainConn) read() map[string]any {
	p.t.Helper()
	line, err := p.r.ReadBytes('\n')
	require.NoError(p.t, err)
	var msg map[string]any
	require.NoError(p.t, json.Unmarshal(line, &msg))
	return msg
}

// Plain mode works without hello, one message per line, and attach says it
// needs framed mode: anything outside internal/herdr may speak it.
func TestPlainMode(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{Name: "shell"})
	p := dialPlain(t, core)

	p.send(`{"jsonrpc":"2.0","id":1,"method":"terminal.list","params":{}}`)
	msg := p.read()
	require.EqualValues(t, 1, msg["id"])
	terms := msg["result"].(map[string]any)["terminals"].([]any)
	require.Len(t, terms, 1)
	require.Equal(t, t1.Address, terms[0].(map[string]any)["address"])

	p.send(`{"jsonrpc":"2.0","id":2,"method":"terminal.attach","params":{"terminal":"t1","cols":80,"rows":24}}`)
	msg = p.read()
	require.Equal(t, "needs_framed", msg["error"].(map[string]any)["data"].(map[string]any)["kind"])

	p.send(`{"jsonrpc":"2.0","id":3,"method":"no.such","params":{}}`)
	msg = p.read()
	require.EqualValues(t, -32601, msg["error"].(map[string]any)["code"])
}

// A plain subscriber gets its snapshot, then only notification lines; an
// event raised as it subscribes still comes after the reply.
func TestPlainSubscribe(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	t1 := core.AddTerminal(herdr.Terminal{})
	p := dialPlain(t, core)

	p.send(`{"jsonrpc":"2.0","id":7,"method":"events.subscribe","params":{}}`)
	msg := p.read()
	require.EqualValues(t, 7, msg["id"])
	result := msg["result"].(map[string]any)
	require.NotContains(t, result, "channel", "a plain subscription has no channel")
	require.EqualValues(t, 1, result["seq"], "the created event of t1")

	core.SetTitle(t1.Address, "vim")
	msg = p.read()
	require.Equal(t, "event", msg["method"])
	params := msg["params"].(map[string]any)
	require.EqualValues(t, 2, params["seq"])
	require.Equal(t, "title", params["type"])

	core.Missed()
	msg = p.read()
	require.Equal(t, "missed", msg["method"])
}

// A hello that names another generation is refused, as herdr-core does,
// and the connection stays in plain mode.
func TestHelloGeneration(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	p := dialPlain(t, core)

	p.send(`{"jsonrpc":"2.0","id":0,"method":"hello","params":{"generation":2,"framed":true}}`)
	msg := p.read()
	require.Equal(t, "unsupported_generation", msg["error"].(map[string]any)["data"].(map[string]any)["kind"])

	p.send(`{"jsonrpc":"2.0","id":1,"method":"doc.get","params":{"name":"x"}}`)
	msg = p.read()
	require.Equal(t, map[string]any{"value": nil, "version": float64(0)}, msg["result"])
}

// doc.set takes the version it read and refuses a stale one.
func TestDocSet(t *testing.T) {
	t.Parallel()
	core := herdrtest.New(t, herdrtest.Options{})
	c := core.Connect()
	ctx := t.Context()

	var r struct {
		Version uint64 `json:"version"`
	}
	require.NoError(t, c.Call(ctx, "doc.set", map[string]any{"name": "d", "value": []int{1}, "version": 0}, &r))
	require.EqualValues(t, 1, r.Version)
	err := c.Call(ctx, "doc.set", map[string]any{"name": "d", "value": []int{2}, "version": 0}, nil)
	require.True(t, herdr.IsKind(err, "conflict"), "%v", err)
	value, version := core.Doc("d")
	require.JSONEq(t, `[1]`, string(value))
	require.EqualValues(t, 1, version)
}
