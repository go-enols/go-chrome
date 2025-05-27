package chrome

import (
	"context"
	"sync"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-enols/go-log"
	"github.com/google/uuid"
)

type MonitNetWork struct {
	ctx              context.Context
	mu               sync.RWMutex
	Event            map[string]func(ev any)
	MonitNetworkChan chan any
}

func NewMonitNetWork(ctx context.Context) *MonitNetWork {
	client := &MonitNetWork{
		ctx:              ctx,
		Event:            make(map[string]func(ev any)),
		MonitNetworkChan: make(chan any, 1000),
	}

	chromedp.ListenTarget(ctx, client.listenTarget)
	go client.Loop()
	return client
}

func (m *MonitNetWork) listenTarget(ev any) {
	m.MonitNetworkChan <- ev
}

func (m *MonitNetWork) Loop() {
	for {
		select {
		case <-m.ctx.Done():
			log.Info("网络监听结束")
			return
		case ev := <-m.MonitNetworkChan:
			m.onEvent(ev)
		}
	}
}

// 添加一个监听事件
func (m *MonitNetWork) AddEvent(f func(ev any)) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Event == nil {
		m.Event = make(map[string]func(ev any))
	}
	key := uuid.New().String()
	m.Event[key] = f
	return key
}

// 移除一个监听事件
func (m *MonitNetWork) RemoveEvent(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Event, key)
}

func (m *MonitNetWork) onEvent(ev any) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, fn := range m.Event {
		go fn(ev)
	}
}

// 添加一个请求监听事件
func (m *MonitNetWork) AddRequestEvent(f func(ev *network.EventRequestWillBeSent)) string {
	return m.AddEvent(func(ev any) {
		if v, ok := ev.(*network.EventRequestWillBeSent); ok {
			f(v)
		}
	})
}

// 添加一个响应监听事件
func (m *MonitNetWork) AddResponseEvent(f func(ev *network.EventResponseReceived)) string {
	return m.AddEvent(func(ev any) {
		if v, ok := ev.(*network.EventResponseReceived); ok {
			f(v)
		}
	})
}

// 添加一个加载监听事件
func (m *MonitNetWork) AddLoadingEvent(f func(ev *network.EventLoadingFinished)) string {
	return m.AddEvent(func(ev any) {
		if v, ok := ev.(*network.EventLoadingFinished); ok {
			f(v)
		}
	})
}
