package sched

import (
	"lnn/internal/queues"
	"time"
)

func (s *Service) WaitForPressureRelease(source SourceID) {
	for s.IsBackPressured(source) {
		s.pressureReleaseCond.L.Lock()

		if s.IsBackPressured(source) {
			s.pressureReleaseCond.Wait()
		}

		s.pressureReleaseCond.L.Unlock()
	}
}

func (s *Service) WaitForPressureReleaseWithTimeout(
	source SourceID,
	timeout time.Duration,
) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for s.IsBackPressured(source) {
		select {
		case <-ticker.C:
		case <-timer.C:
			return !s.IsBackPressured(source)
		}
	}

	return true
}

func (s *Service) WaitForPressureChange() {
	s.pressureChangeCond.L.Lock()
	s.pressureChangeCond.Wait()
	s.pressureChangeCond.L.Unlock()
}

// UpdateQueuePressure takes a delta rather than a total so that enqueue
// and dequeue paths can report their own sizes without coordinating.
func (s *Service) UpdateQueuePressure(queueID queues.QueueID, delta int64) {
	pressure, ok := s.queuePressure[queueID]
	if !ok {
		return
	}

	total := pressure.Queue.QueuedBytes.Add(delta)
	for {
		pressured := total >= pressure.Queue.MaxQueuedBytes
		wasPressured := pressure.Pressured.Swap(pressured)

		if wasPressured && !pressured {
			s.pressureReleaseCond.Broadcast()
		}
		if wasPressured != pressured {
			s.pressureChangeCond.Broadcast()
		}

		// A concurrent update may have swapped the flag between our Add
		// and Swap; re-evaluate until the flag matches the latest total,
		// or a stale "pressured" could outlive an empty queue.
		current := pressure.Queue.QueuedBytes.Load()
		if current == total {
			return
		}
		total = current
	}
}

func (s *Service) IsBackPressured(source SourceID) bool {
	queueID, ok := s.sourceQueues[source]
	if !ok {
		return false
	}

	// A bit of a hack to work around the `any` queue map and get at
	// the shared pressure metrics without messing with the actual queue types.
	pressure, ok := s.queuePressure[queueID]
	if !ok {
		return false
	}

	return pressure.Pressured.Load()
}
