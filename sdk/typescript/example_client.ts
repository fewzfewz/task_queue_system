import { TaskQueueClient } from './TaskQueueClient';

async function main() {
    // 3-line submission in TypeScript:
    const client = new TaskQueueClient({ baseURL: 'http://localhost:8080', apiKey: 'secret-api-key' });
    const job = await client.submit({ type: 'email', payload: { to: 'user@example.com', subject: 'Welcome!' }, priority: 'high' });
    console.log(`Submitted job ID: ${job.id}, Status: ${job.status}`);

    // Query status:
    const status = await client.getStatus(job.id);
    console.log(`Current status: ${status.status}`);
}

if (require.main === module) {
    main().catch(console.error);
}
