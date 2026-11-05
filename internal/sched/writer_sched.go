package sched

import "time"

func (s *Service) WaitWriteTurn(source SourceID) *Writer {
	w := &Writer{
		source:       source,
		turn:         make(chan struct{}),
		blockedSince: time.Now(),
	}

	s.writeTurnMu.Lock()
	s.waitingWriters = append(s.waitingWriters, w)
	s.writeTurnMu.Unlock()

	s.signalScheduler()

	<-w.turn

	return w
}

func (s *Service) ReleaseWriteTurn(w *Writer, writtenBytes int64) {
	if w == nil {
		return
	}

	s.writeTurnMu.Lock()
	defer s.writeTurnMu.Unlock()

	if _, ok := s.activeWriters[w]; !ok {
		return
	}

	s.recordWriteResultLocked(
		w.source,
		writtenBytes,
		time.Since(w.grantedAt),
	)

	delete(s.activeWriters, w)
	w.granted = false

	s.signalScheduler()
}

func (s *Service) signalScheduler() {
	select {
	case s.schedulerWake <- struct{}{}:
	default:
	}
}

func (s *Service) selectHighestPriorityWriter() int {
	bestIndex := 0
	bestPriority := -1

	for i, w := range s.waitingWriters {
		cfg := s.WCB[w.source]
		if cfg == nil {
			continue
		}

		if int(cfg.weight) > bestPriority {
			bestPriority = int(cfg.weight)
			bestIndex = i
		}
	}

	return bestIndex
}

func (s *Service) dispatchWriters() {
	s.writeTurnMu.Lock()
	defer s.writeTurnMu.Unlock()

	limit := s.maxWriters

	switch s.diskLoad.State {
	case DiskLoadNormal:
		limit = s.maxWriters

	case DiskLoadElevated:
		limit = s.maxWritersElevated

	case DiskLoadSaturated:
		limit = s.maxWritersWhenOverloaded
	}

	for len(s.activeWriters) < limit &&
		len(s.waitingWriters) > 0 {

		selected := 0

		if s.diskLoad.State == DiskLoadSaturated ||
			s.diskLoad.State == DiskLoadElevated {
			selected = s.selectHighestPriorityWriter()
		}

		w := s.waitingWriters[selected]

		s.waitingWriters = append(
			s.waitingWriters[:selected],
			s.waitingWriters[selected+1:]...,
		)

		s.activeWriters[w] = struct{}{}
		w.grantedAt = time.Now()
		w.granted = true
		close(w.turn)
	}
}
