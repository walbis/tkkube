#!/usr/bin/env python3
"""
GitOps Structure Integrity Validation Script
Validates Kustomization builds, ArgoCD applications, and Flux manifests
"""
import os
import json
import subprocess
import yaml
from pathlib import Path
from typing import List, Dict, Any, Set

class GitOpsValidator:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.results = {
            'kustomize_available': False,
            'kustomization_files': [],
            'kustomize_build_results': {},
            'argocd_applications': [],
            'flux_manifests': [],
            'directory_structure': {},
            'resource_references': {},
            'errors': [],
            'warnings': []
        }
        self.check_tools()
    
    def check_tools(self):
        """Check if required tools are available"""
        try:
            result = subprocess.run(['kustomize', 'version'], 
                                  capture_output=True, text=True, timeout=10)
            self.results['kustomize_available'] = result.returncode == 0
            if self.results['kustomize_available']:
                print("✅ kustomize is available")
            else:
                print("❌ kustomize not available - build tests will be skipped")
        except Exception:
            print("❌ kustomize not available - build tests will be skipped")
            self.results['kustomize_available'] = False
    
    def find_kustomization_files(self) -> List[Path]:
        """Find all kustomization files"""
        kustomization_files = []
        
        # Look for kustomization.yaml and kustomization.yml files
        for pattern in ['**/kustomization.yaml', '**/kustomization.yml']:
            kustomization_files.extend(self.project_root.glob(pattern))
        
        # Filter out files in excluded directories
        exclude_dirs = ['node_modules', '.git']
        filtered_files = []
        for file_path in kustomization_files:
            if not any(exclude_dir in str(file_path) for exclude_dir in exclude_dirs):
                filtered_files.append(file_path)
        
        return sorted(filtered_files)
    
    def validate_kustomization_file(self, file_path: Path) -> Dict[str, Any]:
        """Validate individual kustomization file"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'valid': False,
            'errors': [],
            'warnings': [],
            'resources': [],
            'missing_resources': [],
            'patches': [],
            'missing_patches': [],
            'bases': [],
            'missing_bases': []
        }
        
        try:
            with open(file_path, 'r') as f:
                kustomization = yaml.safe_load(f)
            
            if not kustomization:
                result['errors'].append('Empty kustomization file')
                return result
            
            result['valid'] = True
            base_dir = file_path.parent
            
            # Check resources
            resources = kustomization.get('resources', [])
            result['resources'] = resources
            
            for resource in resources:
                resource_path = base_dir / resource
                if not resource_path.exists():
                    result['missing_resources'].append(resource)
                    result['warnings'].append(f'Missing resource: {resource}')
            
            # Check patches (both old and new format)
            patches = kustomization.get('patches', [])
            strategic_merge_patches = kustomization.get('patchesStrategicMerge', [])
            json6902_patches = kustomization.get('patchesJson6902', [])
            
            all_patches = patches + strategic_merge_patches
            for patch_info in json6902_patches:
                if isinstance(patch_info, dict) and 'path' in patch_info:
                    all_patches.append(patch_info['path'])
            
            result['patches'] = all_patches
            
            for patch in all_patches:
                if isinstance(patch, str):
                    patch_path = base_dir / patch
                    if not patch_path.exists():
                        result['missing_patches'].append(patch)
                        result['warnings'].append(f'Missing patch file: {patch}')
                elif isinstance(patch, dict) and 'path' in patch:
                    patch_path = base_dir / patch['path']
                    if not patch_path.exists():
                        result['missing_patches'].append(patch['path'])
                        result['warnings'].append(f'Missing patch file: {patch["path"]}')
            
            # Check bases (deprecated but still used)
            bases = kustomization.get('bases', [])
            result['bases'] = bases
            
            if bases:
                result['warnings'].append('Using deprecated "bases" field, consider using "resources"')
            
            for base in bases:
                base_path = base_dir / base
                if not base_path.exists():
                    result['missing_bases'].append(base)
                    result['warnings'].append(f'Missing base: {base}')
                elif not (base_path / 'kustomization.yaml').exists() and not (base_path / 'kustomization.yml').exists():
                    result['warnings'].append(f'Base directory missing kustomization file: {base}')
            
            # Check for common configuration issues
            self.check_kustomization_best_practices(kustomization, result)
            
        except Exception as e:
            result['errors'].append(f'Error parsing kustomization: {str(e)}')
        
        return result
    
    def check_kustomization_best_practices(self, kustomization: dict, result: dict):
        """Check kustomization best practices"""
        # Check for namespace configuration
        if 'namespace' not in kustomization and 'resources' in kustomization:
            result['warnings'].append('Consider setting a namespace for resources')
        
        # Check for proper naming
        if 'namePrefix' in kustomization or 'nameSuffix' in kustomization:
            if not kustomization.get('commonLabels'):
                result['warnings'].append('Using name prefix/suffix without common labels')
        
        # Check for image management
        images = kustomization.get('images', [])
        if images:
            for image in images:
                if isinstance(image, dict):
                    if 'name' not in image:
                        result['warnings'].append('Image configuration missing name field')
    
    def test_kustomize_build(self, kustomization_dir: Path) -> Dict[str, Any]:
        """Test kustomize build for a directory"""
        result = {
            'directory': str(kustomization_dir.relative_to(self.project_root)),
            'build_successful': False,
            'output': '',
            'errors': [],
            'manifest_count': 0,
            'resource_types': set()
        }
        
        if not self.results['kustomize_available']:
            result['errors'].append('kustomize not available')
            return result
        
        try:
            cmd = ['kustomize', 'build', str(kustomization_dir)]
            process = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
            
            result['output'] = process.stdout
            result['build_successful'] = process.returncode == 0
            
            if not result['build_successful']:
                result['errors'].append(f'Build failed: {process.stderr}')
            else:
                # Parse the output to count manifests and resource types
                try:
                    manifests = list(yaml.safe_load_all(result['output']))
                    valid_manifests = [m for m in manifests if m is not None]
                    result['manifest_count'] = len(valid_manifests)
                    
                    for manifest in valid_manifests:
                        if isinstance(manifest, dict) and 'kind' in manifest:
                            result['resource_types'].add(manifest['kind'])
                    
                    result['resource_types'] = list(result['resource_types'])
                except Exception as e:
                    result['errors'].append(f'Error parsing build output: {str(e)}')
            
        except subprocess.TimeoutExpired:
            result['errors'].append('Build timed out')
        except Exception as e:
            result['errors'].append(f'Build error: {str(e)}')
        
        return result
    
    def validate_argocd_applications(self) -> List[Dict[str, Any]]:
        """Validate ArgoCD application manifests"""
        argocd_apps = []
        
        # Find ArgoCD application files
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        yaml_files.extend(list(self.project_root.glob('**/*.yml')))
        
        for file_path in yaml_files:
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if (doc and isinstance(doc, dict) and 
                        doc.get('kind') == 'Application' and 
                        'argoproj.io' in doc.get('apiVersion', '')):
                        
                        app_result = self.validate_argocd_application(file_path, doc)
                        argocd_apps.append(app_result)
                        
            except Exception as e:
                self.results['warnings'].append(f'Error reading {file_path}: {str(e)}')
        
        return argocd_apps
    
    def validate_argocd_application(self, file_path: Path, app_manifest: dict) -> Dict[str, Any]:
        """Validate individual ArgoCD application"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'name': app_manifest.get('metadata', {}).get('name', 'unknown'),
            'valid': True,
            'errors': [],
            'warnings': [],
            'source_path_exists': False,
            'destination_cluster': '',
            'destination_namespace': ''
        }
        
        # Check required fields
        spec = app_manifest.get('spec', {})
        source = spec.get('source', {})
        destination = spec.get('destination', {})
        
        # Validate source
        repo_url = source.get('repoURL', '')
        path = source.get('path', '')
        
        if not repo_url:
            result['errors'].append('Missing source.repoURL')
            result['valid'] = False
        
        if path and not path.startswith('http'):
            # Check if path exists locally (assuming it's a local path)
            source_path = self.project_root / path
            result['source_path_exists'] = source_path.exists()
            if not result['source_path_exists']:
                result['warnings'].append(f'Source path does not exist locally: {path}')
        
        # Validate destination
        result['destination_cluster'] = destination.get('server', destination.get('name', ''))
        result['destination_namespace'] = destination.get('namespace', '')
        
        if not result['destination_cluster']:
            result['errors'].append('Missing destination cluster')
            result['valid'] = False
        
        if not result['destination_namespace']:
            result['warnings'].append('No destination namespace specified')
        
        # Check sync policy
        sync_policy = spec.get('syncPolicy', {})
        if not sync_policy:
            result['warnings'].append('No sync policy defined')
        
        return result
    
    def validate_flux_manifests(self) -> List[Dict[str, Any]]:
        """Validate Flux v2 manifests"""
        flux_manifests = []
        
        # Find Flux manifest files
        yaml_files = list(self.project_root.glob('**/*.yaml'))
        yaml_files.extend(list(self.project_root.glob('**/*.yml')))
        
        flux_kinds = ['GitRepository', 'Kustomization', 'HelmRepository', 'HelmRelease']
        
        for file_path in yaml_files:
            try:
                with open(file_path, 'r') as f:
                    content = f.read()
                
                docs = list(yaml.safe_load_all(content))
                for doc in docs:
                    if (doc and isinstance(doc, dict) and 
                        doc.get('kind') in flux_kinds and 
                        'fluxcd.io' in doc.get('apiVersion', '')):
                        
                        flux_result = self.validate_flux_manifest(file_path, doc)
                        flux_manifests.append(flux_result)
                        
            except Exception as e:
                self.results['warnings'].append(f'Error reading {file_path}: {str(e)}')
        
        return flux_manifests
    
    def validate_flux_manifest(self, file_path: Path, manifest: dict) -> Dict[str, Any]:
        """Validate individual Flux manifest"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'kind': manifest.get('kind'),
            'name': manifest.get('metadata', {}).get('name', 'unknown'),
            'valid': True,
            'errors': [],
            'warnings': []
        }
        
        kind = manifest.get('kind')
        spec = manifest.get('spec', {})
        
        if kind == 'GitRepository':
            # Validate Git repository spec
            if not spec.get('url'):
                result['errors'].append('Missing spec.url')
                result['valid'] = False
            
            if not spec.get('interval'):
                result['errors'].append('Missing spec.interval')
                result['valid'] = False
            
            if not spec.get('ref'):
                result['warnings'].append('No Git ref specified, will use default branch')
        
        elif kind == 'Kustomization':
            # Validate Kustomization spec
            if not spec.get('interval'):
                result['errors'].append('Missing spec.interval')
                result['valid'] = False
            
            source_ref = spec.get('sourceRef')
            if not source_ref:
                result['errors'].append('Missing spec.sourceRef')
                result['valid'] = False
            elif not source_ref.get('name'):
                result['errors'].append('Missing spec.sourceRef.name')
                result['valid'] = False
        
        return result
    
    def analyze_directory_structure(self) -> Dict[str, Any]:
        """Analyze GitOps directory structure"""
        structure = {
            'has_base_directory': False,
            'has_overlays_directory': False,
            'overlay_environments': [],
            'argocd_directory': False,
            'flux_directory': False,
            'recommendations': []
        }
        
        # Check for base directory
        base_dir = self.project_root / 'base'
        structure['has_base_directory'] = base_dir.exists() and base_dir.is_dir()
        
        # Check for overlays directory
        overlays_dir = self.project_root / 'overlays'
        structure['has_overlays_directory'] = overlays_dir.exists() and overlays_dir.is_dir()
        
        if structure['has_overlays_directory']:
            for item in overlays_dir.iterdir():
                if item.is_dir():
                    structure['overlay_environments'].append(item.name)
        
        # Check for ArgoCD directory
        argocd_dir = self.project_root / 'argocd'
        structure['argocd_directory'] = argocd_dir.exists() and argocd_dir.is_dir()
        
        # Check for Flux directory
        flux_dir = self.project_root / 'flux'
        structure['flux_directory'] = flux_dir.exists() and flux_dir.is_dir()
        
        # Generate recommendations
        if not structure['has_base_directory']:
            structure['recommendations'].append('Consider creating a base/ directory for common resources')
        
        if not structure['has_overlays_directory']:
            structure['recommendations'].append('Consider creating overlays/ directory for environment-specific configurations')
        
        if not structure['overlay_environments']:
            structure['recommendations'].append('Consider creating environment-specific overlays (dev, staging, prod)')
        
        common_envs = ['development', 'staging', 'production']
        missing_envs = [env for env in common_envs if env not in structure['overlay_environments']]
        if missing_envs:
            structure['recommendations'].append(f'Consider adding common environments: {missing_envs}')
        
        return structure
    
    def validate_all_gitops_components(self) -> Dict[str, Any]:
        """Validate all GitOps components"""
        print("Starting GitOps structure validation...")
        
        # Find and validate kustomization files
        kustomization_files = self.find_kustomization_files()
        self.results['kustomization_files'] = [str(f.relative_to(self.project_root)) for f in kustomization_files]
        
        print(f"Found {len(kustomization_files)} kustomization files")
        
        for kustomization_file in kustomization_files:
            print(f"Validating kustomization: {kustomization_file.relative_to(self.project_root)}")
            
            # Validate kustomization file
            validation_result = self.validate_kustomization_file(kustomization_file)
            file_key = str(kustomization_file.relative_to(self.project_root))
            self.results['resource_references'][file_key] = validation_result
            
            # Test kustomize build
            if self.results['kustomize_available']:
                build_result = self.test_kustomize_build(kustomization_file.parent)
                self.results['kustomize_build_results'][file_key] = build_result
                
                if not build_result['build_successful']:
                    self.results['errors'].extend([
                        f"{build_result['directory']}: {error}" 
                        for error in build_result['errors']
                    ])
        
        # Validate ArgoCD applications
        print("Validating ArgoCD applications...")
        self.results['argocd_applications'] = self.validate_argocd_applications()
        
        # Validate Flux manifests
        print("Validating Flux manifests...")
        self.results['flux_manifests'] = self.validate_flux_manifests()
        
        # Analyze directory structure
        print("Analyzing directory structure...")
        self.results['directory_structure'] = self.analyze_directory_structure()
        
        return self.results
    
    def generate_report(self) -> str:
        """Generate comprehensive GitOps validation report"""
        report = []
        report.append("=" * 80)
        report.append("GITOPS STRUCTURE INTEGRITY VALIDATION REPORT")
        report.append("=" * 80)
        
        # Tool availability
        report.append(f"kustomize available: {'Yes' if self.results['kustomize_available'] else 'No'}")
        report.append("")
        
        # Directory structure analysis
        structure = self.results['directory_structure']
        report.append("DIRECTORY STRUCTURE ANALYSIS:")
        report.append("-" * 40)
        report.append(f"Base directory: {'✅ Present' if structure['has_base_directory'] else '❌ Missing'}")
        report.append(f"Overlays directory: {'✅ Present' if structure['has_overlays_directory'] else '❌ Missing'}")
        report.append(f"ArgoCD directory: {'✅ Present' if structure['argocd_directory'] else '❌ Missing'}")
        report.append(f"Flux directory: {'✅ Present' if structure['flux_directory'] else '❌ Missing'}")
        
        if structure['overlay_environments']:
            report.append(f"Overlay environments: {', '.join(structure['overlay_environments'])}")
        
        if structure['recommendations']:
            report.append("Recommendations:")
            for rec in structure['recommendations']:
                report.append(f"  💡 {rec}")
        report.append("")
        
        # Kustomization files validation
        report.append("KUSTOMIZATION FILES VALIDATION:")
        report.append("-" * 40)
        report.append(f"Total kustomization files: {len(self.results['kustomization_files'])}")
        
        for file_path in self.results['kustomization_files']:
            report.append(f"\n📄 {file_path}")
            
            # Resource reference validation
            if file_path in self.results['resource_references']:
                ref_result = self.results['resource_references'][file_path]
                status = "✅ VALID" if ref_result['valid'] else "❌ INVALID"
                report.append(f"  {status} Resource references")
                
                if ref_result['resources']:
                    report.append(f"  Resources: {len(ref_result['resources'])} found")
                if ref_result['missing_resources']:
                    report.append(f"  ❌ Missing resources: {ref_result['missing_resources']}")
                
                if ref_result['patches']:
                    report.append(f"  Patches: {len(ref_result['patches'])} found")
                if ref_result['missing_patches']:
                    report.append(f"  ❌ Missing patches: {ref_result['missing_patches']}")
                
                for error in ref_result['errors']:
                    report.append(f"  ❌ {error}")
                for warning in ref_result['warnings']:
                    report.append(f"  ⚠️  {warning}")
            
            # Kustomize build results
            if file_path in self.results['kustomize_build_results']:
                build_result = self.results['kustomize_build_results'][file_path]
                status = "✅ SUCCESS" if build_result['build_successful'] else "❌ FAILED"
                report.append(f"  {status} Kustomize build")
                
                if build_result['build_successful']:
                    report.append(f"    Manifests generated: {build_result['manifest_count']}")
                    if build_result['resource_types']:
                        report.append(f"    Resource types: {', '.join(build_result['resource_types'])}")
                
                for error in build_result['errors']:
                    report.append(f"    ❌ {error}")
        
        # ArgoCD applications
        if self.results['argocd_applications']:
            report.append(f"\nARGOCD APPLICATIONS:")
            report.append("-" * 40)
            report.append(f"Total applications: {len(self.results['argocd_applications'])}")
            
            for app in self.results['argocd_applications']:
                status = "✅ VALID" if app['valid'] else "❌ INVALID"
                report.append(f"{status} {app['name']} ({app['file']})")
                report.append(f"  Destination: {app['destination_cluster']}/{app['destination_namespace']}")
                
                for error in app['errors']:
                    report.append(f"  ❌ {error}")
                for warning in app['warnings']:
                    report.append(f"  ⚠️  {warning}")
        
        # Flux manifests
        if self.results['flux_manifests']:
            report.append(f"\nFLUX MANIFESTS:")
            report.append("-" * 40)
            report.append(f"Total Flux manifests: {len(self.results['flux_manifests'])}")
            
            for manifest in self.results['flux_manifests']:
                status = "✅ VALID" if manifest['valid'] else "❌ INVALID"
                report.append(f"{status} {manifest['kind']}/{manifest['name']} ({manifest['file']})")
                
                for error in manifest['errors']:
                    report.append(f"  ❌ {error}")
                for warning in manifest['warnings']:
                    report.append(f"  ⚠️  {warning}")
        
        # Summary of all errors and warnings
        if self.results['errors']:
            report.append(f"\nALL ERRORS:")
            report.append("-" * 40)
            for error in self.results['errors']:
                report.append(f"❌ {error}")
        
        if self.results['warnings']:
            report.append(f"\nALL WARNINGS:")
            report.append("-" * 40)
            for warning in self.results['warnings']:
                report.append(f"⚠️  {warning}")
        
        report.append("")
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    validator = GitOpsValidator(project_root)
    
    results = validator.validate_all_gitops_components()
    
    # Generate and save report
    report = validator.generate_report()
    print(report)
    
    # Save results to JSON
    with open('gitops_validation_results.json', 'w') as f:
        json.dump(results, f, indent=2, default=str)
    
    # Exit code based on validation results
    exit_code = 1 if results['errors'] else 0
    print(f"\nValidation complete. Exit code: {exit_code}")
    return exit_code

if __name__ == "__main__":
    exit(main())