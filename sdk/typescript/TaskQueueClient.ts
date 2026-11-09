import * as http from 'http';
import * as https from 'https';
import { URL } from 'url';

export interface ClientOptions {
    baseURL?: string;
    apiKey?: string;
    timeoutMs?: number;
}

export type JobPriority = 'high' | 'medium' | 'low';

export interface SubmitJobOptions {
    type: string;
    payload?: Record<string, any>;
    priority?: JobPriority;
    maxRetries?: number;
    timeout?: number;
    correlationId?: string;
    dedupKey?: string;
    runAt?: string | Date;
    labels?: Record<string, string>;
}

export interface JobResponse {
    id: string;
    type: string;
    status: string;
    priority: string;
    payload?: Record<string, any>;
    result?: any;
    error?: string;
    progress?: number;
    created_at?: string;
    updated_at?: string;
    processed_by?: string;
}

export interface BatchSubmitResponse {
    job_ids: string[];
    count: number;
}

export interface ListJobsFilter {
    status?: string;
    type?: string;
    limit?: number;
    page?: number;
    labelKey?: string;
    labelValue?: string;
}

export interface ListJobsResponse {
    jobs: JobResponse[];
    total: number;
    page: number;
    limit: number;
}

export interface ActionResponse {
    status: string;
    message?: string;
}

export class TaskQueueError extends Error {
    public statusCode: number;
    constructor(statusCode: number, message: string) {
        super(`Task Queue API error (${statusCode}): ${message}`);
        this.name = 'TaskQueueError';
        this.statusCode = statusCode;
    }
}

export class TaskQueueClient {
    private baseURL: string;
    private apiKey: string;
    private timeoutMs: number;

    constructor(options: ClientOptions = {}) {
        this.baseURL = (options.baseURL || 'http://localhost:8080').replace(/\/+$/, '');
        this.apiKey = options.apiKey || '';
        this.timeoutMs = options.timeoutMs || 15000;
    }

    private request<T>(method: string, path: string, body?: any, query?: Record<string, any>): Promise<T> {
        return new Promise((resolve, reject) => {
            const urlObj = new URL(`${this.baseURL}${path}`);
            if (query) {
                for (const [k, v] of Object.entries(query)) {
                    if (v !== undefined && v !== null && v !== '') {
                        urlObj.searchParams.set(k, String(v));
                    }
                }
            }

            const isHttps = urlObj.protocol === 'https:';
            const clientModule = isHttps ? https : http;

            const postData = body !== undefined ? JSON.stringify(body) : undefined;
            const headers: Record<string, string> = {};
            if (this.apiKey) {
                headers['X-API-Key'] = this.apiKey;
            }
            if (postData) {
                headers['Content-Type'] = 'application/json';
                headers['Content-Length'] = Buffer.byteLength(postData).toString();
            }

            const req = clientModule.request(
                urlObj,
                {
                    method,
                    headers,
                    timeout: this.timeoutMs,
                },
                (res) => {
                    let responseData = '';
                    res.on('data', (chunk) => {
                        responseData += chunk;
                    });
                    res.on('end', () => {
                        const statusCode = res.statusCode || 0;
                        if (statusCode >= 200 && statusCode < 300) {
                            if (!responseData) {
                                return resolve({ status: 'ok' } as unknown as T);
                            }
                            try {
                                resolve(JSON.parse(responseData) as T);
                            } catch {
                                resolve(responseData as unknown as T);
                            }
                        } else {
                            reject(new TaskQueueError(statusCode, responseData || res.statusMessage || 'Request failed'));
                        }
                    });
                }
            );

            req.on('error', (err) => {
                reject(new TaskQueueError(0, err.message));
            });

            req.on('timeout', () => {
                req.destroy(new Error('Request timed out'));
            });

            if (postData) {
                req.write(postData);
            }
            req.end();
        });
    }

    public submit(options: SubmitJobOptions): Promise<JobResponse> {
        const body: Record<string, any> = {
            type: options.type,
            payload: options.payload || {},
            priority: options.priority || 'medium',
            max_retries: options.maxRetries !== undefined ? options.maxRetries : 3,
            timeout: options.timeout !== undefined ? options.timeout : 60,
        };
        if (options.correlationId) body['correlation_id'] = options.correlationId;
        if (options.dedupKey) body['dedup_key'] = options.dedupKey;
        if (options.runAt) {
            body['run_at'] = options.runAt instanceof Date ? options.runAt.toISOString() : options.runAt;
        }
        if (options.labels) body['labels'] = options.labels;

        return this.request<JobResponse>('POST', '/jobs', body);
    }

    public submitBatch(jobs: SubmitJobOptions[]): Promise<BatchSubmitResponse> {
        const formattedJobs = jobs.map((opt) => ({
            type: opt.type,
            payload: opt.payload || {},
            priority: opt.priority || 'medium',
            max_retries: opt.maxRetries !== undefined ? opt.maxRetries : 3,
            timeout: opt.timeout !== undefined ? opt.timeout : 60,
            correlation_id: opt.correlationId,
            dedup_key: opt.dedupKey,
            run_at: opt.runAt instanceof Date ? opt.runAt.toISOString() : opt.runAt,
            labels: opt.labels,
        }));
        return this.request<BatchSubmitResponse>('POST', '/jobs/batch', { jobs: formattedJobs });
    }

    public getStatus(jobId: string): Promise<JobResponse> {
        return this.request<JobResponse>('GET', `/jobs/${encodeURIComponent(jobId)}`);
    }

    public listJobs(filter: ListJobsFilter = {}): Promise<ListJobsResponse> {
        return this.request<ListJobsResponse>('GET', '/jobs', undefined, {
            status: filter.status,
            type: filter.type,
            limit: filter.limit,
            page: filter.page,
            label_key: filter.labelKey,
            label_value: filter.labelValue,
        });
    }

    public cancel(jobId: string): Promise<ActionResponse> {
        return this.request<ActionResponse>('POST', `/jobs/${encodeURIComponent(jobId)}/cancel`);
    }

    public pause(jobId: string): Promise<ActionResponse> {
        return this.request<ActionResponse>('POST', `/jobs/${encodeURIComponent(jobId)}/pause`);
    }

    public resume(jobId: string): Promise<ActionResponse> {
        return this.request<ActionResponse>('POST', `/jobs/${encodeURIComponent(jobId)}/resume`);
    }
}
