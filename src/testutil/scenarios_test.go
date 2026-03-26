package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	timeline "github.com/weinaike/timeline/src"
	"github.com/weinaike/timeline/src/scheduler"
	"gopkg.in/yaml.v3"
)

// Scenario represents a test scenario from testdata
type Scenario struct {
	Name          string           `yaml:"name" json:"name"`
	Mode          string           `yaml:"mode" json:"mode"`
	TotalDuration int              `yaml:"totalDuration" json:"totalDuration"`
	Segments      []ScenarioSegment `yaml:"segments" json:"segments"`
}

// ScenarioSegment represents one segment in a scenario
type ScenarioSegment struct {
	OrigStartMs int `yaml:"origStartMs" json:"origStartMs"`
	OrigEndMs   int `yaml:"origEndMs" json:"origEndMs"`
	TransLenMs  int `yaml:"transLenMs" json:"transLenMs"`
}

// ScenarioResult represents the scheduling result for a scenario
type ScenarioResult struct {
	Name         string           `json:"name"`
	Mode         string           `json:"mode"`
	Input        []InputSegment   `json:"input"`
	Output       []OutputSegment  `json:"output"`
	Summary      Summary          `json:"summary"`
}

// InputSegment shows input timing info
type InputSegment struct {
	Index     int     `json:"index"`
	OrigStart float64 `json:"origStart"` // seconds
	OrigEnd   float64 `json:"origEnd"`
	TransLen  float64 `json:"transLen"`
	Gap       float64 `json:"gap"` // gap to next segment
}

// OutputSegment shows scheduled output
type OutputSegment struct {
	Index       int     `json:"index"`
	Start       float64 `json:"start"`       // seconds
	Duration    float64 `json:"duration"`    // play duration
	End         float64 `json:"end"`
	Rate        float64 `json:"rate"`        // compression rate
	Shift       float64 `json:"shift"`       // shift from origStart
	TimeDiff    float64 `json:"timeDiff"`    // timing difference
}

// Summary aggregates statistics
type Summary struct {
	TotalSegments  int     `json:"totalSegments"`
	MaxRate        float64 `json:"maxRate"`
	MinRate        float64 `json:"minRate"`
	AvgRate        float64 `json:"avgRate"`
	MaxShift       float64 `json:"maxShift"`
	TotalShift     float64 `json:"totalShift"`
	CompressionCnt int     `json:"compressionCount"` // rate > 1.0
	StretchCnt     int     `json:"stretchCount"`     // rate < 1.0
	PerfectCnt     int     `json:"perfectCount"`     // rate == 1.0
}

func loadScenario(t *testing.T, filename string) *Scenario {
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("Failed to read %s: %v", filename, err)
	}

	scenario := &Scenario{}
	if filepath.Ext(filename) == ".yaml" || filepath.Ext(filename) == ".yml" {
		if err := yaml.Unmarshal(data, scenario); err != nil {
			t.Fatalf("Failed to parse YAML %s: %v", filename, err)
		}
	} else {
		if err := json.Unmarshal(data, scenario); err != nil {
			t.Fatalf("Failed to parse JSON %s: %v", filename, err)
		}
	}

	return scenario
}

func runScenario(t *testing.T, scenario *Scenario) *ScenarioResult {
	s := scheduler.NewScheduler()
	mode := timeline.ModeA
	if scenario.Mode == "B" {
		mode = timeline.ModeB
	}

	cfg := timeline.SchedulerConfig{
		Mode:           mode,
		Weights:        timeline.NewDefaultPenaltyWeights(mode),
		DP:             timeline.NewDefaultDPConfig(mode),
		TotalDuration:  time.Duration(scenario.TotalDuration) * time.Millisecond,
		MaxNegotiation: 2,
	}

	// Build input
	sourceSegs := make([]timeline.SourceSegment, len(scenario.Segments))
	ttsSegs := make([]timeline.TTSSegment, len(scenario.Segments))

	for i, seg := range scenario.Segments {
		sourceSegs[i] = timeline.SourceSegment{
			Index: i,
			Interval: timeline.TimeInterval{
				Start: time.Duration(seg.OrigStartMs) * time.Millisecond,
				End:   time.Duration(seg.OrigEndMs) * time.Millisecond,
			},
		}
		ttsSegs[i] = timeline.TTSSegment{
			Index:      i,
			NaturalLen: time.Duration(seg.TransLenMs) * time.Millisecond,
		}
	}

	input := &timeline.PipelineInput{
		SourceSegments: sourceSegs,
		TTSSegments:    ttsSegs,
		Config:         cfg,
	}

	result, err := s.Schedule(input)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	// Build result
	sr := &ScenarioResult{
		Name:  scenario.Name,
		Mode:  scenario.Mode,
		Input: make([]InputSegment, len(scenario.Segments)),
		Output: make([]OutputSegment, len(result.Segments)),
	}

	// Input segments
	for i, seg := range scenario.Segments {
		sr.Input[i] = InputSegment{
			Index:     i,
			OrigStart: float64(seg.OrigStartMs) / 1000,
			OrigEnd:   float64(seg.OrigEndMs) / 1000,
			TransLen:  float64(seg.TransLenMs) / 1000,
		}
		if i < len(scenario.Segments)-1 {
			sr.Input[i].Gap = float64(scenario.Segments[i+1].OrigStartMs-seg.OrigEndMs) / 1000
		}
	}

	// Output segments + calculate summary
	var totalRate float64
	sr.Summary.MaxRate = 0
	sr.Summary.MinRate = 999
	sr.Summary.MaxShift = 0
	sr.Summary.TotalShift = 0

	for i, seg := range result.Segments {
		origStart := float64(scenario.Segments[i].OrigStartMs) / 1000
		shift := seg.StartTime.Seconds() - origStart

		sr.Output[i] = OutputSegment{
			Index:    seg.Index,
			Start:    seg.StartTime.Seconds(),
			Duration: seg.PlayDuration.Seconds(),
			End:      seg.StartTime.Seconds() + seg.PlayDuration.Seconds(),
			Rate:     seg.Rate,
			Shift:    shift,
			TimeDiff: seg.PlayDuration.Seconds() - sr.Input[i].TransLen,
		}

		totalRate += seg.Rate
		if seg.Rate > sr.Summary.MaxRate {
			sr.Summary.MaxRate = seg.Rate
		}
		if seg.Rate < sr.Summary.MinRate {
			sr.Summary.MinRate = seg.Rate
		}
		if shift > sr.Summary.MaxShift {
			sr.Summary.MaxShift = shift
		}
		sr.Summary.TotalShift += shift

		// Count rate types
		if seg.Rate > 1.001 {
			sr.Summary.CompressionCnt++
		} else if seg.Rate < 0.999 {
			sr.Summary.StretchCnt++
		} else {
			sr.Summary.PerfectCnt++
		}
	}

	sr.Summary.TotalSegments = len(result.Segments)
	sr.Summary.AvgRate = totalRate / float64(len(result.Segments))

	return sr
}

func printScenarioResult(sr *ScenarioResult) {
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Scenario: %s (Mode %s)\n", sr.Name, sr.Mode)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Input table
	fmt.Println("📥 INPUT:")
	fmt.Println("┌───────┬───────────┬───────────┬──────────┬──────────┐")
	fmt.Println("│ Index │ OrigStart │ OrigEnd   │ TransLen │ Gap      │")
	fmt.Println("├───────┼───────────┼───────────┼──────────┼──────────┤")
	for _, in := range sr.Input {
		gap := "-"
		if in.Gap > 0 {
			gap = fmt.Sprintf("%.3fs", in.Gap)
		}
		fmt.Printf("│   %d   │ %7.3fs  │ %7.3fs  │ %6.3fs  │ %8s │\n",
			in.Index, in.OrigStart, in.OrigEnd, in.TransLen, gap)
	}
	fmt.Println("└───────┴───────────┴───────────┴──────────┴──────────┘")

	// Output table
	fmt.Println("\n📤 OUTPUT:")
	fmt.Println("┌───────┬───────────┬───────────┬───────────┬───────┬─────────┬──────────┐")
	fmt.Println("│ Index │ Start     │ End       │ Duration  │ Rate  │ Shift   │ TimeDiff │")
	fmt.Println("├───────┼───────────┼───────────┼───────────┼───────┼─────────┼──────────┤")
	for i, out := range sr.Output {
		in := sr.Input[i]

		rateIndicator := ""
		if out.Rate > 1.01 {
			rateIndicator = "⚡" // compression
		} else if out.Rate < 0.99 {
			rateIndicator = "🐢" // stretch
		}

		fmt.Printf("│   %d   │ %7.3fs  │ %7.3fs  │ %7.3fs  │ %4.2f%s│ %+7.3fs │ %+8.3fs │\n",
			out.Index, out.Start, out.End, out.Duration,
			out.Rate, rateIndicator, out.Shift, out.Duration-in.TransLen)
	}
	fmt.Println("└───────┴───────────┴───────────┴───────────┴───────┴─────────┴──────────┘")

	// Comparison: Original vs Scheduled
	fmt.Println("\n📊 TIMELINE COMPARISON:")
	fmt.Println("┌───────┬────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ Index │  Original Timeline    →    Scheduled Timeline                 │")
	fmt.Println("├───────┼────────────────────────────────────────────────────────────────┤")
	for i, out := range sr.Output {
		in := sr.Input[i]
		fmt.Printf("│   %d   │ [%6.2fs - %6.2fs] → [%6.2fs - %6.2fs] Rate=%.2f   │\n",
			i, in.OrigStart, in.OrigEnd, out.Start, out.End, out.Rate)
	}
	fmt.Println("└───────┴────────────────────────────────────────────────────────────────┘")

	// Summary
	fmt.Println("\n📈 SUMMARY:")
	fmt.Printf("  Total Segments: %d\n", sr.Summary.TotalSegments)
	fmt.Printf("  Rate Range:     %.2f ~ %.2f (avg: %.2f)\n",
		sr.Summary.MinRate, sr.Summary.MaxRate, sr.Summary.AvgRate)
	fmt.Printf("  Max Shift:      %.3fs\n", sr.Summary.MaxShift)
	fmt.Printf("  Rate Breakdown: %d perfect ⚪ | %d compressed ⚡ | %d stretched 🐢\n",
		sr.Summary.PerfectCnt, sr.Summary.CompressionCnt, sr.Summary.StretchCnt)

	// Effectiveness analysis
	fmt.Println("\n💡 ANALYSIS:")
	if sr.Summary.CompressionCnt > 0 {
		fmt.Printf("  ⚡ Translation longer than original - compressed %d segments\n", sr.Summary.CompressionCnt)
	}
	if sr.Summary.StretchCnt > 0 {
		fmt.Printf("  🐢 Translation shorter than original - stretched %d segments\n", sr.Summary.StretchCnt)
	}
	if sr.Summary.MaxShift > 0.1 {
		fmt.Printf("  → Timeline shifted up to %.3fs to accommodate longer translations\n", sr.Summary.MaxShift)
	}
	if sr.Summary.MaxRate > 1.3 {
		fmt.Printf("  ⚠️  High compression detected (max rate: %.2f) - may affect audio quality\n", sr.Summary.MaxRate)
	}
}

func TestScenario_PerfectMatch(t *testing.T) {
	scenario := loadScenario(t, "perfect_match.json")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_NeedSpeedChange(t *testing.T) {
	scenario := loadScenario(t, "need_speed_change.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_DominoPush(t *testing.T) {
	scenario := loadScenario(t, "domino_push.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_DensePacking(t *testing.T) {
	scenario := loadScenario(t, "dense_packing.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_ExtremeCompression(t *testing.T) {
	scenario := loadScenario(t, "extreme_compression.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_ShorterTranslation(t *testing.T) {
	scenario := loadScenario(t, "shorter_translation.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_Interview(t *testing.T) {
	scenario := loadScenario(t, "interview.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}

func TestScenario_Lecture(t *testing.T) {
	scenario := loadScenario(t, "lecture.yaml")
	result := runScenario(t, scenario)
	printScenarioResult(result)
}
