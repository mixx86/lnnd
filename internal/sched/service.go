package sched

import (
	"lnn/internal/queues"

	"sync/atomic"
)

func NewService(diskDevice string) *Service {
	s := &Service{
		queues: map[queues.QueueID]any{
			queues.RequestsQueueID: queues.NewRequestsQueue(),
			queues.FilesQueueID:    queues.NewFilesQueue(),
		},
		pressure: map[SourceID]*atomic.Uint32{
			RequestsCacheID: &atomic.Uint32{},
			RequestsID:      &atomic.Uint32{},
			FilesID:         &atomic.Uint32{},
		},
		activeWriters: map[*Writer]struct{}{},
	}

	s.WCB = map[SourceID]*WriterConfig{
		RequestsCacheID: {
			baseWeight:    RequestsCacheWeight,
			weight:        RequestsCacheWeight,
			expectedBytes: 4 << 10,
		},
		RequestsID: {
			baseWeight:    RequestsWeight,
			weight:        RequestsWeight,
			expectedBytes: 16 << 10,
		},
		FilesID: {
			baseWeight:    FilesWeight,
			weight:        FilesWeight,
			expectedBytes: 1 << 20,
		},
	}

	s.sourceQueues = map[SourceID]queues.QueueID{
		RequestsID: queues.RequestsQueueID,
		FilesID:    queues.FilesQueueID,
	}
	QueuePressureTemplate := QueuePressure{
		MaxQueuedBytes: 0,
	}
	s.queuePressure = map[queues.QueueID]QueuePressure{
		queues.RequestsQueueID: QueuePressureTemplate,
		queues.FilesQueueID:    QueuePressureTemplate,
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
