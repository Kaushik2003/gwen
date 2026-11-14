package main

import "github.com/kzark/gwen/internal/wire"

// Gwen's agent (plan.md 3.16, W32): her chats, your messages, Stop, and Undo.
// Approvals in a chat are decided with DecideApproval, by a click.

func (a *App) AgentStatus() (*wire.AgentStatus, error) { return call(a.api.AgentStatus(a.ctx)) }
func (a *App) ListAgentChats(project string) (*wire.AgentChatList, error) {
	return call(a.api.ListAgentChats(a.ctx, project))
}
func (a *App) CreateAgentChat(req wire.CreateAgentChat) (*wire.AgentChat, error) {
	return call(a.api.CreateAgentChat(a.ctx, req))
}
func (a *App) GetAgentChat(id string) (*wire.AgentChatDetail, error) {
	return call(a.api.GetAgentChat(a.ctx, id))
}
func (a *App) PatchAgentChat(id string, req wire.PatchAgentChat) (*wire.AgentChat, error) {
	return call(a.api.PatchAgentChat(a.ctx, id, req))
}
func (a *App) DeleteAgentChat(id string) error { return wrap(a.api.DeleteAgentChat(a.ctx, id)) }
func (a *App) SendAgentMessage(id string, req wire.AgentMessageRequest) (*wire.AgentChat, error) {
	return call(a.api.SendAgentMessage(a.ctx, id, req))
}
func (a *App) StopAgentChat(id string) error { return wrap(a.api.StopAgentChat(a.ctx, id)) }
func (a *App) UndoAgentStep(id string) (*wire.AgentStep, error) {
	return call(a.api.UndoAgentStep(a.ctx, id))
}
