package harness

import (
	"context"
	"errors"
	"fmt"

	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
)

// KindRoutine is the routine run kind's name.
const KindRoutine = "routine"

// RoutineOf finds the routine a subject's start runs: its own, else its
// parent's (a task's template, an event's series, a commitment's).
type RoutineOf func(ctx context.Context, subject string) (store.Routine, bool, error)

// Routine is the routine run kind (plan.md 3.11, the DSA workflow): its
// steps are the subject's "when it starts" steps, each one capability call,
// run in order and logged; a failed one is reported, and the rest still run.
func Routine(reg *connect.Registry, routineOf RoutineOf) Kind {
	return routineKind{reg: reg, of: routineOf}
}

type routineKind struct {
	reg *connect.Registry
	of  RoutineOf
}

func (routineKind) Name() string { return KindRoutine }

func (k routineKind) Begin(ctx context.Context, r store.Run) ([]StepSpec, error) {
	if r.Subject == "" {
		return nil, errors.New("a routine run needs a subject")
	}
	rt, ok, err := k.of(ctx, r.Subject)
	if err != nil {
		return nil, err
	}
	if !ok || len(rt.Steps) == 0 {
		return nil, errors.New("there are no steps for when it starts")
	}
	specs := make([]StepSpec, 0, len(rt.Steps))
	for _, st := range rt.Steps {
		specs = append(specs, StepSpec{Title: StepTitle(k.reg, st.Title, st.Capability, st.Input), Capability: st.Capability, Input: st.Input})
	}
	return specs, nil
}

func (routineKind) Step(ctx context.Context, _ store.Run, s store.RunStep, x Exec) (StepResult, error) {
	return callStep(ctx, s, x)
}

// callStep is a step that is one capability call.
func callStep(ctx context.Context, s store.RunStep, x Exec) (StepResult, error) {
	if s.Capability == "" {
		return StepResult{}, fmt.Errorf("step %q has nothing to do", s.Title)
	}
	out, err := x.Invoke(ctx, s.Capability, s.Input)
	return StepResult{Output: out}, err
}

// StepTitle is a step's title: the one given, else what the capability says
// the call will do, else its name.
func StepTitle(reg *connect.Registry, title, capability string, input []byte) string {
	if title != "" {
		return title
	}
	if c, _, ok := reg.Capability(capability); ok {
		if in, err := c.Check(input); err == nil {
			return c.Summary(in)
		}
		return c.Title
	}
	return capability
}
