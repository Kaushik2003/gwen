package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) AssistantSelfHistory(_ context.Context, limit int) (*wire.SelfHistory, error) {
	return result[*wire.SelfHistory](f, "AssistantSelfHistory", limit)
}

func (f *Fake) MemoryStatus(_ context.Context) (*wire.MemoryStatus, error) {
	return result[*wire.MemoryStatus](f, "MemoryStatus")
}

func (f *Fake) ListClaims(_ context.Context, q wire.MemoryClaimQuery) (*wire.MemoryClaimList, error) {
	return result[*wire.MemoryClaimList](f, "ListClaims", q)
}

func (f *Fake) GetClaim(_ context.Context, id string) (*wire.MemoryClaim, error) {
	return result[*wire.MemoryClaim](f, "GetClaim", id)
}

func (f *Fake) EditClaim(_ context.Context, id string, req wire.EditClaimRequest) (*wire.MemoryClaimsResult, error) {
	return result[*wire.MemoryClaimsResult](f, "EditClaim", id, req)
}

func (f *Fake) ConfirmClaim(_ context.Context, id string) (*wire.MemoryClaim, error) {
	return result[*wire.MemoryClaim](f, "ConfirmClaim", id)
}

func (f *Fake) RejectClaim(_ context.Context, id string) (*wire.MemoryClaim, error) {
	return result[*wire.MemoryClaim](f, "RejectClaim", id)
}

func (f *Fake) HideClaim(_ context.Context, id string, req wire.HideClaimRequest) (*wire.MemoryClaim, error) {
	return result[*wire.MemoryClaim](f, "HideClaim", id, req)
}

func (f *Fake) ForgetClaim(_ context.Context, id string) (*wire.ForgetReceipt, error) {
	return result[*wire.ForgetReceipt](f, "ForgetClaim", id)
}

func (f *Fake) ExplainMemory(_ context.Context, ref string) (*wire.MemoryExplanation, error) {
	return result[*wire.MemoryExplanation](f, "ExplainMemory", ref)
}

func (f *Fake) Remember(_ context.Context, req wire.RememberRequest, channel string) (*wire.MemoryClaimsResult, error) {
	return result[*wire.MemoryClaimsResult](f, "Remember", req, channel)
}

func (f *Fake) RecallMemory(_ context.Context, req wire.RecallRequest) (*wire.RecallResult, error) {
	return result[*wire.RecallResult](f, "RecallMemory", req)
}

func (f *Fake) ListConflicts(_ context.Context, status string) (*wire.MemoryConflictList, error) {
	return result[*wire.MemoryConflictList](f, "ListConflicts", status)
}

func (f *Fake) ResolveConflict(_ context.Context, id string, req wire.ResolveConflictRequest) (*wire.MemoryConflict, error) {
	return result[*wire.MemoryConflict](f, "ResolveConflict", id, req)
}

func (f *Fake) ListEntities(_ context.Context) (*wire.MemoryEntityList, error) {
	return result[*wire.MemoryEntityList](f, "ListEntities")
}

func (f *Fake) MemoryProfile(_ context.Context) (*wire.ProfileCard, error) {
	return result[*wire.ProfileCard](f, "MemoryProfile")
}

func (f *Fake) ListResources(_ context.Context) (*wire.MemoryResourceList, error) {
	return result[*wire.MemoryResourceList](f, "ListResources")
}

func (f *Fake) AddResource(_ context.Context, req wire.AddResourceRequest) (*wire.MemoryResource, error) {
	return result[*wire.MemoryResource](f, "AddResource", req)
}

func (f *Fake) RemoveResource(_ context.Context, id string) (*wire.ForgetReceipt, error) {
	return result[*wire.ForgetReceipt](f, "RemoveResource", id)
}

func (f *Fake) RebuildMemory(_ context.Context, req wire.RebuildRequest) error {
	_, err := f.invoke("RebuildMemory", req)
	return err
}

func (f *Fake) SleepMemory(_ context.Context) (*wire.SleepReport, error) {
	return result[*wire.SleepReport](f, "SleepMemory")
}

func (f *Fake) ExportMemory(_ context.Context) (*wire.MemoryExport, error) {
	return result[*wire.MemoryExport](f, "ExportMemory")
}

func (f *Fake) ForgetAllMemory(_ context.Context, confirm string) (*wire.ForgetReceipt, error) {
	return result[*wire.ForgetReceipt](f, "ForgetAllMemory", confirm)
}
