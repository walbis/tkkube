#!/usr/bin/env python3
"""
Comprehensive YAML Syntax Validation Script
Validates all YAML files for syntax correctness, indentation, and encoding
"""
import os
import yaml
import json
from pathlib import Path
from typing import List, Dict, Tuple, Any

class YAMLValidator:
    def __init__(self, project_root: str):
        self.project_root = Path(project_root)
        self.results = {
            'total_files': 0,
            'valid_files': 0,
            'invalid_files': 0,
            'errors': [],
            'warnings': [],
            'file_details': {}
        }
    
    def find_yaml_files(self) -> List[Path]:
        """Find all YAML files in the project"""
        yaml_files = []
        for pattern in ['**/*.yaml', '**/*.yml']:
            yaml_files.extend(self.project_root.glob(pattern))
        return sorted(yaml_files)
    
    def validate_yaml_syntax(self, file_path: Path) -> Dict[str, Any]:
        """Validate individual YAML file syntax"""
        result = {
            'file': str(file_path.relative_to(self.project_root)),
            'valid': False,
            'errors': [],
            'warnings': [],
            'documents': 0,
            'size_bytes': 0,
            'encoding': 'unknown'
        }
        
        try:
            # Get file stats
            result['size_bytes'] = file_path.stat().st_size
            
            # Read file with encoding detection
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                result['encoding'] = 'utf-8'
            except UnicodeDecodeError:
                with open(file_path, 'r', encoding='latin-1') as f:
                    content = f.read()
                result['encoding'] = 'latin-1'
                result['warnings'].append('File not UTF-8 encoded')
            
            # Parse YAML documents
            documents = list(yaml.safe_load_all(content))
            result['documents'] = len([doc for doc in documents if doc is not None])
            
            # Additional syntax checks
            if '---' in content:
                doc_separators = content.count('---')
                if doc_separators != result['documents'] - 1 and result['documents'] > 1:
                    result['warnings'].append(f'Document separator count mismatch: {doc_separators} separators for {result["documents"]} documents')
            
            # Check for common YAML issues
            lines = content.split('\n')
            for i, line in enumerate(lines, 1):
                # Check for tab characters
                if '\t' in line:
                    result['warnings'].append(f'Line {i}: Contains tab characters (should use spaces)')
                
                # Check for trailing spaces
                if line.rstrip() != line and line.strip():
                    result['warnings'].append(f'Line {i}: Contains trailing whitespace')
                
                # Check for mixed indentation
                if line.startswith(' ') and line.lstrip() != line:
                    leading_spaces = len(line) - len(line.lstrip())
                    if leading_spaces % 2 != 0:
                        result['warnings'].append(f'Line {i}: Odd number of leading spaces ({leading_spaces})')
            
            result['valid'] = True
            
        except yaml.YAMLError as e:
            result['errors'].append(f'YAML parsing error: {str(e)}')
        except Exception as e:
            result['errors'].append(f'General error: {str(e)}')
        
        return result
    
    def validate_all_files(self) -> Dict[str, Any]:
        """Validate all YAML files in the project"""
        yaml_files = self.find_yaml_files()
        self.results['total_files'] = len(yaml_files)
        
        print(f"Found {len(yaml_files)} YAML files to validate")
        
        for file_path in yaml_files:
            print(f"Validating: {file_path.relative_to(self.project_root)}")
            file_result = self.validate_yaml_syntax(file_path)
            
            if file_result['valid']:
                self.results['valid_files'] += 1
            else:
                self.results['invalid_files'] += 1
                self.results['errors'].extend([
                    f"{file_result['file']}: {error}" 
                    for error in file_result['errors']
                ])
            
            # Collect warnings
            if file_result['warnings']:
                self.results['warnings'].extend([
                    f"{file_result['file']}: {warning}" 
                    for warning in file_result['warnings']
                ])
            
            self.results['file_details'][file_result['file']] = file_result
        
        return self.results
    
    def generate_report(self) -> str:
        """Generate comprehensive validation report"""
        report = []
        report.append("=" * 80)
        report.append("YAML SYNTAX VALIDATION REPORT")
        report.append("=" * 80)
        report.append(f"Total files scanned: {self.results['total_files']}")
        report.append(f"Valid files: {self.results['valid_files']}")
        report.append(f"Invalid files: {self.results['invalid_files']}")
        report.append(f"Files with warnings: {len([f for f in self.results['file_details'].values() if f['warnings']])}")
        
        success_rate = (self.results['valid_files'] / self.results['total_files'] * 100) if self.results['total_files'] > 0 else 0
        report.append(f"Success rate: {success_rate:.1f}%")
        report.append("")
        
        # Errors section
        if self.results['errors']:
            report.append("ERRORS:")
            report.append("-" * 40)
            for error in self.results['errors']:
                report.append(f"❌ {error}")
            report.append("")
        
        # Warnings section
        if self.results['warnings']:
            report.append("WARNINGS:")
            report.append("-" * 40)
            for warning in self.results['warnings']:
                report.append(f"⚠️  {warning}")
            report.append("")
        
        # File details
        report.append("FILE DETAILS:")
        report.append("-" * 40)
        for file_path, details in self.results['file_details'].items():
            status = "✅ VALID" if details['valid'] else "❌ INVALID"
            report.append(f"{status} {file_path}")
            report.append(f"  Documents: {details['documents']}")
            report.append(f"  Size: {details['size_bytes']} bytes")
            report.append(f"  Encoding: {details['encoding']}")
            if details['errors']:
                report.append("  Errors:")
                for error in details['errors']:
                    report.append(f"    - {error}")
            if details['warnings']:
                report.append("  Warnings:")
                for warning in details['warnings']:
                    report.append(f"    - {warning}")
            report.append("")
        
        return "\n".join(report)

def main():
    project_root = os.path.dirname(os.path.abspath(__file__)) + "/.."
    validator = YAMLValidator(project_root)
    
    print("Starting YAML syntax validation...")
    results = validator.validate_all_files()
    
    # Generate and save report
    report = validator.generate_report()
    print(report)
    
    # Save results to JSON for further processing
    with open('yaml_validation_results.json', 'w') as f:
        json.dump(results, f, indent=2)
    
    # Exit with non-zero code if there are errors
    exit_code = 1 if results['invalid_files'] > 0 else 0
    print(f"\nValidation complete. Exit code: {exit_code}")
    return exit_code

if __name__ == "__main__":
    exit(main())