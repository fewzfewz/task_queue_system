import json
import logging
from typing import Callable, Dict, Any, List, Optional
from http.server import BaseHTTPRequestHandler, HTTPServer
import urllib.request
import urllib.parse
import urllib.error

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")


class TaskQueueError(Exception):
    """Raised when the Task Queue API returns an error response."""
    def __init__(self, status_code: int, message: str):
        super().__init__(f"Task Queue API error ({status_code}): {message}")
        self.status_code = status_code
        self.message = message


class TaskQueueClient:
    """
    Python client for interacting with the Go Task Queue engine.
    Allows submitting, inspecting, querying, and managing asynchronous tasks.
    """
    def __init__(self, base_url: str = "http://localhost:8080", api_key: str = "", timeout: float = 15.0):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout

    def _request(self, method: str, path: str, data: Optional[Dict[str, Any]] = None, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        url = f"{self.base_url}{path}"
        if params:
            filtered_params = {k: v for k, v in params.items() if v is not None and v != ""}
            if filtered_params:
                url += "?" + urllib.parse.urlencode(filtered_params)

        body_bytes = None
        headers = {}
        if self.api_key:
            headers["X-API-Key"] = self.api_key

        if data is not None:
            body_bytes = json.dumps(data).encode("utf-8")
            headers["Content-Type"] = "application/json"

        req = urllib.request.Request(url, data=body_bytes, headers=headers, method=method)

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                resp_body = resp.read().decode("utf-8")
                if resp_body:
                    try:
                        return json.loads(resp_body)
                    except json.JSONDecodeError:
                        return {"raw": resp_body}
                return {"status": "ok"}
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8")
            raise TaskQueueError(e.code, error_body)
        except urllib.error.URLError as e:
            raise TaskQueueError(0, f"Network error: {str(e.reason)}")

    def submit(
        self,
        job_type: str,
        payload: Optional[Dict[str, Any]] = None,
        priority: str = "medium",
        max_retries: int = 3,
        timeout_seconds: int = 60,
        correlation_id: str = "",
        dedup_key: str = "",
        run_at: Optional[str] = None,
        labels: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """
        Submit a single job to the queue.
        Example:
            client.submit("email", {"to": "alice@example.com"}, priority="high")
        """
        body: Dict[str, Any] = {
            "type": job_type,
            "payload": payload or {},
            "priority": priority,
            "max_retries": max_retries,
            "timeout": timeout_seconds,
        }
        if correlation_id:
            body["correlation_id"] = correlation_id
        if dedup_key:
            body["dedup_key"] = dedup_key
        if run_at:
            body["run_at"] = run_at
        if labels:
            body["labels"] = labels

        return self._request("POST", "/jobs", data=body)

    def submit_batch(self, jobs: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Submit a batch of jobs in a single HTTP request.
        """
        return self._request("POST", "/jobs/batch", data={"jobs": jobs})

    def get_status(self, job_id: str) -> Dict[str, Any]:
        """
        Retrieve current status, progress, and result of a job by ID.
        """
        return self._request("GET", f"/jobs/{urllib.parse.quote(job_id)}")

    def list_jobs(
        self,
        status: str = "",
        job_type: str = "",
        limit: int = 20,
        page: int = 1,
        label_key: str = "",
        label_value: str = "",
    ) -> Dict[str, Any]:
        """
        Search and filter jobs with pagination.
        """
        params = {
            "status": status,
            "type": job_type,
            "limit": limit if limit > 0 else None,
            "page": page if page > 0 else None,
            "label_key": label_key,
            "label_value": label_value,
        }
        return self._request("GET", "/jobs", params=params)

    def cancel(self, job_id: str) -> Dict[str, Any]:
        """
        Cancel a pending or active job.
        """
        return self._request("POST", f"/jobs/{urllib.parse.quote(job_id)}/cancel")

    def pause(self, job_id: str) -> Dict[str, Any]:
        """
        Pause execution of a queued job.
        """
        return self._request("POST", f"/jobs/{urllib.parse.quote(job_id)}/pause")

    def resume(self, job_id: str) -> Dict[str, Any]:
        """
        Resume execution of a paused job.
        """
        return self._request("POST", f"/jobs/{urllib.parse.quote(job_id)}/resume")


# Convenient alias for 3-line import usage
Client = TaskQueueClient


class TaskQueueWorker:
    """
    A Python worker SDK that receives payloads from the Go Task Queue engine,
    executes them locally in Python, and returns the results.
    """
    def __init__(self, port: int = 5000):
        self.port = port
        self.handlers: Dict[str, Callable[[Dict[str, Any]], Any]] = {}

    def register_handler(self, job_type: str, handler: Callable[[Dict[str, Any]], Any]):
        """Register a Python function to handle a specific job type."""
        self.handlers[job_type] = handler
        logging.info(f"Registered handler for job type: {job_type}")

    def register_with_server(self, server_url: str, job_type: str, my_url: str, api_key: str):
        """
        Informs the Go Task Queue that jobs of `job_type` should be sent to this Python worker.
        Note: Currently implemented via standard HTTP plugin submission by the user.
        """
        logging.info(f"Worker ready to process '{job_type}' via HTTP pushes at {my_url}")

    def start(self):
        """Start the HTTP server to listen for incoming jobs from the Go engine."""
        handlers = self.handlers

        class RequestHandler(BaseHTTPRequestHandler):
            def do_POST(self):
                content_length = int(self.headers.get('Content-Length', 0))
                job_id = self.headers.get('X-Task-Queue-Job-ID', 'unknown')
                
                body = self.rfile.read(content_length)
                try:
                    payload = json.loads(body.decode('utf-8'))
                except json.JSONDecodeError:
                    self.send_error(400, "Invalid JSON payload")
                    return

                # In this simple model, the URL path determines the job type
                job_type = self.path.strip("/")
                handler = handlers.get(job_type)

                if not handler:
                    self.send_response(404)
                    self.end_headers()
                    self.wfile.write(b"No handler registered for this job type")
                    return

                logging.info(f"Processing job {job_id} of type {job_type}")
                try:
                    # Execute the registered Python function
                    result = handler(payload)
                    
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"result": result, "status": "success"}).encode('utf-8'))
                    logging.info(f"Successfully processed job {job_id}")
                except Exception as e:
                    logging.error(f"Error processing job {job_id}: {str(e)}")
                    self.send_response(500)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"error": str(e)}).encode('utf-8'))

        server = HTTPServer(('', self.port), RequestHandler)
        logging.info(f"Python Task Queue Worker listening on port {self.port}...")
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            logging.info("Shutting down worker...")
            server.server_close()
