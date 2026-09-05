import json
import logging
from typing import Callable, Dict, Any
from http.server import BaseHTTPRequestHandler, HTTPServer
import urllib.request
import urllib.error

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")

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

