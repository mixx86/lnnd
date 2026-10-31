package sched

import "time"

func (s *Service) RecordWriteResult(
    source SourceID,
    writtenBytes int64,
    elapsed time.Duration,
) {
    if writtenBytes <= 0 || elapsed <= 0 {
        return
    }

    s.writeTurnMu.Lock()
    defer s.writeTurnMu.Unlock()

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
