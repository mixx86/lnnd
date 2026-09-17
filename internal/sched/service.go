package sched

import (
	"errors"
	"lnn/internal/queues"
	"sync"
	"sync/atomic"
)

func NewService() *Service {
	s := &Service{
		queues: map[queues.QueueID]any{
			queues.RequestsQueueID: queues.NewRequestsQueue(),
			queues.FilesQueueID:    queues.NewFilesQueue(),
		},
		pressure: map[BPSourceID]*atomic.Uint32{
			BPRequestsID: &atomic.Uint32{},
		},
		threshold: maxMemFromEnv(),
	}

	s.pressureReleaseCond = sync.NewCond(&s.pressureMu)
	s.pressureChangeCond = sync.NewCond(&s.pressureChangeMu)

	return s
}

func (s *Service) WaitForPressureRelease(source BPSourceID) {
	s.pressureReleaseCond.L.Lock()
	defer s.pressureReleaseCond.L.Unlock()

	for s.IsBackPressured(source) {
		s.pressureReleaseCond.Wait()
	}
}

func (s *Service) WaitForPressureChange() {
	s.pressureChangeCond.L.Lock()
	defer s.pressureChangeCond.L.Unlock()

	s.pressureChangeCond.Wait()
}

func (s *Service) ReleasePressure(source BPSourceID) {
	s.pressureReleaseCond.L.Lock()
	defer s.pressureReleaseCond.L.Unlock()
	pressure := s.pressure[source]

	for {
		current := pressure.Load()

		if current == 0 {
			return
		}

		if pressure.CompareAndSwap(current, current-1) {
			s.pressureChangeCond.Broadcast()
			if current == 1 {
				s.pressureReleaseCond.Broadcast()
			}
			return
		}
	}
}

func bpPriority(source BPSourceID) uint16 {
	switch source {
	case BPRequestsID:
		return 0
	case BPFilesID:
		return 1
	default:
		return 0
	}
}

func (s *Service) IsBackPressured(source BPSourceID) bool {
	priority := bpPriority(source)

	for id, pressure := range s.pressure {
		if bpPriority(id) > priority && pressure.Load() > 0 {
			return true
		}
	}

	return false
}

func SubmitRequest() error {
	return nil
}

func SubmitLinkedFile() error {
	return nil
}

func (s *Service) NextRequest() (queues.PendingRequest, error) {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return queues.PendingRequest{}, errors.New("error asserting the queue")
	}
	return q.Dequeue(), nil
}

func (s *Service) TryNextRequest() (queues.PendingRequest, bool, error) {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return queues.PendingRequest{}, false, errors.New("error asserting the queue")
	}

	req, ok := q.TryDequeue()
	return req, ok, nil
}

func NextFile() {

}
