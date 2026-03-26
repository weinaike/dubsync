package testutil

import (
	"path/filepath"
	"testing"

	timeline "github.com/weinaike/timeline/src"
)

func TestLoadYAMLFiles(t *testing.T) {
	testdataDir := filepath.Join(".", "testdata")

	cases, err := LoadAllFromDir(testdataDir, ".yaml")
	if err != nil {
		t.Fatalf("LoadAllFromDir failed: %v", err)
	}

	if len(cases) == 0 {
		t.Fatal("No yaml test cases loaded")
	}

	t.Logf("Loaded %d yaml test cases", len(cases))
	for _, tc := range cases {
		t.Logf("  - %s: %d segments", tc.Name, len(tc.Input.SourceSegments))
	}
}

func TestLoadJSONFiles(t *testing.T) {
	testdataDir := filepath.Join(".", "testdata")

	cases, err := LoadAllFromDir(testdataDir, ".json")
	if err != nil {
		t.Fatalf("LoadAllFromDir failed: %v", err)
	}

	t.Logf("Loaded %d json test cases", len(cases))
	for _, tc := range cases {
		t.Logf("  - %s: %d segments", tc.Name, len(tc.Input.SourceSegments))
	}
}

func TestBuilderScenarios(t *testing.T) {
	scenarios := []struct {
		name  string
		build func() *timeline.PipelineInput
	}{
		{"SingleSegment", NewTestDataBuilder().SingleSegment},
		{"ContinuousSpeech", NewTestDataBuilder().ContinuousSpeech},
		{"LongSilence", NewTestDataBuilder().LongSilence},
		{"ShorterTranslation", NewTestDataBuilder().ShorterTranslation},
		{"ZeroWindow", NewTestDataBuilder().ZeroWindow},
		{"DensePacking", NewTestDataBuilder().DensePacking},
		{"LargeScale", NewTestDataBuilder().LargeScale},
		{"ExtremeCompression", NewTestDataBuilder().ExtremeCompression},
		{"NewsBroadcast", NewTestDataBuilder().NewsBroadcast},
		{"Dialogue", NewTestDataBuilder().Dialogue},
		{"Lecture", NewTestDataBuilder().Lecture},
		{"Interview", NewTestDataBuilder().Interview},
	}

	for _, s := range scenarios {
		input := s.build()
		if input == nil {
			t.Errorf("%s returned nil", s.name)
			continue
		}
		if len(input.SourceSegments) == 0 {
			t.Errorf("%s has no segments", s.name)
		}
		t.Logf("%s: %d segments", s.name, len(input.SourceSegments))
	}
}
