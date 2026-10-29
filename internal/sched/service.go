package sched

import (
	"errors"
	"fmt"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"

	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

const (
    RequestsCacheWeight = 1.0
    RequestsWeight      = 2.0
    FilesWeight         = 4.0
)


const (
    requestEnqueueTimeout = 100 * time.Millisecond
    writeConfirmTimeout   = 10 * time.Second
    pressureWaitTimeout   = 2 * time.Second
)

func NewService() *Service {
	s := &Service{
		queues: map[queues.QueueID]any{
			queues.RequestsQueueID: queues.NewRequestsQueue(),
			queues.FilesQueueID:    queues.NewFilesQueue(),
		},
		pressure: map[SourceID]*atomic.Uint32{
			RequestsCacheID: &atomic.Uint32{},
			RequestsID:      &atomic.Uint32{},
			FilesID: 		   &atomic.Uint32{},
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
    	RequestsID:      queues.RequestsQueueID,
    	FilesID:         queues.FilesQueueID,
	}
	QueuePressureTemplate := QueuePressure{
		MaxQueuedBytes: 0,
	}
	s.queuePressure = map[queues.QueueID]QueuePressure{
		queues.RequestsQueueID:  QueuePressureTemplate,
		queues.FilesQueueID: 	 QueuePressureTemplate,
	}

	s.maxWriters = 8
	s.maxWritersElevated = 4
	s.maxWritersWhenOverloaded = 2
	s.diskDevice = "nvme0n1"
	s.schedulerWake = make(chan struct{}, 1)

	return s
}

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
    deadline := time.Now().Add(timeout)
    ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()

    for s.IsBackPressured(source) {
        remaining := time.Until(deadline)
        if remaining <= 0 {
            return false
        }

        select {
        case <-ticker.C:
        case <-time.After(remaining):
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

func (s *Service) UpdateQueuePressure(
    queueID queues.QueueID,
    queuedBytes int64,
) {
    pressure, ok := s.queuePressure[queueID]
    if !ok {
        return
    }

    pressure.QueuedBytes.Store(queuedBytes)
    pressured := queuedBytes >= pressure.MaxQueuedBytes
    wasPressured := pressure.Pressured.Swap(pressured)

    if wasPressured && !pressured {
        s.pressureReleaseCond.Broadcast()
    }

    if wasPressured != pressured {
        s.pressureChangeCond.Broadcast()
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

func NextFile() {

}
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

func (s *Service) ReleaseWriteTurn(w *Writer) {
	if w == nil {
		return
	}

	s.writeTurnMu.Lock()
	defer s.writeTurnMu.Unlock()

	if _, ok := s.activeWriters[w]; !ok {
		return
	}

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

func DiskUtilization(device string, interval time.Duration) (float64, error) {
	if interval <= 0 {
		return 0, fmt.Errorf("interval must be positive")
	}

	before, err := disk.IOCounters()
	if err != nil {
		return 0, err
	}

	first, ok := before[device]
	if !ok {
		return 0, fmt.Errorf("device %q not found", device)
	}

	time.Sleep(interval)

	after, err := disk.IOCounters()
	if err != nil {
		return 0, err
	}

	last, ok := after[device]
	if !ok {
		return 0, fmt.Errorf("device %q not found", device)
	}

	if last.IoTime < first.IoTime {
		return 0, fmt.Errorf("disk counters reset")
	}

	elapsedMS := float64(interval) / float64(time.Millisecond)
	busyMS := float64(last.IoTime - first.IoTime)

	return busyMS / elapsedMS * 100, nil
}

func (s *Service) sampleDisk() {
	for {
		util, err := DiskUtilization(s.diskDevice, 200*time.Millisecond)
		if err == nil {
			s.UpdateDiskUtilization(util)
		}
		time.Sleep(800 * time.Millisecond)
	}
}

func (s *Service) Run() {
	go s.sampleDisk()

	for range s.schedulerWake {
		s.dispatchWriters()
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

        if s.diskLoad.State == DiskLoadSaturated || s.diskLoad.State == DiskLoadElevated {
            selected = s.selectHighestPriorityWriter()
        }

        w := s.waitingWriters[selected]

        s.waitingWriters = append(
            s.waitingWriters[:selected],
            s.waitingWriters[selected+1:]...,
        )

        s.activeWriters[w] = struct{}{}
        w.granted = true
        close(w.turn)
    }
}

func (s *Service) UpdateDiskUtilization(util float64) {
    const (
        alpha       = 0.2
        highLimit   = 90.0
        lowLimit    = 65.0
        holdTime    = 2 * time.Second
    )

    now := time.Now()

    s.diskTrackerMu.Lock()

    d := &s.diskLoad

    if !d.Initialized {
        d.SmoothedUtil = util
        d.Initialized = true
    } else {
        d.SmoothedUtil = alpha*util +
            (1-alpha)*d.SmoothedUtil
    }

    previousState := d.State

    switch {
    case d.SmoothedUtil > highLimit:
        d.LowSince = time.Time{}

        if d.HighSince.IsZero() {
            d.HighSince = now
        }

        if now.Sub(d.HighSince) >= holdTime {
            d.State = DiskLoadSaturated
        }

    case d.SmoothedUtil < lowLimit:
        d.HighSince = time.Time{}

        if d.LowSince.IsZero() {
            d.LowSince = now
        }

        if now.Sub(d.LowSince) >= holdTime {
            d.State = DiskLoadNormal
        }

    default:
        d.HighSince = time.Time{}
        d.LowSince = time.Time{}

        if d.State == DiskLoadNormal {
            d.State = DiskLoadElevated
        } else if d.State == DiskLoadSaturated {
            d.State = DiskLoadElevated
        }
    }

    changed := previousState != d.State

    s.diskTrackerMu.Unlock()

    if changed {
        s.signalScheduler()
    }
}

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
