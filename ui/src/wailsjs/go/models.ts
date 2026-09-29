export namespace wire {
	
	export class AcceptRunRequest {
	    indexes: number[];
	
	    static createFrom(source: any = {}) {
	        return new AcceptRunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.indexes = source["indexes"];
	    }
	}
	export class BreakdownRequest {
	    instructions: string;
	
	    static createFrom(source: any = {}) {
	        return new BreakdownRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instructions = source["instructions"];
	    }
	}
	export class GoalProgress {
	    done_quantity: number;
	    remaining_quantity: number;
	    required_per_day: number;
	    actual_per_day: number;
	    pace: string;
	    projected_finish_day?: string;
	
	    static createFrom(source: any = {}) {
	        return new GoalProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.done_quantity = source["done_quantity"];
	        this.remaining_quantity = source["remaining_quantity"];
	        this.required_per_day = source["required_per_day"];
	        this.actual_per_day = source["actual_per_day"];
	        this.pace = source["pace"];
	        this.projected_finish_day = source["projected_finish_day"];
	    }
	}
	export class Goal {
	    id: string;
	    title: string;
	    kind: string;
	    unit: string;
	    target_quantity?: number;
	    minutes_per_unit?: number;
	    project_id?: string;
	    start_day: string;
	    due_day: string;
	    status: string;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	    progress: GoalProgress;
	
	    static createFrom(source: any = {}) {
	        return new Goal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.unit = source["unit"];
	        this.target_quantity = source["target_quantity"];
	        this.minutes_per_unit = source["minutes_per_unit"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	        this.progress = this.convertValues(source["progress"], GoalProgress);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Reminder {
	    task_id: string;
	    title: string;
	    due_day: string;
	    days_left: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Reminder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.title = source["title"];
	        this.due_day = source["due_day"];
	        this.days_left = source["days_left"];
	        this.message = source["message"];
	    }
	}
	export class Task {
	    id: string;
	    project_id?: string;
	    title: string;
	    notes: string;
	    status: string;
	    priority: number;
	    due_day?: string;
	    estimate_minutes?: number;
	    done_at?: number;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	    tracked_ms: number;
	    goal_id?: string;
	    quantity?: number;
	    quantity_done?: number;
	    rrule?: string;
	    template_id?: string;
	    occurrence_day?: string;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.project_id = source["project_id"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.status = source["status"];
	        this.priority = source["priority"];
	        this.due_day = source["due_day"];
	        this.estimate_minutes = source["estimate_minutes"];
	        this.done_at = source["done_at"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	        this.tracked_ms = source["tracked_ms"];
	        this.goal_id = source["goal_id"];
	        this.quantity = source["quantity"];
	        this.quantity_done = source["quantity_done"];
	        this.rrule = source["rrule"];
	        this.template_id = source["template_id"];
	        this.occurrence_day = source["occurrence_day"];
	    }
	}
	export class PlanItem {
	    id: string;
	    day: string;
	    task_id: string;
	    planned_minutes: number;
	    start_at?: number;
	    position: number;
	    status: string;
	    pinned: boolean;
	    rolled_from_id?: string;
	    rollover_count: number;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	    task: Task;
	
	    static createFrom(source: any = {}) {
	        return new PlanItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.day = source["day"];
	        this.task_id = source["task_id"];
	        this.planned_minutes = source["planned_minutes"];
	        this.start_at = source["start_at"];
	        this.position = source["position"];
	        this.status = source["status"];
	        this.pinned = source["pinned"];
	        this.rolled_from_id = source["rolled_from_id"];
	        this.rollover_count = source["rollover_count"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	        this.task = this.convertValues(source["task"], Task);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Briefing {
	    day: string;
	    pending: PlanItem[];
	    today: PlanItem[];
	    reminders: Reminder[];
	    goals: Goal[];
	
	    static createFrom(source: any = {}) {
	        return new Briefing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.pending = this.convertValues(source["pending"], PlanItem);
	        this.today = this.convertValues(source["today"], PlanItem);
	        this.reminders = this.convertValues(source["reminders"], Reminder);
	        this.goals = this.convertValues(source["goals"], Goal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CalendarAuth {
	    auth_url: string;
	
	    static createFrom(source: any = {}) {
	        return new CalendarAuth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auth_url = source["auth_url"];
	    }
	}
	export class CalendarConfig {
	    enabled: boolean;
	    name: string;
	    busy_calendars: string[];
	
	    static createFrom(source: any = {}) {
	        return new CalendarConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.name = source["name"];
	        this.busy_calendars = source["busy_calendars"];
	    }
	}
	export class CalendarStatus {
	    enabled: boolean;
	    connected: boolean;
	    calendar_id?: string;
	    last_sync_at?: number;
	    last_error?: string;
	
	    static createFrom(source: any = {}) {
	        return new CalendarStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.connected = source["connected"];
	        this.calendar_id = source["calendar_id"];
	        this.last_sync_at = source["last_sync_at"];
	        this.last_error = source["last_error"];
	    }
	}
	export class ClockInRequest {
	    project_id?: string;
	    task_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClockInRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	    }
	}
	export class Commitment {
	    id: string;
	    title: string;
	    project_id?: string;
	    rrule: string;
	    start_minute?: number;
	    duration_minutes: number;
	    counts_toward_target: boolean;
	    active_from: string;
	    active_until?: string;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	
	    static createFrom(source: any = {}) {
	        return new Commitment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.project_id = source["project_id"];
	        this.rrule = source["rrule"];
	        this.start_minute = source["start_minute"];
	        this.duration_minutes = source["duration_minutes"];
	        this.counts_toward_target = source["counts_toward_target"];
	        this.active_from = source["active_from"];
	        this.active_until = source["active_until"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	    }
	}
	export class CommitmentList {
	    commitments: Commitment[];
	
	    static createFrom(source: any = {}) {
	        return new CommitmentList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commitments = this.convertValues(source["commitments"], Commitment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompleteTaskRequest {
	    quantity_done?: number;
	
	    static createFrom(source: any = {}) {
	        return new CompleteTaskRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quantity_done = source["quantity_done"];
	    }
	}
	export class LogConfig {
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new LogConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	    }
	}
	export class SyncConfig {
	    hub_url: string;
	    interval: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hub_url = source["hub_url"];
	        this.interval = source["interval"];
	    }
	}
	export class LLMConfig {
	    provider: string;
	    model: string;
	    endpoint: string;
	    timeout: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.endpoint = source["endpoint"];
	        this.timeout = source["timeout"];
	    }
	}
	export class PlannerConfig {
	    day_start: string;
	    day_end: string;
	    buffer: string;
	
	    static createFrom(source: any = {}) {
	        return new PlannerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day_start = source["day_start"];
	        this.day_end = source["day_end"];
	        this.buffer = source["buffer"];
	    }
	}
	export class NtfyConfig {
	    server: string;
	    fallback_server: string;
	    topic: string;
	
	    static createFrom(source: any = {}) {
	        return new NtfyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.fallback_server = source["fallback_server"];
	        this.topic = source["topic"];
	    }
	}
	export class NudgeConfig {
	    desktop: boolean;
	    phone: boolean;
	    break_reminder: string;
	    repeat: string;
	    snooze: string;
	
	    static createFrom(source: any = {}) {
	        return new NudgeConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.desktop = source["desktop"];
	        this.phone = source["phone"];
	        this.break_reminder = source["break_reminder"];
	        this.repeat = source["repeat"];
	        this.snooze = source["snooze"];
	    }
	}
	export class TrackingConfig {
	    daily_target: string;
	    soft_idle: string;
	    hard_idle: string;
	    day_rollover: string;
	
	    static createFrom(source: any = {}) {
	        return new TrackingConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.daily_target = source["daily_target"];
	        this.soft_idle = source["soft_idle"];
	        this.hard_idle = source["hard_idle"];
	        this.day_rollover = source["day_rollover"];
	    }
	}
	export class Config {
	    tracking: TrackingConfig;
	    nudge: NudgeConfig;
	    ntfy: NtfyConfig;
	    planner: PlannerConfig;
	    calendar: CalendarConfig;
	    llm: LLMConfig;
	    sync: SyncConfig;
	    log: LogConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tracking = this.convertValues(source["tracking"], TrackingConfig);
	        this.nudge = this.convertValues(source["nudge"], NudgeConfig);
	        this.ntfy = this.convertValues(source["ntfy"], NtfyConfig);
	        this.planner = this.convertValues(source["planner"], PlannerConfig);
	        this.calendar = this.convertValues(source["calendar"], CalendarConfig);
	        this.llm = this.convertValues(source["llm"], LLMConfig);
	        this.sync = this.convertValues(source["sync"], SyncConfig);
	        this.log = this.convertValues(source["log"], LogConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateCommitmentRequest {
	    title: string;
	    project_id?: string;
	    rrule: string;
	    start_minute?: number;
	    duration_minutes: number;
	    counts_toward_target?: boolean;
	    active_from: string;
	    active_until?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateCommitmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.project_id = source["project_id"];
	        this.rrule = source["rrule"];
	        this.start_minute = source["start_minute"];
	        this.duration_minutes = source["duration_minutes"];
	        this.counts_toward_target = source["counts_toward_target"];
	        this.active_from = source["active_from"];
	        this.active_until = source["active_until"];
	    }
	}
	export class CreateGoalRequest {
	    title: string;
	    kind: string;
	    unit?: string;
	    target_quantity?: number;
	    minutes_per_unit?: number;
	    project_id?: string;
	    start_day: string;
	    due_day: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateGoalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.unit = source["unit"];
	        this.target_quantity = source["target_quantity"];
	        this.minutes_per_unit = source["minutes_per_unit"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	    }
	}
	export class CreateProjectRequest {
	    name: string;
	    color?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class CreateSegmentRequest {
	    day: string;
	    kind: string;
	    project_id?: string;
	    task_id?: string;
	    started_at: number;
	    ended_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateSegmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.kind = source["kind"];
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	    }
	}
	export class CreateTaskRequest {
	    project_id?: string;
	    title: string;
	    notes?: string;
	    priority?: number;
	    due_day?: string;
	    estimate_minutes?: number;
	    goal_id?: string;
	    quantity?: number;
	    rrule?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateTaskRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.priority = source["priority"];
	        this.due_day = source["due_day"];
	        this.estimate_minutes = source["estimate_minutes"];
	        this.goal_id = source["goal_id"];
	        this.quantity = source["quantity"];
	        this.rrule = source["rrule"];
	    }
	}
	export class Segment {
	    id: string;
	    work_day_id: string;
	    kind: string;
	    source: string;
	    project_id?: string;
	    task_id?: string;
	    started_at: number;
	    ended_at?: number;
	    truncated: boolean;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	
	    static createFrom(source: any = {}) {
	        return new Segment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.work_day_id = source["work_day_id"];
	        this.kind = source["kind"];
	        this.source = source["source"];
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	        this.truncated = source["truncated"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	    }
	}
	export class ProjectTotal {
	    project_id?: string;
	    name: string;
	    color: string;
	    worked_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.worked_ms = source["worked_ms"];
	    }
	}
	export class DaySummary {
	    day: string;
	    target_seconds: number;
	    worked_ms: number;
	    break_ms: number;
	    target_met: boolean;
	    by_project: ProjectTotal[];
	
	    static createFrom(source: any = {}) {
	        return new DaySummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.target_seconds = source["target_seconds"];
	        this.worked_ms = source["worked_ms"];
	        this.break_ms = source["break_ms"];
	        this.target_met = source["target_met"];
	        this.by_project = this.convertValues(source["by_project"], ProjectTotal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkDay {
	    id: string;
	    day: string;
	    tz: string;
	    clocked_in_at: number;
	    clocked_out_at?: number;
	    target_seconds: number;
	    note: string;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkDay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.day = source["day"];
	        this.tz = source["tz"];
	        this.clocked_in_at = source["clocked_in_at"];
	        this.clocked_out_at = source["clocked_out_at"];
	        this.target_seconds = source["target_seconds"];
	        this.note = source["note"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	    }
	}
	export class DayDetail {
	    work_day: WorkDay;
	    summary: DaySummary;
	    segments: Segment[];
	
	    static createFrom(source: any = {}) {
	        return new DayDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.work_day = this.convertValues(source["work_day"], WorkDay);
	        this.summary = this.convertValues(source["summary"], DaySummary);
	        this.segments = this.convertValues(source["segments"], Segment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DayList {
	    days: DaySummary[];
	
	    static createFrom(source: any = {}) {
	        return new DayList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = this.convertValues(source["days"], DaySummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class GeneratePlanRequest {
	    day: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneratePlanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	    }
	}
	
	export class GoalList {
	    goals: Goal[];
	
	    static createFrom(source: any = {}) {
	        return new GoalList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goals = this.convertValues(source["goals"], Goal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Health {
	    ok: boolean;
	    version: string;
	    schema_version: number;
	    pid: number;
	
	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.version = source["version"];
	        this.schema_version = source["schema_version"];
	        this.pid = source["pid"];
	    }
	}
	export class HeatmapDay {
	    day: string;
	    worked_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new HeatmapDay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.worked_ms = source["worked_ms"];
	    }
	}
	export class Heatmap {
	    year: number;
	    days: HeatmapDay[];
	
	    static createFrom(source: any = {}) {
	        return new Heatmap(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.year = source["year"];
	        this.days = this.convertValues(source["days"], HeatmapDay);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class LlmRun {
	    id: string;
	    kind: string;
	    subject_id: string;
	    status: string;
	    output: any;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new LlmRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.subject_id = source["subject_id"];
	        this.status = source["status"];
	        this.output = source["output"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	
	export class NotifyTestResult {
	    desktop: string;
	    phone: string;
	
	    static createFrom(source: any = {}) {
	        return new NotifyTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.desktop = source["desktop"];
	        this.phone = source["phone"];
	    }
	}
	
	
	export class Optional_int64_ {
	    Set: boolean;
	    Null: boolean;
	    Value: number;
	
	    static createFrom(source: any = {}) {
	        return new Optional_int64_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Set = source["Set"];
	        this.Null = source["Null"];
	        this.Value = source["Value"];
	    }
	}
	export class Optional_int_ {
	    Set: boolean;
	    Null: boolean;
	    Value: number;
	
	    static createFrom(source: any = {}) {
	        return new Optional_int_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Set = source["Set"];
	        this.Null = source["Null"];
	        this.Value = source["Value"];
	    }
	}
	export class Optional_string_ {
	    Set: boolean;
	    Null: boolean;
	    Value: string;
	
	    static createFrom(source: any = {}) {
	        return new Optional_string_(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Set = source["Set"];
	        this.Null = source["Null"];
	        this.Value = source["Value"];
	    }
	}
	export class PatchCommitmentRequest {
	    title?: string;
	    project_id: string | null;
	    rrule?: string;
	    start_minute: number | null;
	    duration_minutes?: number;
	    counts_toward_target?: boolean;
	    active_from?: string;
	    active_until: string | null;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchCommitmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.project_id = source["project_id"];
	        this.rrule = source["rrule"];
	        this.start_minute = source["start_minute"];
	        this.duration_minutes = source["duration_minutes"];
	        this.counts_toward_target = source["counts_toward_target"];
	        this.active_from = source["active_from"];
	        this.active_until = source["active_until"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchDayRequest {
	    target_seconds?: number;
	    note?: string;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchDayRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target_seconds = source["target_seconds"];
	        this.note = source["note"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchGoalRequest {
	    title?: string;
	    kind?: string;
	    unit?: string;
	    target_quantity: number | null;
	    minutes_per_unit: number | null;
	    project_id: string | null;
	    start_day?: string;
	    due_day?: string;
	    status?: string;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchGoalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.unit = source["unit"];
	        this.target_quantity = source["target_quantity"];
	        this.minutes_per_unit = source["minutes_per_unit"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	        this.status = source["status"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchPlanItemRequest {
	    start_at: number | null;
	    position?: number;
	    pinned?: boolean;
	    planned_minutes?: number;
	    status?: string;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchPlanItemRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start_at = source["start_at"];
	        this.position = source["position"];
	        this.pinned = source["pinned"];
	        this.planned_minutes = source["planned_minutes"];
	        this.status = source["status"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchProjectRequest {
	    name?: string;
	    color?: string;
	    archived?: boolean;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	        this.archived = source["archived"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchSegmentRequest {
	    kind?: string;
	    project_id: string | null;
	    task_id: string | null;
	    started_at?: number;
	    ended_at?: number;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchSegmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	        this.rev = source["rev"];
	    }
	}
	export class PatchTaskRequest {
	    project_id: string | null;
	    title?: string;
	    notes?: string;
	    priority?: number;
	    due_day: string | null;
	    estimate_minutes: number | null;
	    goal_id: string | null;
	    quantity: number | null;
	    rrule: string | null;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchTaskRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.title = source["title"];
	        this.notes = source["notes"];
	        this.priority = source["priority"];
	        this.due_day = source["due_day"];
	        this.estimate_minutes = source["estimate_minutes"];
	        this.goal_id = source["goal_id"];
	        this.quantity = source["quantity"];
	        this.rrule = source["rrule"];
	        this.rev = source["rev"];
	    }
	}
	export class Plan {
	    day: string;
	    capacity_minutes: number;
	    planned_minutes: number;
	    items: PlanItem[];
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.capacity_minutes = source["capacity_minutes"];
	        this.planned_minutes = source["planned_minutes"];
	        this.items = this.convertValues(source["items"], PlanItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class Project {
	    id: string;
	    name: string;
	    color: string;
	    archived_at?: number;
	    created_at: number;
	    updated_at: number;
	    rev: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.archived_at = source["archived_at"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	    }
	}
	export class ProjectList {
	    projects: Project[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projects = this.convertValues(source["projects"], Project);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class RetroRequest {
	    week_start: string;
	
	    static createFrom(source: any = {}) {
	        return new RetroRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.week_start = source["week_start"];
	    }
	}
	
	export class SegmentList {
	    segments: Segment[];
	
	    static createFrom(source: any = {}) {
	        return new SegmentList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.segments = this.convertValues(source["segments"], Segment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SplitSegmentRequest {
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new SplitSegmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	    }
	}
	export class StatsSummary {
	    from: string;
	    to: string;
	    worked_ms: number;
	    break_ms: number;
	    days_tracked: number;
	    days_target_met: number;
	    avg_worked_ms: number;
	    by_project: ProjectTotal[];
	    current_streak: number;
	    longest_streak: number;
	
	    static createFrom(source: any = {}) {
	        return new StatsSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	        this.worked_ms = source["worked_ms"];
	        this.break_ms = source["break_ms"];
	        this.days_tracked = source["days_tracked"];
	        this.days_target_met = source["days_target_met"];
	        this.avg_worked_ms = source["avg_worked_ms"];
	        this.by_project = this.convertValues(source["by_project"], ProjectTotal);
	        this.current_streak = source["current_streak"];
	        this.longest_streak = source["longest_streak"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Status {
	    state: string;
	    state_since_at: number;
	    work_day?: WorkDay;
	    open_segment?: Segment;
	    project_id?: string;
	    task_id?: string;
	    idle_since_at?: number;
	    snoozed_until_at?: number;
	    today?: DaySummary;
	    server_now_at: number;
	    activity_backend: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.state_since_at = source["state_since_at"];
	        this.work_day = this.convertValues(source["work_day"], WorkDay);
	        this.open_segment = this.convertValues(source["open_segment"], Segment);
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	        this.idle_since_at = source["idle_since_at"];
	        this.snoozed_until_at = source["snoozed_until_at"];
	        this.today = this.convertValues(source["today"], DaySummary);
	        this.server_now_at = source["server_now_at"];
	        this.activity_backend = source["activity_backend"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SwitchRequest {
	    project_id?: string;
	    task_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new SwitchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.task_id = source["task_id"];
	    }
	}
	
	
	export class TaskList {
	    tasks: Task[];
	
	    static createFrom(source: any = {}) {
	        return new TaskList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tasks = this.convertValues(source["tasks"], Task);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TaskQuery {
	    project_id: string;
	    goal_id: string;
	    status: string;
	    due_before: string;
	    templates: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TaskQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.goal_id = source["goal_id"];
	        this.status = source["status"];
	        this.due_before = source["due_before"];
	        this.templates = source["templates"];
	    }
	}
	

}

