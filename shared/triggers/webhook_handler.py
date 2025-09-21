#!/usr/bin/env python3
"""
Webhook Handler for GitOps Auto-Triggering

Provides a lightweight HTTP webhook server that can receive backup completion
events and trigger GitOps generation accordingly.
"""

import json
import logging
import argparse
from datetime import datetime
from typing import Dict, Any, Optional
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs
from threading import Thread
import signal
import sys

from gitops_trigger import GitOpsTriggerHandler, BackupCompletionEvent, GitOpsTriggerResult


class WebhookRequestHandler(BaseHTTPRequestHandler):
    """HTTP request handler for webhook endpoints."""
    
    def __init__(self, *args, trigger_handler: GitOpsTriggerHandler, **kwargs):
        self.trigger_handler = trigger_handler
        self.logger = logging.getLogger('webhook_handler')
        super().__init__(*args, **kwargs)
    
    def do_GET(self):
        """Handle GET requests (health check)."""
        if self.path == '/health':
            self._send_response(200, {'status': 'healthy', 'timestamp': datetime.now().isoformat()})
        elif self.path == '/':
            self._send_response(200, {
                'service': 'GitOps Auto-Trigger Webhook',
                'version': '1.0.0',
                'endpoints': {
                    'POST /webhook/backup-complete': 'Trigger GitOps generation on backup completion',
                    'GET /health': 'Health check endpoint',
                    'GET /': 'This information endpoint'
                }
            })
        else:
            self._send_error(404, 'Not Found')
    
    def do_POST(self):
        """Handle POST requests (webhook events)."""
        if self.path == '/webhook/backup-complete':
            self._handle_backup_complete_webhook()
        else:
            self._send_error(404, 'Not Found')
    
    def _handle_backup_complete_webhook(self):
        """Handle backup completion webhook events."""
        try:
            # Read and parse request body
            content_length = int(self.headers.get('Content-Length', 0))
            if content_length == 0:
                self._send_error(400, 'Empty request body')
                return
            
            body = self.rfile.read(content_length)
            
            try:
                data = json.loads(body.decode('utf-8'))
            except json.JSONDecodeError as e:
                self._send_error(400, f'Invalid JSON: {e}')
                return
            
            # Validate webhook payload
            if not self._validate_webhook_payload(data):
                self._send_error(400, 'Invalid webhook payload')
                return
            
            # Extract backup event from payload
            if 'backup' in data:
                event_data = data['backup']
            elif 'event_type' in data and data['event_type'] == 'backup_complete':
                event_data = data
            else:
                self._send_error(400, 'Missing backup event data')
                return
            
            # Create backup completion event
            try:
                event = BackupCompletionEvent.from_dict(event_data)
            except Exception as e:
                self._send_error(400, f'Invalid backup event data: {e}')
                return
            
            self.logger.info(f"Received backup completion webhook for {event.backup_id}")
            
            # Trigger GitOps generation
            result = self.trigger_handler.handle_backup_completion(event)
            
            # Send response
            response_data = {
                'status': 'success' if result.success else 'error',
                'backup_id': event.backup_id,
                'cluster': event.cluster_name,
                'trigger_result': {
                    'success': result.success,
                    'duration': result.duration,
                    'method': result.method,
                    'timestamp': result.timestamp.isoformat(),
                }
            }
            
            if result.success:
                response_data['trigger_result']['output'] = result.output
                if result.metadata:
                    response_data['trigger_result']['metadata'] = result.metadata
                self._send_response(200, response_data)
            else:
                response_data['trigger_result']['error'] = result.error
                self._send_response(500, response_data)
        
        except Exception as e:
            self.logger.error(f"Error handling webhook: {e}")
            self._send_error(500, f'Internal server error: {e}')
    
    def _validate_webhook_payload(self, data: Dict[str, Any]) -> bool:
        """Validate webhook payload structure."""
        # Check for required fields
        if 'event_type' in data:
            if data['event_type'] != 'backup_complete':
                return False
        
        # Check for backup event data
        backup_data = data.get('backup', data)
        required_fields = ['backup_id', 'cluster_name', 'timestamp', 'success']
        
        for field in required_fields:
            if field not in backup_data:
                self.logger.error(f"Missing required field: {field}")
                return False
        
        return True
    
    def _send_response(self, status_code: int, data: Dict[str, Any]):
        """Send JSON response."""
        response_body = json.dumps(data, indent=2).encode('utf-8')
        
        self.send_response(status_code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(response_body)))
        self.end_headers()
        self.wfile.write(response_body)
    
    def _send_error(self, status_code: int, message: str):
        """Send error response."""
        error_data = {
            'error': message,
            'status_code': status_code,
            'timestamp': datetime.now().isoformat()
        }
        self._send_response(status_code, error_data)
    
    def log_message(self, format, *args):
        """Override to use custom logger."""
        self.logger.info(f"{self.address_string()} - {format % args}")


class WebhookServer:
    """Webhook server for GitOps auto-triggering."""
    
    def __init__(self, host: str = '0.0.0.0', port: int = 8080, config_path: Optional[str] = None):
        self.host = host
        self.port = port
        self.logger = logging.getLogger('webhook_server')
        
        # Initialize trigger handler
        self.trigger_handler = GitOpsTriggerHandler(config_path=config_path)
        
        # Create HTTP server with custom handler
        def handler(*args, **kwargs):
            return WebhookRequestHandler(*args, trigger_handler=self.trigger_handler, **kwargs)
        
        self.server = HTTPServer((host, port), handler)
        self.running = False
    
    def start(self):
        """Start the webhook server."""
        self.running = True
        self.logger.info(f"Starting webhook server on {self.host}:{self.port}")
        
        # Set up signal handlers for graceful shutdown
        signal.signal(signal.SIGINT, self._signal_handler)
        signal.signal(signal.SIGTERM, self._signal_handler)
        
        try:
            self.server.serve_forever()
        except KeyboardInterrupt:
            self.logger.info("Webhook server interrupted by user")
        finally:
            self.stop()
    
    def stop(self):
        """Stop the webhook server."""
        if self.running:
            self.logger.info("Stopping webhook server")
            self.server.shutdown()
            self.server.server_close()
            self.running = False
    
    def _signal_handler(self, signum, frame):
        """Handle shutdown signals."""
        self.logger.info(f"Received signal {signum}, shutting down gracefully")
        self.stop()


def create_argument_parser() -> argparse.ArgumentParser:
    """Create argument parser for webhook server CLI."""
    parser = argparse.ArgumentParser(
        description="GitOps Auto-Trigger Webhook Server"
    )
    
    parser.add_argument(
        '--host',
        default='0.0.0.0',
        help='Host to bind webhook server (default: 0.0.0.0)'
    )
    
    parser.add_argument(
        '--port', '-p',
        type=int,
        default=8080,
        help='Port to bind webhook server (default: 8080)'
    )
    
    parser.add_argument(
        '--config', '-c',
        help='Path to shared configuration file'
    )
    
    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Enable verbose logging'
    )
    
    parser.add_argument(
        '--test-event',
        help='Send a test event to the webhook (provide event JSON file)'
    )
    
    return parser


def send_test_event(host: str, port: int, event_file: str):
    """Send a test event to the webhook server."""
    import urllib.request
    import urllib.error
    
    # Read test event
    with open(event_file, 'r') as f:
        event_data = json.load(f)
    
    # Prepare webhook payload
    payload = {
        'event_type': 'backup_complete',
        'timestamp': datetime.now().isoformat(),
        'backup': event_data,
        'trigger': 'test'
    }
    
    # Send POST request
    url = f"http://{host}:{port}/webhook/backup-complete"
    data = json.dumps(payload).encode('utf-8')
    
    req = urllib.request.Request(
        url,
        data=data,
        headers={'Content-Type': 'application/json'}
    )
    
    try:
        with urllib.request.urlopen(req) as response:
            response_data = json.loads(response.read().decode('utf-8'))
            print(f"✓ Test event sent successfully")
            print(f"  Status: {response_data.get('status')}")
            print(f"  Backup ID: {response_data.get('backup_id')}")
            if 'trigger_result' in response_data:
                result = response_data['trigger_result']
                print(f"  Trigger Success: {result.get('success')}")
                print(f"  Trigger Duration: {result.get('duration'):.2f}s")
    
    except urllib.error.HTTPError as e:
        print(f"✗ Test event failed with HTTP {e.code}")
        error_data = json.loads(e.read().decode('utf-8'))
        print(f"  Error: {error_data.get('error')}")
    
    except Exception as e:
        print(f"✗ Test event failed: {e}")


def main():
    """Main entry point for webhook server CLI."""
    parser = create_argument_parser()
    args = parser.parse_args()
    
    # Set up logging
    log_level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    if args.test_event:
        # Send test event
        send_test_event(args.host, args.port, args.test_event)
        return
    
    try:
        # Start webhook server
        server = WebhookServer(
            host=args.host,
            port=args.port,
            config_path=args.config
        )
        
        print(f"GitOps Auto-Trigger Webhook Server")
        print(f"Listening on http://{args.host}:{args.port}")
        print(f"Endpoints:")
        print(f"  POST /webhook/backup-complete - Trigger GitOps generation")
        print(f"  GET  /health                 - Health check")
        print(f"  GET  /                       - Service information")
        print(f"Press Ctrl+C to stop")
        
        server.start()
    
    except Exception as e:
        logging.error(f"Webhook server failed: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()