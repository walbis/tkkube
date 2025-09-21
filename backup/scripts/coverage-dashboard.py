#!/usr/bin/env python3

"""
Coverage Dashboard Generator
Generates an interactive HTML dashboard for coverage analysis and trends
"""

import json
import os
import sys
import csv
import argparse
from datetime import datetime, timedelta
from pathlib import Path
import urllib.parse

def load_coverage_data(coverage_dir):
    """Load the latest coverage data from JSON report"""
    reports_dir = coverage_dir / "reports"
    latest_json = reports_dir / "latest.json"
    
    if not latest_json.exists():
        print(f"Error: Latest coverage report not found at {latest_json}")
        return None
    
    try:
        with open(latest_json, 'r') as f:
            return json.load(f)
    except Exception as e:
        print(f"Error loading coverage data: {e}")
        return None

def load_trend_data(coverage_dir):
    """Load coverage trend data from CSV"""
    trends_dir = coverage_dir / "trends"
    trend_file = trends_dir / "coverage_trends.csv"
    
    if not trend_file.exists():
        return []
    
    trends = []
    try:
        with open(trend_file, 'r') as f:
            reader = csv.DictReader(f)
            for row in reader:
                trends.append({
                    'timestamp': row['timestamp'],
                    'coverage': float(row['coverage']),
                    'commit_hash': row['commit_hash']
                })
    except Exception as e:
        print(f"Warning: Error loading trend data: {e}")
    
    return trends

def generate_module_chart_data(coverage_data):
    """Generate data for module coverage chart"""
    modules = coverage_data.get('modules', [])
    
    chart_data = {
        'labels': [],
        'coverage': [],
        'thresholds': [],
        'colors': []
    }
    
    for module in modules:
        name = module['name'].split('/')[-1]  # Get last part of module path
        chart_data['labels'].append(name)
        chart_data['coverage'].append(module['coverage'])
        chart_data['thresholds'].append(module['threshold'])
        
        # Color based on status
        if module['coverage'] >= module['threshold']:
            if module.get('is_critical', False):
                chart_data['colors'].append('#28a745')  # Green for passing critical
            else:
                chart_data['colors'].append('#6f42c1')  # Purple for passing normal
        else:
            if module.get('is_critical', False):
                chart_data['colors'].append('#dc3545')  # Red for failing critical
            else:
                chart_data['colors'].append('#fd7e14')  # Orange for failing normal
    
    return chart_data

def generate_trend_chart_data(trend_data, days=30):
    """Generate data for coverage trend chart"""
    if not trend_data:
        return {'labels': [], 'coverage': []}
    
    # Filter to last N days
    cutoff_date = datetime.now() - timedelta(days=days)
    
    filtered_trends = []
    for trend in trend_data:
        try:
            trend_date = datetime.fromisoformat(trend['timestamp'].replace('Z', '+00:00'))
            if trend_date >= cutoff_date:
                filtered_trends.append(trend)
        except:
            # Skip invalid dates
            continue
    
    # Sort by timestamp
    filtered_trends.sort(key=lambda x: x['timestamp'])
    
    chart_data = {
        'labels': [],
        'coverage': []
    }
    
    for trend in filtered_trends[-50:]:  # Last 50 data points
        # Format timestamp for display
        try:
            dt = datetime.fromisoformat(trend['timestamp'].replace('Z', '+00:00'))
            chart_data['labels'].append(dt.strftime('%m/%d %H:%M'))
            chart_data['coverage'].append(trend['coverage'])
        except:
            continue
    
    return chart_data

def generate_dashboard_html(coverage_data, trend_data, output_file):
    """Generate the HTML dashboard"""
    
    module_chart_data = generate_module_chart_data(coverage_data)
    trend_chart_data = generate_trend_chart_data(trend_data)
    
    # Calculate summary statistics
    overall_coverage = coverage_data.get('overall_coverage', 0)
    global_threshold = coverage_data.get('global_threshold', 80)
    critical_threshold = coverage_data.get('critical_threshold', 90)
    
    modules = coverage_data.get('modules', [])
    total_modules = len(modules)
    passing_modules = len([m for m in modules if m['coverage'] >= m['threshold']])
    critical_modules = len([m for m in modules if m.get('is_critical', False)])
    critical_passing = len([m for m in modules if m.get('is_critical', False) and m['coverage'] >= critical_threshold])
    
    # Determine overall status
    overall_status = "success" if coverage_data.get('meets_global_threshold', False) else "danger"
    overall_icon = "✅" if overall_status == "success" else "❌"
    
    html_content = f"""
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Coverage Dashboard - Kubernetes Backup System</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.1.3/dist/css/bootstrap.min.css" rel="stylesheet">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        .metric-card {{
            transition: transform 0.2s;
        }}
        .metric-card:hover {{
            transform: translateY(-5px);
        }}
        .status-badge {{
            font-size: 1.2em;
        }}
        .chart-container {{
            position: relative;
            height: 400px;
            margin: 20px 0;
        }}
        .critical-path {{
            border-left: 4px solid #dc3545;
        }}
        .normal-path {{
            border-left: 4px solid #6f42c1;
        }}
    </style>
</head>
<body>
    <div class="container-fluid">
        <header class="bg-primary text-white p-4 mb-4">
            <div class="row align-items-center">
                <div class="col">
                    <h1 class="h2 mb-0">
                        <i class="fas fa-chart-line"></i>
                        Coverage Dashboard
                    </h1>
                    <p class="mb-0">Kubernetes Backup and Restore System</p>
                </div>
                <div class="col-auto">
                    <small>Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</small>
                </div>
            </div>
        </header>

        <!-- Summary Cards -->
        <div class="row mb-4">
            <div class="col-md-3">
                <div class="card metric-card bg-{overall_status} text-white">
                    <div class="card-body text-center">
                        <h3 class="card-title">{overall_icon} {overall_coverage:.1f}%</h3>
                        <p class="card-text">Overall Coverage</p>
                        <small>Target: {global_threshold}%</small>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card metric-card bg-info text-white">
                    <div class="card-body text-center">
                        <h3 class="card-title">{passing_modules}/{total_modules}</h3>
                        <p class="card-text">Modules Passing</p>
                        <small>{(passing_modules/total_modules*100):.1f}% pass rate</small>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card metric-card bg-warning text-dark">
                    <div class="card-body text-center">
                        <h3 class="card-title">{critical_passing}/{critical_modules}</h3>
                        <p class="card-text">Critical Paths</p>
                        <small>Target: {critical_threshold}%</small>
                    </div>
                </div>
            </div>
            <div class="col-md-3">
                <div class="card metric-card bg-secondary text-white">
                    <div class="card-body text-center">
                        <h3 class="card-title">{len(trend_data)}</h3>
                        <p class="card-text">Trend Points</p>
                        <small>Historical data</small>
                    </div>
                </div>
            </div>
        </div>

        <!-- Charts Row -->
        <div class="row">
            <div class="col-lg-8">
                <div class="card">
                    <div class="card-header">
                        <h5 class="card-title mb-0">Module Coverage Analysis</h5>
                    </div>
                    <div class="card-body">
                        <div class="chart-container">
                            <canvas id="moduleChart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
            <div class="col-lg-4">
                <div class="card">
                    <div class="card-header">
                        <h5 class="card-title mb-0">Coverage Trend (30 days)</h5>
                    </div>
                    <div class="card-body">
                        <div class="chart-container">
                            <canvas id="trendChart"></canvas>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Module Details -->
        <div class="row mt-4">
            <div class="col-12">
                <div class="card">
                    <div class="card-header">
                        <h5 class="card-title mb-0">Module Details</h5>
                    </div>
                    <div class="card-body">
                        <div class="table-responsive">
                            <table class="table table-striped">
                                <thead>
                                    <tr>
                                        <th>Module</th>
                                        <th>Coverage</th>
                                        <th>Threshold</th>
                                        <th>Status</th>
                                        <th>Type</th>
                                        <th>Gap</th>
                                    </tr>
                                </thead>
                                <tbody>
"""

    # Add module rows
    for module in modules:
        status_class = "success" if module['coverage'] >= module['threshold'] else "danger"
        status_text = "PASS" if module['coverage'] >= module['threshold'] else "FAIL"
        module_type = "Critical" if module.get('is_critical', False) else "Normal"
        type_class = "critical-path" if module.get('is_critical', False) else "normal-path"
        gap = max(0, module['threshold'] - module['coverage'])
        
        html_content += f"""
                                    <tr class="{type_class}">
                                        <td><code>{module['name']}</code></td>
                                        <td><strong>{module['coverage']:.1f}%</strong></td>
                                        <td>{module['threshold']}%</td>
                                        <td><span class="badge bg-{status_class}">{status_text}</span></td>
                                        <td>{module_type}</td>
                                        <td>{gap:.1f}% to go</td>
                                    </tr>
"""

    html_content += f"""
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Footer -->
        <footer class="mt-5 py-4 bg-light text-center">
            <p class="text-muted mb-0">
                Coverage Dashboard for Kubernetes Backup System | 
                Generated from coverage analysis data
            </p>
        </footer>
    </div>

    <script>
        // Module Coverage Chart
        const moduleCtx = document.getElementById('moduleChart').getContext('2d');
        const moduleChart = new Chart(moduleCtx, {{
            type: 'bar',
            data: {{
                labels: {json.dumps(module_chart_data['labels'])},
                datasets: [{{
                    label: 'Coverage',
                    data: {json.dumps(module_chart_data['coverage'])},
                    backgroundColor: {json.dumps(module_chart_data['colors'])},
                    borderColor: {json.dumps(module_chart_data['colors'])},
                    borderWidth: 1
                }}, {{
                    label: 'Threshold',
                    data: {json.dumps(module_chart_data['thresholds'])},
                    type: 'line',
                    borderColor: '#dc3545',
                    backgroundColor: 'transparent',
                    borderWidth: 2,
                    pointRadius: 0,
                    tension: 0
                }}]
            }},
            options: {{
                responsive: true,
                maintainAspectRatio: false,
                scales: {{
                    y: {{
                        beginAtZero: true,
                        max: 100,
                        ticks: {{
                            callback: function(value) {{
                                return value + '%';
                            }}
                        }}
                    }}
                }},
                plugins: {{
                    legend: {{
                        display: true
                    }},
                    tooltip: {{
                        callbacks: {{
                            label: function(context) {{
                                return context.dataset.label + ': ' + context.raw + '%';
                            }}
                        }}
                    }}
                }}
            }}
        }});

        // Trend Chart
        const trendCtx = document.getElementById('trendChart').getContext('2d');
        const trendChart = new Chart(trendCtx, {{
            type: 'line',
            data: {{
                labels: {json.dumps(trend_chart_data['labels'])},
                datasets: [{{
                    label: 'Coverage %',
                    data: {json.dumps(trend_chart_data['coverage'])},
                    borderColor: '#007bff',
                    backgroundColor: 'rgba(0, 123, 255, 0.1)',
                    borderWidth: 2,
                    fill: true,
                    tension: 0.4
                }}]
            }},
            options: {{
                responsive: true,
                maintainAspectRatio: false,
                scales: {{
                    y: {{
                        beginAtZero: true,
                        max: 100,
                        ticks: {{
                            callback: function(value) {{
                                return value + '%';
                            }}
                        }}
                    }}
                }},
                plugins: {{
                    legend: {{
                        display: false
                    }}
                }}
            }}
        }});
    </script>
</body>
</html>
"""

    try:
        with open(output_file, 'w') as f:
            f.write(html_content)
        print(f"Dashboard generated successfully: {output_file}")
        return True
    except Exception as e:
        print(f"Error generating dashboard: {e}")
        return False

def main():
    parser = argparse.ArgumentParser(description='Generate coverage dashboard')
    parser.add_argument('--coverage-dir', type=str, default='../coverage',
                       help='Coverage directory path (default: ../coverage)')
    parser.add_argument('--output', type=str, default='../coverage/dashboard.html',
                       help='Output HTML file (default: ../coverage/dashboard.html)')
    
    args = parser.parse_args()
    
    # Convert to Path objects
    script_dir = Path(__file__).parent
    coverage_dir = (script_dir / args.coverage_dir).resolve()
    output_file = (script_dir / args.output).resolve()
    
    print(f"Loading coverage data from: {coverage_dir}")
    
    # Load data
    coverage_data = load_coverage_data(coverage_dir)
    if not coverage_data:
        sys.exit(1)
    
    trend_data = load_trend_data(coverage_dir)
    
    # Generate dashboard
    print("Generating dashboard...")
    if generate_dashboard_html(coverage_data, trend_data, output_file):
        print(f"Dashboard available at: file://{output_file}")
        print("Open in your web browser to view the interactive dashboard")
    else:
        sys.exit(1)

if __name__ == "__main__":
    main()