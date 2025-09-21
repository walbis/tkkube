#!/usr/bin/env python3
"""
GitOps Auto-Trigger Handler

Handles automatic GitOps generation when triggered by backup completion events.
Supports multiple trigger mechanisms including file-based, webhook, and direct invocation.
"""

import os
import sys
import json
import time
import logging
import argparse
import subprocess
from pathlib import Path
from typing import Dict, Any, Optional, List
from dataclasses import dataclass, asdict
from datetime import datetime, timedelta

# Add shared config to path
current_dir = Path(__file__).parent
shared_dir = current_dir.parent
config_dir = shared_dir / 'config'
sys.path.insert(0, str(config_dir))

from loader import ConfigLoader, SharedConfig


@dataclass
class BackupCompletionEvent:
    """Represents a backup completion event."""
    backup_id: str
    cluster_name: str
    timestamp: datetime
    duration: float  # seconds
    namespaces_count: int
    resources_count: int
    success: bool
    errors: List[str]
    minio_bucket: str
    backup_size: Optional[int] = None
    backup_location: str = ""
    metadata: Optional[Dict[str, str]] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> 'BackupCompletionEvent':
        """Create BackupCompletionEvent from dictionary."""
        # Handle timestamp conversion
        if isinstance(data.get('timestamp'), str):
            data['timestamp'] = datetime.fromisoformat(data['timestamp'].replace('Z', '+00:00'))
        elif isinstance(data.get('timestamp'), (int, float)):
            data['timestamp'] = datetime.fromtimestamp(data['timestamp'])
        
        return cls(**data)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        data = asdict(self)
        data['timestamp'] = self.timestamp.isoformat()
        return data


@dataclass
class GitOpsTriggerResult:
    """Represents the result of a GitOps trigger operation."""
    success: bool
    timestamp: datetime
    duration: float
    method: str
    output: str = ""
    error: str = ""
    metadata: Optional[Dict[str, str]] = None


class GitOpsTriggerHandler:
    """Handles automatic GitOps generation triggered by backup completion."""
    
    def __init__(self, config_path: Optional[str] = None, logger: Optional[logging.Logger] = None):
        """Initialize the trigger handler."""
        self.logger = logger or self._setup_logging()
        
        # Load shared configuration
        config_paths = [config_path] if config_path else None
        self.config_loader = ConfigLoader(config_paths)
        self.config = self.config_loader.load()
        
        self.logger.info(f"GitOps trigger handler initialized for cluster: {self.config.cluster.name}")
    
    def _setup_logging(self) -> logging.Logger:
        """Set up logging for the trigger handler."""
        logger = logging.getLogger('gitops_trigger')
        logger.setLevel(logging.INFO)
        
        if not logger.handlers:
            handler = logging.StreamHandler()
            formatter = logging.Formatter(
                '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
            )
            handler.setFormatter(formatter)
            logger.addHandler(handler)
        
        return logger
    
    def handle_backup_completion(self, event: BackupCompletionEvent) -> GitOpsTriggerResult:
        """Handle a backup completion event by triggering GitOps generation."""
        start_time = datetime.now()
        
        self.logger.info(
            f"Processing backup completion event: {event.backup_id} "
            f"for cluster {event.cluster_name}"
        )
        
        # Check if auto-triggering is enabled
        if not self.config.pipeline.automation.enabled:
            self.logger.info("Auto-triggering is disabled")
            return GitOpsTriggerResult(
                success=True,
                timestamp=start_time,
                duration=0.0,
                method="disabled",
                output="Auto-triggering is disabled in configuration"
            )
        
        try:
            # Trigger GitOps generation
            result = self._trigger_gitops_generation(event)
            
            duration = (datetime.now() - start_time).total_seconds()
            
            self.logger.info(
                f"GitOps generation triggered successfully for backup {event.backup_id} "
                f"in {duration:.2f} seconds"
            )
            
            return GitOpsTriggerResult(
                success=True,
                timestamp=start_time,
                duration=duration,
                method="direct_invocation",
                output=result,
                metadata={
                    "backup_id": event.backup_id,
                    "cluster": event.cluster_name,
                    "resources_processed": str(event.resources_count),
                }
            )
            
        except Exception as e:
            duration = (datetime.now() - start_time).total_seconds()
            error_msg = str(e)
            
            self.logger.error(f"Failed to trigger GitOps generation: {error_msg}")
            
            return GitOpsTriggerResult(
                success=False,
                timestamp=start_time,
                duration=duration,
                method="direct_invocation",
                error=error_msg
            )
    
    def _trigger_gitops_generation(self, event: BackupCompletionEvent) -> str:
        """Trigger the actual GitOps generation process."""
        self.logger.info(f"Starting GitOps generation for backup {event.backup_id}")
        
        # Set environment variables for the GitOps process
        env = os.environ.copy()
        env.update({
            'MINIO_ENDPOINT': self.config.storage.endpoint,
            'MINIO_ACCESS_KEY': self.config.storage.access_key,
            'MINIO_SECRET_KEY': self.config.storage.secret_key,
            'MINIO_BUCKET': self.config.storage.bucket,
            'CLUSTER_NAME': self.config.cluster.name,
            'GIT_REPOSITORY': self.config.gitops.repository.url,
            'GIT_BRANCH': self.config.gitops.repository.branch,
            'BACKUP_TRIGGER_ID': event.backup_id,
            'BACKUP_TIMESTAMP': str(int(event.timestamp.timestamp())),
            'BACKUP_SUCCESS': str(event.success).lower(),
        })
        
        # Find GitOps generator
        gitops_binary = self._find_gitops_binary()
        if not gitops_binary:
            raise RuntimeError("GitOps binary not found")
        
        # Prepare command arguments
        cmd = [gitops_binary]
        
        # Add configuration file if available
        config_path = self._find_config_file()
        if config_path:
            cmd.extend(['--config', config_path])
        
        # Add verbose flag if configured
        if self.config.observability.logging.level.lower() in ['debug', 'trace']:
            cmd.append('--verbose')
        
        self.logger.debug(f"Executing command: {' '.join(cmd)}")
        
        # Execute GitOps generation
        try:
            result = subprocess.run(
                cmd,
                env=env,
                capture_output=True,
                text=True,
                timeout=self.config.pipeline.automation.max_wait_time,
                check=True
            )
            
            output = result.stdout
            if result.stderr:
                self.logger.warning(f"GitOps generation stderr: {result.stderr}")
                output += f"\nStderr: {result.stderr}"
            
            return output
            
        except subprocess.TimeoutExpired:
            raise RuntimeError(f"GitOps generation timed out after {self.config.pipeline.automation.max_wait_time} seconds")
        except subprocess.CalledProcessError as e:
            raise RuntimeError(f"GitOps generation failed with exit code {e.returncode}: {e.stderr}")
    
    def _find_gitops_binary(self) -> Optional[str]:
        """Find the GitOps generation binary."""
        candidates = [
            'minio-to-git',
            './minio-to-git',
            '../kOTN/minio-to-git',
            str(Path(__file__).parent.parent.parent / 'kOTN' / 'minio-to-git'),
            '/usr/local/bin/minio-to-git',
            '/usr/bin/minio-to-git',
        ]
        
        for candidate in candidates:
            if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
                self.logger.debug(f"Found GitOps binary: {candidate}")
                return candidate
            
            # Also check PATH
            try:
                result = subprocess.run(['which', candidate], capture_output=True, text=True)
                if result.returncode == 0 and result.stdout.strip():
                    binary_path = result.stdout.strip()
                    self.logger.debug(f"Found GitOps binary in PATH: {binary_path}")
                    return binary_path
            except:
                continue
        
        self.logger.error("GitOps binary not found in any of the expected locations")
        return None
    
    def _find_config_file(self) -> Optional[str]:
        """Find the shared configuration file."""
        candidates = [
            'shared-config.yaml',
            './config/shared-config.yaml',
            '../shared/config/shared-config.yaml',
            str(Path(__file__).parent.parent / 'config' / 'shared-config.yaml'),
            '/etc/backup-gitops/config.yaml',
        ]
        
        for candidate in candidates:
            if os.path.isfile(candidate):
                self.logger.debug(f"Found config file: {candidate}")
                return candidate
        
        return None
    
    def monitor_trigger_files(self, trigger_dir: str = "/tmp/backup-gitops-triggers", poll_interval: int = 5):
        """Monitor for backup completion trigger files."""
        self.logger.info(f"Starting trigger file monitor in directory: {trigger_dir}")
        
        # Create trigger directory if it doesn't exist
        Path(trigger_dir).mkdir(parents=True, exist_ok=True)
        
        processed_files = set()
        
        while True:
            try:
                # Scan for new trigger files
                trigger_files = list(Path(trigger_dir).glob("backup-complete-*.json"))
                
                for trigger_file in trigger_files:
                    if trigger_file.name in processed_files:
                        continue
                    
                    try:
                        # Process the trigger file
                        self.logger.info(f"Processing trigger file: {trigger_file}")
                        
                        with open(trigger_file, 'r') as f:
                            event_data = json.load(f)
                        
                        event = BackupCompletionEvent.from_dict(event_data)
                        result = self.handle_backup_completion(event)
                        
                        if result.success:
                            self.logger.info(f"Successfully processed trigger file: {trigger_file}")
                            # Mark as processed
                            processed_files.add(trigger_file.name)
                            # Optionally remove the trigger file
                            trigger_file.unlink()
                        else:
                            self.logger.error(f"Failed to process trigger file: {trigger_file}, error: {result.error}")
                    
                    except Exception as e:
                        self.logger.error(f"Error processing trigger file {trigger_file}: {e}")
                
                # Wait before next scan
                time.sleep(poll_interval)
                
            except KeyboardInterrupt:
                self.logger.info("Trigger file monitor stopped by user")
                break
            except Exception as e:
                self.logger.error(f"Error in trigger file monitor: {e}")
                time.sleep(poll_interval)


def create_argument_parser() -> argparse.ArgumentParser:
    """Create argument parser for CLI usage."""
    parser = argparse.ArgumentParser(
        description="GitOps Auto-Trigger Handler"
    )
    
    parser.add_argument(
        '--config', '-c',
        help='Path to shared configuration file'
    )
    
    parser.add_argument(
        '--event-file',
        help='Path to backup completion event JSON file'
    )
    
    parser.add_argument(
        '--monitor', '-m',
        action='store_true',
        help='Monitor for trigger files'
    )
    
    parser.add_argument(
        '--trigger-dir',
        default='/tmp/backup-gitops-triggers',
        help='Directory to monitor for trigger files'
    )
    
    parser.add_argument(
        '--poll-interval',
        type=int,
        default=5,
        help='Polling interval in seconds for file monitoring'
    )
    
    parser.add_argument(
        '--verbose', '-v',
        action='store_true',
        help='Enable verbose logging'
    )
    
    return parser


def main():
    """Main entry point for CLI usage."""
    parser = create_argument_parser()
    args = parser.parse_args()
    
    # Set up logging
    log_level = logging.DEBUG if args.verbose else logging.INFO
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    try:
        # Initialize trigger handler
        handler = GitOpsTriggerHandler(config_path=args.config)
        
        if args.monitor:
            # Monitor for trigger files
            handler.monitor_trigger_files(
                trigger_dir=args.trigger_dir,
                poll_interval=args.poll_interval
            )
        elif args.event_file:
            # Process a specific event file
            with open(args.event_file, 'r') as f:
                event_data = json.load(f)
            
            event = BackupCompletionEvent.from_dict(event_data)
            result = handler.handle_backup_completion(event)
            
            if result.success:
                print(f"✓ GitOps generation triggered successfully")
                print(f"  Duration: {result.duration:.2f} seconds")
                print(f"  Output: {result.output[:200]}...")
            else:
                print(f"✗ GitOps generation failed: {result.error}")
                sys.exit(1)
        else:
            print("Error: Must specify either --monitor or --event-file")
            parser.print_help()
            sys.exit(1)
    
    except Exception as e:
        logging.error(f"GitOps trigger handler failed: {e}")
        sys.exit(1)


if __name__ == '__main__':
    main()