import * as http from 'http';
import test from 'node:test';
import assert from 'node:assert';
import { TaskQueueClient } from './TaskQueueClient.ts';

test('TaskQueueClient test suite', async (t) => {
    let requestsLog: any[] = [];
    const server = http.createServer((req, res) => {
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
            const data = body ? JSON.parse(body) : {};
            const apiKey = req.headers['x-api-key'] || '';
            requestsLog.push({ method: req.method, path: req.url, data, apiKey });

            if (req.method === 'POST' && req.url === '/jobs') {
                res.writeHead(201, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({
                    id: 'ts-job-123',
                    type: data.type,
                    status: 'pending',
                    priority: data.priority,
                    payload: data.payload,
                }));
            } else if (req.method === 'POST' && req.url === '/jobs/batch') {
                res.writeHead(201, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({
                    job_ids: ['ts-batch-1', 'ts-batch-2'],
                    count: 2,
                }));
            } else if (req.method === 'GET' && req.url === '/jobs/ts-job-123') {
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({
                    id: 'ts-job-123',
                    type: 'email',
                    status: 'completed',
                    result: { sent: true },
                }));
            } else if (req.method === 'GET' && req.url?.startsWith('/jobs')) {
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({
                    jobs: [{ id: 'ts-job-123', status: 'completed' }],
                    total: 1,
                    page: 1,
                    limit: 20,
                }));
            } else if (req.method === 'POST' && req.url?.includes('/pause')) {
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ status: 'paused' }));
            } else if (req.method === 'POST' && req.url?.includes('/resume')) {
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ status: 'resumed' }));
            } else if (req.method === 'POST' && req.url?.includes('/cancel')) {
                res.writeHead(200, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ status: 'cancelled' }));
            } else {
                res.writeHead(404);
                res.end();
            }
        });
    });

    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', () => resolve()));
    const addr = server.address() as any;
    const baseURL = `http://127.0.0.1:${addr.port}`;

    t.after(() => {
        server.close();
    });

    await t.test('3-line job submission in TypeScript', async () => {
        const client = new TaskQueueClient({ baseURL, apiKey: 'ts-api-key' });
        const job = await client.submit({ type: 'email', payload: { to: 'bob@example.com' }, priority: 'high' });

        assert.strictEqual(job.id, 'ts-job-123');
        assert.strictEqual(job.type, 'email');
        assert.strictEqual(job.status, 'pending');
        assert.strictEqual(requestsLog[requestsLog.length - 1].apiKey, 'ts-api-key');
    });

    await t.test('submitBatch submits array of jobs', async () => {
        const client = new TaskQueueClient({ baseURL, apiKey: 'ts-api-key' });
        const batch = await client.submitBatch([
            { type: 'email', payload: { to: '1@a.com' } },
            { type: 'email', payload: { to: '2@a.com' } },
        ]);
        assert.strictEqual(batch.count, 2);
        assert.strictEqual(batch.job_ids.length, 2);
    });

    await t.test('getStatus queries single job by ID', async () => {
        const client = new TaskQueueClient({ baseURL, apiKey: 'ts-api-key' });
        const status = await client.getStatus('ts-job-123');
        assert.strictEqual(status.id, 'ts-job-123');
        assert.strictEqual(status.status, 'completed');
    });

    await t.test('listJobs searches jobs with filter', async () => {
        const client = new TaskQueueClient({ baseURL, apiKey: 'ts-api-key' });
        const res = await client.listJobs({ status: 'completed', type: 'email', limit: 10 });
        assert.strictEqual(res.total, 1);
        assert.strictEqual(res.jobs.length, 1);
    });

    await t.test('lifecycle operations (pause, resume, cancel)', async () => {
        const client = new TaskQueueClient({ baseURL, apiKey: 'ts-api-key' });
        const pauseRes = await client.pause('ts-job-123');
        assert.strictEqual(pauseRes.status, 'paused');

        const resumeRes = await client.resume('ts-job-123');
        assert.strictEqual(resumeRes.status, 'resumed');

        const cancelRes = await client.cancel('ts-job-123');
        assert.strictEqual(cancelRes.status, 'cancelled');
    });
});
