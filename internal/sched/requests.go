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
	tag uint32,
) error {
	_, err := s.submitRequest(from, req, tag, false)
	return err
}

func (s *Service) SubmitSentRequest(
	to *peer.RemoteNode,
	req protocol.InboundRequest,
) (uint32, error) {
	return s.submitRequest(to, req, 0, true)
}

func (s *Service) submitRequest(
	remote *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint32,
	sent bool,
) (uint32, error) {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return 0, errors.New("error asserting the requests queue")
	}

	if !s.WaitForPressureReleaseWithTimeout(RequestsID, pressureWaitTimeout) {
		return 0, ErrOverloaded
	}

	pr, ok := q.TryEnqueueWithTimeout(
		remote,
		req,
		tag,
		sent,
		requestEnqueueTimeout,
	)
	if !ok {
		return 0, ErrRequestsQueueFull
	}

	s.UpdateQueuePressure(queues.RequestsQueueID, int64(pr.Req.Size()))

	select {
	case res := <-pr.Written:
		if res.Err != nil {
			return 0, res.Err
		}
		return res.LCLReqID, nil
	case <-time.After(writeConfirmTimeout):
		return 0, ErrWriteTimeout
	}
}

func (s *Service) NextRequest() (queues.PendingRequest, error) {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return queues.PendingRequest{}, errors.New("error asserting the queue")
	}
	pr := q.Dequeue()
	s.UpdateQueuePressure(queues.RequestsQueueID, -int64(pr.Req.Size()))
	return pr, nil
}

func (s *Service) TryNextRequest() (queues.PendingRequest, bool, error) {
	q, ok := s.queues[queues.RequestsQueueID].(*queues.RequestsQueue)
	if !ok {
		return queues.PendingRequest{}, false, errors.New(
			"error asserting the queue",
		)
	}

	req, ok := q.TryDequeue()
	if ok {
		s.UpdateQueuePressure(
			queues.RequestsQueueID,
			-int64(req.Req.Size()),
		)
	}
	return req, ok, nil
}
