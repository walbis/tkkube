#!/usr/bin/env python3
"""
Example Integration Script for GitOps Auto-Triggering

This script demonstrates how to integrate the auto-trigger functionality
into existing backup and GitOps workflows.
"""

import os
import json
import time
import logging
from pathlib import Path
from datetime import datetime
from typing import Dict, Any
import argparse

# Import auto-trigger components
from gitops_trigger import GitOpsTriggerHandler, BackupCompletionEvent
from webhook_handler import WebhookServer


def simulate_backup_completion() -> BackupCompletionEvent:
    """Simulate a backup completion event."""
    return BackupCompletionEvent(
        backup_id=f"demo-backup-{int(time.time())}",
        cluster_name="demo-cluster",
        timestamp=datetime.now(),
        duration=120.5,  # 2 minutes
        namespaces_count=8,
        resources_count=150,
        success=True,
        errors=[],
        minio_bucket="demo-cluster-backups",
        backup_location="demo-cluster-backups/demo-cluster",
        metadata={
            "backup_tool_version": "1.0.0",
            "cluster_domain": "cluster.local",
            "storage_endpoint": "localhost:9000",
        }
    )


def demo_file_based_trigger(config_path: str = None):
    """Demonstrate file-based triggering."""
    print("🔄 Demo: File-Based Auto-Triggering")
    print("=" * 50)
    
    # Initialize trigger handler
    handler = GitOpsTriggerHandler(config_path=config_path)
    
    # Simulate backup completion
    event = simulate_backup_completion()
    print(f"📦 Simulated backup completion: {event.backup_id}")
    print(f"   Cluster: {event.cluster_name}")
    print(f"   Resources: {event.resources_count}")
    print(f"   Success: {event.success}")
    
    # Create trigger file
    trigger_dir = Path("/tmp/backup-gitops-triggers")
    trigger_dir.mkdir(exist_ok=True)
    
    trigger_file = trigger_dir / f"backup-complete-{event.cluster_name}-{int(event.timestamp.timestamp())}.json"
    
    print(f"📝 Creating trigger file: {trigger_file}")
    with open(trigger_file, 'w') as f:
        json.dump(event.to_dict(), f, indent=2)
    
    print("✅ Trigger file created successfully")
    print(f"📍 File location: {trigger_file}")
    
    # Simulate monitoring (in real scenario, this would be a separate process)
    print("\n🔍 Simulating trigger file monitoring...")
    
    if trigger_file.exists():
        print("📖 Reading trigger file...")
        with open(trigger_file, 'r') as f:
            event_data = json.load(f)
        
        loaded_event = BackupCompletionEvent.from_dict(event_data)
        
        print("🚀 Triggering GitOps generation...")
        result = handler.handle_backup_completion(loaded_event)
        
        print(f"📊 Trigger Result:")
        print(f"   Success: {result.success}")
        print(f"   Method: {result.method}")
        print(f"   Duration: {result.duration:.2f}s")
        
        if result.success:
            print(f"   Output: {result.output[:200]}...")
        else:
            print(f"   Error: {result.error}")
        
        # Clean up
        trigger_file.unlink()
        print("🧹 Cleaned up trigger file")
    
    print("\n✅ File-based trigger demo completed")


def demo_webhook_trigger(config_path: str = None, port: int = 8080):
    """Demonstrate webhook-based triggering."""
    print("🌐 Demo: Webhook-Based Auto-Triggering")
    print("=" * 50)
    
    print(f"🚀 Starting webhook server on port {port}...")
    
    # Start webhook server in a separate thread
    import threading
    
    webhook_server = WebhookServer(host='localhost', port=port, config_path=config_path)
    server_thread = threading.Thread(target=webhook_server.start, daemon=True)
    server_thread.start()
    
    # Wait for server to start
    time.sleep(2)
    
    print(f"✅ Webhook server started at http://localhost:{port}")
    print(f"📡 Webhook endpoint: http://localhost:{port}/webhook/backup-complete")
    
    # Simulate sending webhook
    event = simulate_backup_completion()
    
    webhook_payload = {
        'event_type': 'backup_complete',
        'timestamp': datetime.now().isoformat(),
        'backup': event.to_dict(),
        'trigger': 'demo'
    }
    
    print(f"\n📦 Simulating webhook for backup: {event.backup_id}")
    
    # Send webhook request
    import urllib.request
    import urllib.error
    
    try:
        url = f"http://localhost:{port}/webhook/backup-complete"
        data = json.dumps(webhook_payload).encode('utf-8')
        
        req = urllib.request.Request(
            url,
            data=data,
            headers={'Content-Type': 'application/json'}
        )
        
        print(f"📤 Sending webhook to {url}...")
        
        with urllib.request.urlopen(req, timeout=10) as response:
            response_data = json.loads(response.read().decode('utf-8'))
            
            print("📨 Webhook Response:")
            print(f"   Status: {response_data.get('status')}")
            print(f"   Backup ID: {response_data.get('backup_id')}")
            
            if 'trigger_result' in response_data:
                result = response_data['trigger_result']
                print(f"   Trigger Success: {result.get('success')}")
                print(f"   Trigger Duration: {result.get('duration'):.2f}s")
                print(f"   Trigger Method: {result.get('method')}")
    
    except urllib.error.HTTPError as e:
        print(f"❌ Webhook failed with HTTP {e.code}")
        try:
            error_data = json.loads(e.read().decode('utf-8'))
            print(f"   Error: {error_data.get('error')}")
        except:
            print(f"   Raw error: {e}")
    
    except Exception as e:
        print(f"❌ Webhook failed: {e}")
    
    finally:
        print("\n🛑 Stopping webhook server...")
        webhook_server.stop()
        time.sleep(1)
    
    print("✅ Webhook-based trigger demo completed")


def demo_monitoring_setup(config_path: str = None):
    """Demonstrate setting up continuous monitoring."""
    print("👁️  Demo: Continuous Monitoring Setup")
    print("=" * 50)
    
    # Initialize trigger handler
    handler = GitOpsTriggerHandler(config_path=config_path)
    
    print("🔧 Setting up trigger file monitoring...")
    
    trigger_dir = "/tmp/backup-gitops-triggers"
    print(f"📁 Monitoring directory: {trigger_dir}")
    
    # Create test trigger files
    print("\n📝 Creating sample trigger files...")
    
    for i in range(3):
        event = simulate_backup_completion()
        event.backup_id = f"monitoring-demo-{i+1}-{int(time.time())}"
        
        trigger_file = Path(trigger_dir) / f"backup-complete-{event.cluster_name}-{int(event.timestamp.timestamp())}-{i}.json"
        
        with open(trigger_file, 'w') as f:
            json.dump(event.to_dict(), f, indent=2)
        
        print(f"   Created: {trigger_file.name}")
        time.sleep(0.1)  # Small delay between files
    
    print(f"\n🔍 Starting monitoring (will process {len(list(Path(trigger_dir).glob('*.json')))} files)...")
    
    # Simulate monitoring for a short time
    processed_count = 0
    start_time = time.time()
    
    while time.time() - start_time < 10:  # Monitor for 10 seconds
        trigger_files = list(Path(trigger_dir).glob("backup-complete-*.json"))
        
        for trigger_file in trigger_files:
            try:
                print(f"📖 Processing: {trigger_file.name}")
                
                with open(trigger_file, 'r') as f:
                    event_data = json.load(f)
                
                event = BackupCompletionEvent.from_dict(event_data)
                result = handler.handle_backup_completion(event)
                
                print(f"   Result: {'✅ Success' if result.success else '❌ Failed'}")
                if not result.success:
                    print(f"   Error: {result.error}")
                
                # Remove processed file
                trigger_file.unlink()
                processed_count += 1
                
            except Exception as e:
                print(f"   ❌ Error processing {trigger_file.name}: {e}")
        
        if not trigger_files:
            break
        
        time.sleep(1)
    
    print(f"\n📊 Monitoring completed:")
    print(f"   Processed files: {processed_count}")
    print(f"   Duration: {time.time() - start_time:.1f}s")
    
    print("✅ Monitoring setup demo completed")


def demo_integration_patterns():
    """Demonstrate integration patterns for different scenarios."""
    print("🔗 Demo: Integration Patterns")
    print("=" * 50)
    
    patterns = [
        {
            "name": "Synchronous Integration",
            "description": "Backup tool directly calls GitOps trigger after completion",
            "code": """
# In backup tool completion handler:
from triggers.backup_integration import BackupTriggerIntegration

integration = BackupTriggerIntegration(config, logger)
await integration.OnBackupComplete(ctx, backup_result)
"""
        },
        {
            "name": "Asynchronous File-Based",
            "description": "Backup tool drops signal file, GitOps monitor picks it up",
            "code": """
# Backup tool creates signal file:
signal_file = f"/tmp/triggers/backup-complete-{cluster}-{timestamp}.json"
with open(signal_file, 'w') as f:
    json.dump(backup_event.to_dict(), f)

# GitOps monitor processes files:
handler.monitor_trigger_files(trigger_dir="/tmp/triggers")
"""
        },
        {
            "name": "Webhook Integration",
            "description": "Backup tool sends HTTP webhook to GitOps service",
            "code": """
# Start webhook server:
webhook_server = WebhookServer(host='0.0.0.0', port=8080)
webhook_server.start()

# Backup tool sends webhook:
webhook_payload = {
    'event_type': 'backup_complete',
    'backup': backup_event.to_dict()
}
requests.post('http://gitops-server:8080/webhook/backup-complete', 
              json=webhook_payload)
"""
        },
        {
            "name": "Message Queue Integration",
            "description": "Using message queues for enterprise-grade reliability",
            "code": """
# Producer (backup tool):
import pika
connection = pika.BlockingConnection(pika.ConnectionParameters('localhost'))
channel = connection.channel()
channel.queue_declare(queue='backup_events')
channel.basic_publish(exchange='', routing_key='backup_events', 
                      body=json.dumps(backup_event.to_dict()))

# Consumer (GitOps service):
def process_backup_event(ch, method, properties, body):
    event = BackupCompletionEvent.from_dict(json.loads(body))
    handler.handle_backup_completion(event)
    
channel.basic_consume(queue='backup_events', on_message_callback=process_backup_event)
"""
        }
    ]
    
    for i, pattern in enumerate(patterns, 1):
        print(f"\n{i}. {pattern['name']}")
        print(f"   {pattern['description']}")
        print(f"   Example:")
        for line in pattern['code'].strip().split('\n'):
            print(f"     {line}")
    
    print("\n💡 Recommendation:")
    print("   - Use synchronous integration for simple setups")
    print("   - Use file-based for loose coupling and reliability")
    print("   - Use webhooks for distributed systems")
    print("   - Use message queues for enterprise production environments")
    
    print("\n✅ Integration patterns demo completed")


def main():
    """Main entry point for example integration."""
    parser = argparse.ArgumentParser(
        description="GitOps Auto-Trigger Integration Examples"
    )
    
    parser.add_argument(
        '--config', '-c',
        help='Path to shared configuration file'
    )
    
    parser.add_argument(
        '--demo', '-d',
        choices=['file', 'webhook', 'monitoring', 'patterns', 'all'],
        default='all',
        help='Which demo to run'
    )
    
    parser.add_argument(
        '--webhook-port',
        type=int,
        default=8080,
        help='Port for webhook demo'
    )
    
    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Enable verbose logging'
    )
    
    args = parser.parse_args()
    
    # Set up logging
    log_level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    print("🚀 GitOps Auto-Trigger Integration Examples")
    print(f"⏰ Started at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    if args.config:
        print(f"📋 Using config: {args.config}")
    
    print()
    
    try:
        if args.demo == 'file' or args.demo == 'all':
            demo_file_based_trigger(args.config)
            if args.demo == 'all':
                print("\n" + "=" * 60 + "\n")
        
        if args.demo == 'webhook' or args.demo == 'all':
            demo_webhook_trigger(args.config, args.webhook_port)
            if args.demo == 'all':
                print("\n" + "=" * 60 + "\n")
        
        if args.demo == 'monitoring' or args.demo == 'all':
            demo_monitoring_setup(args.config)
            if args.demo == 'all':
                print("\n" + "=" * 60 + "\n")
        
        if args.demo == 'patterns' or args.demo == 'all':
            demo_integration_patterns()
        
        print(f"\n🎉 All demos completed successfully!")
        print(f"⏰ Finished at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    except KeyboardInterrupt:
        print("\n⏹️  Demos interrupted by user")
    except Exception as e:
        print(f"\n❌ Demo failed: {e}")
        import traceback
        if args.verbose:
            traceback.print_exc()


if __name__ == '__main__':
    main()