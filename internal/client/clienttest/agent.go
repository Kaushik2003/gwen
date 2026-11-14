package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) AgentStatus(_ context.Context) (*wire.AgentStatus, error) {
	return result[*wire.AgentStatus](f, "AgentStatus")
}

func (f *Fake) ListAgentChats(_ context.Context, project string) (*wire.AgentChatList, error) {
	return result[*wire.AgentChatList](f, "ListAgentChats", project)
}

func (f *Fake) CreateAgentChat(_ context.Context, req wire.CreateAgentChat) (*wire.AgentChat, error) {
	return result[*wire.AgentChat](f, "CreateAgentChat", req)
}

func (f *Fake) GetAgentChat(_ context.Context, id string) (*wire.AgentChatDetail, error) {
	return result[*wire.AgentChatDetail](f, "GetAgentChat", id)
}

func (f *Fake) PatchAgentChat(_ context.Context, id string, req wire.PatchAgentChat) (*wire.AgentChat, error) {
	return result[*wire.AgentChat](f, "PatchAgentChat", id, req)
}

func (f *Fake) DeleteAgentChat(_ context.Context, id string) error {
	_, err := f.invoke("DeleteAgentChat", id)
	return err
}

func (f *Fake) SendAgentMessage(_ context.Context, id string, req wire.AgentMessageRequest) (*wire.AgentChat, error) {
	return result[*wire.AgentChat](f, "SendAgentMessage", id, req)
}

func (f *Fake) StopAgentChat(_ context.Context, id string) error {
	_, err := f.invoke("StopAgentChat", id)
	return err
}

func (f *Fake) UndoAgentStep(_ context.Context, id string) (*wire.AgentStep, error) {
	return result[*wire.AgentStep](f, "UndoAgentStep", id)
}
