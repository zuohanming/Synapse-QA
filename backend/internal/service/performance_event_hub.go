package service

import (
	"encoding/json"
	"sync"
	"time"

	"synapseqa/backend/internal/model"
)

// PerfStreamMessage 是 SSE 层消费的统一消息。
type PerfStreamMessage struct {
	ID    int64
	Event string
	Data  any
}

type perfHubRun struct {
	series         model.PerfSeries
	ring           []model.PerfSampleEvent
	terminal       bool
	terminalStatus string
	subscribers    map[int]chan PerfStreamMessage
	nextSubID      int
	terminalAt     time.Time
}

// PerfEventHub 是单实例内存事件总线；事件丢失时通过 reset 让客户端重新取完整快照。
type PerfEventHub struct {
	mu   sync.Mutex
	runs map[int64]*perfHubRun
}

const perfHubTerminalTTL = 2 * time.Minute

func NewPerfEventHub() *PerfEventHub {
	return &PerfEventHub{runs: make(map[int64]*perfHubRun)}
}

func (h *PerfEventHub) ensure(runID int64) *perfHubRun {
	item := h.runs[runID]
	if item == nil {
		item = &perfHubRun{
			series:      model.PerfSeries{Version: 1, Points: []model.PerfSampleEvent{}, StatusCodes: map[string]int{}, ErrorTopN: []model.PerfErrorTop{}, Thresholds: []model.PerfSampleThreshold{}},
			subscribers: make(map[int]chan PerfStreamMessage),
		}
		h.runs[runID] = item
	}
	return item
}

func (h *PerfEventHub) Publish(runID int64, event model.PerfSampleEvent) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(time.Now())
	item := h.ensure(runID)
	if item.terminal || event.Sequence <= item.series.LastSequence {
		return false
	}
	item.series.Version = 1
	item.series.LastSequence = event.Sequence
	item.series.IntervalMs = int(event.WindowMs)
	if item.series.StartedAt == "" {
		item.series.StartedAt = event.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	item.series.Points = append(item.series.Points, event)
	if len(item.series.Points) > 3000 {
		item.series.Points = downsamplePerfPoints(item.series.Points, 3000)
	}
	item.series.StatusCodes = cloneStatusCodes(event.StatusCodes)
	item.series.ErrorTopN = cloneErrorTop(event.ErrorTopN)
	item.series.Thresholds = cloneThresholds(event.Thresholds)
	item.ring = append(item.ring, event)
	if len(item.ring) > 256 {
		item.ring = item.ring[len(item.ring)-256:]
	}
	message := PerfStreamMessage{ID: event.Sequence, Event: "perf.sample", Data: perfSampleEventPayload(event)}
	for _, subscriber := range item.subscribers {
		nonBlockingSend(subscriber, message, PerfStreamMessage{ID: item.series.LastSequence, Event: "perf.reset", Data: cloneSeries(item.series)})
	}
	return true
}

func (h *PerfEventHub) MarkTerminal(runID int64, status string, series model.PerfSeries) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(time.Now())
	item := h.ensure(runID)
	if item.terminal {
		return
	}
	item.terminal = true
	item.terminalStatus = status
	item.terminalAt = time.Now()
	item.series = cloneSeries(series)
	item.series.Partial = series.Partial
	// 终态回调可能先于最后一个 sample callback 到达；用持久化 series 补齐 replay ring，避免刷新只收到 terminal。
	if len(item.series.Points) > 0 {
		start := 0
		if len(item.series.Points) > 256 {
			start = len(item.series.Points) - 256
		}
		item.ring = append([]model.PerfSampleEvent{}, item.series.Points[start:]...)
	}
	message := PerfStreamMessage{ID: item.series.LastSequence, Event: "perf.terminal", Data: map[string]any{"status": status, "series": item.series}}
	for id, subscriber := range item.subscribers {
		nonBlockingSend(subscriber, message, PerfStreamMessage{ID: item.series.LastSequence, Event: "perf.reset", Data: item.series})
		close(subscriber)
		delete(item.subscribers, id)
	}
}

func (h *PerfEventHub) Snapshot(runID int64) (model.PerfSeries, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(time.Now())
	item, ok := h.runs[runID]
	if !ok || item.series.LastSequence == 0 {
		return model.PerfSeries{}, false
	}
	return cloneSeries(item.series), true
}

func (h *PerfEventHub) Subscribe(runID int64, after int64) (<-chan PerfStreamMessage, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(time.Now())
	item := h.ensure(runID)
	channel := make(chan PerfStreamMessage, len(item.ring)+2)
	item.nextSubID++
	subID := item.nextSubID
	item.subscribers[subID] = channel
	if len(item.ring) > 0 && after < item.ring[0].Sequence-1 {
		nonBlockingSend(channel, PerfStreamMessage{ID: item.series.LastSequence, Event: "perf.reset", Data: cloneSeries(item.series)}, PerfStreamMessage{})
	} else {
		for _, event := range item.ring {
			if event.Sequence > after {
				nonBlockingSend(channel, PerfStreamMessage{ID: event.Sequence, Event: "perf.sample", Data: perfSampleEventPayload(event)}, PerfStreamMessage{})
			}
		}
	}
	if item.terminal {
		nonBlockingSend(channel, PerfStreamMessage{ID: item.series.LastSequence, Event: "perf.terminal", Data: map[string]any{"status": item.terminalStatus, "series": item.series}}, PerfStreamMessage{})
		close(channel)
		delete(item.subscribers, subID)
	}
	return channel, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if current, ok := item.subscribers[subID]; ok {
			delete(item.subscribers, subID)
			close(current)
		}
	}
}

func perfSampleEventPayload(event model.PerfSampleEvent) map[string]any {
	encoded, _ := json.Marshal(event)
	var payload map[string]any
	_ = json.Unmarshal(encoded, &payload)
	if payload["statusCodes"] == nil {
		payload["statusCodes"] = map[string]int{}
	}
	if payload["errorTopN"] == nil {
		payload["errorTopN"] = []model.PerfErrorTop{}
	}
	if payload["thresholds"] == nil {
		payload["thresholds"] = []model.PerfSampleThreshold{}
	}
	if _, ok := payload["message"]; !ok {
		payload["message"] = ""
	}
	return payload
}

// CleanupExpired 删除已结束且超过短 TTL 的运行事件，供定时器和测试调用。
func (h *PerfEventHub) CleanupExpired(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(now)
}

func (h *PerfEventHub) cleanupLocked(now time.Time) {
	for runID, item := range h.runs {
		if item.terminal && !item.terminalAt.IsZero() && now.Sub(item.terminalAt) >= perfHubTerminalTTL {
			delete(h.runs, runID)
		}
	}
}

func nonBlockingSend(channel chan PerfStreamMessage, message, fallback PerfStreamMessage) {
	select {
	case channel <- message:
	default:
		select {
		case <-channel:
		default:
		}
		if fallback.Event != "" {
			select {
			case channel <- fallback:
			default:
			}
		}
	}
}

func cloneSeries(series model.PerfSeries) model.PerfSeries {
	series.Points = append([]model.PerfSampleEvent{}, series.Points...)
	series.StatusCodes = cloneStatusCodes(series.StatusCodes)
	series.ErrorTopN = cloneErrorTop(series.ErrorTopN)
	series.Thresholds = cloneThresholds(series.Thresholds)
	return series
}

func cloneStatusCodes(values map[string]int) map[string]int {
	result := map[string]int{}
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneErrorTop(values []model.PerfErrorTop) []model.PerfErrorTop {
	return append([]model.PerfErrorTop{}, values...)
}

func cloneThresholds(values []model.PerfSampleThreshold) []model.PerfSampleThreshold {
	return append([]model.PerfSampleThreshold{}, values...)
}

func downsamplePerfPoints(points []model.PerfSampleEvent, limit int) []model.PerfSampleEvent {
	if len(points) <= limit {
		return points
	}
	result := make([]model.PerfSampleEvent, 0, limit)
	for index := 0; index < limit; index++ {
		position := index * (len(points) - 1) / (limit - 1)
		result = append(result, points[position])
	}
	return result
}
