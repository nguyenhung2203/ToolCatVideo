export namespace main {
	
	export class ExportResult {
	    clipId: string;
	    index: number;
	    ok: boolean;
	    outPath: string;
	    duration: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clipId = source["clipId"];
	        this.index = source["index"];
	        this.ok = source["ok"];
	        this.outPath = source["outPath"];
	        this.duration = source["duration"];
	        this.error = source["error"];
	    }
	}

}

export namespace project {
	
	export class SignalWeights {
	    visualChange: number;
	    blackFrame: number;
	    silence: number;
	    layoutChange: number;
	    audioChange: number;
	    continuityPen: number;
	
	    static createFrom(source: any = {}) {
	        return new SignalWeights(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.visualChange = source["visualChange"];
	        this.blackFrame = source["blackFrame"];
	        this.silence = source["silence"];
	        this.layoutChange = source["layoutChange"];
	        this.audioChange = source["audioChange"];
	        this.continuityPen = source["continuityPen"];
	    }
	}
	export class AnalyzerConfig {
	    mode: string;
	    sceneThreshold: number;
	    minClipDuration: number;
	    maxClipDuration: number;
	    autoAcceptScore: number;
	    reviewMinScore: number;
	    silenceThreshold: number;
	    silenceDuration: number;
	    proxyFPS: number;
	    weights: SignalWeights;
	    exportPreset: string;
	    exportCRF: number;
	
	    static createFrom(source: any = {}) {
	        return new AnalyzerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.sceneThreshold = source["sceneThreshold"];
	        this.minClipDuration = source["minClipDuration"];
	        this.maxClipDuration = source["maxClipDuration"];
	        this.autoAcceptScore = source["autoAcceptScore"];
	        this.reviewMinScore = source["reviewMinScore"];
	        this.silenceThreshold = source["silenceThreshold"];
	        this.silenceDuration = source["silenceDuration"];
	        this.proxyFPS = source["proxyFPS"];
	        this.weights = this.convertValues(source["weights"], SignalWeights);
	        this.exportPreset = source["exportPreset"];
	        this.exportCRF = source["exportCRF"];
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
	export class AspectOp {
	    enabled: boolean;
	    ratio: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new AspectOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ratio = source["ratio"];
	        this.mode = source["mode"];
	    }
	}
	export class AudioOp {
	    volume: number;
	    mute: boolean;
	    musicPath: string;
	    musicVolume: number;
	    fadeIn: number;
	    fadeOut: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volume = source["volume"];
	        this.mute = source["mute"];
	        this.musicPath = source["musicPath"];
	        this.musicVolume = source["musicVolume"];
	        this.fadeIn = source["fadeIn"];
	        this.fadeOut = source["fadeOut"];
	    }
	}
	export class TransitionOp {
	    type: string;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new TransitionOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.duration = source["duration"];
	    }
	}
	export class WatermarkOp {
	    enabled: boolean;
	    imgPath: string;
	    x: string;
	    y: string;
	    opacity: number;
	    scale: number;
	
	    static createFrom(source: any = {}) {
	        return new WatermarkOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.imgPath = source["imgPath"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.opacity = source["opacity"];
	        this.scale = source["scale"];
	    }
	}
	export class TextOp {
	    content: string;
	    fontSize: number;
	    color: string;
	    x: string;
	    y: string;
	    startTime: number;
	    endTime: number;
	    bgBox: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TextOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.fontSize = source["fontSize"];
	        this.color = source["color"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.startTime = source["startTime"];
	        this.endTime = source["endTime"];
	        this.bgBox = source["bgBox"];
	    }
	}
	export class ColorOp {
	    enabled: boolean;
	    brightness: number;
	    contrast: number;
	    saturation: number;
	    preset: string;
	
	    static createFrom(source: any = {}) {
	        return new ColorOp(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.brightness = source["brightness"];
	        this.contrast = source["contrast"];
	        this.saturation = source["saturation"];
	        this.preset = source["preset"];
	    }
	}
	export class EditOps {
	    aspect: AspectOp;
	    color: ColorOp;
	    speed: number;
	    hflip: boolean;
	    texts: TextOp[];
	    watermark: WatermarkOp;
	    audio: AudioOp;
	    transition: TransitionOp;
	
	    static createFrom(source: any = {}) {
	        return new EditOps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.aspect = this.convertValues(source["aspect"], AspectOp);
	        this.color = this.convertValues(source["color"], ColorOp);
	        this.speed = source["speed"];
	        this.hflip = source["hflip"];
	        this.texts = this.convertValues(source["texts"], TextOp);
	        this.watermark = this.convertValues(source["watermark"], WatermarkOp);
	        this.audio = this.convertValues(source["audio"], AudioOp);
	        this.transition = this.convertValues(source["transition"], TransitionOp);
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
	export class Clip {
	    id: string;
	    index: number;
	    startTime: number;
	    endTime: number;
	    duration: number;
	    status: string;
	    thumbnail: string;
	    thumbEnd: string;
	    confidence: number;
	    tier: string;
	    reason: string;
	    signals: string[];
	    edit: EditOps;
	
	    static createFrom(source: any = {}) {
	        return new Clip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.index = source["index"];
	        this.startTime = source["startTime"];
	        this.endTime = source["endTime"];
	        this.duration = source["duration"];
	        this.status = source["status"];
	        this.thumbnail = source["thumbnail"];
	        this.thumbEnd = source["thumbEnd"];
	        this.confidence = source["confidence"];
	        this.tier = source["tier"];
	        this.reason = source["reason"];
	        this.signals = source["signals"];
	        this.edit = this.convertValues(source["edit"], EditOps);
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
	    sourcePath: string;
	    name: string;
	    duration: number;
	    width: number;
	    height: number;
	    fps: number;
	    status: string;
	    config: AnalyzerConfig;
	    clips: Clip[];
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sourcePath = source["sourcePath"];
	        this.name = source["name"];
	        this.duration = source["duration"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.status = source["status"];
	        this.config = this.convertValues(source["config"], AnalyzerConfig);
	        this.clips = this.convertValues(source["clips"], Clip);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
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
	
	
	
	export class VideoInfo {
	    SourcePath: string;
	    Duration: number;
	    Width: number;
	    Height: number;
	    FPS: number;
	    TimeBase: string;
	    sizeByte: number;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SourcePath = source["SourcePath"];
	        this.Duration = source["Duration"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.FPS = source["FPS"];
	        this.TimeBase = source["TimeBase"];
	        this.sizeByte = source["sizeByte"];
	    }
	}

}

export namespace storage {
	
	export class ProjectSummary {
	    id: string;
	    sourcePath: string;
	    name: string;
	    duration: number;
	    status: string;
	    clipCount: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sourcePath = source["sourcePath"];
	        this.name = source["name"];
	        this.duration = source["duration"];
	        this.status = source["status"];
	        this.clipCount = source["clipCount"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

