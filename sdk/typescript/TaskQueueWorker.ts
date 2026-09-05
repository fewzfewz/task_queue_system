import * as http from 'http';

type JobHandler = (payload: any) => Promise<any>;

export class TaskQueueWorker {
    private port: number;
    private handlers: Map<string, JobHandler> = new Map();

    constructor(port: number = 5000) {
        this.port = port;
    }

    public registerHandler(jobType: string, handler: JobHandler) {
        this.handlers.set(jobType, handler);
        console.log(`[INFO] Registered handler for job type: ${jobType}`);
    }

    public start() {
        const server = http.createServer((req, res) => {
            if (req.method === 'POST') {
                let body = '';
                req.on('data', chunk => {
                    body += chunk.toString();
                });
                
                req.on('end', async () => {
                    const jobId = req.headers['x-task-queue-job-id'] || 'unknown';
                    const jobType = req.url ? req.url.replace(/^\//, '') : '';
                    
                    const handler = this.handlers.get(jobType);
                    if (!handler) {
                        res.writeHead(404);
                        res.end('No handler registered for this job type');
                        return;
                    }

                    console.log(`[INFO] Processing job ${jobId} of type ${jobType}`);
                    try {
                        const payload = JSON.parse(body);
                        const result = await handler(payload);
                        
                        res.writeHead(200, { 'Content-Type': 'application/json' });
                        res.end(JSON.stringify({ result, status: "success" }));
                        console.log(`[INFO] Successfully processed job ${jobId}`);
                    } catch (error: any) {
                        console.error(`[ERROR] processing job ${jobId}:`, error);
                        res.writeHead(500, { 'Content-Type': 'application/json' });
                        res.end(JSON.stringify({ error: error.message }));
                    }
                });
            } else {
                res.writeHead(405);
                res.end('Method Not Allowed');
            }
        });

        server.listen(this.port, () => {
            console.log(`[INFO] TypeScript Task Queue Worker listening on port ${this.port}...`);
        });
    }
}
