package sched

import "time"

// WARNING: recordWriteResultLocked must be called with writeTurnMu held.
func (s *Service) recordWriteResultLocked(
	source SourceID,
	writtenBytes int64,
	elapsed time.Duration,
) {
	if writtenBytes <= 0 || elapsed <= 0 {
		return
	}

	cfg, ok := s.WCB[source]
	if !ok || cfg.expectedBytes <= 0 {
		return
	}

	normalized := time.Duration(
		float64(elapsed) *
			float64(cfg.expectedBytes) /
			float64(writtenBytes),
	)

	const alpha = 0.1

	if cfg.samples == 0 {
		cfg.baseline = normalized
	} else {
		cfg.baseline = time.Duration(
			(1-alpha)*float64(cfg.baseline) +
				alpha*float64(normalized),
		)
	}

	cfg.samples++
}
