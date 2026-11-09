package sched

import "time"

const (
	recentWriteTimeAlpha  = 0.1
	typicalWriteTimeAlpha = 0.01

	expectedBytesAlpha = 0.0025

	maxOverrun = 2
)

// WARNING: recordWriteResultLocked must be called with writeTurnMu held.
func (s *Service) recordWriteResultLocked(
	source SourceID,
	writtenBytes int64,
	elapsed time.Duration,
) {
	if writtenBytes <= 0 || elapsed <= 0 {
		return
	}

	if int(source) >= len(s.WCB) {
		return
	}
	cfg := &s.WCB[source]
	if cfg.expectedBytes <= 0 {
		return
	}

	normalized := time.Duration(
		float64(elapsed) *
			float64(cfg.expectedBytes) /
			float64(writtenBytes),
	)

	cfg.expectedBytes = ema(
		cfg.expectedBytes,
		writtenBytes,
		expectedBytesAlpha,
	)

	if cfg.samples == 0 {
		cfg.recentWriteTime = normalized
	} else {
		cfg.recentWriteTime = ema(
			cfg.recentWriteTime,
			normalized,
			recentWriteTimeAlpha,
		)
	}
	cfg.samples++

	if cfg.typicalWriteTime > 0 {
		overrun := float64(cfg.recentWriteTime) /
			float64(cfg.typicalWriteTime)
		cfg.weight = cfg.baseWeight * min(max(overrun, 1), maxOverrun)
	}

	diskNormal := s.diskLoad.GetState() == DiskLoadNormal

	if !diskNormal {
		return
	}
	if cfg.typicalWriteTime == 0 {
		cfg.typicalWriteTime = normalized
	} else {
		cfg.typicalWriteTime = ema(
			cfg.typicalWriteTime,
			normalized,
			typicalWriteTimeAlpha,
		)
	}
}

type emaValue interface {
	~int64 | ~float64
}

func ema[T emaValue](prev T, sample T, alpha float64) T {
	return T((1-alpha)*float64(prev) + alpha*float64(sample))
}
