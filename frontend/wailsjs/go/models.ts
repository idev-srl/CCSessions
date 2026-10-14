export namespace ccdata {
	
	export class CleanupPreview {
	    days: number;
	    sessions: number;
	    transcriptBytes: number;
	    extraBytes: number;
	    totalSessions: number;
	
	    static createFrom(source: any = {}) {
	        return new CleanupPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	        this.sessions = source["sessions"];
	        this.transcriptBytes = source["transcriptBytes"];
	        this.extraBytes = source["extraBytes"];
	        this.totalSessions = source["totalSessions"];
	    }
	}
	export class Session {
	    id: string;
	    file: string;
	    cwd: string;
	    firstEvent: string;
	    lastEvent: string;
	    modTime: string;
	    userMessages: number;
	    assistantMessages: number;
	    fileSize: number;
	    extraSize: number;
	    title: string;
	    firstPrompt: string;
	    preview: string;
	    badLines: number;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.file = source["file"];
	        this.cwd = source["cwd"];
	        this.firstEvent = source["firstEvent"];
	        this.lastEvent = source["lastEvent"];
	        this.modTime = source["modTime"];
	        this.userMessages = source["userMessages"];
	        this.assistantMessages = source["assistantMessages"];
	        this.fileSize = source["fileSize"];
	        this.extraSize = source["extraSize"];
	        this.title = source["title"];
	        this.firstPrompt = source["firstPrompt"];
	        this.preview = source["preview"];
	        this.badLines = source["badLines"];
	        this.active = source["active"];
	    }
	}
	export class Directory {
	    path: string;
	    key: string;
	    exists: boolean;
	    inClaudeJson: boolean;
	    unresolved: boolean;
	    sessions: Session[];
	    sessionCount: number;
	    lastSession: string;
	    totalSize: number;
	    transcriptDirs: string[];
	    projectFolder: string;
	    hasMemory: boolean;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Directory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.key = source["key"];
	        this.exists = source["exists"];
	        this.inClaudeJson = source["inClaudeJson"];
	        this.unresolved = source["unresolved"];
	        this.sessions = this.convertValues(source["sessions"], Session);
	        this.sessionCount = source["sessionCount"];
	        this.lastSession = source["lastSession"];
	        this.totalSize = source["totalSize"];
	        this.transcriptDirs = source["transcriptDirs"];
	        this.projectFolder = source["projectFolder"];
	        this.hasMemory = source["hasMemory"];
	        this.active = source["active"];
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
	export class Locations {
	    Base: string;
	    ClaudeJSON: string;
	
	    static createFrom(source: any = {}) {
	        return new Locations(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Base = source["Base"];
	        this.ClaudeJSON = source["ClaudeJSON"];
	    }
	}
	export class Override {
	    source: string;
	    kind: string;
	    days: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new Override(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.kind = source["kind"];
	        this.days = source["days"];
	        this.note = source["note"];
	    }
	}
	export class RemovalPlan {
	    path: string;
	    key: string;
	    claudeJsonKeys: string[];
	    projectFolder: string;
	    hasMemory: boolean;
	    sessionFiles: string[];
	    sessions: number;
	    bytes: number;
	    blockers: string[];
	
	    static createFrom(source: any = {}) {
	        return new RemovalPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.key = source["key"];
	        this.claudeJsonKeys = source["claudeJsonKeys"];
	        this.projectFolder = source["projectFolder"];
	        this.hasMemory = source["hasMemory"];
	        this.sessionFiles = source["sessionFiles"];
	        this.sessions = source["sessions"];
	        this.bytes = source["bytes"];
	        this.blockers = source["blockers"];
	    }
	}
	export class RemovalResult {
	    claudeJsonBackup: string;
	    removedKeys: string[];
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new RemovalResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.claudeJsonBackup = source["claudeJsonBackup"];
	        this.removedKeys = source["removedKeys"];
	        this.errors = source["errors"];
	    }
	}
	export class ScanResult {
	    directories: Directory[];
	    errors: string[];
	    scannedAt: string;
	    durationMs: number;
	    locations: Locations;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directories = this.convertValues(source["directories"], Directory);
	        this.errors = source["errors"];
	        this.scannedAt = source["scannedAt"];
	        this.durationMs = source["durationMs"];
	        this.locations = this.convertValues(source["locations"], Locations);
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

export namespace main {
	
	export class CleanupInfo {
	    settingsPath: string;
	    days: number;
	    found: boolean;
	    error: string;
	    defaultDays: number;
	    minDays: number;
	    maxDays: number;
	    lastCleanup: string;
	    overrides: ccdata.Override[];
	
	    static createFrom(source: any = {}) {
	        return new CleanupInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settingsPath = source["settingsPath"];
	        this.days = source["days"];
	        this.found = source["found"];
	        this.error = source["error"];
	        this.defaultDays = source["defaultDays"];
	        this.minDays = source["minDays"];
	        this.maxDays = source["maxDays"];
	        this.lastCleanup = source["lastCleanup"];
	        this.overrides = this.convertValues(source["overrides"], ccdata.Override);
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
	export class Environment {
	    version: string;
	    hasWT: boolean;
	    hasVSCode: boolean;
	    license: string;
	    company: string;
	    websiteUrl: string;
	    repoUrl: string;
	    licenseUrl: string;
	    configDir: string;
	    claudeJson: string;
	
	    static createFrom(source: any = {}) {
	        return new Environment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.hasWT = source["hasWT"];
	        this.hasVSCode = source["hasVSCode"];
	        this.license = source["license"];
	        this.company = source["company"];
	        this.websiteUrl = source["websiteUrl"];
	        this.repoUrl = source["repoUrl"];
	        this.licenseUrl = source["licenseUrl"];
	        this.configDir = source["configDir"];
	        this.claudeJson = source["claudeJson"];
	    }
	}

}

