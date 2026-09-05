import { TaskQueueWorker } from './TaskQueueWorker';

const worker = new TaskQueueWorker(5060);

worker.registerHandler('generate_report', async (payload: any) => {
    console.log("Generating report for:", payload.companyId);
    
    // Simulate async DB queries, PDF generation, etc.
    return new Promise((resolve) => {
        setTimeout(() => {
            resolve({
                reportUrl: "https://reports.example.com/123.pdf",
                pages: 42
            });
        }, 1000);
    });
});

worker.start();
