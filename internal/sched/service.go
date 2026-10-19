package sched

import (
	"errors"
	"lnn/internal/config"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"sync"
	"sync/atomic"
	"time"
)

func NewService() *Service {
	s := &Service{
		queues: map[queues.QueueID]any{
			queues.RequestsQueueID: queues.NewRequestsQueue(),
			queues.FilesQueueID:    queues.NewFilesQueue(),
		},
		pressure: map[BPSourceID]*atomic.Uint32{
			BPRequestsCacheID: &atomic.Uint32{},
			BPRequestsID:      &atomic.Uint32{},
			BPFilesID: 		   &atomic.Uint32{},
		},
		threshold: config.MaxMemFromEnv(),
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

func (s *Service) AddPressure(source BPSourceID) {
	s.pressureChangeCond.L.Lock()
	defer s.pressureChangeCond.L.Unlock()

	pressure := s.pressure[source]
	if pressure == nil {
		return
	}

	pressure.Add(1)
	s.pressureChangeCond.Broadcast()
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
	case BPRequestsCacheID:
		return 0
	case BPRequestsID:
		return 1
	case BPFilesID:
		return 2
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

const requestEnqueueTimeout = 100 * time.Millisecond
const writeConfirmTimeout = 10 * time.Second

func (s *Service) SubmitRequest(
	from *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint16,
) error {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return errors.New("error asserting the requests queue")
	}

	s.WaitForPressureRelease(BPRequestsID)

	pr, ok := q.TryEnqueueWithTimeout(from, req, tag, requestEnqueueTimeout)
	if !ok {
		return ErrRequestsQueueFull
	}

	select {
	case err := <-pr.Written:
		if err != nil {
			return err
		}
	case <-time.After(writeConfirmTimeout):
		return ErrWriteTimeout
	}

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
