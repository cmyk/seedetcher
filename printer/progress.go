package printer

// PrintStage represents the phase of a print job for progress reporting.
type PrintStage int

const (
	StagePrepare PrintStage = iota // Rendering plate bitmaps
	StageCompose                   // Assembling pages
	StageSend                      // Streaming to printer
)

// ProgressFunc reports progress for a given stage (current out of total).
// Implementations should handle multiple calls per stage.
type ProgressFunc func(stage PrintStage, current, total int64)

