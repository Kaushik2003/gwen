// Package clienttest provides Fake, an in-memory client.API whose responses
// tests script and whose received requests tests inspect.
package clienttest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
)

// ErrNotScripted is returned by a method that has no handler.
var ErrNotScripted = errors.New("clienttest: method not scripted")

// Call is one method call received by the Fake.
type Call struct {
	Method string // the client.API method name, e.g. "ClockIn"
	Args   []any  // the arguments after ctx, e.g. [wire.ClockInRequest{...}]
}

// Handler answers a call. It returns the method's result (a pointer such as
// *wire.Status, a client.EventStream, or nil for methods that return only an
// error) and the error.
type Handler func(args ...any) (any, error)

// Fake is an in-memory client.API. Script it with On or Returns; unscripted
// methods return ErrNotScripted. It is safe for concurrent use.
type Fake struct {
	mu       sync.Mutex
	handlers map[string]Handler
	calls    []Call
}

var _ client.API = (*Fake)(nil)

var apiType = reflect.TypeFor[client.API]()

// New returns a Fake with nothing scripted.
func New() *Fake { return &Fake{handlers: map[string]Handler{}} }

// On makes h answer every call of method. Naming a method client.API does not
// have panics.
func (f *Fake) On(method string, h Handler) *Fake {
	if _, ok := apiType.MethodByName(method); !ok {
		panic(fmt.Sprintf("clienttest: client.API has no method %q", method))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = h
	return f
}

// Returns makes every call of method return v and err. A v of the wrong type
// for the method panics.
func (f *Fake) Returns(method string, v any, err error) *Fake {
	m, ok := apiType.MethodByName(method)
	if !ok {
		panic(fmt.Sprintf("clienttest: client.API has no method %q", method))
	}
	if v != nil {
		if m.Type.NumOut() != 2 || !reflect.TypeOf(v).AssignableTo(m.Type.Out(0)) {
			panic(fmt.Sprintf("clienttest: %s cannot return %T", method, v))
		}
	}
	return f.On(method, func(...any) (any, error) { return v, err })
}

// Calls returns every call received so far, in order.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// CallsTo returns the calls of one method, in order.
func (f *Fake) CallsTo(method string) []Call {
	var out []Call
	for _, c := range f.Calls() {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// Reset forgets the recorded calls; handlers stay.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *Fake) invoke(method string, args ...any) (any, error) {
	f.mu.Lock()
	f.calls = append(f.calls, Call{Method: method, Args: args})
	h := f.handlers[method]
	f.mu.Unlock()
	if h == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotScripted, method)
	}
	return h(args...)
}

func result[T any](f *Fake, method string, args ...any) (T, error) {
	var zero T
	v, err := f.invoke(method, args...)
	if v == nil {
		return zero, err
	}
	t, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("clienttest: %s handler returned %T, want %T", method, v, zero))
	}
	return t, err
}

func (f *Fake) Health(context.Context) (*wire.Health, error) {
	return result[*wire.Health](f, "Health")
}

func (f *Fake) Status(context.Context) (*wire.Status, error) {
	return result[*wire.Status](f, "Status")
}

func (f *Fake) ClockIn(_ context.Context, req wire.ClockInRequest) (*wire.Status, error) {
	return result[*wire.Status](f, "ClockIn", req)
}

func (f *Fake) ClockOut(context.Context) (*wire.Status, error) {
	return result[*wire.Status](f, "ClockOut")
}

func (f *Fake) BreakStart(context.Context) (*wire.Status, error) {
	return result[*wire.Status](f, "BreakStart")
}

func (f *Fake) BreakEnd(context.Context) (*wire.Status, error) {
	return result[*wire.Status](f, "BreakEnd")
}

func (f *Fake) Switch(_ context.Context, req wire.SwitchRequest) (*wire.Status, error) {
	return result[*wire.Status](f, "Switch", req)
}

func (f *Fake) Snooze(context.Context) (*wire.Status, error) {
	return result[*wire.Status](f, "Snooze")
}

func (f *Fake) ListDays(_ context.Context, from, to string) (*wire.DayList, error) {
	return result[*wire.DayList](f, "ListDays", from, to)
}

func (f *Fake) GetDay(_ context.Context, day string) (*wire.DayDetail, error) {
	return result[*wire.DayDetail](f, "GetDay", day)
}

func (f *Fake) PatchDay(_ context.Context, day string, req wire.PatchDayRequest) (*wire.WorkDay, error) {
	return result[*wire.WorkDay](f, "PatchDay", day, req)
}

func (f *Fake) CreateSegment(_ context.Context, req wire.CreateSegmentRequest) (*wire.Segment, error) {
	return result[*wire.Segment](f, "CreateSegment", req)
}

func (f *Fake) PatchSegment(_ context.Context, id string, req wire.PatchSegmentRequest) (*wire.Segment, error) {
	return result[*wire.Segment](f, "PatchSegment", id, req)
}

func (f *Fake) SplitSegment(_ context.Context, id string, req wire.SplitSegmentRequest) (*wire.SegmentList, error) {
	return result[*wire.SegmentList](f, "SplitSegment", id, req)
}

func (f *Fake) DeleteSegment(_ context.Context, id string) error {
	_, err := f.invoke("DeleteSegment", id)
	return err
}

func (f *Fake) ListProjects(_ context.Context, archived string) (*wire.ProjectList, error) {
	return result[*wire.ProjectList](f, "ListProjects", archived)
}

func (f *Fake) CreateProject(_ context.Context, req wire.CreateProjectRequest) (*wire.Project, error) {
	return result[*wire.Project](f, "CreateProject", req)
}

func (f *Fake) GetProject(_ context.Context, id string) (*wire.Project, error) {
	return result[*wire.Project](f, "GetProject", id)
}

func (f *Fake) PatchProject(_ context.Context, id string, req wire.PatchProjectRequest) (*wire.Project, error) {
	return result[*wire.Project](f, "PatchProject", id, req)
}

func (f *Fake) DeleteProject(_ context.Context, id string) error {
	_, err := f.invoke("DeleteProject", id)
	return err
}

func (f *Fake) ListTasks(_ context.Context, q wire.TaskQuery) (*wire.TaskList, error) {
	return result[*wire.TaskList](f, "ListTasks", q)
}

func (f *Fake) CreateTask(_ context.Context, req wire.CreateTaskRequest) (*wire.Task, error) {
	return result[*wire.Task](f, "CreateTask", req)
}

func (f *Fake) GetTask(_ context.Context, id string) (*wire.Task, error) {
	return result[*wire.Task](f, "GetTask", id)
}

func (f *Fake) PatchTask(_ context.Context, id string, req wire.PatchTaskRequest) (*wire.Task, error) {
	return result[*wire.Task](f, "PatchTask", id, req)
}

func (f *Fake) CompleteTask(_ context.Context, id string, req wire.CompleteTaskRequest) (*wire.Task, error) {
	return result[*wire.Task](f, "CompleteTask", id, req)
}

func (f *Fake) ReopenTask(_ context.Context, id string) (*wire.Task, error) {
	return result[*wire.Task](f, "ReopenTask", id)
}

func (f *Fake) DeleteTask(_ context.Context, id string) error {
	_, err := f.invoke("DeleteTask", id)
	return err
}

func (f *Fake) DeleteTasks(_ context.Context, req wire.DeleteTasksRequest) (*wire.DeletedTasks, error) {
	return result[*wire.DeletedTasks](f, "DeleteTasks", req)
}

func (f *Fake) StatsSummary(_ context.Context, from, to string) (*wire.StatsSummary, error) {
	return result[*wire.StatsSummary](f, "StatsSummary", from, to)
}

func (f *Fake) StatsHeatmap(_ context.Context, year int) (*wire.Heatmap, error) {
	return result[*wire.Heatmap](f, "StatsHeatmap", year)
}

func (f *Fake) GetConfig(context.Context) (*wire.Config, error) {
	return result[*wire.Config](f, "GetConfig")
}

func (f *Fake) PatchConfig(_ context.Context, patch wire.ConfigPatch) (*wire.Config, error) {
	return result[*wire.Config](f, "PatchConfig", patch)
}

func (f *Fake) NotifyTest(context.Context) (*wire.NotifyTestResult, error) {
	return result[*wire.NotifyTestResult](f, "NotifyTest")
}

func (f *Fake) ListGoals(_ context.Context, status string) (*wire.GoalList, error) {
	return result[*wire.GoalList](f, "ListGoals", status)
}

func (f *Fake) CreateGoal(_ context.Context, req wire.CreateGoalRequest) (*wire.Goal, error) {
	return result[*wire.Goal](f, "CreateGoal", req)
}

func (f *Fake) GetGoal(_ context.Context, id string) (*wire.Goal, error) {
	return result[*wire.Goal](f, "GetGoal", id)
}

func (f *Fake) PatchGoal(_ context.Context, id string, req wire.PatchGoalRequest) (*wire.Goal, error) {
	return result[*wire.Goal](f, "PatchGoal", id, req)
}

func (f *Fake) DeleteGoal(_ context.Context, id string, openTasks bool) error {
	_, err := f.invoke("DeleteGoal", id, openTasks)
	return err
}

func (f *Fake) PreviewGoal(_ context.Context, req wire.GoalPreviewRequest) (*wire.GoalProgress, error) {
	return result[*wire.GoalProgress](f, "PreviewGoal", req)
}

func (f *Fake) ListCommitments(context.Context) (*wire.CommitmentList, error) {
	return result[*wire.CommitmentList](f, "ListCommitments")
}

func (f *Fake) CreateCommitment(_ context.Context, req wire.CreateCommitmentRequest) (*wire.Commitment, error) {
	return result[*wire.Commitment](f, "CreateCommitment", req)
}

func (f *Fake) PatchCommitment(_ context.Context, id string, req wire.PatchCommitmentRequest) (*wire.Commitment, error) {
	return result[*wire.Commitment](f, "PatchCommitment", id, req)
}

func (f *Fake) DeleteCommitment(_ context.Context, id string) error {
	_, err := f.invoke("DeleteCommitment", id)
	return err
}

func (f *Fake) GetPlan(_ context.Context, day string) (*wire.Plan, error) {
	return result[*wire.Plan](f, "GetPlan", day)
}

func (f *Fake) GeneratePlan(_ context.Context, req wire.GeneratePlanRequest) (*wire.Plan, error) {
	return result[*wire.Plan](f, "GeneratePlan", req)
}

func (f *Fake) SetDayHours(_ context.Context, req wire.SetDayHoursRequest) (*wire.Plan, error) {
	return result[*wire.Plan](f, "SetDayHours", req)
}

func (f *Fake) PatchPlanItem(_ context.Context, id string, req wire.PatchPlanItemRequest) (*wire.PlanItem, error) {
	return result[*wire.PlanItem](f, "PatchPlanItem", id, req)
}

func (f *Fake) MovePlanTask(_ context.Context, req wire.MovePlanTaskRequest) (*wire.Plan, error) {
	return result[*wire.Plan](f, "MovePlanTask", req)
}

func (f *Fake) Briefing(context.Context) (*wire.Briefing, error) {
	return result[*wire.Briefing](f, "Briefing")
}

// Events returns what the "Events" handler returns, typically a *Stream.
func (f *Fake) Events(context.Context) (client.EventStream, error) {
	return result[client.EventStream](f, "Events")
}

// Stream is a client.EventStream a test feeds by hand.
type Stream struct {
	events chan wire.Event
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	err    error
}

var _ client.EventStream = (*Stream)(nil)

// NewStream returns an open stream with room for 64 unread events.
func NewStream() *Stream {
	return &Stream{events: make(chan wire.Event, 64), done: make(chan struct{})}
}

// Send queues an event for Next. It must not be called after End.
func (s *Stream) Send(ev wire.Event) { s.events <- ev }

// End ends the stream: once the queued events are read, Next returns err, or
// io.EOF when err is nil.
func (s *Stream) End(err error) {
	s.once.Do(func() {
		if err == nil {
			err = io.EOF
		}
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		close(s.done)
	})
}

// Next returns the next queued event, or the End error once none are left.
func (s *Stream) Next() (wire.Event, error) {
	select {
	case ev := <-s.events:
		return ev, nil
	default:
	}
	select {
	case ev := <-s.events:
		return ev, nil
	case <-s.done:
		select {
		case ev := <-s.events:
			return ev, nil
		default:
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return wire.Event{}, s.err
	}
}

// Close ends the stream with io.EOF.
func (s *Stream) Close() error {
	s.End(nil)
	return nil
}
