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
	export class CompatibilityRequest {
	    port: string;
	    baud: number;

	    static createFrom(source: any = {}) {
	        return new CompatibilityRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.baud = source["baud"];
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
	    hardwareVersion: string;
	    display: string;
	    minimumFlashBytes: number;
	    minimumPsramBytes: number;
	    psramMode: string;
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
	        this.hardwareVersion = source["hardwareVersion"];
	        this.display = source["display"];
	        this.minimumFlashBytes = source["minimumFlashBytes"];
	        this.minimumPsramBytes = source["minimumPsramBytes"];
	        this.psramMode = source["psramMode"];
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

export namespace preflight {

	export class CheckResult {
	    code: string;
	    status: string;
	    title: string;
	    summary: string;
	    resolution: string;
	    steps: string[];
	    technical: string;

	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.status = source["status"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.resolution = source["resolution"];
	        this.steps = source["steps"];
	        this.technical = source["technical"];
	    }
	}
	export class DeviceCapabilities {
	    chip: string;
	    revision: string;
	    flashBytes: number;
	    flashDescription: string;
	    psramBytes: number;
	    psramDescription: string;
	    psramMode: string;
	    psramKnown: boolean;
	    secureBoot: string;
	    flashEncryption: string;
	    rawOutput: string[];

	    static createFrom(source: any = {}) {
	        return new DeviceCapabilities(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chip = source["chip"];
	        this.revision = source["revision"];
	        this.flashBytes = source["flashBytes"];
	        this.flashDescription = source["flashDescription"];
	        this.psramBytes = source["psramBytes"];
	        this.psramDescription = source["psramDescription"];
	        this.psramMode = source["psramMode"];
	        this.psramKnown = source["psramKnown"];
	        this.secureBoot = source["secureBoot"];
	        this.flashEncryption = source["flashEncryption"];
	        this.rawOutput = source["rawOutput"];
	    }
	}
	export class Partition {
	    type: number;
	    subtype: number;
	    offset: number;
	    size: number;
	    label: string;
	    flags: number;

	    static createFrom(source: any = {}) {
	        return new Partition(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.subtype = source["subtype"];
	        this.offset = source["offset"];
	        this.size = source["size"];
	        this.label = source["label"];
	        this.flags = source["flags"];
	    }
	}
	export class FlashRegion {
	    offset: number;
	    size: number;
	    path: string;

	    static createFrom(source: any = {}) {
	        return new FlashRegion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.size = source["size"];
	        this.path = source["path"];
	    }
	}
	export class FirmwareRequirements {
	    chip: string;
	    hardwareVersion: string;
	    display: string;
	    minimumFlashBytes: number;
	    requiredPsramBytes: number;
	    requiresOctalPsram: boolean;
	    flashFiles: FlashRegion[];
	    partitions: Partition[];

	    static createFrom(source: any = {}) {
	        return new FirmwareRequirements(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chip = source["chip"];
	        this.hardwareVersion = source["hardwareVersion"];
	        this.display = source["display"];
	        this.minimumFlashBytes = source["minimumFlashBytes"];
	        this.requiredPsramBytes = source["requiredPsramBytes"];
	        this.requiresOctalPsram = source["requiresOctalPsram"];
	        this.flashFiles = this.convertValues(source["flashFiles"], FlashRegion);
	        this.partitions = this.convertValues(source["partitions"], Partition);
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


	export class Report {
	    overall: string;
	    firmware: FirmwareRequirements;
	    device: DeviceCapabilities;
	    checks: CheckResult[];
	    rawLog: string[];

	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overall = source["overall"];
	        this.firmware = this.convertValues(source["firmware"], FirmwareRequirements);
	        this.device = this.convertValues(source["device"], DeviceCapabilities);
	        this.checks = this.convertValues(source["checks"], CheckResult);
	        this.rawLog = source["rawLog"];
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
	    eimPath: string;
	    eimJsonPath: string;
	    idfVersion: string;
	    gitPath: string;
	    componentCachePath: string;
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
	        this.eimPath = source["eimPath"];
	        this.eimJsonPath = source["eimJsonPath"];
	        this.idfVersion = source["idfVersion"];
	        this.gitPath = source["gitPath"];
	        this.componentCachePath = source["componentCachePath"];
	        this.message = source["message"];
	    }
	}

}
