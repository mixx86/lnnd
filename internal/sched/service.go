package sched

import (
	"lnn/internal/queues"

	"sync"
	"sync/atomic"
)

func NewService(diskDevice string) *Service {
	requestsQueue := queues.NewRequestsQueue()
	s := &Service{
		queues: map[queues.QueueID]any{
			queues.RequestsQueueID: requestsQueue,
			queues.FilesQueueID:    queues.NewFilesQueue(),
		},
		pressure: map[SourceID]*atomic.Uint32{
			RequestsCacheID: &atomic.Uint32{},
			RequestsID:      &atomic.Uint32{},
			FilesID:         &atomic.Uint32{},
		},
		activeWriters: map[*Writer]struct{}{},

		pressureChangeCond:  sync.NewCond(&sync.Mutex{}),
		pressureReleaseCond: sync.NewCond(&sync.Mutex{}),
	}

	s.WCB = map[SourceID]*WriterConfig{
		RequestsCacheID: {
			baseWeight:        RequestsCacheWeight,
			weight:            RequestsCacheWeight,
			baseExpectedBytes: 4 << 10,
			expectedBytes:     4 << 10,
		},
		RequestsID: {
			baseWeight:        RequestsWeight,
			weight:            RequestsWeight,
			baseExpectedBytes: 16 << 10,
			expectedBytes:     16 << 10,
		},
		FilesID: {
			baseWeight:        FilesWeight,
			weight:            FilesWeight,
			baseExpectedBytes: 1 << 20,
			expectedBytes:     1 << 20,
		},
	}

	s.sourceQueues = map[SourceID]queues.QueueID{
		RequestsID: queues.RequestsQueueID,
		FilesID:    queues.FilesQueueID,
	}
	s.queuePressure = map[queues.QueueID]*QueuePressure{
		queues.RequestsQueueID: {Queue: requestsQueue.Config()},
	}

	s.maxWriters = 8
	s.maxWritersElevated = 4
	s.maxWritersWhenOverloaded = 2
	s.diskDevice = diskDevice
	s.schedulerWake = make(chan struct{}, 1)

	return s
}

func (s *Service) Run() {
	if s.diskDevice != "" {
		go s.sampleDisk()
	}

	for range s.schedulerWake {
		s.dispatchWriters()
	}
}
