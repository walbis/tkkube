#!/usr/bin/env python3
"""
Master Validation Report Generator
Combines all validation results into a comprehensive report with remediation steps
"""
import os
import json
import subprocess
from pathlib import Path
from datetime import datetime
from typing import Dict, Any, List

class MasterValidationReporter:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.validation_results = {}
        self.overall_score = 0.0
        self.critical_issues = []
        self.remediation_steps = []
        
    def run_all_validations(self) -> Dict[str, Any]:
        """Run all validation scripts and collect results"""
        print("=" * 80)
        print("EXECUTING COMPREHENSIVE VALIDATION SUITE")
        print("=" * 80)
        
        validators = {
            'yaml_syntax': 'validate_yaml_syntax.py',
            'kubernetes': 'validate_kubernetes.py', 
            'gitops': 'validate_gitops.py',
            'data_integrity': 'validate_data_integrity.py',
            'cross_platform': 'validate_cross_platform.py'
        }
        
        for name, script in validators.items():
            print(f"\n🔍 Running {name} validation...")
            try:
                # Run validation script
                result = subprocess.run(
                    ['python3', script],
                    cwd=self.project_root / 'claudedocs',
                    capture_output=True,
                    text=True,
                    timeout=120
                )
                
                self.validation_results[name] = {
                    'exit_code': result.returncode,
                    'stdout': result.stdout,
                    'stderr': result.stderr,
                    'success': result.returncode == 0
                }
                
                # Try to load JSON results if available
                json_file = self.project_root / 'claudedocs' / f'{name.replace("_", "_")}_results.json'
                if json_file.exists():
                    try:
                        with open(json_file, 'r') as f:
                            self.validation_results[name]['detailed_results'] = json.load(f)
                    except Exception as e:
                        print(f"Warning: Could not load {json_file}: {e}")
                
                status = "✅ PASSED" if result.returncode == 0 else "❌ FAILED"
                print(f"   {status}")
                
            except subprocess.TimeoutExpired:
                print(f"   ⏰ TIMEOUT")
                self.validation_results[name] = {
                    'exit_code': -1,
                    'stdout': '',
                    'stderr': 'Validation timed out',
                    'success': False
                }
            except Exception as e:
                print(f"   💥 ERROR: {e}")
                self.validation_results[name] = {
                    'exit_code': -1,
                    'stdout': '',
                    'stderr': str(e),
                    'success': False
                }
        
        return self.validation_results
    
    def analyze_results(self):
        """Analyze all validation results and calculate overall score"""
        scores = {}
        
        # YAML Syntax (20% weight)
        yaml_results = self.validation_results.get('yaml_syntax', {})
        if yaml_results.get('success', False):
            scores['yaml_syntax'] = 100.0
        else:
            scores['yaml_syntax'] = 0.0
            self.critical_issues.append("YAML syntax validation failed")
        
        # Kubernetes Compliance (25% weight)
        k8s_results = self.validation_results.get('kubernetes', {})
        if k8s_results.get('success', False):
            scores['kubernetes'] = 100.0
        else:
            scores['kubernetes'] = 50.0  # Partial credit if no manifests found
            if 'kubectl' in k8s_results.get('stderr', ''):
                self.critical_issues.append("Kubernetes manifests failed validation")
        
        # GitOps Structure (25% weight)
        gitops_results = self.validation_results.get('gitops', {})
        if gitops_results.get('success', False):
            scores['gitops'] = 100.0
        else:
            scores['gitops'] = 0.0
            self.critical_issues.append("GitOps structure validation failed")
        
        # Data Integrity (20% weight) 
        data_results = self.validation_results.get('data_integrity', {})
        data_detailed = data_results.get('detailed_results', {})
        if data_detailed:
            quality_metrics = data_detailed.get('quality_metrics', {})
            overall_metrics = quality_metrics.get('overall', {})
            scores['data_integrity'] = overall_metrics.get('data_integrity_score', 0.0)
            
            readiness = overall_metrics.get('readiness_for_deployment', 'not_ready')
            if readiness == 'not_ready':
                self.critical_issues.append("Data integrity issues prevent deployment")
        else:
            scores['data_integrity'] = 0.0 if not data_results.get('success', False) else 50.0
        
        # Cross-platform (10% weight)
        cross_results = self.validation_results.get('cross_platform', {})
        if cross_results.get('success', False):
            scores['cross_platform'] = 100.0
        else:
            scores['cross_platform'] = 50.0  # Partial credit for no critical failures
        
        # Calculate weighted average
        weights = {
            'yaml_syntax': 0.20,
            'kubernetes': 0.25,
            'gitops': 0.25,
            'data_integrity': 0.20,
            'cross_platform': 0.10
        }
        
        self.overall_score = sum(scores[key] * weights[key] for key in scores.keys())
        
        return scores
    
    def generate_remediation_steps(self):
        """Generate specific remediation steps based on validation results"""
        self.remediation_steps = []
        
        # YAML Syntax Issues
        yaml_results = self.validation_results.get('yaml_syntax', {})
        if not yaml_results.get('success', False):
            self.remediation_steps.append({
                'category': 'YAML Syntax',
                'priority': 'HIGH',
                'steps': [
                    'Run python3 validate_yaml_syntax.py to identify specific syntax errors',
                    'Fix YAML indentation and formatting issues',
                    'Ensure all YAML files use UTF-8 encoding',
                    'Remove any tab characters and use spaces consistently'
                ]
            })
        
        # GitOps Structure Issues
        gitops_results = self.validation_results.get('gitops', {})
        if not gitops_results.get('success', False):
            remediation_steps = ['Run python3 validate_gitops.py to identify specific issues']
            
            # Check for specific kustomize build errors
            if 'kustomize build' in gitops_results.get('stderr', ''):
                remediation_steps.extend([
                    'Fix kustomize build errors by ensuring all referenced resources exist',
                    'Check that all resources in kustomization.yaml files have proper metadata.name fields',
                    'Verify resource file paths are correct relative to kustomization.yaml'
                ])
            
            self.remediation_steps.append({
                'category': 'GitOps Structure',
                'priority': 'HIGH',
                'steps': remediation_steps
            })
        
        # Data Integrity Issues
        data_results = self.validation_results.get('data_integrity', {})
        data_detailed = data_results.get('detailed_results', {})
        if data_detailed:
            structure_issues = data_detailed.get('structure_issues', [])
            if structure_issues:
                steps = ['Run python3 validate_data_integrity.py for detailed analysis']
                
                # Check for specific issues
                missing_names = [issue for issue in structure_issues if issue['type'] == 'missing_name']
                if missing_names:
                    steps.append('Add missing metadata.name fields to Kubernetes resources')
                
                missing_metadata = [issue for issue in structure_issues if issue['type'] == 'missing_metadata']
                if missing_metadata:
                    steps.append('Add missing metadata sections to Kubernetes resources')
                
                self.remediation_steps.append({
                    'category': 'Data Integrity',
                    'priority': 'HIGH',
                    'steps': steps
                })
        
        # Kubernetes Compliance Issues
        k8s_results = self.validation_results.get('kubernetes', {})
        if not k8s_results.get('success', False):
            self.remediation_steps.append({
                'category': 'Kubernetes Compliance',
                'priority': 'MEDIUM',
                'steps': [
                    'Run python3 validate_kubernetes.py for detailed analysis',
                    'Test manifests with kubectl apply --dry-run=client',
                    'Fix any API version compatibility issues',
                    'Ensure all required Kubernetes resource fields are present'
                ]
            })
        
        # Cross-platform Compatibility Issues
        cross_results = self.validation_results.get('cross_platform', {})
        if not cross_results.get('success', False):
            self.remediation_steps.append({
                'category': 'Cross-platform Compatibility',
                'priority': 'LOW',
                'steps': [
                    'Run python3 validate_cross_platform.py for detailed analysis',
                    'Install ArgoCD CLI for full compatibility testing: curl -sSL -o /usr/local/bin/argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-linux-amd64',
                    'Install Flux CLI for full compatibility testing: curl -s https://fluxcd.io/install.sh | sudo bash',
                    'Test deployment with your preferred GitOps tool'
                ]
            })
    
    def get_quality_grade(self, score: float) -> str:
        """Convert numerical score to letter grade"""
        if score >= 95:
            return "A+"
        elif score >= 90:
            return "A"
        elif score >= 85:
            return "A-"
        elif score >= 80:
            return "B+"
        elif score >= 75:
            return "B"
        elif score >= 70:
            return "B-"
        elif score >= 65:
            return "C+"
        elif score >= 60:
            return "C"
        elif score >= 55:
            return "C-"
        elif score >= 50:
            return "D"
        else:
            return "F"
    
    def get_deployment_readiness(self, score: float) -> tuple:
        """Get deployment readiness status and emoji"""
        if score >= 90:
            return "PRODUCTION READY", "🚀"
        elif score >= 80:
            return "STAGING READY", "🎯"
        elif score >= 70:
            return "DEVELOPMENT READY", "🔧"
        elif score >= 60:
            return "NEEDS IMPROVEMENT", "⚠️"
        else:
            return "NOT READY", "❌"
    
    def generate_master_report(self) -> str:
        """Generate the comprehensive master validation report"""
        report = []
        
        # Header
        report.append("=" * 100)
        report.append("COMPREHENSIVE GITOPS VALIDATION REPORT")
        report.append("=" * 100)
        report.append(f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        report.append(f"Project: {self.project_root.name}")
        report.append("")
        
        # Executive Summary
        scores = self.analyze_results()
        self.generate_remediation_steps()
        
        grade = self.get_quality_grade(self.overall_score)
        readiness, emoji = self.get_deployment_readiness(self.overall_score)
        
        report.append("🎯 EXECUTIVE SUMMARY")
        report.append("=" * 50)
        report.append(f"Overall Quality Score: {self.overall_score:.1f}/100 ({grade})")
        report.append(f"Deployment Readiness: {emoji} {readiness}")
        report.append("")
        
        # Critical Issues
        if self.critical_issues:
            report.append("🚨 CRITICAL ISSUES")
            report.append("=" * 30)
            for i, issue in enumerate(self.critical_issues, 1):
                report.append(f"{i}. {issue}")
            report.append("")
        else:
            report.append("✅ NO CRITICAL ISSUES FOUND")
            report.append("")
        
        # Validation Summary
        report.append("📊 VALIDATION SUMMARY")
        report.append("=" * 40)
        
        validation_header = f"{'Component':<25} {'Score':<10} {'Status':<10} {'Details'}"
        report.append(validation_header)
        report.append("-" * len(validation_header))
        
        for name, result in self.validation_results.items():
            score = scores.get(name, 0.0)
            status = "✅ PASS" if result['success'] else "❌ FAIL"
            exit_code = result.get('exit_code', -1)
            
            component_name = name.replace('_', ' ').title()
            line = f"{component_name:<25} {score:>6.1f}/100 {status:<10} Exit: {exit_code}"
            report.append(line)
        
        report.append("")
        
        # Detailed Results
        report.append("📋 DETAILED VALIDATION RESULTS")
        report.append("=" * 50)
        
        for name, result in self.validation_results.items():
            component_name = name.replace('_', ' ').title()
            report.append(f"\n🔍 {component_name} Validation:")
            report.append("-" * (len(component_name) + 15))
            
            if result['success']:
                report.append("✅ Status: PASSED")
            else:
                report.append("❌ Status: FAILED")
                if result['stderr']:
                    report.append("Error Details:")
                    # Limit error output to avoid overly long reports
                    error_lines = result['stderr'].split('\n')[:10]
                    for line in error_lines:
                        if line.strip():
                            report.append(f"  {line}")
                    if len(result['stderr'].split('\n')) > 10:
                        report.append("  ... (output truncated)")
            
            # Add key metrics if available
            detailed_results = result.get('detailed_results', {})
            if detailed_results:
                if name == 'yaml_syntax':
                    total = detailed_results.get('total_files', 0)
                    valid = detailed_results.get('valid_files', 0)
                    report.append(f"Files processed: {total}, Valid: {valid}")
                
                elif name == 'data_integrity':
                    quality_metrics = detailed_results.get('quality_metrics', {})
                    if quality_metrics:
                        completeness = quality_metrics.get('completeness', {})
                        preservation_rate = completeness.get('resource_preservation_rate', 0)
                        report.append(f"Resource preservation rate: {preservation_rate:.1f}%")
                
                elif name == 'gitops':
                    kustomization_files = detailed_results.get('kustomization_files', [])
                    report.append(f"Kustomization files found: {len(kustomization_files)}")
        
        # Remediation Steps
        if self.remediation_steps:
            report.append(f"\n🔧 REMEDIATION STEPS")
            report.append("=" * 30)
            
            for i, remediation in enumerate(self.remediation_steps, 1):
                report.append(f"\n{i}. {remediation['category']} (Priority: {remediation['priority']})")
                for j, step in enumerate(remediation['steps'], 1):
                    report.append(f"   {j}. {step}")
        
        # Recommendations
        report.append(f"\n💡 RECOMMENDATIONS")
        report.append("=" * 30)
        
        if self.overall_score >= 90:
            report.append("🚀 Your GitOps setup is production-ready!")
            report.append("   - Consider implementing monitoring and alerting")
            report.append("   - Set up automated backup and disaster recovery procedures")
            report.append("   - Document deployment and rollback procedures")
            
        elif self.overall_score >= 80:
            report.append("🎯 Your GitOps setup is staging-ready with minor improvements needed:")
            report.append("   - Address any remaining validation warnings")
            report.append("   - Test deployment in a staging environment")
            report.append("   - Prepare production deployment checklist")
            
        elif self.overall_score >= 70:
            report.append("🔧 Your GitOps setup is development-ready but needs work for production:")
            report.append("   - Fix all critical validation issues")
            report.append("   - Implement proper resource naming and labeling")
            report.append("   - Add resource limits and security contexts")
            
        else:
            report.append("⚠️ Your GitOps setup needs significant improvements:")
            report.append("   - Focus on fixing critical validation failures")
            report.append("   - Ensure proper Kubernetes resource structure")
            report.append("   - Test basic functionality before proceeding")
        
        # Next Steps
        report.append(f"\n📋 NEXT STEPS")
        report.append("=" * 25)
        report.append("1. Address all critical issues identified above")
        report.append("2. Re-run validation suite to verify fixes")
        report.append("3. Test deployment in development environment") 
        report.append("4. Implement CI/CD pipeline with validation gates")
        report.append("5. Deploy to staging/production when score > 90")
        
        # File Summary
        report.append(f"\n📁 PROJECT STRUCTURE SUMMARY")
        report.append("=" * 40)
        
        # Count different file types
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        kustomization_files = list(self.project_root.glob('**/kustomization.yaml'))
        
        report.append(f"Total YAML files: {len(yaml_files)}")
        report.append(f"Kustomization files: {len(kustomization_files)}")
        report.append(f"Directory structure:")
        
        key_dirs = ['base', 'overlays', 'argocd', 'flux', 'backup-source']
        for dir_name in key_dirs:
            dir_path = self.project_root / dir_name
            if dir_path.exists():
                file_count = len(list(dir_path.glob('*.yaml')))
                report.append(f"  {dir_name}/: {file_count} files")
        
        report.append("")
        report.append("=" * 100)
        report.append("END OF VALIDATION REPORT")
        report.append("=" * 100)
        
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    reporter = MasterValidationReporter(project_root)
    
    # Run all validations
    results = reporter.run_all_validations()
    
    # Generate comprehensive report
    master_report = reporter.generate_master_report()
    
    print("\n")
    print(master_report)
    
    # Save master report
    report_file = Path(project_root) / 'claudedocs' / 'COMPREHENSIVE_VALIDATION_REPORT.md'
    with open(report_file, 'w') as f:
        f.write(master_report)
    
    # Save detailed results
    results_file = Path(project_root) / 'claudedocs' / 'master_validation_results.json'
    with open(results_file, 'w') as f:
        json.dump({
            'overall_score': reporter.overall_score,
            'critical_issues': reporter.critical_issues,
            'remediation_steps': reporter.remediation_steps,
            'validation_results': results
        }, f, indent=2)
    
    print(f"\n📄 Reports saved to:")
    print(f"   - {report_file}")
    print(f"   - {results_file}")
    
    # Exit with appropriate code
    exit_code = 0 if reporter.overall_score >= 70 else 1
    print(f"\n🏁 Master validation complete. Overall score: {reporter.overall_score:.1f}/100")
    print(f"   Exit code: {exit_code}")
    
    return exit_code

if __name__ == "__main__":
    exit(main())