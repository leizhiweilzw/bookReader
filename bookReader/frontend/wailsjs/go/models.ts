export namespace main {
	
	export class Chapter {
	    index: number;
	    title: string;
	    start: number;
	    length: number;
	    isRead: boolean;
	    readAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Chapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.title = source["title"];
	        this.start = source["start"];
	        this.length = source["length"];
	        this.isRead = source["isRead"];
	        this.readAt = source["readAt"];
	    }
	}
	export class ReadingProgress {
	    bookId: string;
	    bookTitle: string;
	    filePath: string;
	    chapterIndex: number;
	    scrollPercent: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadingProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bookId = source["bookId"];
	        this.bookTitle = source["bookTitle"];
	        this.filePath = source["filePath"];
	        this.chapterIndex = source["chapterIndex"];
	        this.scrollPercent = source["scrollPercent"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class OpenResult {
	    id: string;
	    title: string;
	    filePath: string;
	    chapterCount: number;
	    chapters: Chapter[];
	    progress?: ReadingProgress;
	
	    static createFrom(source: any = {}) {
	        return new OpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.filePath = source["filePath"];
	        this.chapterCount = source["chapterCount"];
	        this.chapters = this.convertValues(source["chapters"], Chapter);
	        this.progress = this.convertValues(source["progress"], ReadingProgress);
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

