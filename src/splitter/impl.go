package splitter

import (
	timeline "github.com/wnk/timeline/src"
)

// splitterImpl implements the Splitter interface
type splitterImpl struct{}

// NewSplitter creates a new Splitter instance
func NewSplitter() timeline.Splitter {
	return &splitterImpl{}
}

// Split divides the segment list into independent sub-problems at natural breakpoints
// A breakpoint is identified when the gap between consecutive segments >= threshold
func (s *splitterImpl) Split(
	sources []timeline.SourceSegment,
	tts []timeline.TTSSegment,
	threshold timeline.Duration,
) []timeline.SubProblem {
	n := len(sources)
	if n == 0 {
		return []timeline.SubProblem{}
	}

	var subProblems []timeline.SubProblem
	currentSources := make([]timeline.SourceSegment, 0, n)
	currentTTS := make([]timeline.TTSSegment, 0, n)
	subProblemID := 0

	for i := 0; i < n; i++ {
		// Add current segment to the current sub-problem
		currentSources = append(currentSources, sources[i])
		currentTTS = append(currentTTS, tts[i])

		// Check if this is the last segment or there's a breakpoint
		isLast := i == n-1
		var isBreakpoint bool

		if !isLast {
			gap := sources[i+1].OrigStart() - sources[i].OrigEnd()
			isBreakpoint = gap >= threshold
		}

		if isLast || isBreakpoint {
			// Finalize current sub-problem
			// Make copies to avoid slice mutation issues
			sourcesCopy := make([]timeline.SourceSegment, len(currentSources))
			ttsCopy := make([]timeline.TTSSegment, len(currentTTS))
			copy(sourcesCopy, currentSources)
			copy(ttsCopy, currentTTS)

			subProblems = append(subProblems, timeline.SubProblem{
				ID:             subProblemID,
				SourceSegments: sourcesCopy,
				TTSSegments:    ttsCopy,
			})

			subProblemID++

			// Reset for next sub-problem
			currentSources = currentSources[:0]
			currentTTS = currentTTS[:0]
		}
	}

	return subProblems
}
