from http.server import BaseHTTPRequestHandler, HTTPServer
import json

class WebhookHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        post_data = self.rfile.read(content_length)
        
        print("\n--- WEBHOOK RECEIVED ---")
        print("Headers:")
        for k, v in self.headers.items():
            print(f"  {k}: {v}")
            
        print("\nPayload:")
        try:
            print(json.dumps(json.loads(post_data), indent=2))
        except:
            print(post_data.decode('utf-8'))
        print("------------------------\n")
        
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')

def run(server_class=HTTPServer, handler_class=WebhookHandler, port=9090):
    server_address = ('', port)
    httpd = server_class(server_address, handler_class)
    print(f"Starting simple webhook receiver on port {port}...")
    httpd.serve_forever()

if __name__ == '__main__':
    run()
