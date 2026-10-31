package sched

import (
	"errors"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"time"
)

func (s *Service) SubmitRequest(
	from *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint16,
) error {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return errors.New("error asserting the requests queue")
	}

	if !s.WaitForPressureReleaseWithTimeout(RequestsID, pressureWaitTimeout) {
		return ErrOverloaded
	}

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
