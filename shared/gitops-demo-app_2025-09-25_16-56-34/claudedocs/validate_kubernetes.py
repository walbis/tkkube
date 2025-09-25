#!/usr/bin/env python3
"""
Kubernetes Compliance Validation Script
Tests kubectl dry-run, schema validation, and Kubernetes best practices
"""
import os
import json
import subprocess
import yaml
from pathlib import Path
from typing import List, Dict, Any, Tuple

class KubernetesValidator:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.results = {
            'kubectl_available': False,
            'kubernetes_files': [],
            'dry_run_results': {},
            'schema_validation': {},
            'best_practices': {},
            'total_manifests': 0,
            'valid_manifests': 0,
            'warnings': [],
            'errors': []
        }
        self.check_kubectl()
    
    def check_kubectl(self):
        """Check if kubectl is available"""
        try:
            result = subprocess.run(['kubectl', 'version', '--client'], 
                                  capture_output=True, text=True, timeout=10)
            self.results['kubectl_available'] = result.returncode == 0
            if self.results['kubectl_available']:
                print("✅ kubectl is available")
            else:
                print("❌ kubectl not available - dry-run tests will be skipped")
        except Exception as e:
            print(f"❌ kubectl check failed: {e}")
            self.results['kubectl_available'] = False
    
    def find_kubernetes_manifests(self) -> List[Path]:
        """Find Kubernetes manifest files"""
        k8s_files = []
        
        # Look for YAML files that are likely Kubernetes manifests
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        yaml_files.extend(list(self.project_root.glob('**/*.yml')))
        
        for file_path in yaml_files:
            # Skip certain directories and files but include base directory
            skip_patterns = ['claudedocs', 'node_modules', '.git', 'backup-source']
            if any(pattern in str(file_path) for pattern in skip_patterns):
                continue
                
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                    # Check if it's a Kubernetes manifest
                    if self.is_kubernetes_manifest(content):
                        k8s_files.append(file_path)
            except Exception as e:
                self.results['warnings'].append(f"Could not read {file_path}: {e}")
        
        return sorted(k8s_files)
    
    def is_kubernetes_manifest(self, content: str) -> bool:
        """Check if YAML content is a Kubernetes manifest"""
        try:
            docs = list(yaml.safe_load_all(content))
            for doc in docs:
                if doc and isinstance(doc, dict):
                    # Look for Kubernetes-specific fields
                    if 'apiVersion' in doc and 'kind' in doc:
                        # Skip kustomization files - they'll be handled by GitOps validator
                        kind = doc.get('kind', '')
                        api_version = doc.get('apiVersion', '')
                        
                        # Skip all Kustomization types
                        if kind == 'Kustomization':
                            continue
                            
                        # This is a standard Kubernetes resource
                        return True
        except:
            pass
        return False
    
    def validate_with_kubectl_dry_run(self, file_path: Path) -> Dict[str, Any]:
        """Validate manifest using kubectl dry-run"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'valid': False,
            'warnings': [],
            'errors': [],
            'output': ''
        }
        
        if not self.results['kubectl_available']:
            result['errors'].append('kubectl not available')
            return result
        
        try:
            # Try dry-run validation
            cmd = ['kubectl', 'apply', '--dry-run=client', '-f', str(file_path)]
            process = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            
            result['output'] = process.stdout + process.stderr
            result['valid'] = process.returncode == 0
            
            if not result['valid']:
                result['errors'].append(f"kubectl dry-run failed: {process.stderr}")
            
            # Check for deprecation warnings
            if 'deprecated' in result['output'].lower():
                result['warnings'].append('Contains deprecated API versions')
            
        except subprocess.TimeoutExpired:
            result['errors'].append('kubectl dry-run timed out')
        except Exception as e:
            result['errors'].append(f'kubectl dry-run error: {str(e)}')
        
        return result
    
    def validate_kubernetes_schema(self, file_path: Path) -> Dict[str, Any]:
        """Validate Kubernetes manifest schema"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'manifests': [],
            'errors': [],
            'warnings': []
        }
        
        try:
            with open(file_path, 'r') as f:
                content = f.read()
            
            docs = list(yaml.safe_load_all(content))
            for i, doc in enumerate(docs):
                if not doc:
                    continue
                    
                manifest_result = {
                    'document_index': i,
                    'valid': True,
                    'errors': [],
                    'warnings': [],
                    'kind': doc.get('kind', 'Unknown'),
                    'apiVersion': doc.get('apiVersion', 'Unknown'),
                    'name': doc.get('metadata', {}).get('name', 'Unknown'),
                    'namespace': doc.get('metadata', {}).get('namespace', 'default')
                }
                
                # Basic schema validation
                required_fields = ['apiVersion', 'kind']
                for field in required_fields:
                    if field not in doc:
                        manifest_result['errors'].append(f'Missing required field: {field}')
                        manifest_result['valid'] = False
                
                # Metadata validation
                if 'metadata' in doc:
                    metadata = doc['metadata']
                    if 'name' not in metadata:
                        manifest_result['errors'].append('Missing metadata.name')
                        manifest_result['valid'] = False
                    
                    # Check name format
                    name = metadata.get('name', '')
                    if name and not self.is_valid_k8s_name(name):
                        manifest_result['warnings'].append(f'Name "{name}" may not follow Kubernetes naming conventions')
                
                # Check for common issues
                self.check_best_practices(doc, manifest_result)
                
                result['manifests'].append(manifest_result)
                
        except Exception as e:
            result['errors'].append(f'Schema validation error: {str(e)}')
        
        return result
    
    def is_valid_k8s_name(self, name: str) -> bool:
        """Check if name follows Kubernetes naming conventions"""
        import re
        # Kubernetes names must be lowercase alphanumeric or '-', max 253 chars
        pattern = r'^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'
        return bool(re.match(pattern, name)) and len(name) <= 253
    
    def check_best_practices(self, manifest: dict, result: dict):
        """Check Kubernetes best practices"""
        kind = manifest.get('kind', '')
        
        # Check for resource limits and requests
        if kind in ['Deployment', 'StatefulSet', 'DaemonSet']:
            spec = manifest.get('spec', {})
            template = spec.get('template', {})
            pod_spec = template.get('spec', {})
            containers = pod_spec.get('containers', [])
            
            for container in containers:
                resources = container.get('resources', {})
                if not resources.get('limits'):
                    result['warnings'].append(f'Container "{container.get("name", "unknown")}" missing resource limits')
                if not resources.get('requests'):
                    result['warnings'].append(f'Container "{container.get("name", "unknown")}" missing resource requests')
        
        # Check for security contexts
        if kind in ['Deployment', 'StatefulSet', 'DaemonSet', 'Pod']:
            if kind == 'Pod':
                pod_spec = manifest.get('spec', {})
            else:
                pod_spec = manifest.get('spec', {}).get('template', {}).get('spec', {})
            
            if not pod_spec.get('securityContext'):
                result['warnings'].append('Missing pod security context')
        
        # Check for labels and selectors
        metadata = manifest.get('metadata', {})
        labels = metadata.get('labels', {})
        
        recommended_labels = ['app', 'version', 'component']
        missing_labels = [label for label in recommended_labels if label not in labels]
        if missing_labels:
            result['warnings'].append(f'Missing recommended labels: {missing_labels}')
    
    def validate_all_manifests(self) -> Dict[str, Any]:
        """Validate all Kubernetes manifests"""
        k8s_files = self.find_kubernetes_manifests()
        self.results['kubernetes_files'] = [str(f.relative_to(self.project_root)) for f in k8s_files]
        self.results['total_manifests'] = len(k8s_files)
        
        print(f"Found {len(k8s_files)} Kubernetes manifest files")
        
        for file_path in k8s_files:
            print(f"Validating: {file_path.relative_to(self.project_root)}")
            
            # Kubectl dry-run validation
            if self.results['kubectl_available']:
                dry_run_result = self.validate_with_kubectl_dry_run(file_path)
                self.results['dry_run_results'][str(file_path.relative_to(self.project_root))] = dry_run_result
                
                if dry_run_result['valid']:
                    self.results['valid_manifests'] += 1
                else:
                    self.results['errors'].extend([
                        f"{dry_run_result['file']}: {error}" 
                        for error in dry_run_result['errors']
                    ])
            
            # Schema validation
            schema_result = self.validate_kubernetes_schema(file_path)
            self.results['schema_validation'][str(file_path.relative_to(self.project_root))] = schema_result
            
            # Collect warnings
            for manifest in schema_result['manifests']:
                if manifest['warnings']:
                    self.results['warnings'].extend([
                        f"{schema_result['file']}: {warning}" 
                        for warning in manifest['warnings']
                    ])
        
        return self.results
    
    def generate_report(self) -> str:
        """Generate comprehensive Kubernetes validation report"""
        report = []
        report.append("=" * 80)
        report.append("KUBERNETES COMPLIANCE VALIDATION REPORT")
        report.append("=" * 80)
        report.append(f"kubectl available: {'Yes' if self.results['kubectl_available'] else 'No'}")
        report.append(f"Total manifest files: {self.results['total_manifests']}")
        report.append(f"Valid manifests (dry-run): {self.results['valid_manifests']}")
        
        if self.results['total_manifests'] > 0:
            success_rate = (self.results['valid_manifests'] / self.results['total_manifests'] * 100)
            report.append(f"Success rate: {success_rate:.1f}%")
        report.append("")
        
        # Files found
        report.append("KUBERNETES MANIFEST FILES:")
        report.append("-" * 40)
        for file_path in self.results['kubernetes_files']:
            report.append(f"📄 {file_path}")
        report.append("")
        
        # Dry-run results
        if self.results['dry_run_results']:
            report.append("KUBECTL DRY-RUN RESULTS:")
            report.append("-" * 40)
            for file_path, result in self.results['dry_run_results'].items():
                status = "✅ VALID" if result['valid'] else "❌ INVALID"
                report.append(f"{status} {file_path}")
                if result['errors']:
                    for error in result['errors']:
                        report.append(f"  ❌ {error}")
                if result['warnings']:
                    for warning in result['warnings']:
                        report.append(f"  ⚠️  {warning}")
            report.append("")
        
        # Schema validation results
        if self.results['schema_validation']:
            report.append("SCHEMA VALIDATION RESULTS:")
            report.append("-" * 40)
            for file_path, result in self.results['schema_validation'].items():
                report.append(f"📄 {file_path}")
                for manifest in result['manifests']:
                    status = "✅ VALID" if manifest['valid'] else "❌ INVALID"
                    report.append(f"  {status} {manifest['kind']}/{manifest['name']}")
                    report.append(f"    API Version: {manifest['apiVersion']}")
                    report.append(f"    Namespace: {manifest['namespace']}")
                    
                    if manifest['errors']:
                        for error in manifest['errors']:
                            report.append(f"    ❌ {error}")
                    if manifest['warnings']:
                        for warning in manifest['warnings']:
                            report.append(f"    ⚠️  {warning}")
                report.append("")
        
        # Summary of all errors and warnings
        if self.results['errors']:
            report.append("ALL ERRORS:")
            report.append("-" * 40)
            for error in self.results['errors']:
                report.append(f"❌ {error}")
            report.append("")
        
        if self.results['warnings']:
            report.append("ALL WARNINGS:")
            report.append("-" * 40)
            for warning in self.results['warnings']:
                report.append(f"⚠️  {warning}")
            report.append("")
        
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    validator = KubernetesValidator(project_root)
    
    print("Starting Kubernetes compliance validation...")
    results = validator.validate_all_manifests()
    
    # Generate and save report
    report = validator.generate_report()
    print(report)
    
    # Save results to JSON
    with open('kubernetes_validation_results.json', 'w') as f:
        json.dump(results, f, indent=2)
    
    # Exit code based on validation results
    exit_code = 1 if results['errors'] else 0
    print(f"\nValidation complete. Exit code: {exit_code}")
    return exit_code

if __name__ == "__main__":
    exit(main())