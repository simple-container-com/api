package modes

import (
	"fmt"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/assistant/llm"
)

// ProgressDisplay provides visual feedback during streaming generation
type ProgressDisplay struct {
	startTime    time.Time
	lastUpdate   time.Time
	totalContent string
	prefix       string
	completed    bool
}

// NewProgressDisplay creates a new progress display
func NewProgressDisplay(prefix string) *ProgressDisplay {
	return &ProgressDisplay{
		startTime:  time.Now(),
		lastUpdate: time.Now(),
		prefix:     prefix,
		completed:  false,
	}
}

// StreamCallback returns a callback function that displays progress
func (p *ProgressDisplay) StreamCallback() llm.StreamCallback {
	return func(chunk llm.StreamChunk) error {
		now := time.Now()

		// Update content
		p.totalContent = chunk.Content

		// Show progress every 100ms to avoid too frequent updates
		if now.Sub(p.lastUpdate) > 100*time.Millisecond || chunk.IsComplete {
			p.lastUpdate = now

			if chunk.IsComplete {
				p.completed = true
				elapsed := now.Sub(p.startTime)

				// Show completion message
				fmt.Printf("\n✅ %s completed in %.2fs", p.prefix, elapsed.Seconds())

				if chunk.Usage != nil {
					fmt.Printf(" | Tokens: %d | Cost: $%.4f",
						chunk.Usage.TotalTokens,
						chunk.Usage.Cost)
				}
				fmt.Printf("\n")
			} else {
				// Show progress indicator
				progressChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
				spinnerIndex := int(now.UnixNano()/100000000) % len(progressChars)

				// Estimate progress based on content length (rough approximation)
				contentLength := len(chunk.Content)
				var progressBar string
				if contentLength > 0 {
					// Assume typical YAML is around 500-1000 characters
					estimatedTotal := 750
					progress := float64(contentLength) / float64(estimatedTotal)
					if progress > 1.0 {
						progress = 1.0
					}

					barWidth := 20
					filledWidth := int(progress * float64(barWidth))
					progressBar = fmt.Sprintf(" [%s%s] %d%%",
						strings.Repeat("█", filledWidth),
						strings.Repeat("░", barWidth-filledWidth),
						int(progress*100))
				}

				fmt.Printf("\r%s %s%s", progressChars[spinnerIndex], p.prefix, progressBar)
			}
		}

		return nil
	}
}

// Reset resets the progress display for reuse
func (p *ProgressDisplay) Reset(prefix string) {
	p.startTime = time.Now()
	p.lastUpdate = time.Now()
	p.totalContent = ""
	p.prefix = prefix
	p.completed = false
}

// IsCompleted returns whether the operation is completed
func (p *ProgressDisplay) IsCompleted() bool {
	return p.completed
}

// GetContent returns the current content
func (p *ProgressDisplay) GetContent() string {
	return p.totalContent
}

// SimpleSpinner provides a basic spinner without progress estimation
type SimpleSpinner struct {
	message   string
	startTime time.Time
	stopChan  chan bool
	isRunning bool
}

// NewSimpleSpinner creates a new simple spinner
func NewSimpleSpinner(message string) *SimpleSpinner {
	return &SimpleSpinner{
		message:   message,
		startTime: time.Now(),
		stopChan:  make(chan bool, 1),
		isRunning: false,
	}
}

// Start starts the spinner in a goroutine
func (s *SimpleSpinner) Start() {
	if s.isRunning {
		return
	}

	s.isRunning = true
	s.startTime = time.Now()

	go func() {
		spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		index := 0

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s", spinnerChars[index], s.message)
				index = (index + 1) % len(spinnerChars)
			}
		}
	}()
}

// Stop stops the spinner and shows completion
func (s *SimpleSpinner) Stop(success bool) {
	if !s.isRunning {
		return
	}

	s.isRunning = false
	s.stopChan <- true
	close(s.stopChan)

	elapsed := time.Since(s.startTime)

	if success {
		fmt.Printf("\r✅ %s completed in %.2fs\n", s.message, elapsed.Seconds())
	} else {
		fmt.Printf("\r❌ %s failed after %.2fs\n", s.message, elapsed.Seconds())
	}
}
