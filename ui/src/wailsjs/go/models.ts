export namespace main {
	
	export class ClaudeCodeStatus {
	    command: string;
	    found: boolean;
	    is_claude: boolean;
	    logged_in: boolean;
	    plan: string;
	    auth_method: string;
	    detail: string;
	    suggested: string;
	
	    static createFrom(source: any = {}) {
	        return new ClaudeCodeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.found = source["found"];
	        this.is_claude = source["is_claude"];
	        this.logged_in = source["logged_in"];
	        this.plan = source["plan"];
	        this.auth_method = source["auth_method"];
	        this.detail = source["detail"];
	        this.suggested = source["suggested"];
	    }
	}
	export class SpeechVoice {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechVoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class SpeechProvider {
	    id: string;
	    name: string;
	    about: string;
	    online: boolean;
	    voices: SpeechVoice[];
	    download_mb: number;
	    installed: boolean;
	    installing: boolean;
	    key: string;
	    key_url: string;
	    has_key: boolean;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SpeechProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.about = source["about"];
	        this.online = source["online"];
	        this.voices = this.convertValues(source["voices"], SpeechVoice);
	        this.download_mb = source["download_mb"];
	        this.installed = source["installed"];
	        this.installing = source["installing"];
	        this.key = source["key"];
	        this.key_url = source["key_url"];
	        this.has_key = source["has_key"];
	        this.ready = source["ready"];
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
	export class SpeechStatus {
	    player: boolean;
	    speaking: number;
	    providers: SpeechProvider[];
	    provider: string;
	    voices: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new SpeechStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.player = source["player"];
	        this.speaking = source["speaking"];
	        this.providers = this.convertValues(source["providers"], SpeechProvider);
	        this.provider = source["provider"];
	        this.voices = source["voices"];
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
	
	export class TermInfo {
	    address: string;
	    name: string;
	    cwd: string;
	    status: string;
	    app: string;
	    session_id: string;
	    project: string;
	    task: string;
	    claude: boolean;
	    parent: string;
	    cols: number;
	    rows: number;
	    live: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TermInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.status = source["status"];
	        this.app = source["app"];
	        this.session_id = source["session_id"];
	        this.project = source["project"];
	        this.task = source["task"];
	        this.claude = source["claude"];
	        this.parent = source["parent"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	        this.live = source["live"];
	    }
	}
	export class TermList {
	    core: string;
	    message: string;
	    terminals: TermInfo[];
	
	    static createFrom(source: any = {}) {
	        return new TermList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.core = source["core"];
	        this.message = source["message"];
	        this.terminals = this.convertValues(source["terminals"], TermInfo);
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
	export class TermStartRequest {
	    cwd: string;
	    name: string;
	    project: string;
	    argv: string[];
	
	    static createFrom(source: any = {}) {
	        return new TermStartRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cwd = source["cwd"];
	        this.name = source["name"];
	        this.project = source["project"];
	        this.argv = source["argv"];
	    }
	}
	export class VoiceStatus {
	    installed: boolean;
	    installing: boolean;
	    recorder: boolean;
	    listening: boolean;
	    download_mb: number;
	
	    static createFrom(source: any = {}) {
	        return new VoiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.installing = source["installing"];
	        this.recorder = source["recorder"];
	        this.listening = source["listening"];
	        this.download_mb = source["download_mb"];
	    }
	}

}

export namespace repo {
	
	export class BlameLine {
	    commit: string;
	    short: string;
	    author: string;
	    summary: string;
	    when: number;
	    uncommitted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BlameLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commit = source["commit"];
	        this.short = source["short"];
	        this.author = source["author"];
	        this.summary = source["summary"];
	        this.when = source["when"];
	        this.uncommitted = source["uncommitted"];
	    }
	}
	export class Ref {
	    name: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new Ref(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	    }
	}
	export class Commit {
	    hash: string;
	    short: string;
	    parents: string[];
	    subject: string;
	    author: string;
	    author_email: string;
	    when: number;
	    refs: Ref[];
	    added: number;
	    removed: number;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.short = source["short"];
	        this.parents = source["parents"];
	        this.subject = source["subject"];
	        this.author = source["author"];
	        this.author_email = source["author_email"];
	        this.when = source["when"];
	        this.refs = this.convertValues(source["refs"], Ref);
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.files = source["files"];
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
	export class FileChange {
	    path: string;
	    old_path: string;
	    status: string;
	    added: number;
	    removed: number;
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.old_path = source["old_path"];
	        this.status = source["status"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.binary = source["binary"];
	    }
	}
	export class CommitDetail {
	    hash: string;
	    short: string;
	    parents: string[];
	    subject: string;
	    author: string;
	    author_email: string;
	    when: number;
	    refs: Ref[];
	    added: number;
	    removed: number;
	    files: number;
	    body: string;
	    changes: FileChange[];
	
	    static createFrom(source: any = {}) {
	        return new CommitDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.short = source["short"];
	        this.parents = source["parents"];
	        this.subject = source["subject"];
	        this.author = source["author"];
	        this.author_email = source["author_email"];
	        this.when = source["when"];
	        this.refs = this.convertValues(source["refs"], Ref);
	        this.added = source["added"];
	        this.removed = source["removed"];
	        this.files = source["files"];
	        this.body = source["body"];
	        this.changes = this.convertValues(source["changes"], FileChange);
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
	export class DiffText {
	    path: string;
	    text: string;
	    binary: boolean;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffText(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.text = source["text"];
	        this.binary = source["binary"];
	        this.truncated = source["truncated"];
	    }
	}
	export class File {
	    path: string;
	    text: string;
	    binary: boolean;
	    size: number;
	    truncated: boolean;
	    lines: number;
	    lang: string;
	
	    static createFrom(source: any = {}) {
	        return new File(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.text = source["text"];
	        this.binary = source["binary"];
	        this.size = source["size"];
	        this.truncated = source["truncated"];
	        this.lines = source["lines"];
	        this.lang = source["lang"];
	    }
	}
	
	export class Uncommitted {
	    files: number;
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new Uncommitted(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	export class Log {
	    commits: Commit[];
	    more: boolean;
	    uncommitted?: Uncommitted;
	    branch: string;
	    upstream: string;
	    ahead: number;
	    behind: number;
	
	    static createFrom(source: any = {}) {
	        return new Log(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commits = this.convertValues(source["commits"], Commit);
	        this.more = source["more"];
	        this.uncommitted = this.convertValues(source["uncommitted"], Uncommitted);
	        this.branch = source["branch"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
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
	export class LogQuery {
	    limit: number;
	    skip: number;
	    search: string;
	    author: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new LogQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	        this.skip = source["skip"];
	        this.search = source["search"];
	        this.author = source["author"];
	        this.path = source["path"];
	    }
	}
	export class Mark {
	    line: number;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new Mark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.kind = source["kind"];
	    }
	}
	export class Marks {
	    source: string;
	    commit: string;
	    short: string;
	    subject: string;
	    when: number;
	    lines: Mark[];
	
	    static createFrom(source: any = {}) {
	        return new Marks(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.commit = source["commit"];
	        this.short = source["short"];
	        this.subject = source["subject"];
	        this.when = source["when"];
	        this.lines = this.convertValues(source["lines"], Mark);
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
	
	export class TreeFile {
	    path: string;
	    status: string;
	    staged: boolean;
	    old_path: string;
	
	    static createFrom(source: any = {}) {
	        return new TreeFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.staged = source["staged"];
	        this.old_path = source["old_path"];
	    }
	}
	export class Tree {
	    root: string;
	    git: boolean;
	    branch: string;
	    upstream: string;
	    ahead: number;
	    behind: number;
	    files: TreeFile[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Tree(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.git = source["git"];
	        this.branch = source["branch"];
	        this.upstream = source["upstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.files = this.convertValues(source["files"], TreeFile);
	        this.truncated = source["truncated"];
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
	

}

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
	export class ActionResult {
	    type: string;
	    ok: boolean;
	    summary: string;
	    error?: string;
	    task_id?: string;
	    project_id?: string;
	    goal_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.ok = source["ok"];
	        this.summary = source["summary"];
	        this.error = source["error"];
	        this.task_id = source["task_id"];
	        this.project_id = source["project_id"];
	        this.goal_id = source["goal_id"];
	    }
	}
	export class AddResourceRequest {
	    path: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new AddResourceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.title = source["title"];
	    }
	}
	export class AgentChat {
	    id: string;
	    title: string;
	    project_id?: string;
	    state: string;
	    error: string;
	    queued: number;
	    messages: number;
	    last_at: number;
	    pending: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentChat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.project_id = source["project_id"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.queued = source["queued"];
	        this.messages = source["messages"];
	        this.last_at = source["last_at"];
	        this.pending = source["pending"];
	    }
	}
	export class Approval {
	    id: string;
	    run_id: string;
	    run_title: string;
	    thread_id: string;
	    step_id: string;
	    capability: string;
	    risk: string;
	    pattern: string;
	    summary: string;
	    payload: any;
	    grant_scopes: string[];
	    status: string;
	    created_at: number;
	    decided_at?: number;
	    expires_at: number;
	
	    static createFrom(source: any = {}) {
	        return new Approval(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.run_id = source["run_id"];
	        this.run_title = source["run_title"];
	        this.thread_id = source["thread_id"];
	        this.step_id = source["step_id"];
	        this.capability = source["capability"];
	        this.risk = source["risk"];
	        this.pattern = source["pattern"];
	        this.summary = source["summary"];
	        this.payload = source["payload"];
	        this.grant_scopes = source["grant_scopes"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.decided_at = source["decided_at"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class AgentStep {
	    id: string;
	    tool_use_id: string;
	    turn: number;
	    position: number;
	    tool: string;
	    capability: string;
	    risk: string;
	    title: string;
	    input: any;
	    output: string;
	    status: string;
	    approval_id?: string;
	    can_undo: boolean;
	    started_at: number;
	    ended_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tool_use_id = source["tool_use_id"];
	        this.turn = source["turn"];
	        this.position = source["position"];
	        this.tool = source["tool"];
	        this.capability = source["capability"];
	        this.risk = source["risk"];
	        this.title = source["title"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.status = source["status"];
	        this.approval_id = source["approval_id"];
	        this.can_undo = source["can_undo"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	    }
	}
	export class MemoryRef {
	    ref: string;
	    kind: string;
	    text: string;
	    source: string;
	    thread_id: string;
	    gone: boolean;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.source = source["source"];
	        this.thread_id = source["thread_id"];
	        this.gone = source["gone"];
	        this.status = source["status"];
	    }
	}
	export class AssistantMessage {
	    id?: string;
	    role: string;
	    text: string;
	    at: number;
	    mood?: string;
	    attitude?: string;
	    actions?: ActionResult[];
	    cites?: MemoryRef[];
	    forget?: MemoryRef[];
	    learned?: MemoryRef[];
	
	    static createFrom(source: any = {}) {
	        return new AssistantMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.at = source["at"];
	        this.mood = source["mood"];
	        this.attitude = source["attitude"];
	        this.actions = this.convertValues(source["actions"], ActionResult);
	        this.cites = this.convertValues(source["cites"], MemoryRef);
	        this.forget = this.convertValues(source["forget"], MemoryRef);
	        this.learned = this.convertValues(source["learned"], MemoryRef);
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
	export class AgentChatDetail {
	    chat: AgentChat;
	    messages: AssistantMessage[];
	    steps: AgentStep[];
	    approvals: Approval[];
	    queue: string[];
	    partial: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentChatDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chat = this.convertValues(source["chat"], AgentChat);
	        this.messages = this.convertValues(source["messages"], AssistantMessage);
	        this.steps = this.convertValues(source["steps"], AgentStep);
	        this.approvals = this.convertValues(source["approvals"], Approval);
	        this.queue = source["queue"];
	        this.partial = source["partial"];
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
	export class AgentChatList {
	    chats: AgentChat[];
	
	    static createFrom(source: any = {}) {
	        return new AgentChatList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chats = this.convertValues(source["chats"], AgentChat);
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
	export class AgentConfig {
	    enabled: boolean;
	    autonomy: string;
	    idle_stop: string;
	    max_live: number;
	    turn_timeout: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.autonomy = source["autonomy"];
	        this.idle_stop = source["idle_stop"];
	        this.max_live = source["max_live"];
	        this.turn_timeout = source["turn_timeout"];
	    }
	}
	export class AgentFocus {
	    kind: string;
	    screen?: string;
	    project_id?: string;
	    path?: string;
	    terminal?: string;
	    selection?: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentFocus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.screen = source["screen"];
	        this.project_id = source["project_id"];
	        this.path = source["path"];
	        this.terminal = source["terminal"];
	        this.selection = source["selection"];
	    }
	}
	export class AgentMessageRequest {
	    text: string;
	    focus?: AgentFocus;
	
	    static createFrom(source: any = {}) {
	        return new AgentMessageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.focus = this.convertValues(source["focus"], AgentFocus);
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
	export class AgentStatus {
	    enabled: boolean;
	    available: boolean;
	    reason: string;
	    command: string;
	    autonomy: string;
	    live: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.command = source["command"];
	        this.autonomy = source["autonomy"];
	        this.live = source["live"];
	    }
	}
	
	
	export class ApprovalList {
	    approvals: Approval[];
	
	    static createFrom(source: any = {}) {
	        return new ApprovalList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.approvals = this.convertValues(source["approvals"], Approval);
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
	export class AssistantChatRequest {
	    message: string;
	    thread_id?: string;
	    run_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new AssistantChatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.thread_id = source["thread_id"];
	        this.run_id = source["run_id"];
	    }
	}
	
	export class AssistantSelf {
	    attitude: string;
	    note: string;
	    since?: number;
	
	    static createFrom(source: any = {}) {
	        return new AssistantSelf(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.attitude = source["attitude"];
	        this.note = source["note"];
	        this.since = source["since"];
	    }
	}
	export class Attachment {
	    id: string;
	    task_id: string;
	    kind: string;
	    title: string;
	    url: string;
	    path: string;
	    app: string;
	    ref: string;
	    body: string;
	    fetched_at?: number;
	    created_at: number;
	    rev: number;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task_id = source["task_id"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.path = source["path"];
	        this.app = source["app"];
	        this.ref = source["ref"];
	        this.body = source["body"];
	        this.fetched_at = source["fetched_at"];
	        this.created_at = source["created_at"];
	        this.rev = source["rev"];
	    }
	}
	export class AttachmentList {
	    attachments: Attachment[];
	
	    static createFrom(source: any = {}) {
	        return new AttachmentList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.attachments = this.convertValues(source["attachments"], Attachment);
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
	export class BreakdownRequest {
	    instructions: string;
	    sessions?: number;
	
	    static createFrom(source: any = {}) {
	        return new BreakdownRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instructions = source["instructions"];
	        this.sessions = source["sessions"];
	    }
	}
	export class GoalProgress {
	    done_quantity: number;
	    remaining_quantity: number;
	    required_per_day: number;
	    actual_per_day: number;
	    pace: string;
	    projected_finish_day?: string;
	    per_session?: number;
	    reachable_quantity: number;
	    needed_daily_minutes?: number;
	    needed_due_day?: string;
	    lined_up: number;
	
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
	        this.per_session = source["per_session"];
	        this.reachable_quantity = source["reachable_quantity"];
	        this.needed_daily_minutes = source["needed_daily_minutes"];
	        this.needed_due_day = source["needed_due_day"];
	        this.lined_up = source["lined_up"];
	    }
	}
	export class Goal {
	    id: string;
	    title: string;
	    kind: string;
	    unit: string;
	    target_quantity?: number;
	    minutes_per_unit?: number;
	    daily_minutes?: number;
	    project_id?: string;
	    start_day: string;
	    due_day: string;
	    status: string;
	    specific: string;
	    measurable: string;
	    assignable: string;
	    realistic: string;
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
	        this.daily_minutes = source["daily_minutes"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	        this.status = source["status"];
	        this.specific = source["specific"];
	        this.measurable = source["measurable"];
	        this.assignable = source["assignable"];
	        this.realistic = source["realistic"];
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
	    parent_id?: string;
	    start_day?: string;
	    start_minute?: number;
	    stage: string;
	    effort?: number;
	    delegated_to: string;
	    attachments: number;
	
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
	        this.parent_id = source["parent_id"];
	        this.start_day = source["start_day"];
	        this.start_minute = source["start_minute"];
	        this.stage = source["stage"];
	        this.effort = source["effort"];
	        this.delegated_to = source["delegated_to"];
	        this.attachments = source["attachments"];
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
	    steps: Task[];
	
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
	        this.steps = this.convertValues(source["steps"], Task);
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
	export class CalendarEvent {
	    calendar_id: string;
	    title: string;
	    start_at: number;
	    end_at: number;
	
	    static createFrom(source: any = {}) {
	        return new CalendarEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.calendar_id = source["calendar_id"];
	        this.title = source["title"];
	        this.start_at = source["start_at"];
	        this.end_at = source["end_at"];
	    }
	}
	export class CalendarInfo {
	    id: string;
	    name: string;
	    color: string;
	    primary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CalendarInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.primary = source["primary"];
	    }
	}
	export class CalendarList {
	    calendars: CalendarInfo[];
	
	    static createFrom(source: any = {}) {
	        return new CalendarList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.calendars = this.convertValues(source["calendars"], CalendarInfo);
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
	export class Capability {
	    name: string;
	    title: string;
	    risk: string;
	    input: string;
	
	    static createFrom(source: any = {}) {
	        return new Capability(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	        this.risk = source["risk"];
	        this.input = source["input"];
	    }
	}
	export class ChatHit {
	    thread_id: string;
	    thread_title: string;
	    message: AssistantMessage;
	
	    static createFrom(source: any = {}) {
	        return new ChatHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.thread_id = source["thread_id"];
	        this.thread_title = source["thread_title"];
	        this.message = this.convertValues(source["message"], AssistantMessage);
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
	export class ChatSearchResult {
	    hits: ChatHit[];
	
	    static createFrom(source: any = {}) {
	        return new ChatSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hits = this.convertValues(source["hits"], ChatHit);
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
	export class ChatThread {
	    id: string;
	    title: string;
	    messages: number;
	    created_at: number;
	    last_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatThread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.messages = source["messages"];
	        this.created_at = source["created_at"];
	        this.last_at = source["last_at"];
	    }
	}
	export class ChatThreadDetail {
	    thread: ChatThread;
	    messages: AssistantMessage[];
	
	    static createFrom(source: any = {}) {
	        return new ChatThreadDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.thread = this.convertValues(source["thread"], ChatThread);
	        this.messages = this.convertValues(source["messages"], AssistantMessage);
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
	export class ChatThreadList {
	    threads: ChatThread[];
	
	    static createFrom(source: any = {}) {
	        return new ChatThreadList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.threads = this.convertValues(source["threads"], ChatThread);
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
	export class UsageWindow {
	    used_percent: number;
	    resets_at: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageWindow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used_percent = source["used_percent"];
	        this.resets_at = source["resets_at"];
	    }
	}
	export class ClaudeUsage {
	    five_hour?: UsageWindow;
	    seven_day?: UsageWindow;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new ClaudeUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.five_hour = this.convertValues(source["five_hour"], UsageWindow);
	        this.seven_day = this.convertValues(source["seven_day"], UsageWindow);
	        this.at = source["at"];
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
	export class CodeAnswer {
	    key: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.message = source["message"];
	    }
	}
	export class CodeAnswerResult {
	    ok: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeAnswerResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	    }
	}
	export class CodeControl {
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeControl(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	    }
	}
	export class CodeConversation {
	    claude_session_id: string;
	    title: string;
	    active_at: number;
	    sandbox: boolean;
	    session_id: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claude_session_id = source["claude_session_id"];
	        this.title = source["title"];
	        this.active_at = source["active_at"];
	        this.sandbox = source["sandbox"];
	        this.session_id = source["session_id"];
	    }
	}
	export class CodeConversations {
	    conversations: CodeConversation[];
	
	    static createFrom(source: any = {}) {
	        return new CodeConversations(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversations = this.convertValues(source["conversations"], CodeConversation);
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
	export class CodeMark {
	    pinned?: boolean;
	    seen?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CodeMark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pinned = source["pinned"];
	        this.seen = source["seen"];
	    }
	}
	export class CodeOption {
	    key: string;
	    label: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	    }
	}
	export class CodeStat {
	    files: number;
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new CodeStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = source["files"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	export class CodeTerminal {
	    terminal: string;
	    name: string;
	    cwd: string;
	    status: string;
	    app: string;
	    session_id: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new CodeTerminal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.terminal = source["terminal"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.status = source["status"];
	        this.app = source["app"];
	        this.session_id = source["session_id"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class CodeWaiting {
	    kind: string;
	    held: boolean;
	    tool: string;
	    summary: string;
	    question: string;
	    options: CodeOption[];
	    note: string;
	    held_until?: number;
	
	    static createFrom(source: any = {}) {
	        return new CodeWaiting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.held = source["held"];
	        this.tool = source["tool"];
	        this.summary = source["summary"];
	        this.question = source["question"];
	        this.options = this.convertValues(source["options"], CodeOption);
	        this.note = source["note"];
	        this.held_until = source["held_until"];
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
	export class CodeSession {
	    id: string;
	    terminal: string;
	    name: string;
	    cwd: string;
	    branch: string;
	    sandbox: boolean;
	    state: string;
	    need: string;
	    step: string;
	    last_message: string;
	    permission_mode: string;
	    claude_session_id: string;
	    pinned: boolean;
	    unread: boolean;
	    window: boolean;
	    notice: string;
	    waiting?: CodeWaiting;
	    helpers: CodeTerminal[];
	    changes?: CodeStat;
	    started_at: number;
	    updated_at: number;
	    ended_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new CodeSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.terminal = source["terminal"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.branch = source["branch"];
	        this.sandbox = source["sandbox"];
	        this.state = source["state"];
	        this.need = source["need"];
	        this.step = source["step"];
	        this.last_message = source["last_message"];
	        this.permission_mode = source["permission_mode"];
	        this.claude_session_id = source["claude_session_id"];
	        this.pinned = source["pinned"];
	        this.unread = source["unread"];
	        this.window = source["window"];
	        this.notice = source["notice"];
	        this.waiting = this.convertValues(source["waiting"], CodeWaiting);
	        this.helpers = this.convertValues(source["helpers"], CodeTerminal);
	        this.changes = this.convertValues(source["changes"], CodeStat);
	        this.started_at = source["started_at"];
	        this.updated_at = source["updated_at"];
	        this.ended_at = source["ended_at"];
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
	export class CodeSessionList {
	    sessions: CodeSession[];
	    core: string;
	    message: string;
	    machine: string;
	
	    static createFrom(source: any = {}) {
	        return new CodeSessionList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], CodeSession);
	        this.core = source["core"];
	        this.message = source["message"];
	        this.machine = source["machine"];
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
	
	
	
	export class CommitSession {
	    commit: string;
	    at: number;
	    claude_session_id: string;
	    title: string;
	    first_prompt: string;
	    cwd: string;
	    sandbox: boolean;
	    session_id: string;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new CommitSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commit = source["commit"];
	        this.at = source["at"];
	        this.claude_session_id = source["claude_session_id"];
	        this.title = source["title"];
	        this.first_prompt = source["first_prompt"];
	        this.cwd = source["cwd"];
	        this.sandbox = source["sandbox"];
	        this.session_id = source["session_id"];
	        this.summary = source["summary"];
	    }
	}
	export class CommitSessions {
	    commits: CommitSession[];
	
	    static createFrom(source: any = {}) {
	        return new CommitSessions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.commits = this.convertValues(source["commits"], CommitSession);
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
	export class CommitSummary {
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new CommitSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
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
	export class RemoteConfig {
	    enabled: boolean;
	    port: number;
	    roots: string[];
	    keep_awake: string;
	    push: boolean;
	    sandbox_extra: string[];
	    core_socket: string;
	
	    static createFrom(source: any = {}) {
	        return new RemoteConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.roots = source["roots"];
	        this.keep_awake = source["keep_awake"];
	        this.push = source["push"];
	        this.sandbox_extra = source["sandbox_extra"];
	        this.core_socket = source["core_socket"];
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
	export class MemoryConfig {
	    enabled: boolean;
	    learn: string;
	    local_endpoint: string;
	    local_model: string;
	    verify: boolean;
	    embed_endpoint: string;
	    embed_model: string;
	    files_dir: string;
	    mem0_url: string;
	    supermemory_url: string;
	    privacy: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.learn = source["learn"];
	        this.local_endpoint = source["local_endpoint"];
	        this.local_model = source["local_model"];
	        this.verify = source["verify"];
	        this.embed_endpoint = source["embed_endpoint"];
	        this.embed_model = source["embed_model"];
	        this.files_dir = source["files_dir"];
	        this.mem0_url = source["mem0_url"];
	        this.supermemory_url = source["supermemory_url"];
	        this.privacy = source["privacy"];
	    }
	}
	export class LLMConfig {
	    provider: string;
	    model: string;
	    endpoint: string;
	    command: string;
	    timeout: string;
	    assistant_name: string;
	    instructions: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.endpoint = source["endpoint"];
	        this.command = source["command"];
	        this.timeout = source["timeout"];
	        this.assistant_name = source["assistant_name"];
	        this.instructions = source["instructions"];
	    }
	}
	export class PlannerConfig {
	    day_start: string;
	    day_end: string;
	    buffer: string;
	    eat_the_frog: boolean;
	    prime_start: string;
	    prime_end: string;
	
	    static createFrom(source: any = {}) {
	        return new PlannerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day_start = source["day_start"];
	        this.day_end = source["day_end"];
	        this.buffer = source["buffer"];
	        this.eat_the_frog = source["eat_the_frog"];
	        this.prime_start = source["prime_start"];
	        this.prime_end = source["prime_end"];
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
	    follow_tabs: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrackingConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.daily_target = source["daily_target"];
	        this.soft_idle = source["soft_idle"];
	        this.hard_idle = source["hard_idle"];
	        this.day_rollover = source["day_rollover"];
	        this.follow_tabs = source["follow_tabs"];
	    }
	}
	export class Config {
	    tracking: TrackingConfig;
	    nudge: NudgeConfig;
	    ntfy: NtfyConfig;
	    planner: PlannerConfig;
	    calendar: CalendarConfig;
	    llm: LLMConfig;
	    memory: MemoryConfig;
	    sync: SyncConfig;
	    remote: RemoteConfig;
	    agent: AgentConfig;
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
	        this.memory = this.convertValues(source["memory"], MemoryConfig);
	        this.sync = this.convertValues(source["sync"], SyncConfig);
	        this.remote = this.convertValues(source["remote"], RemoteConfig);
	        this.agent = this.convertValues(source["agent"], AgentConfig);
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
	export class Connection {
	    id: string;
	    name: string;
	    state: string;
	    message: string;
	    linkable: boolean;
	    capabilities: Capability[];
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.linkable = source["linkable"];
	        this.capabilities = this.convertValues(source["capabilities"], Capability);
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
	export class ConnectionList {
	    connections: Connection[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], Connection);
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
	export class Connector {
	    app: string;
	    name: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Connector(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = source["app"];
	        this.name = source["name"];
	        this.status = source["status"];
	    }
	}
	export class ConnectorItem {
	    app: string;
	    ref: string;
	    title: string;
	    url: string;
	    snippet: string;
	    when?: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectorItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = source["app"];
	        this.ref = source["ref"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.snippet = source["snippet"];
	        this.when = source["when"];
	    }
	}
	export class ConnectorItems {
	    items: ConnectorItem[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectorItems(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ConnectorItem);
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
	export class ConnectorLink {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectorLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}
	export class ConnectorList {
	    connectors: Connector[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectorList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectors = this.convertValues(source["connectors"], Connector);
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
	export class CreateAgentChat {
	    project_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateAgentChat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	    }
	}
	export class CreateAttachmentRequest {
	    kind: string;
	    title: string;
	    url: string;
	    path: string;
	    app: string;
	    ref: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateAttachmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.path = source["path"];
	        this.app = source["app"];
	        this.ref = source["ref"];
	        this.body = source["body"];
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
	    daily_minutes?: number;
	    project_id?: string;
	    start_day: string;
	    due_day: string;
	    specific?: string;
	    measurable?: string;
	    assignable?: string;
	    realistic?: string;
	
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
	        this.daily_minutes = source["daily_minutes"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	        this.specific = source["specific"];
	        this.measurable = source["measurable"];
	        this.assignable = source["assignable"];
	        this.realistic = source["realistic"];
	    }
	}
	export class CreateProjectRequest {
	    name: string;
	    color?: string;
	    folder?: string;
	    parent_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	        this.folder = source["folder"];
	        this.parent_id = source["parent_id"];
	    }
	}
	export class CreateRunRequest {
	    kind: string;
	    subject?: string;
	    title: string;
	    brief?: string;
	    start?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CreateRunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.subject = source["subject"];
	        this.title = source["title"];
	        this.brief = source["brief"];
	        this.start = source["start"];
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
	export class TaskSchedule {
	    day: string;
	    start_at?: number;
	    planned_minutes: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskSchedule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.start_at = source["start_at"];
	        this.planned_minutes = source["planned_minutes"];
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
	    parent_id?: string;
	    start_day?: string;
	    start_minute?: number;
	    stage?: string;
	    effort?: number;
	    delegated_to?: string;
	    schedule?: TaskSchedule;
	
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
	        this.parent_id = source["parent_id"];
	        this.start_day = source["start_day"];
	        this.start_minute = source["start_minute"];
	        this.stage = source["stage"];
	        this.effort = source["effort"];
	        this.delegated_to = source["delegated_to"];
	        this.schedule = this.convertValues(source["schedule"], TaskSchedule);
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
	export class DayHours {
	    start_minute?: number;
	    work_minutes?: number;
	
	    static createFrom(source: any = {}) {
	        return new DayHours(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start_minute = source["start_minute"];
	        this.work_minutes = source["work_minutes"];
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
	
	export class DecideApprovalRequest {
	    decision: string;
	    payload?: any;
	    remember?: string;
	
	    static createFrom(source: any = {}) {
	        return new DecideApprovalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decision = source["decision"];
	        this.payload = source["payload"];
	        this.remember = source["remember"];
	    }
	}
	export class DeleteTasksRequest {
	    ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new DeleteTasksRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ids = source["ids"];
	    }
	}
	export class DeletedTasks {
	    task_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new DeletedTasks(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_ids = source["task_ids"];
	    }
	}
	export class EditClaimRequest {
	    text: string;
	    value?: string;
	    valid_from?: string;
	    valid_to?: string;
	
	    static createFrom(source: any = {}) {
	        return new EditClaimRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.value = source["value"];
	        this.valid_from = source["valid_from"];
	        this.valid_to = source["valid_to"];
	    }
	}
	export class EnergyHour {
	    hour: number;
	    average: number;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new EnergyHour(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hour = source["hour"];
	        this.average = source["average"];
	        this.count = source["count"];
	    }
	}
	export class EnergyLog {
	    id: string;
	    at: number;
	    level: number;
	
	    static createFrom(source: any = {}) {
	        return new EnergyLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.level = source["level"];
	    }
	}
	export class EnergyReport {
	    days: number;
	    logs: EnergyLog[];
	    hours: EnergyHour[];
	    suggested_start?: string;
	    suggested_end?: string;
	
	    static createFrom(source: any = {}) {
	        return new EnergyReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	        this.logs = this.convertValues(source["logs"], EnergyLog);
	        this.hours = this.convertValues(source["hours"], EnergyHour);
	        this.suggested_start = source["suggested_start"];
	        this.suggested_end = source["suggested_end"];
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
	export class LaneReceipt {
	    lane: string;
	    ok: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new LaneReceipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lane = source["lane"];
	        this.ok = source["ok"];
	        this.note = source["note"];
	    }
	}
	export class ForgetReceipt {
	    claims: number;
	    episodes: number;
	    lanes: LaneReceipt[];
	
	    static createFrom(source: any = {}) {
	        return new ForgetReceipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claims = source["claims"];
	        this.episodes = source["episodes"];
	        this.lanes = this.convertValues(source["lanes"], LaneReceipt);
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
	export class GoalPreviewRequest {
	    goal_id?: string;
	    kind: string;
	    target_quantity?: number;
	    minutes_per_unit?: number;
	    daily_minutes?: number;
	    start_day: string;
	    due_day: string;
	
	    static createFrom(source: any = {}) {
	        return new GoalPreviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goal_id = source["goal_id"];
	        this.kind = source["kind"];
	        this.target_quantity = source["target_quantity"];
	        this.minutes_per_unit = source["minutes_per_unit"];
	        this.daily_minutes = source["daily_minutes"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	    }
	}
	
	export class Grant {
	    id: string;
	    capability: string;
	    pattern: string;
	    scope: string;
	    decision: string;
	    created_at: number;
	    expires_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new Grant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.capability = source["capability"];
	        this.pattern = source["pattern"];
	        this.scope = source["scope"];
	        this.decision = source["decision"];
	        this.created_at = source["created_at"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class GrantList {
	    grants: Grant[];
	
	    static createFrom(source: any = {}) {
	        return new GrantList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.grants = this.convertValues(source["grants"], Grant);
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
	
	export class HideClaimRequest {
	    hidden: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HideClaimRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hidden = source["hidden"];
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
	
	export class LogEnergyRequest {
	    level: number;
	    at?: number;
	
	    static createFrom(source: any = {}) {
	        return new LogEnergyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.at = source["at"];
	    }
	}
	export class MemoryClaim {
	    id: string;
	    key: string;
	    label: string;
	    subject: string;
	    subject_id?: string;
	    text: string;
	    value: any;
	    status: string;
	    trust: string;
	    sensitivity: string;
	    confidence: number;
	    valid_from?: number;
	    valid_to?: number;
	    mentioned_at: number;
	    recorded_at: number;
	    superseded_by?: string;
	    hidden: boolean;
	    source: string;
	    source_ref: string;
	    thread_id: string;
	    quote: string;
	    disputed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MemoryClaim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.key = source["key"];
	        this.label = source["label"];
	        this.subject = source["subject"];
	        this.subject_id = source["subject_id"];
	        this.text = source["text"];
	        this.value = source["value"];
	        this.status = source["status"];
	        this.trust = source["trust"];
	        this.sensitivity = source["sensitivity"];
	        this.confidence = source["confidence"];
	        this.valid_from = source["valid_from"];
	        this.valid_to = source["valid_to"];
	        this.mentioned_at = source["mentioned_at"];
	        this.recorded_at = source["recorded_at"];
	        this.superseded_by = source["superseded_by"];
	        this.hidden = source["hidden"];
	        this.source = source["source"];
	        this.source_ref = source["source_ref"];
	        this.thread_id = source["thread_id"];
	        this.quote = source["quote"];
	        this.disputed = source["disputed"];
	    }
	}
	export class MemoryClaimList {
	    claims: MemoryClaim[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryClaimList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claims = this.convertValues(source["claims"], MemoryClaim);
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
	export class MemoryClaimQuery {
	    status: string;
	    key: string;
	    subject: string;
	    q: string;
	    hidden: boolean;
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryClaimQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.key = source["key"];
	        this.subject = source["subject"];
	        this.q = source["q"];
	        this.hidden = source["hidden"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class MemoryClaimsResult {
	    claims: MemoryClaim[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryClaimsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claims = this.convertValues(source["claims"], MemoryClaim);
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
	
	export class MemoryConflict {
	    id: string;
	    reason: string;
	    status: string;
	    resolution?: string;
	    a: MemoryClaim;
	    b: MemoryClaim;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.reason = source["reason"];
	        this.status = source["status"];
	        this.resolution = source["resolution"];
	        this.a = this.convertValues(source["a"], MemoryClaim);
	        this.b = this.convertValues(source["b"], MemoryClaim);
	        this.created_at = source["created_at"];
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
	export class MemoryConflictList {
	    conflicts: MemoryConflict[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryConflictList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conflicts = this.convertValues(source["conflicts"], MemoryConflict);
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
	export class MemoryEntity {
	    id: string;
	    kind: string;
	    name: string;
	    aliases: string[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryEntity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.aliases = source["aliases"];
	    }
	}
	export class MemoryEntityList {
	    entities: MemoryEntity[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryEntityList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entities = this.convertValues(source["entities"], MemoryEntity);
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
	export class MemoryEpisode {
	    id: string;
	    kind: string;
	    channel: string;
	    speaker: string;
	    title: string;
	    body: string;
	    source: string;
	    at: number;
	    sensitivity: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryEpisode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.channel = source["channel"];
	        this.speaker = source["speaker"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.source = source["source"];
	        this.at = source["at"];
	        this.sensitivity = source["sensitivity"];
	    }
	}
	export class MemoryLink {
	    to_kind: string;
	    to_id: string;
	    relation: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.to_kind = source["to_kind"];
	        this.to_id = source["to_id"];
	        this.relation = source["relation"];
	    }
	}
	export class MemoryExplanation {
	    ref: string;
	    claim?: MemoryClaim;
	    quote: string;
	    context: AssistantMessage[];
	    episode?: MemoryEpisode;
	    history: MemoryClaim[];
	    links: MemoryLink[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryExplanation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.claim = this.convertValues(source["claim"], MemoryClaim);
	        this.quote = source["quote"];
	        this.context = this.convertValues(source["context"], AssistantMessage);
	        this.episode = this.convertValues(source["episode"], MemoryEpisode);
	        this.history = this.convertValues(source["history"], MemoryClaim);
	        this.links = this.convertValues(source["links"], MemoryLink);
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
	export class MemoryResource {
	    id: string;
	    title: string;
	    path: string;
	    mime: string;
	    size: number;
	    added_at: number;
	    document_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.path = source["path"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	        this.added_at = source["added_at"];
	        this.document_at = source["document_at"];
	    }
	}
	export class MemoryExport {
	    exported_at: number;
	    claims: MemoryClaim[];
	    entities: MemoryEntity[];
	    episodes: MemoryEpisode[];
	    conflicts: MemoryConflict[];
	    resources: MemoryResource[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryExport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exported_at = source["exported_at"];
	        this.claims = this.convertValues(source["claims"], MemoryClaim);
	        this.entities = this.convertValues(source["entities"], MemoryEntity);
	        this.episodes = this.convertValues(source["episodes"], MemoryEpisode);
	        this.conflicts = this.convertValues(source["conflicts"], MemoryConflict);
	        this.resources = this.convertValues(source["resources"], MemoryResource);
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
	export class MemoryHit {
	    ref: string;
	    kind: string;
	    key: string;
	    subject: string;
	    text: string;
	    source: string;
	    at: number;
	    valid_from?: number;
	    valid_to?: number;
	    trust: string;
	    sensitivity: string;
	    disputed: boolean;
	    score: number;
	    lanes: string[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.kind = source["kind"];
	        this.key = source["key"];
	        this.subject = source["subject"];
	        this.text = source["text"];
	        this.source = source["source"];
	        this.at = source["at"];
	        this.valid_from = source["valid_from"];
	        this.valid_to = source["valid_to"];
	        this.trust = source["trust"];
	        this.sensitivity = source["sensitivity"];
	        this.disputed = source["disputed"];
	        this.score = source["score"];
	        this.lanes = source["lanes"];
	    }
	}
	export class MemoryLane {
	    lane: string;
	    enabled: boolean;
	    ok: boolean;
	    items: number;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryLane(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lane = source["lane"];
	        this.enabled = source["enabled"];
	        this.ok = source["ok"];
	        this.items = source["items"];
	        this.detail = source["detail"];
	    }
	}
	
	
	
	export class MemoryResourceList {
	    resources: MemoryResource[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryResourceList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resources = this.convertValues(source["resources"], MemoryResource);
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
	export class ProfileCard {
	    day: string;
	    text: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfileCard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.text = source["text"];
	        this.created_at = source["created_at"];
	    }
	}
	export class MemoryStatus {
	    enabled: boolean;
	    learning: boolean;
	    claims: Record<string, number>;
	    conflicts: number;
	    jobs: Record<string, number>;
	    lanes: MemoryLane[];
	    last_learned_at?: number;
	    last_slept_at?: number;
	    profile?: ProfileCard;
	    ledger_rows: number;
	    ledger_ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MemoryStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.learning = source["learning"];
	        this.claims = source["claims"];
	        this.conflicts = source["conflicts"];
	        this.jobs = source["jobs"];
	        this.lanes = this.convertValues(source["lanes"], MemoryLane);
	        this.last_learned_at = source["last_learned_at"];
	        this.last_slept_at = source["last_slept_at"];
	        this.profile = this.convertValues(source["profile"], ProfileCard);
	        this.ledger_rows = source["ledger_rows"];
	        this.ledger_ok = source["ledger_ok"];
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
	export class MovePlanTaskRequest {
	    day: string;
	    task_id: string;
	    before_task_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new MovePlanTaskRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.task_id = source["task_id"];
	        this.before_task_id = source["before_task_id"];
	    }
	}
	export class NewResource {
	    kind: string;
	    title?: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new NewResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.value = source["value"];
	    }
	}
	export class NudgeAction {
	    id: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new NudgeAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	    }
	}
	export class Nudge {
	    kind: string;
	    title: string;
	    body: string;
	    markup: string;
	    mood: string;
	    actions: NudgeAction[];
	    at: number;
	    until_at?: number;
	    started_at?: number;
	    preview: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Nudge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.body = source["body"];
	        this.markup = source["markup"];
	        this.mood = source["mood"];
	        this.actions = this.convertValues(source["actions"], NudgeAction);
	        this.at = source["at"];
	        this.until_at = source["until_at"];
	        this.started_at = source["started_at"];
	        this.preview = source["preview"];
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
	export class NotifyPreview {
	    nudges: Nudge[];
	
	    static createFrom(source: any = {}) {
	        return new NotifyPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nudges = this.convertValues(source["nudges"], Nudge);
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
	
	
	
	
	export class NudgeList {
	    nudges: Nudge[];
	
	    static createFrom(source: any = {}) {
	        return new NudgeList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nudges = this.convertValues(source["nudges"], Nudge);
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
	export class PatchAgentChat {
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new PatchAgentChat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	    }
	}
	export class PatchAttachmentRequest {
	    title?: string;
	    url?: string;
	    body?: string;
	    rev?: number;
	
	    static createFrom(source: any = {}) {
	        return new PatchAttachmentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.url = source["url"];
	        this.body = source["body"];
	        this.rev = source["rev"];
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
	    daily_minutes: number | null;
	    project_id: string | null;
	    start_day?: string;
	    due_day?: string;
	    status?: string;
	    specific?: string;
	    measurable?: string;
	    assignable?: string;
	    realistic?: string;
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
	        this.daily_minutes = source["daily_minutes"];
	        this.project_id = source["project_id"];
	        this.start_day = source["start_day"];
	        this.due_day = source["due_day"];
	        this.status = source["status"];
	        this.specific = source["specific"];
	        this.measurable = source["measurable"];
	        this.assignable = source["assignable"];
	        this.realistic = source["realistic"];
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
	    folder?: string;
	    parent_id: string | null;
	    agent_autonomy?: string;
	
	    static createFrom(source: any = {}) {
	        return new PatchProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.color = source["color"];
	        this.archived = source["archived"];
	        this.rev = source["rev"];
	        this.folder = source["folder"];
	        this.parent_id = source["parent_id"];
	        this.agent_autonomy = source["agent_autonomy"];
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
	    parent_id: string | null;
	    start_day: string | null;
	    start_minute: number | null;
	    stage?: string;
	    effort: number | null;
	    delegated_to?: string;
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
	        this.parent_id = source["parent_id"];
	        this.start_day = source["start_day"];
	        this.start_minute = source["start_minute"];
	        this.stage = source["stage"];
	        this.effort = source["effort"];
	        this.delegated_to = source["delegated_to"];
	        this.rev = source["rev"];
	    }
	}
	export class PlanWindow {
	    start_minute: number;
	    end_minute: number;
	
	    static createFrom(source: any = {}) {
	        return new PlanWindow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start_minute = source["start_minute"];
	        this.end_minute = source["end_minute"];
	    }
	}
	export class Plan {
	    day: string;
	    capacity_minutes: number;
	    planned_minutes: number;
	    window: PlanWindow;
	    hours?: DayHours;
	    items: PlanItem[];
	    events: CalendarEvent[];
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.capacity_minutes = source["capacity_minutes"];
	        this.planned_minutes = source["planned_minutes"];
	        this.window = this.convertValues(source["window"], PlanWindow);
	        this.hours = this.convertValues(source["hours"], DayHours);
	        this.items = this.convertValues(source["items"], PlanItem);
	        this.events = this.convertValues(source["events"], CalendarEvent);
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
	export class PlanChatRequest {
	    day: string;
	    message: string;
	    run_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlanChatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.message = source["message"];
	        this.run_id = source["run_id"];
	    }
	}
	
	
	
	
	export class Project {
	    id: string;
	    name: string;
	    color: string;
	    archived_at?: number;
	    folder: string;
	    parent_id?: string;
	    agent_autonomy: string;
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
	        this.folder = source["folder"];
	        this.parent_id = source["parent_id"];
	        this.agent_autonomy = source["agent_autonomy"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.rev = source["rev"];
	    }
	}
	export class ProjectDir {
	    path: string;
	    test_command: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectDir(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.test_command = source["test_command"];
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
	
	export class PutResourcesRequest {
	    resources: NewResource[];
	
	    static createFrom(source: any = {}) {
	        return new PutResourcesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resources = this.convertValues(source["resources"], NewResource);
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
	export class RoutineStep {
	    title?: string;
	    capability: string;
	    input: any;
	
	    static createFrom(source: any = {}) {
	        return new RoutineStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.capability = source["capability"];
	        this.input = source["input"];
	    }
	}
	export class PutRoutineRequest {
	    steps: RoutineStep[];
	    auto_start: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PutRoutineRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.steps = this.convertValues(source["steps"], RoutineStep);
	        this.auto_start = source["auto_start"];
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
	export class RebuildRequest {
	    lane: string;
	
	    static createFrom(source: any = {}) {
	        return new RebuildRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lane = source["lane"];
	    }
	}
	export class RecallRequest {
	    query: string;
	    as_of: string;
	    lanes: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecallRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.as_of = source["as_of"];
	        this.lanes = source["lanes"];
	    }
	}
	export class RecallResult {
	    intent: string;
	    hits: MemoryHit[];
	    tokens: number;
	    abstain: boolean;
	    degraded: string[];
	
	    static createFrom(source: any = {}) {
	        return new RecallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.intent = source["intent"];
	        this.hits = this.convertValues(source["hits"], MemoryHit);
	        this.tokens = source["tokens"];
	        this.abstain = source["abstain"];
	        this.degraded = source["degraded"];
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
	export class RememberRequest {
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new RememberRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	    }
	}
	
	
	export class RemoteDevice {
	    id: string;
	    name: string;
	    created_at: number;
	    last_seen_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new RemoteDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.created_at = source["created_at"];
	        this.last_seen_at = source["last_seen_at"];
	    }
	}
	export class RemoteDevices {
	    devices: RemoteDevice[];
	
	    static createFrom(source: any = {}) {
	        return new RemoteDevices(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.devices = this.convertValues(source["devices"], RemoteDevice);
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
	export class RemotePairCode {
	    code: string;
	    url: string;
	    expires_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RemotePairCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.url = source["url"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class RemoteStatus {
	    enabled: boolean;
	    tailscale: string;
	    host: string;
	    certificates: boolean;
	    published: boolean;
	    port: number;
	    url: string;
	    owner: string;
	    socket: string;
	    hooks: boolean;
	    hooks_note: string;
	    core: string;
	    core_version: string;
	    terminal: boolean;
	    voice: boolean;
	    sandbox: boolean;
	    sandbox_signed_in: boolean;
	    devices: number;
	
	    static createFrom(source: any = {}) {
	        return new RemoteStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.tailscale = source["tailscale"];
	        this.host = source["host"];
	        this.certificates = source["certificates"];
	        this.published = source["published"];
	        this.port = source["port"];
	        this.url = source["url"];
	        this.owner = source["owner"];
	        this.socket = source["socket"];
	        this.hooks = source["hooks"];
	        this.hooks_note = source["hooks_note"];
	        this.core = source["core"];
	        this.core_version = source["core_version"];
	        this.terminal = source["terminal"];
	        this.voice = source["voice"];
	        this.sandbox = source["sandbox"];
	        this.sandbox_signed_in = source["sandbox_signed_in"];
	        this.devices = source["devices"];
	    }
	}
	export class ResolveConflictRequest {
	    resolution: string;
	
	    static createFrom(source: any = {}) {
	        return new ResolveConflictRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resolution = source["resolution"];
	    }
	}
	export class Resource {
	    id: string;
	    kind: string;
	    title: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new Resource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.value = source["value"];
	    }
	}
	export class ResourceList {
	    subject: string;
	    resources: Resource[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subject = source["subject"];
	        this.resources = this.convertValues(source["resources"], Resource);
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
	export class Routine {
	    subject: string;
	    steps: RoutineStep[];
	    auto_start: boolean;
	    inherited: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Routine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.subject = source["subject"];
	        this.steps = this.convertValues(source["steps"], RoutineStep);
	        this.auto_start = source["auto_start"];
	        this.inherited = source["inherited"];
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
	
	export class Run {
	    id: string;
	    kind: string;
	    subject: string;
	    title: string;
	    brief: string;
	    status: string;
	    reason: string;
	    external_ref: string;
	    result: any;
	    created_at: number;
	    updated_at: number;
	    started_at?: number;
	    ended_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.subject = source["subject"];
	        this.title = source["title"];
	        this.brief = source["brief"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.external_ref = source["external_ref"];
	        this.result = source["result"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	    }
	}
	export class RunEvent {
	    seq: number;
	    step_id: string;
	    kind: string;
	    data: any;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new RunEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.step_id = source["step_id"];
	        this.kind = source["kind"];
	        this.data = source["data"];
	        this.at = source["at"];
	    }
	}
	export class RunStep {
	    id: string;
	    position: number;
	    title: string;
	    capability: string;
	    risk: string;
	    status: string;
	    input: any;
	    output: any;
	    error: string;
	    started_at?: number;
	    ended_at?: number;
	
	    static createFrom(source: any = {}) {
	        return new RunStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.position = source["position"];
	        this.title = source["title"];
	        this.capability = source["capability"];
	        this.risk = source["risk"];
	        this.status = source["status"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	    }
	}
	export class RunDetail {
	    run: Run;
	    steps: RunStep[];
	    log: RunEvent[];
	    approvals: Approval[];
	
	    static createFrom(source: any = {}) {
	        return new RunDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run = this.convertValues(source["run"], Run);
	        this.steps = this.convertValues(source["steps"], RunStep);
	        this.log = this.convertValues(source["log"], RunEvent);
	        this.approvals = this.convertValues(source["approvals"], Approval);
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
	
	export class RunList {
	    runs: Run[];
	
	    static createFrom(source: any = {}) {
	        return new RunList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runs = this.convertValues(source["runs"], Run);
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
	export class RunMessageRequest {
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new RunMessageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	    }
	}
	
	export class SaveReviewRequest {
	    went_well?: string;
	    went_badly?: string;
	    energy?: string;
	    decisions?: string;
	    improvements?: string;
	    checklist?: number;
	
	    static createFrom(source: any = {}) {
	        return new SaveReviewRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.went_well = source["went_well"];
	        this.went_badly = source["went_badly"];
	        this.energy = source["energy"];
	        this.decisions = source["decisions"];
	        this.improvements = source["improvements"];
	        this.checklist = source["checklist"];
	    }
	}
	export class ScheduleRequest {
	    task_id: string;
	    day: string;
	    start_at?: number;
	    planned_minutes: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.day = source["day"];
	        this.start_at = source["start_at"];
	        this.planned_minutes = source["planned_minutes"];
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
	export class SelfVersion {
	    id: string;
	    attitude: string;
	    note: string;
	    since?: number;
	    reason: string;
	    thread_id?: string;
	    at: number;
	
	    static createFrom(source: any = {}) {
	        return new SelfVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.attitude = source["attitude"];
	        this.note = source["note"];
	        this.since = source["since"];
	        this.reason = source["reason"];
	        this.thread_id = source["thread_id"];
	        this.at = source["at"];
	    }
	}
	export class SelfHistory {
	    versions: SelfVersion[];
	
	    static createFrom(source: any = {}) {
	        return new SelfHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versions = this.convertValues(source["versions"], SelfVersion);
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
	
	export class SetDayHoursRequest {
	    day: string;
	    start_minute?: number;
	    work_minutes?: number;
	
	    static createFrom(source: any = {}) {
	        return new SetDayHoursRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.start_minute = source["start_minute"];
	        this.work_minutes = source["work_minutes"];
	    }
	}
	export class SleepReport {
	    learned: number;
	    merged: number;
	    conflicts: number;
	    insights: number;
	    observed: number;
	    reconciled: number;
	    profile: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SleepReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.learned = source["learned"];
	        this.merged = source["merged"];
	        this.conflicts = source["conflicts"];
	        this.insights = source["insights"];
	        this.observed = source["observed"];
	        this.reconciled = source["reconciled"];
	        this.profile = source["profile"];
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
	export class StartCodeSession {
	    dir: string;
	    create?: boolean;
	    git_init?: boolean;
	    sandbox?: boolean;
	    prompt?: string;
	    window?: boolean;
	    resume?: string;
	    permission_mode?: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new StartCodeSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.create = source["create"];
	        this.git_init = source["git_init"];
	        this.sandbox = source["sandbox"];
	        this.prompt = source["prompt"];
	        this.window = source["window"];
	        this.resume = source["resume"];
	        this.permission_mode = source["permission_mode"];
	        this.name = source["name"];
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
	export class SummarizeCommit {
	    dir: string;
	    claude_session_id: string;
	    commit: string;
	    subject: string;
	
	    static createFrom(source: any = {}) {
	        return new SummarizeCommit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.claude_session_id = source["claude_session_id"];
	        this.commit = source["commit"];
	        this.subject = source["subject"];
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
	
	export class SyncStatus {
	    configured: boolean;
	    last_push_at?: number;
	    last_pull_at?: number;
	    last_error?: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.last_push_at = source["last_push_at"];
	        this.last_pull_at = source["last_pull_at"];
	        this.last_error = source["last_error"];
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
	
	
	export class UnscheduleRequest {
	    task_id: string;
	    day: string;
	
	    static createFrom(source: any = {}) {
	        return new UnscheduleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.day = source["day"];
	    }
	}
	export class Unscheduled {
	    days: string[];
	
	    static createFrom(source: any = {}) {
	        return new Unscheduled(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	    }
	}
	
	export class WeeklyReview {
	    id: string;
	    week_start: string;
	    went_well: string;
	    went_badly: string;
	    energy: string;
	    decisions: string;
	    improvements: string;
	    checklist: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new WeeklyReview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.week_start = source["week_start"];
	        this.went_well = source["went_well"];
	        this.went_badly = source["went_badly"];
	        this.energy = source["energy"];
	        this.decisions = source["decisions"];
	        this.improvements = source["improvements"];
	        this.checklist = source["checklist"];
	        this.updated_at = source["updated_at"];
	    }
	}

}

