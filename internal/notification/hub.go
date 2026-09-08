package notification

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrConnectionLimit 表示本进程或账号连接数达到上限。
	ErrConnectionLimit = errors.New("notification connection limit reached")
	// ErrClosed 表示通知服务已经关闭。
	ErrClosed = errors.New("notification service is closed")
)

// Hub 保存进程内在线连接。Hub 不保存历史消息；连接断开或进程重启后不回放。
type Hub struct {
	mu                    sync.RWMutex
	subscribers           map[uint]map[*subscription]struct{}
	connections           int
	maxConnectionsPerUser int
	maxConnections        int
	queueSize             int
	mergeWindow           time.Duration
	closed                bool
	dropped               atomic.Uint64
	rejected              atomic.Uint64
}

// HubMetrics 是进程内通知通道的结构化运行指标快照。它不包含消息内容，
// 可由健康检查、日志或未来指标适配器按需读取。
type HubMetrics struct {
	ActiveConnections   int    `json:"active_connections"`
	DroppedMessages     uint64 `json:"dropped_messages"`
	RejectedConnections uint64 `json:"rejected_connections"`
	Closed              bool   `json:"closed"`
}

type subscription struct {
	userID  uint
	events  chan Notification
	hub     *Hub
	mu      sync.Mutex
	closed  bool
	pending map[string]*pendingNotification
}

type pendingNotification struct {
	n     Notification
	timer *time.Timer
}

// NewHub 创建有界的进程内通知 Hub。
func NewHub(config Config) *Hub {
	config = config.withDefaults()
	return &Hub{
		subscribers:           make(map[uint]map[*subscription]struct{}),
		maxConnectionsPerUser: config.MaxConnectionsPerUser,
		maxConnections:        config.MaxConnections,
		queueSize:             config.QueueSize,
		mergeWindow:           config.MergeWindow,
	}
}

// Subscribe 注册一个账号连接，并返回只读消息队列和取消函数。
//
// 每个连接都有独立有界队列。队列满时丢弃该通知，不阻塞业务写请求。
func (h *Hub) Subscribe(userID uint) (<-chan Notification, func(), error) {
	if h == nil || userID == 0 {
		return nil, func() {}, ErrClosed
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, func() {}, ErrClosed
	}
	if h.connections >= h.maxConnections {
		h.rejected.Add(1)
		return nil, func() {}, ErrConnectionLimit
	}
	byUser := h.subscribers[userID]
	if len(byUser) >= h.maxConnectionsPerUser {
		h.rejected.Add(1)
		return nil, func() {}, ErrConnectionLimit
	}
	sub := &subscription{
		userID:  userID,
		events:  make(chan Notification, h.queueSize),
		hub:     h,
		pending: make(map[string]*pendingNotification),
	}
	if byUser == nil {
		byUser = make(map[*subscription]struct{})
		h.subscribers[userID] = byUser
	}
	byUser[sub] = struct{}{}
	h.connections++
	var once sync.Once
	cancel := func() { once.Do(func() { h.unsubscribe(sub) }) }
	return sub.events, cancel, nil
}

func (h *Hub) unsubscribe(sub *subscription) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	if byUser, ok := h.subscribers[sub.userID]; ok {
		if _, exists := byUser[sub]; exists {
			delete(byUser, sub)
			h.connections--
			if len(byUser) == 0 {
				delete(h.subscribers, sub.userID)
			}
		}
	}
	h.mu.Unlock()
	sub.close()
}

// PublishTo 将已经构造好的通知投递给指定账号的全部在线连接。
//
// 投递不等待连接消费，保证慢终端不会阻塞其他用户或业务请求。
func (h *Hub) PublishTo(userIDs []uint, n Notification) {
	if h == nil || len(userIDs) == 0 {
		return
	}
	seen := make(map[uint]struct{}, len(userIDs))
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	subs := make([]*subscription, 0)
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		for sub := range h.subscribers[userID] {
			subs = append(subs, sub)
		}
	}
	h.mu.RUnlock()
	for _, sub := range subs {
		sub.enqueue(n)
	}
}

func (s *subscription) enqueue(n Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	key := n.Module
	if key == "" {
		key = "_"
	}
	if pending, ok := s.pending[key]; ok {
		pending.n = mergeNotifications(pending.n, n)
		if pending.timer != nil {
			pending.timer.Reset(s.hub.mergeWindow)
		}
		return
	}
	pending := &pendingNotification{n: n}
	s.pending[key] = pending
	pending.timer = time.AfterFunc(s.hub.mergeWindow, func() {
		s.flush(key, pending)
	})
}

func (s *subscription) flush(key string, pending *pendingNotification) {
	disconnect := false
	s.mu.Lock()
	current, ok := s.pending[key]
	if !ok || current != pending {
		s.mu.Unlock()
		return
	}
	delete(s.pending, key)
	if s.closed {
		s.mu.Unlock()
		return
	}
	select {
	case s.events <- pending.n:
	default:
		s.hub.dropped.Add(1)
		// 连接已经无法在有界队列内跟上实时流；关闭它让客户端按
		// SSE 重连策略恢复，而不是无限保留一个失效订阅。
		disconnect = true
	}
	s.mu.Unlock()
	if disconnect {
		s.hub.unsubscribe(s)
	}
}

func (s *subscription) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for key, pending := range s.pending {
		if pending != nil && pending.timer != nil {
			pending.timer.Stop()
		}
		delete(s.pending, key)
	}
	close(s.events)
	s.mu.Unlock()
}

// Close 停止全部连接并丢弃未发送消息。
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	subs := make([]*subscription, 0, h.connections)
	for _, byUser := range h.subscribers {
		for sub := range byUser {
			subs = append(subs, sub)
		}
	}
	h.subscribers = make(map[uint]map[*subscription]struct{})
	h.connections = 0
	h.mu.Unlock()
	for _, sub := range subs {
		sub.close()
	}
}

// ConnectionCount 返回当前在线连接数。
func (h *Hub) ConnectionCount() int {
	if h == nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connections
}

// DroppedCount 返回因慢连接队列已满而丢弃的通知数量。
func (h *Hub) DroppedCount() uint64 {
	if h == nil {
		return 0
	}
	return h.dropped.Load()
}

// Metrics 返回当前连接、丢弃和连接拒绝计数的结构化快照。
func (h *Hub) Metrics() HubMetrics {
	if h == nil {
		return HubMetrics{}
	}
	h.mu.RLock()
	metrics := HubMetrics{ActiveConnections: h.connections, Closed: h.closed}
	h.mu.RUnlock()
	metrics.DroppedMessages = h.dropped.Load()
	metrics.RejectedConnections = h.rejected.Load()
	return metrics
}

func mergeNotifications(a, b Notification) Notification {
	if a.Module == "" {
		return b
	}
	if a.OccurredAt.Before(b.OccurredAt) {
		a.OccurredAt = b.OccurredAt
	}
	a.Count += b.Count
	if a.Count <= 0 {
		a.Count = 1
	}
	a.Items = mergeItems(a.Items, b.Items, &a.Truncated)
	if b.Truncated {
		a.Truncated = true
	}
	a.Refresh.InvalidateAll = a.Refresh.InvalidateAll || b.Refresh.InvalidateAll
	if a.Refresh.Module == "" {
		a.Refresh = b.Refresh
	} else if a.Refresh.Module != b.Refresh.Module || a.Refresh.EntityType != b.Refresh.EntityType || a.Refresh.EntityID != b.Refresh.EntityID {
		a.Refresh.EntityType = ""
		a.Refresh.EntityID = 0
		a.Refresh.InvalidateAll = true
	}
	if a.Action.Type != b.Action.Type || a.Action.Module != b.Action.Module || a.Action.EntityType != b.Action.EntityType || a.Action.EntityID != b.Action.EntityID || a.Count != 1 {
		a.Action = NotificationAction{Type: ActionOpenModule, Module: a.Module}
	}
	a.Title = mergedTitle(a.Module, a.Count)
	a.Summary = summaryFor(a)
	return a
}

func mergeItems(a, b []NotificationItem, truncated *bool) []NotificationItem {
	result := make([]NotificationItem, 0, minInt(3, len(a)+len(b)))
	seen := make(map[string]struct{}, len(a)+len(b))
	appendItem := func(item NotificationItem) {
		if item.EntityID == 0 {
			return
		}
		key := item.EntityType + ":" + uintString(item.EntityID)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		if len(result) < 3 {
			result = append(result, item)
			return
		}
		*truncated = true
	}
	for _, item := range a {
		appendItem(item)
	}
	for _, item := range b {
		appendItem(item)
	}
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
