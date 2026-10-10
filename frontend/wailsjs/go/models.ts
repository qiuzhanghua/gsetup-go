export namespace main {
	
	export class Installation {
	    toolchain: string;
	    home: string;
	    homeDisplay: string;
	    installed: boolean;
	    version: string;
	    cmd: string;
	    needsUpgrade: boolean;
	    verified: boolean;
	    uninstallable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Installation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.toolchain = source["toolchain"];
	        this.home = source["home"];
	        this.homeDisplay = source["homeDisplay"];
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.cmd = source["cmd"];
	        this.needsUpgrade = source["needsUpgrade"];
	        this.verified = source["verified"];
	        this.uninstallable = source["uninstallable"];
	    }
	}
	export class Plan {
	    ok: boolean;
	    message: string;
	    toolchain: string;
	    home: string;
	    homeDisplay: string;
	    addToPath: boolean;
	    program: string;
	    args: string[];
	    display: string;
	    dir: string;
	    script: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.toolchain = source["toolchain"];
	        this.home = source["home"];
	        this.homeDisplay = source["homeDisplay"];
	        this.addToPath = source["addToPath"];
	        this.program = source["program"];
	        this.args = source["args"];
	        this.display = source["display"];
	        this.dir = source["dir"];
	        this.script = source["script"];
	        this.warning = source["warning"];
	    }
	}
	export class Removal {
	    kind: string;
	    path: string;
	    display: string;
	    present: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Removal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.display = source["display"];
	        this.present = source["present"];
	    }
	}
	export class State {
	    ok: boolean;
	    message: string;
	    root: string;
	    short: string;
	    id: string;
	    name: string;
	    version: string;
	    cmd: string;
	    goos: string;
	    userHome: string;
	    toolchains: string[];
	    defaultToolchain: string;
	    defaultHome: string;
	    installs: Installation[];
	    script: string;
	    scriptDisplay: string;
	    hasSetup: boolean;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.root = source["root"];
	        this.short = source["short"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.cmd = source["cmd"];
	        this.goos = source["goos"];
	        this.userHome = source["userHome"];
	        this.toolchains = source["toolchains"];
	        this.defaultToolchain = source["defaultToolchain"];
	        this.defaultHome = source["defaultHome"];
	        this.installs = this.convertValues(source["installs"], Installation);
	        this.script = source["script"];
	        this.scriptDisplay = source["scriptDisplay"];
	        this.hasSetup = source["hasSetup"];
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
	export class UninstallPlan {
	    ok: boolean;
	    message: string;
	    id: string;
	    name: string;
	    version: string;
	    home: string;
	    homeDisplay: string;
	    items: Removal[];
	    program: string;
	    args: string[];
	    display: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new UninstallPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.home = source["home"];
	        this.homeDisplay = source["homeDisplay"];
	        this.items = this.convertValues(source["items"], Removal);
	        this.program = source["program"];
	        this.args = source["args"];
	        this.display = source["display"];
	        this.warning = source["warning"];
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

export namespace runner {
	
	export class Result {
	    code: number;
	    signal?: string;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.signal = source["signal"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}

}

