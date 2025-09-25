#!/usr/bin/env python3
"""
Data Integrity Validation Script
Compares backup source data with GitOps artifacts to verify completeness and accuracy
"""
import os
import json
import yaml
from pathlib import Path
from typing import List, Dict, Any, Set, Tuple
from collections import defaultdict

class DataIntegrityValidator:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.backup_source_dir = self.project_root / 'backup-source'
        self.base_dir = self.project_root / 'base'
        
        self.results = {
            'backup_source_files': [],
            'gitops_base_files': [],
            'resource_comparison': {},
            'missing_resources': [],
            'extra_resources': [],
            'data_mismatches': [],
            'structure_issues': [],
            'transformation_analysis': {},
            'quality_metrics': {},
            'errors': [],
            'warnings': []
        }
    
    def load_yaml_resources(self, directory: Path) -> Dict[str, List[Dict]]:
        """Load all YAML resources from a directory"""
        resources = defaultdict(list)
        
        if not directory.exists():
            return resources
        
        yaml_files = list(directory.glob('*.yaml'))
        yaml_files.extend(list(directory.glob('*.yml')))
        
        for file_path in yaml_files:
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                # Parse all documents in the file
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if doc is None:
                        continue
                    
                    # Handle different formats
                    if isinstance(doc, dict) and 'kind' in doc:
                        # Standard Kubernetes resource format
                        kind = doc.get('kind')
                        resources[kind].append({
                            'file': file_path.name,
                            'resource': doc
                        })
                    elif isinstance(doc, list):
                        # List format - try to infer kind from filename or structure
                        filename = file_path.stem.lower()
                        if 'deployment' in filename:
                            for item in doc:
                                if isinstance(item, dict):
                                    # Add kind and apiVersion if missing
                                    if 'kind' not in item:
                                        item['kind'] = 'Deployment'
                                    if 'apiVersion' not in item:
                                        item['apiVersion'] = 'apps/v1'
                                    resources['Deployment'].append({
                                        'file': file_path.name,
                                        'resource': item
                                    })
                        elif 'service' in filename:
                            for item in doc:
                                if isinstance(item, dict):
                                    if 'kind' not in item:
                                        item['kind'] = 'Service'
                                    if 'apiVersion' not in item:
                                        item['apiVersion'] = 'v1'
                                    resources['Service'].append({
                                        'file': file_path.name,
                                        'resource': item
                                    })
                        elif 'configmap' in filename:
                            for item in doc:
                                if isinstance(item, dict):
                                    if 'kind' not in item:
                                        item['kind'] = 'ConfigMap'
                                    if 'apiVersion' not in item:
                                        item['apiVersion'] = 'v1'
                                    resources['ConfigMap'].append({
                                        'file': file_path.name,
                                        'resource': item
                                    })
                        
            except Exception as e:
                self.results['errors'].append(f'Error loading {file_path}: {str(e)}')
        
        return resources
    
    def extract_resource_key(self, resource: Dict) -> Tuple[str, str, str]:
        """Extract identifying key for a resource (kind, namespace, name)"""
        kind = resource.get('kind', 'Unknown')
        metadata = resource.get('metadata', {})
        name = metadata.get('name', 'unnamed')
        namespace = metadata.get('namespace', 'default')
        return (kind, namespace, name)
    
    def compare_resource_structures(self, source_resources: Dict, target_resources: Dict) -> Dict:
        """Compare resource structures between source and target"""
        comparison = {
            'resource_counts': {
                'source': {kind: len(resources) for kind, resources in source_resources.items()},
                'target': {kind: len(resources) for kind, resources in target_resources.items()}
            },
            'missing_kinds': [],
            'extra_kinds': [],
            'resource_details': {}
        }
        
        source_kinds = set(source_resources.keys())
        target_kinds = set(target_resources.keys())
        
        comparison['missing_kinds'] = list(source_kinds - target_kinds)
        comparison['extra_kinds'] = list(target_kinds - source_kinds)
        
        # Compare individual resources
        for kind in source_kinds.union(target_kinds):
            source_items = source_resources.get(kind, [])
            target_items = target_resources.get(kind, [])
            
            source_keys = {self.extract_resource_key(item['resource']) for item in source_items}
            target_keys = {self.extract_resource_key(item['resource']) for item in target_items}
            
            comparison['resource_details'][kind] = {
                'source_count': len(source_items),
                'target_count': len(target_items),
                'missing_in_target': list(source_keys - target_keys),
                'extra_in_target': list(target_keys - source_keys),
                'common_resources': list(source_keys.intersection(target_keys))
            }
        
        return comparison
    
    def validate_resource_content(self, source_resource: Dict, target_resource: Dict) -> Dict:
        """Validate content consistency between source and target resource"""
        validation = {
            'metadata_changes': {},
            'spec_changes': {},
            'data_changes': {},
            'removed_fields': [],
            'added_fields': [],
            'critical_changes': []
        }
        
        # Check metadata changes
        source_metadata = source_resource.get('metadata', {})
        target_metadata = target_resource.get('metadata', {})
        
        # Key fields that should be preserved
        critical_metadata = ['name', 'namespace', 'labels', 'annotations']
        for field in critical_metadata:
            source_val = source_metadata.get(field)
            target_val = target_metadata.get(field)
            
            if source_val != target_val:
                validation['metadata_changes'][field] = {
                    'source': source_val,
                    'target': target_val
                }
                
                # Critical changes that affect functionality
                if field in ['name', 'namespace']:
                    validation['critical_changes'].append(f'Critical metadata change: {field}')
        
        # Check spec changes
        source_spec = source_resource.get('spec', {})
        target_spec = target_resource.get('spec', {})
        
        if source_spec != target_spec:
            validation['spec_changes'] = {
                'modified': True,
                'source_keys': set(source_spec.keys()) if isinstance(source_spec, dict) else [],
                'target_keys': set(target_spec.keys()) if isinstance(target_spec, dict) else []
            }
        
        # Check data changes (for ConfigMaps, Secrets)
        source_data = source_resource.get('data', {})
        target_data = target_resource.get('data', {})
        
        if source_data != target_data:
            validation['data_changes'] = {
                'modified': True,
                'source_keys': set(source_data.keys()) if isinstance(source_data, dict) else [],
                'target_keys': set(target_data.keys()) if isinstance(target_data, dict) else []
            }
        
        # Check for removed Kubernetes system fields (these are expected)
        system_fields = ['status', 'creationTimestamp', 'resourceVersion', 'uid', 'managedFields', 'generation']
        validation['removed_system_fields'] = [
            field for field in system_fields 
            if field in source_resource and field not in target_resource
        ]
        
        return validation
    
    def analyze_transformations(self, source_resources: Dict, target_resources: Dict) -> Dict:
        """Analyze what transformations occurred during GitOps conversion"""
        analysis = {
            'field_removals': defaultdict(int),
            'field_modifications': defaultdict(int),
            'common_transformations': [],
            'quality_improvements': [],
            'potential_issues': []
        }
        
        # Analyze common transformation patterns
        for kind in source_resources.keys():
            if kind not in target_resources:
                continue
                
            source_items = source_resources[kind]
            target_items = target_resources[kind]
            
            # Create lookup for target resources
            target_lookup = {}
            for item in target_items:
                key = self.extract_resource_key(item['resource'])
                target_lookup[key] = item['resource']
            
            for source_item in source_items:
                source_res = source_item['resource']
                key = self.extract_resource_key(source_res)
                
                if key in target_lookup:
                    target_res = target_lookup[key]
                    validation = self.validate_resource_content(source_res, target_res)
                    
                    # Track field removals
                    if validation['removed_system_fields']:
                        analysis['field_removals']['system_fields'] += len(validation['removed_system_fields'])
                        if 'system_field_cleanup' not in analysis['common_transformations']:
                            analysis['common_transformations'].append('system_field_cleanup')
                    
                    # Track critical changes
                    if validation['critical_changes']:
                        analysis['potential_issues'].extend(validation['critical_changes'])
                    
                    # Check for quality improvements
                    if 'namespace' not in source_res.get('metadata', {}) and 'namespace' in target_res.get('metadata', {}):
                        analysis['quality_improvements'].append('namespace_addition')
        
        return analysis
    
    def check_structural_issues(self, resources: Dict, location: str) -> List[Dict]:
        """Check for structural issues in resources"""
        issues = []
        
        for kind, items in resources.items():
            for item in items:
                resource = item['resource']
                file_name = item['file']
                
                # Check required fields
                if 'apiVersion' not in resource:
                    issues.append({
                        'type': 'missing_api_version',
                        'location': location,
                        'file': file_name,
                        'resource': f"{kind}/unknown",
                        'description': 'Missing apiVersion field'
                    })
                
                metadata = resource.get('metadata', {})
                if not metadata:
                    issues.append({
                        'type': 'missing_metadata',
                        'location': location,
                        'file': file_name,
                        'resource': f"{kind}/unknown",
                        'description': 'Missing metadata section'
                    })
                elif 'name' not in metadata:
                    issues.append({
                        'type': 'missing_name',
                        'location': location,
                        'file': file_name,
                        'resource': f"{kind}/unnamed",
                        'description': 'Missing metadata.name field'
                    })
                
                # Check kind-specific requirements
                if kind == 'Deployment':
                    spec = resource.get('spec', {})
                    if 'selector' not in spec:
                        issues.append({
                            'type': 'missing_selector',
                            'location': location,
                            'file': file_name,
                            'resource': f"{kind}/{metadata.get('name', 'unnamed')}",
                            'description': 'Deployment missing spec.selector'
                        })
                    
                    template = spec.get('template', {})
                    if 'spec' not in template:
                        issues.append({
                            'type': 'missing_pod_spec',
                            'location': location,
                            'file': file_name,
                            'resource': f"{kind}/{metadata.get('name', 'unnamed')}",
                            'description': 'Deployment missing template.spec'
                        })
        
        return issues
    
    def calculate_quality_metrics(self) -> Dict:
        """Calculate quality and completeness metrics"""
        metrics = {
            'completeness': {
                'resource_preservation_rate': 0.0,
                'data_integrity_rate': 0.0,
                'structural_quality_score': 0.0
            },
            'transformation': {
                'system_field_cleanup_rate': 0.0,
                'quality_improvement_count': 0,
                'potential_issue_count': 0
            },
            'overall': {
                'data_integrity_score': 0.0,
                'transformation_quality_score': 0.0,
                'readiness_for_deployment': 'unknown'
            }
        }
        
        comparison = self.results.get('resource_comparison', {})
        
        if comparison:
            # Calculate resource preservation rate
            source_total = sum(comparison['resource_counts']['source'].values())
            target_total = sum(comparison['resource_counts']['target'].values())
            
            if source_total > 0:
                metrics['completeness']['resource_preservation_rate'] = min(100.0, (target_total / source_total) * 100)
        
        # Calculate structural quality
        structure_issues = self.results.get('structure_issues', [])
        total_resources = len([issue for issue in structure_issues if issue['location'] == 'target'])
        critical_issues = len([issue for issue in structure_issues if issue['type'] in ['missing_name', 'missing_metadata', 'missing_api_version']])
        
        if total_resources > 0:
            metrics['completeness']['structural_quality_score'] = max(0.0, (1 - critical_issues / total_resources) * 100)
        else:
            metrics['completeness']['structural_quality_score'] = 100.0
        
        # Transformation analysis
        transform_analysis = self.results.get('transformation_analysis', {})
        if transform_analysis:
            metrics['transformation']['quality_improvement_count'] = len(transform_analysis.get('quality_improvements', []))
            metrics['transformation']['potential_issue_count'] = len(transform_analysis.get('potential_issues', []))
        
        # Overall scores
        metrics['overall']['data_integrity_score'] = (
            metrics['completeness']['resource_preservation_rate'] * 0.4 +
            metrics['completeness']['structural_quality_score'] * 0.6
        )
        
        metrics['overall']['transformation_quality_score'] = max(0.0, 100.0 - metrics['transformation']['potential_issue_count'] * 10)
        
        # Deployment readiness assessment
        integrity_score = metrics['overall']['data_integrity_score']
        if integrity_score >= 90:
            metrics['overall']['readiness_for_deployment'] = 'ready'
        elif integrity_score >= 70:
            metrics['overall']['readiness_for_deployment'] = 'needs_review'
        else:
            metrics['overall']['readiness_for_deployment'] = 'not_ready'
        
        return metrics
    
    def validate_data_integrity(self) -> Dict[str, Any]:
        """Main validation method"""
        print("Starting data integrity validation...")
        
        # Load source and target resources
        print("Loading backup source resources...")
        source_resources = self.load_yaml_resources(self.backup_source_dir)
        self.results['backup_source_files'] = list(self.backup_source_dir.glob('*.yaml'))
        
        print("Loading GitOps base resources...")
        target_resources = self.load_yaml_resources(self.base_dir)
        self.results['gitops_base_files'] = list(self.base_dir.glob('*.yaml'))
        
        if not source_resources:
            self.results['warnings'].append('No source resources found for comparison')
            return self.results
        
        if not target_resources:
            self.results['errors'].append('No target resources found for comparison')
            return self.results
        
        # Compare resource structures
        print("Comparing resource structures...")
        self.results['resource_comparison'] = self.compare_resource_structures(source_resources, target_resources)
        
        # Check structural issues
        print("Checking structural issues...")
        source_issues = self.check_structural_issues(source_resources, 'source')
        target_issues = self.check_structural_issues(target_resources, 'target')
        self.results['structure_issues'] = source_issues + target_issues
        
        # Analyze transformations
        print("Analyzing transformations...")
        self.results['transformation_analysis'] = self.analyze_transformations(source_resources, target_resources)
        
        # Calculate quality metrics
        print("Calculating quality metrics...")
        self.results['quality_metrics'] = self.calculate_quality_metrics()
        
        return self.results
    
    def generate_report(self) -> str:
        """Generate comprehensive data integrity report"""
        report = []
        report.append("=" * 80)
        report.append("DATA INTEGRITY VALIDATION REPORT")
        report.append("=" * 80)
        
        # File summary
        report.append("FILE SUMMARY:")
        report.append("-" * 40)
        report.append(f"Backup source files: {len(self.results['backup_source_files'])}")
        report.append(f"GitOps base files: {len(self.results['gitops_base_files'])}")
        report.append("")
        
        # Resource comparison
        comparison = self.results.get('resource_comparison', {})
        if comparison:
            report.append("RESOURCE COMPARISON:")
            report.append("-" * 40)
            
            source_counts = comparison['resource_counts']['source']
            target_counts = comparison['resource_counts']['target']
            
            report.append("Resource Counts:")
            all_kinds = set(source_counts.keys()).union(set(target_counts.keys()))
            for kind in sorted(all_kinds):
                source_count = source_counts.get(kind, 0)
                target_count = target_counts.get(kind, 0)
                status = "✅" if source_count == target_count else "⚠️" if target_count > 0 else "❌"
                report.append(f"  {status} {kind}: {source_count} → {target_count}")
            
            if comparison['missing_kinds']:
                report.append(f"\n❌ Missing resource kinds in target: {', '.join(comparison['missing_kinds'])}")
            
            if comparison['extra_kinds']:
                report.append(f"\n💡 Extra resource kinds in target: {', '.join(comparison['extra_kinds'])}")
            
            # Detailed resource analysis
            report.append("\nDetailed Resource Analysis:")
            for kind, details in comparison['resource_details'].items():
                if details['missing_in_target']:
                    report.append(f"  ❌ {kind} missing in target: {len(details['missing_in_target'])} resources")
                if details['extra_in_target']:
                    report.append(f"  💡 {kind} extra in target: {len(details['extra_in_target'])} resources")
                if details['common_resources']:
                    report.append(f"  ✅ {kind} common resources: {len(details['common_resources'])}")
            report.append("")
        
        # Structural issues
        structure_issues = self.results.get('structure_issues', [])
        if structure_issues:
            report.append("STRUCTURAL ISSUES:")
            report.append("-" * 40)
            
            # Group by location
            source_issues = [issue for issue in structure_issues if issue['location'] == 'source']
            target_issues = [issue for issue in structure_issues if issue['location'] == 'target']
            
            if source_issues:
                report.append("Source (backup) issues:")
                for issue in source_issues:
                    report.append(f"  ⚠️  {issue['file']}: {issue['description']}")
            
            if target_issues:
                report.append("Target (GitOps base) issues:")
                for issue in target_issues:
                    report.append(f"  ❌ {issue['file']}: {issue['description']}")
            report.append("")
        
        # Transformation analysis
        transform_analysis = self.results.get('transformation_analysis', {})
        if transform_analysis:
            report.append("TRANSFORMATION ANALYSIS:")
            report.append("-" * 40)
            
            if transform_analysis.get('common_transformations'):
                report.append("Common transformations applied:")
                for transform in transform_analysis['common_transformations']:
                    report.append(f"  ✅ {transform.replace('_', ' ').title()}")
            
            if transform_analysis.get('quality_improvements'):
                report.append("Quality improvements:")
                for improvement in transform_analysis['quality_improvements']:
                    report.append(f"  💡 {improvement.replace('_', ' ').title()}")
            
            if transform_analysis.get('potential_issues'):
                report.append("Potential issues:")
                for issue in transform_analysis['potential_issues']:
                    report.append(f"  ⚠️  {issue}")
            report.append("")
        
        # Quality metrics
        metrics = self.results.get('quality_metrics', {})
        if metrics:
            report.append("QUALITY METRICS:")
            report.append("-" * 40)
            
            completeness = metrics.get('completeness', {})
            report.append(f"Resource preservation rate: {completeness.get('resource_preservation_rate', 0):.1f}%")
            report.append(f"Structural quality score: {completeness.get('structural_quality_score', 0):.1f}%")
            
            overall = metrics.get('overall', {})
            report.append(f"Data integrity score: {overall.get('data_integrity_score', 0):.1f}%")
            report.append(f"Transformation quality score: {overall.get('transformation_quality_score', 0):.1f}%")
            
            readiness = overall.get('readiness_for_deployment', 'unknown')
            readiness_emoji = {'ready': '✅', 'needs_review': '⚠️', 'not_ready': '❌'}.get(readiness, '❓')
            report.append(f"Deployment readiness: {readiness_emoji} {readiness.replace('_', ' ').upper()}")
            report.append("")
        
        # Errors and warnings
        if self.results['errors']:
            report.append("ERRORS:")
            report.append("-" * 40)
            for error in self.results['errors']:
                report.append(f"❌ {error}")
            report.append("")
        
        if self.results['warnings']:
            report.append("WARNINGS:")
            report.append("-" * 40)
            for warning in self.results['warnings']:
                report.append(f"⚠️  {warning}")
            report.append("")
        
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    validator = DataIntegrityValidator(project_root)
    
    results = validator.validate_data_integrity()
    
    # Generate and save report
    report = validator.generate_report()
    print(report)
    
    # Save results to JSON
    with open('data_integrity_results.json', 'w') as f:
        json.dump(results, f, indent=2, default=str)
    
    # Exit code based on validation results
    metrics = results.get('quality_metrics', {})
    overall = metrics.get('overall', {})
    readiness = overall.get('readiness_for_deployment', 'unknown')
    
    exit_code = 0 if readiness == 'ready' else 1
    print(f"\nValidation complete. Exit code: {exit_code}")
    return exit_code

if __name__ == "__main__":
    exit(main())