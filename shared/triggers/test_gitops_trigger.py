#!/usr/bin/env python3
"""
Tests for GitOps Auto-Trigger Handler
"""

import os
import json
import tempfile
import unittest
from unittest.mock import Mock, patch, MagicMock
from pathlib import Path
from datetime import datetime
import sys

# Add the parent directory to Python path for importing
current_dir = Path(__file__).parent
sys.path.insert(0, str(current_dir))

from gitops_trigger import GitOpsTriggerHandler, BackupCompletionEvent, GitOpsTriggerResult


class TestBackupCompletionEvent(unittest.TestCase):
    """Test cases for BackupCompletionEvent."""
    
    def test_from_dict_basic(self):
        """Test creating BackupCompletionEvent from dictionary."""
        data = {
            'backup_id': 'test-backup-123',
            'cluster_name': 'test-cluster',
            'timestamp': '2023-01-01T12:00:00',
            'duration': 120.5,
            'namespaces_count': 5,
            'resources_count': 50,
            'success': True,
            'errors': [],
            'minio_bucket': 'test-bucket',
            'backup_location': 'test-bucket/test-cluster',
            'metadata': {'test': 'value'}
        }
        
        event = BackupCompletionEvent.from_dict(data)
        
        self.assertEqual(event.backup_id, 'test-backup-123')
        self.assertEqual(event.cluster_name, 'test-cluster')
        self.assertEqual(event.namespaces_count, 5)
        self.assertEqual(event.resources_count, 50)
        self.assertTrue(event.success)
        self.assertEqual(event.minio_bucket, 'test-bucket')
    
    def test_from_dict_with_timestamp_int(self):
        """Test creating event with integer timestamp."""
        data = {
            'backup_id': 'test-backup-123',
            'cluster_name': 'test-cluster',
            'timestamp': 1672574400,  # Unix timestamp
            'duration': 120.5,
            'namespaces_count': 5,
            'resources_count': 50,
            'success': True,
            'errors': [],
            'minio_bucket': 'test-bucket'
        }
        
        event = BackupCompletionEvent.from_dict(data)
        
        self.assertIsInstance(event.timestamp, datetime)
        self.assertEqual(event.backup_id, 'test-backup-123')
    
    def test_to_dict(self):
        """Test converting event to dictionary."""
        event = BackupCompletionEvent(
            backup_id='test-backup-123',
            cluster_name='test-cluster',
            timestamp=datetime(2023, 1, 1, 12, 0, 0),
            duration=120.5,
            namespaces_count=5,
            resources_count=50,
            success=True,
            errors=[],
            minio_bucket='test-bucket'
        )
        
        data = event.to_dict()
        
        self.assertEqual(data['backup_id'], 'test-backup-123')
        self.assertEqual(data['cluster_name'], 'test-cluster')
        self.assertIn('timestamp', data)
        self.assertEqual(data['duration'], 120.5)


class TestGitOpsTriggerHandler(unittest.TestCase):
    """Test cases for GitOpsTriggerHandler."""
    
    def setUp(self):
        """Set up test fixtures."""
        # Create a temporary config file
        self.temp_config = tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False)
        config_content = """
storage:
  endpoint: localhost:9000
  access_key: testkey
  secret_key: testsecret
  bucket: test-bucket

cluster:
  name: test-cluster
  domain: cluster.local

pipeline:
  automation:
    enabled: true
    max_wait_time: 300

observability:
  logging:
    level: info
"""
        self.temp_config.write(config_content)
        self.temp_config.close()
        
        # Mock logger
        self.mock_logger = Mock()
    
    def tearDown(self):
        """Clean up test fixtures."""
        os.unlink(self.temp_config.name)
    
    def create_test_event(self):
        """Create a test backup completion event."""
        return BackupCompletionEvent(
            backup_id='test-backup-123',
            cluster_name='test-cluster',
            timestamp=datetime.now(),
            duration=120.5,
            namespaces_count=5,
            resources_count=50,
            success=True,
            errors=[],
            minio_bucket='test-bucket',
            backup_location='test-bucket/test-cluster'
        )
    
    @patch('gitops_trigger.ConfigLoader')
    def test_handler_initialization(self, mock_config_loader):
        """Test handler initialization."""
        mock_config = Mock()
        mock_config.cluster.name = 'test-cluster'
        mock_config.pipeline.automation.enabled = True
        
        mock_loader_instance = Mock()
        mock_loader_instance.load.return_value = mock_config
        mock_config_loader.return_value = mock_loader_instance
        
        handler = GitOpsTriggerHandler(config_path=self.temp_config.name, logger=self.mock_logger)
        
        self.assertIsNotNone(handler)
        self.assertEqual(handler.config, mock_config)
        self.assertEqual(handler.logger, self.mock_logger)
    
    @patch('gitops_trigger.ConfigLoader')
    def test_handle_backup_completion_disabled(self, mock_config_loader):
        """Test handling backup completion when auto-triggering is disabled."""
        mock_config = Mock()
        mock_config.pipeline.automation.enabled = False
        
        mock_loader_instance = Mock()
        mock_loader_instance.load.return_value = mock_config
        mock_config_loader.return_value = mock_loader_instance
        
        handler = GitOpsTriggerHandler(config_path=self.temp_config.name, logger=self.mock_logger)
        event = self.create_test_event()
        
        result = handler.handle_backup_completion(event)
        
        self.assertTrue(result.success)
        self.assertEqual(result.method, "disabled")
        self.assertIn("disabled", result.output)
    
    @patch('gitops_trigger.subprocess.run')
    @patch('gitops_trigger.ConfigLoader')
    def test_handle_backup_completion_success(self, mock_config_loader, mock_subprocess):
        """Test successful backup completion handling."""
        # Mock configuration
        mock_config = Mock()
        mock_config.pipeline.automation.enabled = True
        mock_config.pipeline.automation.max_wait_time = 300
        mock_config.storage.endpoint = 'localhost:9000'
        mock_config.storage.access_key = 'testkey'
        mock_config.storage.secret_key = 'testsecret'
        mock_config.storage.bucket = 'test-bucket'
        mock_config.cluster.name = 'test-cluster'
        mock_config.gitops.repository.url = 'https://github.com/test/repo.git'
        mock_config.gitops.repository.branch = 'main'
        mock_config.observability.logging.level = 'info'
        
        mock_loader_instance = Mock()
        mock_loader_instance.load.return_value = mock_config
        mock_config_loader.return_value = mock_loader_instance
        
        # Mock subprocess execution
        mock_result = Mock()
        mock_result.stdout = "GitOps generation completed successfully"
        mock_result.stderr = ""
        mock_result.returncode = 0
        mock_subprocess.return_value = mock_result
        
        # Mock finding GitOps binary
        handler = GitOpsTriggerHandler(config_path=self.temp_config.name, logger=self.mock_logger)
        with patch.object(handler, '_find_gitops_binary', return_value='/usr/bin/minio-to-git'):
            with patch.object(handler, '_find_config_file', return_value=self.temp_config.name):
                event = self.create_test_event()
                result = handler.handle_backup_completion(event)
        
        self.assertTrue(result.success)
        self.assertEqual(result.method, "direct_invocation")
        self.assertIn("GitOps generation completed successfully", result.output)
        
        # Verify subprocess was called
        mock_subprocess.assert_called_once()
    
    @patch('gitops_trigger.subprocess.run')
    @patch('gitops_trigger.ConfigLoader')
    def test_handle_backup_completion_failure(self, mock_config_loader, mock_subprocess):
        """Test backup completion handling with GitOps failure."""
        # Mock configuration
        mock_config = Mock()
        mock_config.pipeline.automation.enabled = True
        mock_config.pipeline.automation.max_wait_time = 300
        mock_config.storage.endpoint = 'localhost:9000'
        mock_config.storage.access_key = 'testkey'
        mock_config.storage.secret_key = 'testsecret'
        mock_config.storage.bucket = 'test-bucket'
        mock_config.cluster.name = 'test-cluster'
        mock_config.gitops.repository.url = 'https://github.com/test/repo.git'
        mock_config.gitops.repository.branch = 'main'
        mock_config.observability.logging.level = 'info'
        
        mock_loader_instance = Mock()
        mock_loader_instance.load.return_value = mock_config
        mock_config_loader.return_value = mock_loader_instance
        
        # Mock subprocess execution failure
        from subprocess import CalledProcessError
        mock_subprocess.side_effect = CalledProcessError(1, 'minio-to-git', stderr='Error message')
        
        # Mock finding GitOps binary
        handler = GitOpsTriggerHandler(config_path=self.temp_config.name, logger=self.mock_logger)
        with patch.object(handler, '_find_gitops_binary', return_value='/usr/bin/minio-to-git'):
            event = self.create_test_event()
            result = handler.handle_backup_completion(event)
        
        self.assertFalse(result.success)
        self.assertEqual(result.method, "direct_invocation")
        self.assertIn("failed with exit code", result.error)
    
    def test_find_gitops_binary_not_found(self):
        """Test finding GitOps binary when not available."""
        with patch('os.path.isfile', return_value=False):
            with patch('subprocess.run', side_effect=FileNotFoundError):
                with patch('gitops_trigger.ConfigLoader'):
                    handler = GitOpsTriggerHandler(logger=self.mock_logger)
                    binary = handler._find_gitops_binary()
                    self.assertIsNone(binary)
    
    def test_find_config_file_found(self):
        """Test finding configuration file."""
        with patch('os.path.isfile') as mock_isfile:
            # First few return False, then True for one candidate
            mock_isfile.side_effect = [False, False, True]
            
            with patch('gitops_trigger.ConfigLoader'):
                handler = GitOpsTriggerHandler(logger=self.mock_logger)
                config_path = handler._find_config_file()
                self.assertIsNotNone(config_path)
    
    @patch('gitops_trigger.ConfigLoader')
    def test_monitor_trigger_files(self, mock_config_loader):
        """Test monitoring trigger files."""
        mock_config = Mock()
        mock_loader_instance = Mock()
        mock_loader_instance.load.return_value = mock_config
        mock_config_loader.return_value = mock_loader_instance
        
        handler = GitOpsTriggerHandler(logger=self.mock_logger)
        
        # Create a temporary trigger directory
        with tempfile.TemporaryDirectory() as temp_dir:
            # Create a test trigger file
            trigger_file = Path(temp_dir) / "backup-complete-test-123.json"
            event_data = self.create_test_event().to_dict()
            
            with open(trigger_file, 'w') as f:
                json.dump(event_data, f)
            
            # Mock the handle_backup_completion method
            with patch.object(handler, 'handle_backup_completion') as mock_handle:
                mock_result = GitOpsTriggerResult(
                    success=True,
                    timestamp=datetime.now(),
                    duration=1.0,
                    method="file",
                    output="Success"
                )
                mock_handle.return_value = mock_result
                
                # Mock time.sleep to avoid actual sleeping
                with patch('time.sleep', side_effect=KeyboardInterrupt):
                    try:
                        handler.monitor_trigger_files(trigger_dir=temp_dir, poll_interval=0.1)
                    except KeyboardInterrupt:
                        pass  # Expected to break the loop
                
                # Verify the handler was called
                mock_handle.assert_called_once()


class TestIntegration(unittest.TestCase):
    """Integration tests for the auto-trigger system."""
    
    def test_file_trigger_integration(self):
        """Test file-based trigger integration."""
        # Create a temporary trigger directory
        with tempfile.TemporaryDirectory() as temp_dir:
            # Create a test event
            event = BackupCompletionEvent(
                backup_id='integration-test-123',
                cluster_name='test-cluster',
                timestamp=datetime.now(),
                duration=120.5,
                namespaces_count=5,
                resources_count=50,
                success=True,
                errors=[],
                minio_bucket='test-bucket'
            )
            
            # Write event to trigger file
            trigger_file = Path(temp_dir) / f"backup-complete-{event.cluster_name}-{int(event.timestamp.timestamp())}.json"
            with open(trigger_file, 'w') as f:
                json.dump(event.to_dict(), f, indent=2)
            
            # Verify file was created
            self.assertTrue(trigger_file.exists())
            
            # Read and verify content
            with open(trigger_file, 'r') as f:
                loaded_data = json.load(f)
            
            loaded_event = BackupCompletionEvent.from_dict(loaded_data)
            self.assertEqual(loaded_event.backup_id, event.backup_id)
            self.assertEqual(loaded_event.cluster_name, event.cluster_name)
    
    def test_webhook_payload_structure(self):
        """Test webhook payload structure."""
        event = BackupCompletionEvent(
            backup_id='webhook-test-123',
            cluster_name='test-cluster',
            timestamp=datetime.now(),
            duration=120.5,
            namespaces_count=5,
            resources_count=50,
            success=True,
            errors=[],
            minio_bucket='test-bucket'
        )
        
        # Create webhook payload
        webhook_payload = {
            'event_type': 'backup_complete',
            'timestamp': datetime.now().isoformat(),
            'backup': event.to_dict(),
            'trigger': 'auto_gitops'
        }
        
        # Verify payload structure
        self.assertEqual(webhook_payload['event_type'], 'backup_complete')
        self.assertIn('backup', webhook_payload)
        self.assertIn('backup_id', webhook_payload['backup'])
        self.assertEqual(webhook_payload['backup']['cluster_name'], 'test-cluster')


if __name__ == '__main__':
    unittest.main()