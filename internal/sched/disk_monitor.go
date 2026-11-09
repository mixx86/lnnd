package sched

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

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

func (s *Service) pollDiskLoad() {
	for {
		util, err := DiskUtilization(s.diskDevice, 200*time.Millisecond)
		if err == nil {
			s.UpdateDiskUtilization(util)
		}
		time.Sleep(800 * time.Millisecond)
	}
}

func (s *Service) UpdateDiskUtilization(util float64) {
	const (
		alpha     = 0.2
		highLimit = 90.0
		lowLimit  = 65.0
		holdTime  = 2 * time.Second
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

	previousState := d.GetState()

	switch {
	case d.SmoothedUtil > highLimit:
		d.LowSince = time.Time{}

		if d.HighSince.IsZero() {
			d.HighSince = now
		}

		if now.Sub(d.HighSince) >= holdTime {
			d.SetState(uint32(DiskLoadSaturated))
		}

	case d.SmoothedUtil < lowLimit:
		d.HighSince = time.Time{}

		if d.LowSince.IsZero() {
			d.LowSince = now
		}

		if now.Sub(d.LowSince) >= holdTime {
			d.SetState(uint32(DiskLoadNormal))
		}

	default:
		d.HighSince = time.Time{}
		d.LowSince = time.Time{}

		if d.GetState() == DiskLoadNormal {
			d.SetState(uint32(DiskLoadElevated))
		} else if d.GetState() == DiskLoadSaturated {
			d.SetState(uint32(DiskLoadElevated))
		}
	}

	changed := previousState != d.GetState()

	s.diskTrackerMu.Unlock()

	if changed {
		s.signalScheduler()
	}
}
