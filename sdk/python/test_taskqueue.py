import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
import unittest
import taskqueue


class MockTaskQueueServer(BaseHTTPRequestHandler):
    jobs = {}
    requests_log = []

    def log_message(self, format, *args):
        pass  # Suppress logging in test output

    def do_POST(self):
        content_len = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_len).decode("utf-8") if content_len > 0 else ""
        data = json.loads(body) if body else {}

        api_key = self.headers.get("X-API-Key", "")
        self.requests_log.append({"method": "POST", "path": self.path, "data": data, "api_key": api_key})

        if self.path == "/jobs":
            job_id = "mock-job-123"
            resp = {
                "id": job_id,
                "type": data.get("type"),
                "status": "pending",
                "priority": data.get("priority", "medium"),
                "payload": data.get("payload", {}),
            }
            self.jobs[job_id] = resp
            self.send_response(201)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode("utf-8"))
        elif self.path == "/jobs/batch":
            jobs_list = data.get("jobs", [])
            ids = [f"batch-{i}" for i in range(len(jobs_list))]
            resp = {"job_ids": ids, "count": len(ids)}
            self.send_response(201)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode("utf-8"))
        elif "/cancel" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "cancelled"}).encode("utf-8"))
        elif "/pause" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "paused"}).encode("utf-8"))
        elif "/resume" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "resumed"}).encode("utf-8"))
        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        api_key = self.headers.get("X-API-Key", "")
        self.requests_log.append({"method": "GET", "path": self.path, "api_key": api_key})

        if self.path.startswith("/jobs/mock-job-123"):
            resp = {
                "id": "mock-job-123",
                "type": "email",
                "status": "completed",
                "result": {"sent": True},
            }
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode("utf-8"))
        elif self.path.startswith("/jobs"):
            resp = {
                "jobs": [{"id": "mock-job-123", "status": "completed"}],
                "total": 1,
                "page": 1,
                "limit": 20,
            }
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode("utf-8"))
        else:
            self.send_response(404)
            self.end_headers()


class TestTaskQueueClient(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 0), MockTaskQueueServer)
        cls.port = cls.server.server_port
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def setUp(self):
        MockTaskQueueServer.requests_log = []

    def test_three_line_submission(self):
        # Acceptance Criteria: Developers can import taskqueue and submit in 3 lines
        client = taskqueue.Client(self.base_url, api_key="secret-key")
        job = client.submit("email", {"to": "user@example.com"})

        self.assertEqual(job["id"], "mock-job-123")
        self.assertEqual(job["type"], "email")
        self.assertEqual(job["status"], "pending")
        self.assertEqual(MockTaskQueueServer.requests_log[-1]["api_key"], "secret-key")

    def test_submit_batch(self):
        client = taskqueue.Client(self.base_url, api_key="test-key")
        batch_resp = client.submit_batch([
            {"type": "email", "payload": {"to": "1@a.com"}},
            {"type": "email", "payload": {"to": "2@a.com"}},
        ])
        self.assertEqual(batch_resp["count"], 2)
        self.assertEqual(len(batch_resp["job_ids"]), 2)

    def test_get_status(self):
        client = taskqueue.Client(self.base_url, api_key="test-key")
        status = client.get_status("mock-job-123")
        self.assertEqual(status["id"], "mock-job-123")
        self.assertEqual(status["status"], "completed")

    def test_list_jobs(self):
        client = taskqueue.Client(self.base_url, api_key="test-key")
        res = client.list_jobs(status="completed", job_type="email", limit=10, page=1)
        self.assertEqual(res["total"], 1)
        self.assertEqual(len(res["jobs"]), 1)

    def test_lifecycle_methods(self):
        client = taskqueue.Client(self.base_url, api_key="test-key")
        pause_res = client.pause("mock-job-123")
        self.assertEqual(pause_res["status"], "paused")

        resume_res = client.resume("mock-job-123")
        self.assertEqual(resume_res["status"], "resumed")

        cancel_res = client.cancel("mock-job-123")
        self.assertEqual(cancel_res["status"], "cancelled")


if __name__ == "__main__":
    unittest.main()
