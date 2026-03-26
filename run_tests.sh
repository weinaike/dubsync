#!/bin/bash
# ============================================================
# Timeline Test Runner Script
# Comprehensive test execution with multiple output formats
# ============================================================

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}" && pwd)"
REPORT_DIR="${PROJECT_ROOT}/test-reports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")

# Default settings
VERBOSE=false
FORMAT="console"
OUTPUT_DIR=""
PACKAGE="./..."
COVERAGE=false
RACE=false
BENCHMARK=false
INTEGRATION=false
GOLDEN_DIR=""
UPDATE_GOLDEN=false
VERIFY_GOLDEN=false
FAILFAST=false

# Colors for console output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print usage
usage() {
    cat << EOF
Timeline Test Runner

Usage: $0 [options]

Options:
    -v, --verbose         Enable verbose test output
    -f, --format FORMAT   Output format: console, json, junit, html, all (default: console)
    -o, --output DIR      Output directory for reports (default: test-reports/)
    -p, --package PKG     Package to test (default: ./...)
    -c, --coverage        Enable coverage collection
    -r, --race            Enable race detection
    -b, --benchmark       Run benchmarks instead of tests
    -i, --integration     Include integration tests
    --golden-dir DIR      Directory for golden files
    --update-golden       Update golden files with current results
    --verify              Verify results against golden files
    --failfast            Stop on first test failure
    -h, --help            Show this help message

Examples:
    $0                                    # Run tests, console output
    $0 -v -c -f all                      # Verbose with coverage, all formats
    $0 -f junit -o reports/              # JUnit XML to reports/
    $0 --verify --golden-dir golden/     # Verify against golden files
    $0 -b --coverage                     # Benchmarks with coverage
EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -f|--format)
            FORMAT="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -p|--package)
            PACKAGE="$2"
            shift 2
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -r|--race)
            RACE=true
            shift
            ;;
        -b|--benchmark)
            BENCHMARK=true
            shift
            ;;
        -i|--integration)
            INTEGRATION=true
            shift
            ;;
        --golden-dir)
            GOLDEN_DIR="$2"
            shift 2
            ;;
        --update-golden)
            UPDATE_GOLDEN=true
            shift
            ;;
        --verify)
            VERIFY_GOLDEN=true
            shift
            ;;
        --failfast)
            FAILFAST=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# Set output directory
if [ -z "$OUTPUT_DIR" ]; then
    OUTPUT_DIR="$REPORT_DIR"
fi
mkdir -p "$OUTPUT_DIR"

# Build test command
build_test_cmd() {
    local cmd="go test"

    if [ "$VERBOSE" = true ]; then
        cmd="$cmd -v"
    fi

    if [ "$COVERAGE" = true ]; then
        cmd="$cmd -coverprofile=$OUTPUT_DIR/coverage.out -covermode=atomic"
    fi

    if [ "$RACE" = true ]; then
        cmd="$cmd -race"
    fi

    if [ "$FAILFAST" = true ]; then
        cmd="$cmd -failfast"
    fi

    if [ "$BENCHMARK" = true ]; then
        cmd="$cmd -bench=. -benchmem"
    fi

    if [ "$INTEGRATION" = false ] && [ "$BENCHMARK" = false ]; then
        cmd="$cmd -short"
    fi

    cmd="$cmd $PACKAGE"

    echo "$cmd"
}

# Run tests with JSON output (for report generation)
run_tests_json() {
    local output_file="$1"
    local cmd=$(build_test_cmd)

    # Use go test -json for structured output
    cmd="$cmd -json"

    echo -e "${BLUE}Running: $cmd${NC}"

    local exit_code=0
    if $cmd > "$output_file" 2>&1; then
        echo -e "${GREEN}Tests completed successfully${NC}"
    else
        exit_code=$?
        echo -e "${RED}Some tests failed (exit code: $exit_code)${NC}"
    fi

    return $exit_code
}

# Run tests with console output
run_tests_console() {
    local cmd=$(build_test_cmd)

    echo -e "${BLUE}Running: $cmd${NC}"

    local exit_code=0
    if $cmd; then
        echo -e "${GREEN}Tests completed successfully${NC}"
    else
        exit_code=$?
    fi

    return $exit_code
}

# Generate JSON report from go test -json output
generate_json_report() {
    local json_input="$1"
    local json_output="$2"

    # Use go test -json output directly, then summarize
    echo "Generating JSON report..."

    # Parse the JSON lines and create summary
    cat > "$json_output" << 'SCRIPT'
#!/usr/bin/env python3
import json
import sys
from datetime import datetime

report = {
    "timestamp": datetime.now().isoformat(),
    "total_tests": 0,
    "passed": 0,
    "failed": 0,
    "skipped": 0,
    "packages": []
}

current_pkg = None
pkg_data = {}

for line in sys.stdin:
    try:
        event = json.loads(line.strip())
    except:
        continue

    action = event.get("Action", "")
    pkg = event.get("Package", "")

    if pkg and pkg not in pkg_data:
        pkg_data[pkg] = {"tests": [], "passed": 0, "failed": 0, "skipped": 0}

    if action == "run":
        test_name = event.get("Test", "")
        if test_name:
            pkg_data[pkg]["tests"].append({"name": test_name, "status": "run"})
    elif action == "pass":
        pkg_data[pkg]["passed"] += 1
        report["passed"] += 1
    elif action == "fail":
        pkg_data[pkg]["failed"] += 1
        report["failed"] += 1
    elif action == "skip":
        pkg_data[pkg]["skipped"] += 1
        report["skipped"] += 1

for pkg, data in pkg_data.items():
    report["packages"].append({
        "name": pkg,
        "tests": data["tests"],
        "passed": data["passed"],
        "failed": data["failed"],
        "skipped": data["skipped"]
    })
    report["total_tests"] += data["passed"] + data["failed"] + data["skipped"]

print(json.dumps(report, indent=2))
SCRIPT

    # Run Python script to parse JSON
    if command -v python3 >/dev/null; then
        python3 "$json_output" < "$json_input" > "${json_output}.tmp"
        mv "${json_output}.tmp" "$json_output"
    else
        # Fallback: just copy the raw output
        cp "$json_input" "$json_output"
    fi

    echo "JSON report: $json_output"
}

# Generate JUnit XML report
generate_junit_report() {
    local json_input="$1"
    local xml_output="$2"

    echo "Generating JUnit XML report..."

    # Check for go-junit-report tool
    if command -v go-junit-report >/dev/null; then
        go-junit-report -in "$json_input" -out "$xml_output"
    else
        # Generate simplified JUnit XML
        cat > "$xml_output" << 'XMLEOF'
<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="timeline">
  <!-- Generated from go test -json output -->
  <!-- For full JUnit support, install: go install github.com/jstemmer/go-junit-report/v2@latest -->
</testsuites>
XMLEOF
        echo "Note: Install go-junit-report for full JUnit XML: go install github.com/jstemmer/go-junit-report/v2@latest"
    fi

    echo "JUnit report: $xml_output"
}

# Generate HTML report
generate_html_report() {
    local json_input="$1"
    local html_output="$2"
    local coverage_file="$3"

    echo "Generating HTML report..."

    # Extract summary stats from go test -json
    local passed=0 failed=0 skipped=0

    if [ -f "$json_input" ]; then
        passed=$(grep -c '"Action":"pass"' "$json_input" 2>/dev/null | wc -l || echo "0")
        failed=$(grep -c '"Action":"fail"' "$json_input" 2>/dev/null | wc -l || echo "0")
        skipped=$(grep -c '"Action":"skip"' "$json_input" 2>/dev/null | wc -l || echo "0")
    fi

    # Get coverage percentage if available
    local coverage="N/A"
    if [ -f "$coverage_file" ]; then
        coverage=$(go tool cover -func="$coverage_file" 2>/dev/null | grep "total:" | awk '{print $3}' | tr -d '%' || echo "N/A")
    fi

    # Get git info
    local git_commit="unknown"
    local git_branch="unknown"
    if command -v git >/dev/null && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        git_commit=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        git_branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
    fi

    cat > "$html_output" << HTMLEOF
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Timeline Test Report</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif; background: #f5f5f5; color: #333; line-height: 1.6; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 20px; }
        .header h1 { font-size: 2em; margin-bottom: 10px; }
        .header .timestamp { opacity: 0.8; }
        .summary { display: grid; grid-template-columns: repeat(4, 1fr); gap: 15px; margin-bottom: 20px; }
        .card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); text-align: center; }
        .card.passed { border-left: 4px solid #4caf50; }
        .card.failed { border-left: 4px solid #f44336; }
        .card.skipped { border-left: 4px solid #ff9800; }
        .card.coverage { border-left: 4px solid #2196f3; }
        .card .number { font-size: 2.5em; font-weight: bold; }
        .card .label { color: #666; font-size: 0.9em; margin-top: 5px; }
        .details { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .details h2 { margin-bottom: 15px; color: #333; }
        .info-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #eee; }
        .info-row:last-child { border-bottom: none; }
        .info-label { color: #666; }
        .info-value { font-weight: 500; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 0.9em; }
        .pass { color: #4caf50; }
        .fail { color: #f44336; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Timeline Test Report</h1>
            <div class="timestamp">Generated on: $(date -u)</div>
        </div>

        <div class="summary">
            <div class="card passed">
                <div class="number">$passed</div>
                <div class="label">Passed</div>
            </div>
            <div class="card failed">
                <div class="number">$failed</div>
                <div class="label">Failed</div>
            </div>
            <div class="card skipped">
                <div class="number">$skipped</div>
                <div class="label">Skipped</div>
            </div>
            <div class="card coverage">
                <div class="number">${coverage}%</div>
                <div class="label">Coverage</div>
            </div>
        </div>

        <div class="details">
            <h2>Build Information</h2>
            <div class="info-row">
                <span class="info-label">Git Commit</span>
                <span class="info-value">${git_commit}</span>
            </div>
            <div class="info-row">
                <span class="info-label">Git Branch</span>
                <span class="info-value">${git_branch}</span>
            </div>
            <div class="info-row">
                <span class="info-label">Go Version</span>
                <span class="info-value">$(go version | head -1)</span>
            </div>
            <div class="info-row">
                <span class="info-label">Platform</span>
                <span class="info-value">$(uname -s)</span>
            </div>
        </div>

        <div class="footer">
            Timeline Test Execution System |
            <a href="https://github.com/weinaike/timeline">View Project</a>
        </div>
    </div>
</body>
</html>
HTMLEOF

    echo "HTML report: $html_output"
}

# Generate coverage HTML if available
generate_coverage_html() {
    local coverage_file="$1"
    local html_output="$2"

    if [ -f "$coverage_file" ]; then
        echo "No coverage file found, skipping HTML coverage generation"
        return
    fi

    echo "Generating HTML coverage visualization..."
    go tool cover -html="$coverage_file" -o "$html_output"
    echo "Coverage HTML: $html_output"
}

# Print summary to console
print_console_summary() {
    local json_file="$1"

    echo ""
    echo "========================================="
    echo "           TEST SUMMARY"
    echo "========================================="

    local passed=$(grep -c '"Action":"pass"' "$json_file" 2>/dev/null | wc -l 2>/dev/null || echo "0")
    local failed=$(grep -c '"Action":"fail"' "$json_file" 1>/dev/null | wc -l 2>/dev/null || echo "0")
    local skipped=$(grep -c '"Action":"skip"' "$json_file" 1>/dev/null | wc -l 2>/dev/null || echo "0")

    echo -e "Passed:  ${GREEN}$passed${NC}"
    echo -e "Failed:  ${RED}$failed${NC}"
    echo -e "Skipped: ${YELLOW}$skipped${NC}"

    if [ "$COVERAGE" = true ] && [ -f "$OUTPUT_DIR/coverage.out" ]; then
        echo ""
        echo "Coverage:"
        go tool cover -func="$OUTPUT_DIR/coverage.out" | grep "total:" || echo "Coverage data not available"
    fi

    echo "========================================="
}

# Golden file operations
handle_golden() {
    local json_output="$1"

    if [ "$UPDATE_GOLDEN" = true ] && [ -n "$GOLDEN_DIR" ]; then
        mkdir -p "$GOLDEN_DIR"
        cp "$json_output" "$GOLDEN_DIR/expected_results.json"
        echo -e "${GREEN}Golden files updated in $GOLDEN_DIR${NC}"
    fi

    if [ "$VERIFY_GOLDEN" = true ] && [ -n "$GOLDEN_DIR" ]; then
        if [ -f "$GOLDEN_DIR/expected_results.json" ]; then
            echo -e "${BLUE}Verifying against golden files...${NC}"
            # Simple comparison (full implementation would use proper diff)
            if diff -q "$GOLDEN_DIR/expected_results.json" "$json_output" > /dev/null 2>&1; then
                echo -e "${GREEN}✓ Golden file verification passed${NC}"
                return 0
            else
                echo -e "${YELLOW}⚠ Golden file verification: differences found${NC}"
                return 1
            fi
        else
            echo -e "${YELLOW}No golden files found at $GOLDEN_DIR${NC}"
        fi
    fi

    return 0
}

# Main execution
main() {
    echo -e "${BLUE}=== Timeline Test Runner ===${NC}"
    echo ""

    local exit_code=0

    case $FORMAT in
        console)
            run_tests_console || exit_code=$?
            ;;

        json)
            local json_output="$OUTPUT_DIR/test_output.json"
            run_tests_json "$json_output" || exit_code=$?
            generate_json_report "$json_output" "$OUTPUT_DIR/report.json"
            handle_golden "$OUTPUT_DIR/report.json" || exit_code=$?
            ;;

        junit)
            local json_output="$OUTPUT_DIR/test_output.json"
            run_tests_json "$json_output" || exit_code=$?
            generate_junit_report "$json_output" "$OUTPUT_DIR/junit.xml"
            ;;

        html)
            local json_output="$OUTPUT_DIR/test_output.json"
            run_tests_json "$json_output" || exit_code=$?
            generate_html_report "$json_output" "$OUTPUT_DIR/report.html" "$OUTPUT_DIR/coverage.out"
            if [ "$COVERAGE" = true ]; then
                generate_coverage_html "$OUTPUT_DIR/coverage.out" "$OUTPUT_DIR/coverage.html"
            fi
            ;;

        all)
            local json_output="$OUTPUT_DIR/test_output.json"
            run_tests_json "$json_output" || exit_code=$?

            print_console_summary "$json_output"

            # Generate all report formats
            cp "$json_output" "$OUTPUT_DIR/report.json"
            generate_junit_report "$json_output" "$OUTPUT_DIR/junit.xml"
            generate_html_report "$json_output" "$OUTPUT_DIR/report.html" "$OUTPUT_DIR/coverage.out"

            if [ "$COVERAGE" = true ]; then
                generate_coverage_html "$OUTPUT_DIR/coverage.out" "$OUTPUT_DIR/coverage.html"
                echo ""
                echo "Coverage Summary:"
                go tool cover -func="$OUTPUT_DIR/coverage.out" | grep "total:" || true
            fi

            handle_golden "$OUTPUT_DIR/report.json" || exit_code=$?

            echo ""
            echo -e "${GREEN}Reports generated in $OUTPUT_DIR/:${NC}"
            ls -la "$OUTPUT_DIR/" 2>/dev/null | grep -E "\.(json|xml|html)$" || true
            ;;

        *)
            echo "Unknown format: $FORMAT"
            usage
            ;;
    esac

    exit $exit_code
}

main "$@"
