package testutil

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TestReport represents a complete test execution report
type TestReport struct {
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration  `json:"duration"`
	GoVersion  string         `json:"goVersion"`
	TotalTests int            `json:"totalTests"`
	Passed     int            `json:"passed"`
	Failed     int            `json:"failed"`
	Skipped    int            `json:"skipped"`
	Coverage   *CoverageData  `json:"coverage,omitempty"`
	TestSuites []TestSuite    `json:"testSuites" xml:"testsuite"`
	Metadata   ReportMeta     `json:"metadata"`
}

// TestSuite represents a single test suite (typically one test file)
type TestSuite struct {
	Name      string     `json:"name" xml:"name,attr"`
	Package   string     `json:"package" xml:"package,attr"`
	Tests     int        `json:"tests" xml:"tests,attr"`
	Failures  int        `json:"failures" xml:"failures,attr"`
	Errors    int        `json:"errors" xml:"errors,attr"`
	Skipped   int        `json:"skipped" xml:"skipped,attr"`
	Time      float64    `json:"time" xml:"time,attr"`
	Timestamp string     `json:"timestamp" xml:"timestamp,attr"`
	TestCases []TestCase `json:"testCases" xml:"testcase"`
}

// TestCase represents a single test case
type TestCase struct {
	Name      string   `json:"name" xml:"name,attr"`
	ClassName string   `json:"className" xml:"classname,attr"`
	Time      float64  `json:"time" xml:"time,attr"`
	Status    string   `json:"status" xml:"status,attr"`
	Failure   *Failure `json:"failure,omitempty" xml:"failure,omitempty"`
	Output    string   `json:"output,omitempty" xml:"system-out,omitempty"`
}

// Failure represents test failure details
type Failure struct {
	Message string `json:"message" xml:"message,attr"`
	Type    string `json:"type" xml:"type,attr"`
	Content string `json:"content" xml:",chardata"`
}

// CoverageData represents coverage metrics
type CoverageData struct {
	Total     float64            `json:"total"`
	Packages  map[string]float64 `json:"packages"`
	Files     map[string]float64 `json:"files"`
	Functions map[string]float64 `json:"functions"`
}

// ReportMeta contains additional metadata
type ReportMeta struct {
	GitCommit   string `json:"gitCommit"`
	GitBranch   string `json:"gitBranch"`
	GitTag      string `json:"gitTag"`
	CI          bool   `json:"ci"`
	BuildNumber string `json:"buildNumber"`
	Platform    string `json:"platform"`
}

// ReportWriter interface for different output formats
type ReportWriter interface {
	Write(report *TestReport) ([]byte, error)
	Extension() string
}

// JSONWriter implements ReportWriter for JSON
type JSONWriter struct{}

func (w *JSONWriter) Write(report *TestReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func (w *JSONWriter) Extension() string {
	return "json"
}

// JUnitWriter implements ReportWriter for JUnit XML
type JUnitWriter struct{}

// JUnitXML is the root element for JUnit XML
type JUnitXML struct {
	XMLName    xml.Name    `xml:"testsuites"`
	Name       string      `xml:"name,attr"`
	Tests      int         `xml:"tests,attr"`
	Failures   int         `xml:"failures,attr"`
	Errors     int         `xml:"errors,attr"`
	Time       float64     `xml:"time,attr"`
	TestSuites []TestSuite `xml:"testsuite"`
}

func (w *JUnitWriter) Write(report *TestReport) ([]byte, error) {
	junit := JUnitXML{
		Name:       "timeline",
		Tests:      report.TotalTests,
		Failures:   report.Failed,
		Errors:     0,
		Time:       report.Duration.Seconds(),
		TestSuites: report.TestSuites,
	}

	output, err := xml.MarshalIndent(junit, "", "  ")
	if err != nil {
		return nil, err
	}

	// Add XML header
	header := []byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	return append(header, output...), nil
}

func (w *JUnitWriter) Extension() string {
	return "xml"
}

// HTMLWriter implements ReportWriter for HTML dashboard
type HTMLWriter struct{}

func (w *HTMLWriter) Write(report *TestReport) ([]byte, error) {
	var buf bytes.Buffer

	coveragePercent := "N/A"
	if report.Coverage != nil {
		coveragePercent = fmt.Sprintf("%.1f", report.Coverage.Total)
	}

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Timeline Test Report</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; color: #333; line-height: 1.6; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 10px; margin-bottom: 20px; }
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
        .test-suite { margin-bottom: 15px; border: 1px solid #e0e0e0; border-radius: 8px; overflow: hidden; }
        .suite-header { padding: 15px; background: #f8f9fa; cursor: pointer; display: flex; justify-content: space-between; align-items: center; }
        .suite-header:hover { background: #f0f0f0; }
        .suite-tests { padding: 10px 15px; display: none; }
        .suite-tests.show { display: block; }
        .test-case { padding: 8px 0; border-bottom: 1px solid #f0f0f0; display: flex; justify-content: space-between; align-items: center; }
        .test-case:last-child { border-bottom: none; }
        .test-name { font-family: monospace; font-size: 0.9em; }
        .test-status { padding: 2px 8px; border-radius: 4px; font-size: 0.8em; }
        .status-passed { background: #e8f5e9; color: #2e7d32; }
        .status-failed { background: #ffebee; color: #c62828; }
        .status-skipped { background: #fff3e0; color: #ef6c00; }
        .failure-details { margin-top: 10px; padding: 10px; background: #fff3f3; border-radius: 4px; font-family: monospace; font-size: 0.85em; white-space: pre-wrap; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 0.9em; }
        .pass { color: #4caf50; }
        .fail { color: #f44336; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Timeline Test Report</h1>
            <div class="timestamp">Generated on: `)

	buf.WriteString(report.Timestamp.Format("2006-01-02 15:04:05 MST"))
	buf.WriteString(`</div>
        </div>

        <div class="summary">
            <div class="card passed">
                <div class="number">`)
	buf.WriteString(fmt.Sprintf("%d", report.Passed))
	buf.WriteString(`</div>
                <div class="label">Passed</div>
            </div>
            <div class="card failed">
                <div class="number">`)
	buf.WriteString(fmt.Sprintf("%d", report.Failed))
	buf.WriteString(`</div>
                <div class="label">Failed</div>
            </div>
            <div class="card skipped">
                <div class="number">`)
	buf.WriteString(fmt.Sprintf("%d", report.Skipped))
	buf.WriteString(`</div>
                <div class="label">Skipped</div>
            </div>
            <div class="card coverage">
                <div class="number">`)
	buf.WriteString(coveragePercent)
	buf.WriteString(`%</div>
                <div class="label">Coverage</div>
            </div>
        </div>

        <div class="details">
            <h2>Build Information</h2>
            <div class="info-row">
                <span class="info-label">Duration</span>
                <span class="info-value">`)
	buf.WriteString(report.Duration.String())
	buf.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Go Version</span>
                <span class="info-value">`)
	buf.WriteString(report.GoVersion)
	buf.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Git Commit</span>
                <span class="info-value">`)
	buf.WriteString(report.Metadata.GitCommit)
	buf.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Git Branch</span>
                <span class="info-value">`)
	buf.WriteString(report.Metadata.GitBranch)
	buf.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Platform</span>
                <span class="info-value">`)
	buf.WriteString(report.Metadata.Platform)
	buf.WriteString(`</span>
            </div>
        </div>

        <div class="details">
            <h2>Test Suites</h2>
`)
	// Add test suites
	for _, suite := range report.TestSuites {
		fmt.Fprintf(&buf, `            <div class="test-suite">
                <div class="suite-header" onclick="toggleSuite(this)">
                    <span class="suite-name">%s</span>
                    <span class="suite-stats">
                        <span class="status-passed">%d passed</span>
                        <span class="status-failed">%d failed</span>
                        <span class="status-skipped">%d skipped</span>
                    </span>
                </div>
                <div class="suite-tests">
`, suite.Name, suite.Tests-suite.Failures-suite.Skipped, suite.Failures, suite.Skipped)

		for _, tc := range suite.TestCases {
			statusClass := "status-passed"
			if tc.Status == "failed" {
				statusClass = "status-failed"
			} else if tc.Status == "skipped" {
				statusClass = "status-skipped"
			}
			fmt.Fprintf(&buf, `                    <div class="test-case">
                        <span class="test-name">%s</span>
                        <span class="test-status %s">%s</span>
                    </div>
`, tc.Name, statusClass, tc.Status)
		}

		buf.WriteString(`                </div>
            </div>
`)
	}

	buf.WriteString(`        </div>

        <div class="footer">
            Timeline Test Execution System
        </div>
    </div>

    <script>
        function toggleSuite(header) {
            const tests = header.nextElementSibling;
            tests.classList.toggle('show');
        }
    </script>
</body>
</html>
`)

	return buf.Bytes(), nil
}

func (w *HTMLWriter) Extension() string {
	return "html"
}

// ReportGenerator creates reports from go test output
type ReportGenerator struct {
	writers []ReportWriter
}

// NewReportGenerator creates a new report generator with the given writers
func NewReportGenerator(writers ...ReportWriter) *ReportGenerator {
	return &ReportGenerator{writers: writers}
}

// Generate creates reports in all configured formats
func (rg *ReportGenerator) Generate(report *TestReport) (map[string][]byte, error) {
	results := make(map[string][]byte)
	for _, writer := range rg.writers {
		data, err := writer.Write(report)
		if err != nil {
			return nil, fmt.Errorf("writer %T failed: %w", writer, err)
		}
		results["report."+writer.Extension()] = data
	}
	return results, nil
}

// WriteReports writes all reports to the specified directory
func (rg *ReportGenerator) WriteReports(report *TestReport, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	results, err := rg.Generate(report)
	if err != nil {
		return err
	}

	for filename, data := range results {
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	return nil
}

// CollectMetadata gathers environment metadata for the report
func CollectMetadata() ReportMeta {
	meta := ReportMeta{
		Platform: strings.Title(strings.ToLower(os.Getenv("GOOS"))),
		CI:       os.Getenv("CI") != "",
	}

	// Git info
	if commit, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output(); err == nil {
		meta.GitCommit = strings.TrimSpace(string(commit))
	}
	if branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		meta.GitBranch = strings.TrimSpace(string(branch))
	}
	if tag, err := exec.Command("git", "describe", "--tags", "--exact-match", "HEAD").Output(); err == nil {
		meta.GitTag = strings.TrimSpace(string(tag))
	}

	// Go version
	if version, err := exec.Command("go", "version").Output(); err == nil {
		meta.Platform = strings.TrimSpace(string(version))
	}

	return meta
}

// ParseCoverage parses go tool cover -func output
func ParseCoverage(coverageFile string) (*CoverageData, error) {
	if coverageFile == "" {
		return nil, nil
	}

	data, err := os.ReadFile(coverageFile)
	if err != nil {
		return nil, err
	}

	coverage := &CoverageData{
		Packages:  make(map[string]float64),
		Files:     make(map[string]float64),
		Functions: make(map[string]float64),
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse: filename:function     percentage
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		loc := parts[0]
		percentStr := parts[len(parts)-1]
		percentStr = strings.TrimSuffix(percentStr, "%")

		var percent float64
		if _, err := fmt.Sscanf(percentStr, "%f", &percent); err != nil {
			continue
		}

		// Extract package and file info
		if idx := strings.LastIndex(loc, "/"); idx >= 0 {
			pkg := loc[:idx]
			if current, ok := coverage.Packages[pkg]; !ok || percent > current {
				coverage.Packages[pkg] = percent
			}
		}

		if idx := strings.Index(loc, ":"); idx >= 0 {
			file := loc[:idx]
			coverage.Files[file] = percent
		}

		coverage.Functions[loc] = percent
	}

	// Calculate total from last line if available
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "total:") {
			parts := strings.Fields(lines[i])
			if len(parts) >= 3 {
				percentStr := strings.TrimSuffix(parts[len(parts)-1], "%")
				if _, err := fmt.Sscanf(percentStr, "%f", &coverage.Total); err == nil {
					break
				}
			}
		}
	}

	return coverage, nil
}

// SortTestSuitesByFailure sorts test suites with failures first
func SortTestSuitesByFailure(suites []TestSuite) {
	sort.Slice(suites, func(i, j int) bool {
		if suites[i].Failures != suites[j].Failures {
			return suites[i].Failures > suites[j].Failures
		}
		return suites[i].Name < suites[j].Name
	})
}

// WriteToFile is a helper to write data to a file
func WriteToFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
