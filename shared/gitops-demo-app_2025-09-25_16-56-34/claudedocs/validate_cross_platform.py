#!/usr/bin/env python3
"""
Cross-Platform Compatibility Validation Script
Tests manifests work with different Kubernetes versions, ArgoCD, Flux, and manual deployment
"""
import os
import json
import subprocess
import yaml
from pathlib import Path
from typing import List, Dict, Any, Tuple

class CrossPlatformValidator:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.results = {
            'tool_versions': {},
            'kubernetes_versions': [],
            'compatibility_matrix': {},
            'portability_issues': [],
            'platform_specific_tests': {
                'argocd': [],
                'flux': [],
                'manual': []
            },
            'api_version_analysis': {},
            'deprecation_warnings': [],
            'errors': [],
            'warnings': []
        }
        self.check_tool_versions()
    
    def check_tool_versions(self):
        """Check versions of relevant tools"""
        tools = {
            'kubectl': ['kubectl', 'version', '--client'],
            'kustomize': ['kustomize', 'version'],
            'argocd': ['argocd', 'version'],
            'flux': ['flux', 'version']
        }
        
        for tool, cmd in tools.items():
            try:
                result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
                if result.returncode == 0:
                    self.results['tool_versions'][tool] = {
                        'available': True,
                        'version': result.stdout.strip(),
                        'error': None
                    }
                else:
                    self.results['tool_versions'][tool] = {
                        'available': False,
                        'version': None,
                        'error': result.stderr.strip()
                    }
            except Exception as e:
                self.results['tool_versions'][tool] = {
                    'available': False,
                    'version': None,
                    'error': str(e)
                }
        
        # Extract Kubernetes version info
        kubectl_info = self.results['tool_versions'].get('kubectl', {})
        if kubectl_info.get('available'):
            try:
                # Try to get cluster version if connected
                result = subprocess.run(['kubectl', 'version'], capture_output=True, text=True, timeout=10)
                if result.returncode == 0:
                    self.results['kubernetes_versions'].append({
                        'type': 'cluster',
                        'version': result.stdout,
                        'accessible': True
                    })
                else:
                    self.results['kubernetes_versions'].append({
                        'type': 'client_only',
                        'version': kubectl_info['version'],
                        'accessible': False
                    })
            except Exception:
                pass
    
    def analyze_api_versions(self) -> Dict[str, Any]:
        """Analyze API versions used in manifests for compatibility"""
        analysis = {
            'api_versions_used': {},
            'deprecated_apis': [],
            'removed_apis': [],
            'compatibility_issues': []
        }
        
        # Known deprecated/removed APIs across Kubernetes versions
        deprecated_apis = {
            'extensions/v1beta1': {
                'removed_in': 'v1.16',
                'replacement': 'apps/v1',
                'affects': ['Deployment', 'DaemonSet', 'ReplicaSet']
            },
            'apps/v1beta1': {
                'removed_in': 'v1.16',
                'replacement': 'apps/v1',
                'affects': ['Deployment', 'StatefulSet', 'DaemonSet']
            },
            'apps/v1beta2': {
                'removed_in': 'v1.16',
                'replacement': 'apps/v1',
                'affects': ['Deployment', 'StatefulSet', 'DaemonSet']
            }
        }
        
        # Find all YAML manifest files
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        yaml_files.extend(list(self.project_root.glob('**/*.yml')))
        
        for file_path in yaml_files:
            if 'claudedocs' in str(file_path):
                continue
                
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if doc and isinstance(doc, dict) and 'apiVersion' in doc and 'kind' in doc:
                        api_version = doc.get('apiVersion')
                        kind = doc.get('kind')
                        
                        # Track API versions
                        if api_version not in analysis['api_versions_used']:
                            analysis['api_versions_used'][api_version] = []
                        analysis['api_versions_used'][api_version].append({
                            'file': str(file_path.relative_to(self.project_root)),
                            'kind': kind
                        })
                        
                        # Check for deprecated APIs
                        if api_version in deprecated_apis:
                            deprecated_info = deprecated_apis[api_version]
                            analysis['deprecated_apis'].append({
                                'file': str(file_path.relative_to(self.project_root)),
                                'kind': kind,
                                'api_version': api_version,
                                'removed_in': deprecated_info['removed_in'],
                                'replacement': deprecated_info['replacement'],
                                'severity': 'high'
                            })
            
            except Exception as e:
                analysis['compatibility_issues'].append(f'Error analyzing {file_path}: {str(e)}')
        
        return analysis
    
    def test_argocd_compatibility(self) -> List[Dict[str, Any]]:
        """Test ArgoCD-specific compatibility"""
        tests = []
        
        # Find ArgoCD application manifests
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        for file_path in yaml_files:
            if 'claudedocs' in str(file_path):
                continue
                
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if (doc and isinstance(doc, dict) and 
                        doc.get('kind') == 'Application' and 
                        'argoproj.io' in doc.get('apiVersion', '')):
                        
                        test_result = self.validate_argocd_application(file_path, doc)
                        tests.append(test_result)
            
            except Exception as e:
                tests.append({
                    'test': 'argocd_application_parse',
                    'file': str(file_path.relative_to(self.project_root)),
                    'status': 'error',
                    'message': f'Parse error: {str(e)}'
                })
        
        # Test kustomize build compatibility (ArgoCD uses kustomize internally)
        kustomization_files = list(self.project_root.glob('**/kustomization.yaml'))
        for kustomization_file in kustomization_files:
            if 'claudedocs' in str(kustomization_file):
                continue
                
            test_result = self.test_kustomize_argocd_compatibility(kustomization_file)
            tests.append(test_result)
        
        return tests
    
    def validate_argocd_application(self, file_path: Path, app_manifest: Dict) -> Dict[str, Any]:
        """Validate ArgoCD application for compatibility"""
        test = {
            'test': 'argocd_application_validation',
            'file': str(file_path.relative_to(self.project_root)),
            'status': 'pass',
            'issues': [],
            'recommendations': []
        }
        
        spec = app_manifest.get('spec', {})
        source = spec.get('source', {})
        destination = spec.get('destination', {})
        sync_policy = spec.get('syncPolicy', {})
        
        # Check source configuration
        if not source.get('repoURL'):
            test['issues'].append('Missing source.repoURL')
            test['status'] = 'fail'
        
        if not source.get('path'):
            test['issues'].append('Missing source.path')
        
        # Check destination
        if not destination.get('server') and not destination.get('name'):
            test['issues'].append('Missing destination cluster')
            test['status'] = 'fail'
        
        # Check sync policy recommendations
        if not sync_policy:
            test['recommendations'].append('Consider adding syncPolicy for automated sync')
        
        automated = sync_policy.get('automated', {})
        if automated and not automated.get('prune'):
            test['recommendations'].append('Consider enabling prune in automated sync')
        
        # Check for ArgoCD-specific annotations
        annotations = app_manifest.get('metadata', {}).get('annotations', {})
        if 'argocd.argoproj.io/sync-wave' not in annotations:
            test['recommendations'].append('Consider adding sync-wave annotation for deployment ordering')
        
        return test
    
    def test_kustomize_argocd_compatibility(self, kustomization_file: Path) -> Dict[str, Any]:
        """Test kustomize compatibility with ArgoCD"""
        test = {
            'test': 'kustomize_argocd_build',
            'file': str(kustomization_file.relative_to(self.project_root)),
            'status': 'pass',
            'issues': [],
            'build_output': ''
        }
        
        try:
            # Test kustomize build (which ArgoCD uses internally)
            cmd = ['kustomize', 'build', str(kustomization_file.parent)]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            
            test['build_output'] = result.stdout[:1000]  # Limit output size
            
            if result.returncode != 0:
                test['status'] = 'fail'
                test['issues'].append(f'Kustomize build failed: {result.stderr}')
            else:
                # Validate the built output
                try:
                    built_manifests = list(yaml.safe_load_all(result.stdout))
                    valid_manifests = [m for m in built_manifests if m is not None]
                    
                    if not valid_manifests:
                        test['issues'].append('No valid manifests produced by kustomize build')
                        test['status'] = 'warning'
                    
                    # Check for ArgoCD compatibility issues
                    for manifest in valid_manifests:
                        if isinstance(manifest, dict):
                            # Check for missing names (ArgoCD requirement)
                            metadata = manifest.get('metadata', {})
                            if not metadata.get('name'):
                                test['issues'].append('Manifest missing metadata.name (required by ArgoCD)')
                                test['status'] = 'fail'
                
                except Exception as e:
                    test['issues'].append(f'Error validating built manifests: {str(e)}')
                    test['status'] = 'warning'
        
        except subprocess.TimeoutExpired:
            test['status'] = 'fail'
            test['issues'].append('Kustomize build timed out')
        except Exception as e:
            test['status'] = 'fail'
            test['issues'].append(f'Error running kustomize build: {str(e)}')
        
        return test
    
    def test_flux_compatibility(self) -> List[Dict[str, Any]]:
        """Test Flux-specific compatibility"""
        tests = []
        
        # Find Flux manifests
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        for file_path in yaml_files:
            if 'claudedocs' in str(file_path):
                continue
                
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if (doc and isinstance(doc, dict) and 
                        'fluxcd.io' in doc.get('apiVersion', '')):
                        
                        test_result = self.validate_flux_manifest(file_path, doc)
                        tests.append(test_result)
            
            except Exception as e:
                tests.append({
                    'test': 'flux_manifest_parse',
                    'file': str(file_path.relative_to(self.project_root)),
                    'status': 'error',
                    'message': f'Parse error: {str(e)}'
                })
        
        return tests
    
    def validate_flux_manifest(self, file_path: Path, manifest: Dict) -> Dict[str, Any]:
        """Validate Flux manifest for compatibility"""
        test = {
            'test': 'flux_manifest_validation',
            'file': str(file_path.relative_to(self.project_root)),
            'kind': manifest.get('kind'),
            'status': 'pass',
            'issues': [],
            'recommendations': []
        }
        
        kind = manifest.get('kind')
        spec = manifest.get('spec', {})
        
        # Validate based on Flux CRD kind
        if kind == 'GitRepository':
            if not spec.get('url'):
                test['issues'].append('GitRepository missing spec.url')
                test['status'] = 'fail'
            
            if not spec.get('interval'):
                test['issues'].append('GitRepository missing spec.interval')
                test['status'] = 'fail'
            
            # Check interval format
            interval = spec.get('interval', '')
            if interval and not any(unit in interval for unit in ['s', 'm', 'h']):
                test['issues'].append('GitRepository interval should include time unit (s/m/h)')
                test['status'] = 'warning'
        
        elif kind == 'Kustomization':
            if not spec.get('interval'):
                test['issues'].append('Kustomization missing spec.interval')
                test['status'] = 'fail'
            
            source_ref = spec.get('sourceRef', {})
            if not source_ref:
                test['issues'].append('Kustomization missing spec.sourceRef')
                test['status'] = 'fail'
            elif not source_ref.get('name'):
                test['issues'].append('Kustomization sourceRef missing name')
                test['status'] = 'fail'
            
            # Recommendations
            if not spec.get('prune'):
                test['recommendations'].append('Consider enabling prune for resource cleanup')
            
            if not spec.get('targetNamespace'):
                test['recommendations'].append('Consider specifying targetNamespace')
        
        return test
    
    def test_manual_deployment_compatibility(self) -> List[Dict[str, Any]]:
        """Test compatibility with manual kubectl deployment"""
        tests = []
        
        # Test kubectl apply dry-run on all manifests
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        for file_path in yaml_files:
            if ('claudedocs' in str(file_path) or 
                'kustomization' in file_path.name.lower() or
                'backup-source' in str(file_path)):
                continue
            
            # Skip non-Kubernetes manifests
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                has_k8s_resources = any(
                    doc and isinstance(doc, dict) and 
                    'apiVersion' in doc and 'kind' in doc and
                    doc.get('kind') != 'Kustomization'
                    for doc in docs
                )
                
                if not has_k8s_resources:
                    continue
                
                test_result = self.test_kubectl_apply_dry_run(file_path)
                tests.append(test_result)
            
            except Exception as e:
                tests.append({
                    'test': 'manual_deployment_parse',
                    'file': str(file_path.relative_to(self.project_root)),
                    'status': 'error',
                    'message': f'Parse error: {str(e)}'
                })
        
        return tests
    
    def test_kubectl_apply_dry_run(self, file_path: Path) -> Dict[str, Any]:
        """Test kubectl apply dry-run on a manifest"""
        test = {
            'test': 'kubectl_apply_dry_run',
            'file': str(file_path.relative_to(self.project_root)),
            'status': 'pass',
            'issues': [],
            'output': ''
        }
        
        if not self.results['tool_versions'].get('kubectl', {}).get('available'):
            test['status'] = 'skip'
            test['issues'].append('kubectl not available')
            return test
        
        try:
            cmd = ['kubectl', 'apply', '--dry-run=client', '-f', str(file_path)]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            
            test['output'] = (result.stdout + result.stderr)[:500]
            
            if result.returncode != 0:
                test['status'] = 'fail'
                test['issues'].append(f'kubectl apply dry-run failed: {result.stderr}')
            
            # Check for warnings
            output_lower = test['output'].lower()
            if 'deprecated' in output_lower:
                test['issues'].append('Contains deprecated API versions')
                if test['status'] == 'pass':
                    test['status'] = 'warning'
            
            if 'warning' in output_lower:
                test['issues'].append('kubectl reported warnings')
                if test['status'] == 'pass':
                    test['status'] = 'warning'
        
        except subprocess.TimeoutExpired:
            test['status'] = 'fail'
            test['issues'].append('kubectl apply dry-run timed out')
        except Exception as e:
            test['status'] = 'fail'
            test['issues'].append(f'Error running kubectl: {str(e)}')
        
        return test
    
    def validate_cross_platform_compatibility(self) -> Dict[str, Any]:
        """Main cross-platform validation method"""
        print("Starting cross-platform compatibility validation...")
        
        # Analyze API versions
        print("Analyzing API versions...")
        self.results['api_version_analysis'] = self.analyze_api_versions()
        
        # Test platform compatibility
        print("Testing ArgoCD compatibility...")
        self.results['platform_specific_tests']['argocd'] = self.test_argocd_compatibility()
        
        print("Testing Flux compatibility...")
        self.results['platform_specific_tests']['flux'] = self.test_flux_compatibility()
        
        print("Testing manual deployment compatibility...")
        self.results['platform_specific_tests']['manual'] = self.test_manual_deployment_compatibility()
        
        # Build compatibility matrix
        self.build_compatibility_matrix()
        
        return self.results
    
    def build_compatibility_matrix(self):
        """Build a compatibility matrix across platforms"""
        matrix = {
            'argocd': {'pass': 0, 'warning': 0, 'fail': 0, 'skip': 0, 'error': 0},
            'flux': {'pass': 0, 'warning': 0, 'fail': 0, 'skip': 0, 'error': 0},
            'manual': {'pass': 0, 'warning': 0, 'fail': 0, 'skip': 0, 'error': 0}
        }
        
        for platform, tests in self.results['platform_specific_tests'].items():
            for test in tests:
                status = test.get('status', 'error')
                if status in matrix[platform]:
                    matrix[platform][status] += 1
        
        self.results['compatibility_matrix'] = matrix
    
    def generate_report(self) -> str:
        """Generate comprehensive cross-platform compatibility report"""
        report = []
        report.append("=" * 80)
        report.append("CROSS-PLATFORM COMPATIBILITY VALIDATION REPORT")
        report.append("=" * 80)
        
        # Tool versions
        report.append("TOOL AVAILABILITY:")
        report.append("-" * 40)
        for tool, info in self.results['tool_versions'].items():
            status = "✅ Available" if info['available'] else "❌ Not Available"
            report.append(f"{tool}: {status}")
            if info['available']:
                # Clean up version output
                version = info['version'].split('\n')[0][:100]
                report.append(f"  Version: {version}")
            elif info['error']:
                report.append(f"  Error: {info['error']}")
        report.append("")
        
        # API Version Analysis
        api_analysis = self.results.get('api_version_analysis', {})
        if api_analysis:
            report.append("API VERSION ANALYSIS:")
            report.append("-" * 40)
            
            api_versions = api_analysis.get('api_versions_used', {})
            report.append(f"API versions in use: {len(api_versions)}")
            for api_version, usages in api_versions.items():
                report.append(f"  {api_version}: {len(usages)} resources")
            
            deprecated = api_analysis.get('deprecated_apis', [])
            if deprecated:
                report.append("\n❌ DEPRECATED APIS FOUND:")
                for dep in deprecated:
                    report.append(f"  {dep['file']}: {dep['kind']} uses {dep['api_version']}")
                    report.append(f"    Removed in: {dep['removed_in']}, Use: {dep['replacement']}")
            else:
                report.append("\n✅ No deprecated APIs found")
            report.append("")
        
        # Compatibility Matrix
        matrix = self.results.get('compatibility_matrix', {})
        if matrix:
            report.append("COMPATIBILITY MATRIX:")
            report.append("-" * 40)
            header = f"{'Platform':<12} {'Pass':<6} {'Warn':<6} {'Fail':<6} {'Skip':<6} {'Error':<6}"
            report.append(header)
            report.append("-" * len(header))
            
            for platform, results in matrix.items():
                line = f"{platform:<12} {results['pass']:<6} {results['warning']:<6} {results['fail']:<6} {results['skip']:<6} {results['error']:<6}"
                report.append(line)
            report.append("")
        
        # Platform-specific results
        for platform, tests in self.results['platform_specific_tests'].items():
            if not tests:
                continue
                
            report.append(f"{platform.upper()} COMPATIBILITY TESTS:")
            report.append("-" * 40)
            
            for test in tests:
                status_emoji = {
                    'pass': '✅',
                    'warning': '⚠️',
                    'fail': '❌',
                    'error': '💥',
                    'skip': '⏭️'
                }.get(test['status'], '❓')
                
                report.append(f"{status_emoji} {test['test']}: {test['file']}")
                
                if test.get('issues'):
                    for issue in test['issues']:
                        report.append(f"    ❌ {issue}")
                
                if test.get('recommendations'):
                    for rec in test['recommendations']:
                        report.append(f"    💡 {rec}")
                
                if test.get('message'):
                    report.append(f"    📝 {test['message']}")
            
            report.append("")
        
        # Summary recommendations
        report.append("PLATFORM DEPLOYMENT RECOMMENDATIONS:")
        report.append("-" * 40)
        
        # ArgoCD readiness
        argocd_results = matrix.get('argocd', {})
        if argocd_results['fail'] == 0 and argocd_results['error'] == 0:
            report.append("✅ ArgoCD: Ready for deployment")
        elif argocd_results['warning'] > 0:
            report.append("⚠️  ArgoCD: Ready with warnings - review issues")
        else:
            report.append("❌ ArgoCD: Not ready - fix critical issues")
        
        # Flux readiness
        flux_results = matrix.get('flux', {})
        if flux_results['fail'] == 0 and flux_results['error'] == 0:
            report.append("✅ Flux: Ready for deployment")
        elif flux_results['warning'] > 0:
            report.append("⚠️  Flux: Ready with warnings - review issues")
        else:
            report.append("❌ Flux: Not ready - fix critical issues")
        
        # Manual deployment readiness
        manual_results = matrix.get('manual', {})
        if manual_results['fail'] == 0 and manual_results['error'] == 0:
            report.append("✅ Manual (kubectl): Ready for deployment")
        elif manual_results['warning'] > 0:
            report.append("⚠️  Manual (kubectl): Ready with warnings - review issues")
        else:
            report.append("❌ Manual (kubectl): Not ready - fix critical issues")
        
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    validator = CrossPlatformValidator(project_root)
    
    results = validator.validate_cross_platform_compatibility()
    
    # Generate and save report
    report = validator.generate_report()
    print(report)
    
    # Save results to JSON
    with open('cross_platform_results.json', 'w') as f:
        json.dump(results, f, indent=2, default=str)
    
    # Exit code based on critical failures
    matrix = results.get('compatibility_matrix', {})
    total_fails = sum(platform.get('fail', 0) + platform.get('error', 0) for platform in matrix.values())
    
    exit_code = 1 if total_fails > 0 else 0
    print(f"\nValidation complete. Exit code: {exit_code}")
    return exit_code

if __name__ == "__main__":
    exit(main())