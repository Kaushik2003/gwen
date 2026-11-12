// The shapes of an LlmRun's output (internal/wire/llm.go), which arrives as plain JSON:
//   breakdown: {tasks: [{title, notes, estimate_minutes, due_day, priority, quantity}]}
//   retro:     {markdown, generated_by: "llm" | "rules"}
//   day plan:  {messages: [{role, text}], items: [{task_id, title, project_id, start_at, planned_minutes}], hours: {start_minute, work_minutes} | null}
//   assistant: {thread_id, messages: [{id, role, text, at, mood, attitude, actions: [{type, ok, summary, error, task_id, project_id, goal_id}]}]}
//   a failed run: {error}
// Moods (the faces in assets/gwen): smiling happy love winking cool smug thinking shocked annoyed grumpy angry crying.

/** Each attitude in words, with the face that goes with it. */
export const attitudes = {
  playful: { label: "Playful", means: "Teasing and flirty: things are going fine.", face: "winking" },
  focused: { label: "Focused", means: "Businesslike: less banter, more getting it done.", face: "cool" },
  soft: { label: "Soft", means: "Gentle and supportive: she thinks you're having a hard time.", face: "love" },
  competitive: { label: "Competitive", means: "Daring you to aim higher.", face: "smug" },
  protective: { label: "Protective", means: "Slowing you down: rest, boundaries, no burnout.", face: "thinking" },
  excited: { label: "Excited", means: "Thrilled about a win or a new idea.", face: "happy" },
  quiet: { label: "Quiet", means: "Calm company: few words, no advice unless you ask.", face: "smiling" },
  angry: { label: "Angry", means: "Fed up with excuses: blunt until you follow through.", face: "angry" },
};

/** The attitude of a name, or null for none this build knows. */
export function attitudeOf(id) {
  return id && id in attitudes ? attitudes[id] : null;
}
