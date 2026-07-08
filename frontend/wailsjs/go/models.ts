export namespace device {
	
	export class Device {
	    name: string;
	    port: string;
	    pnpDeviceId: string;
	    mode: string;
	    canFlash: boolean;
	    score: number;
	    hint: string;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.port = source["port"];
	        this.pnpDeviceId = source["pnpDeviceId"];
	        this.mode = source["mode"];
	        this.canFlash = source["canFlash"];
	        this.score = source["score"];
	        this.hint = source["hint"];
	    }
	}

}

export namespace main {
	
	export class AnalyzeResponse {
	    analysis?: packagekit.Analysis;
	    runtime: runtimekit.Status;
	    devices: device.Device[];
	
	    static createFrom(source: any = {}) {
	        return new AnalyzeResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.analysis = this.convertValues(source["analysis"], packagekit.Analysis);
	        this.runtime = this.convertValues(source["runtime"], runtimekit.Status);
	        this.devices = this.convertValues(source["devices"], device.Device);
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
	export class FlashRequest {
	    port: string;
	    baud: number;
	
	    static createFrom(source: any = {}) {
	        return new FlashRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.baud = source["baud"];
	    }
	}
	export class GitHubRequest {
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new GitHubRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	    }
	}

}

export namespace packagekit {
	
	export class FlashFile {
	    offset: string;
	    path: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new FlashFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.path = source["path"];
	        this.size = source["size"];
	    }
	}
	export class Analysis {
	    sourcePath: string;
	    root: string;
	    kind: string;
	    projectName: string;
	    canFlash: boolean;
	    needsBuild: boolean;
	    chip: string;
	    writeFlashArgs: string[];
	    before: string;
	    after: string;
	    flashFiles: FlashFile[];
	    messages: string[];
	
	    static createFrom(source: any = {}) {
	        return new Analysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourcePath = source["sourcePath"];
	        this.root = source["root"];
	        this.kind = source["kind"];
	        this.projectName = source["projectName"];
	        this.canFlash = source["canFlash"];
	        this.needsBuild = source["needsBuild"];
	        this.chip = source["chip"];
	        this.writeFlashArgs = source["writeFlashArgs"];
	        this.before = source["before"];
	        this.after = source["after"];
	        this.flashFiles = this.convertValues(source["flashFiles"], FlashFile);
	        this.messages = source["messages"];
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

export namespace runtimekit {
	
	export class Status {
	    available: boolean;
	    canBuild: boolean;
	    kind: string;
	    toolPath: string;
	    pythonPath: string;
	    idfPyPath: string;
	    exportScript: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.canBuild = source["canBuild"];
	        this.kind = source["kind"];
	        this.toolPath = source["toolPath"];
	        this.pythonPath = source["pythonPath"];
	        this.idfPyPath = source["idfPyPath"];
	        this.exportScript = source["exportScript"];
	        this.message = source["message"];
	    }
	}

}

