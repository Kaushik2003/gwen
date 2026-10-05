package clienttest

import (
	"context"

	"github.com/kzark/gwen/internal/wire"
)

func (f *Fake) AssistantChat(_ context.Context, req wire.AssistantChatRequest) (*wire.LlmRun, error) {
	return result[*wire.LlmRun](f, "AssistantChat", req)
}

func (f *Fake) ScheduleTask(_ context.Context, req wire.ScheduleRequest) (*wire.PlanItem, error) {
	return result[*wire.PlanItem](f, "ScheduleTask", req)
}

func (f *Fake) UnscheduleTask(_ context.Context, req wire.UnscheduleRequest) (*wire.Unscheduled, error) {
	return result[*wire.Unscheduled](f, "UnscheduleTask", req)
}

func (f *Fake) GetReview(_ context.Context, weekStart string) (*wire.WeeklyReview, error) {
	return result[*wire.WeeklyReview](f, "GetReview", weekStart)
}

func (f *Fake) SaveReview(_ context.Context, weekStart string, req wire.SaveReviewRequest) (*wire.WeeklyReview, error) {
	return result[*wire.WeeklyReview](f, "SaveReview", weekStart, req)
}

func (f *Fake) EnergyReport(_ context.Context, days int) (*wire.EnergyReport, error) {
	return result[*wire.EnergyReport](f, "EnergyReport", days)
}

func (f *Fake) LogEnergy(_ context.Context, req wire.LogEnergyRequest) (*wire.EnergyLog, error) {
	return result[*wire.EnergyLog](f, "LogEnergy", req)
}

func (f *Fake) DeleteEnergy(_ context.Context, id string) error {
	_, err := f.invoke("DeleteEnergy", id)
	return err
}
