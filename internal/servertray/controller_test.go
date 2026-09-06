package servertray

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRuntime struct {
	info      ServiceInfo
	fail      chan error
	shutdowns atomic.Int32
}

func newFakeRuntime(port int) *fakeRuntime {
	return &fakeRuntime{info: ServiceInfo{Version: "test", Port: port, LogDir: "logs", ServiceDir: "."}, fail: make(chan error, 1)}
}

func (r *fakeRuntime) RunContext(ctx context.Context, ready chan<- struct{}) error {
	close(ready)
	select {
	case err := <-r.fail:
		return err
	case <-ctx.Done():
		return nil
	}
}

func (r *fakeRuntime) Shutdown(context.Context) error {
	r.shutdowns.Add(1)
	return nil
}

func (r *fakeRuntime) Info() ServiceInfo { return r.info }

func TestControllerStartRestartAndStop(t *testing.T) {
	var created atomic.Int32
	events := make(chan Event, 16)
	factory := func() (Runtime, error) {
		created.Add(1)
		return newFakeRuntime(18080), nil
	}
	controller := NewController(factory, func(context.Context, int) error { return nil }, ServiceInfo{Port: 18080}, func(event Event) {
		events <- event
	})
	if err := controller.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if event := waitForState(t, events, StateRunning); event.Notice != NoticeStarted {
		t.Fatalf("start notice = %q", event.Notice)
	}
	if err := controller.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if event := waitForState(t, events, StateRunning); event.Notice != NoticeRestarted {
		t.Fatalf("restart notice = %q", event.Notice)
	}
	if created.Load() != 2 {
		t.Fatalf("created runtimes = %d", created.Load())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	waitForState(t, events, StateStopped)
}

func TestControllerRejectsConcurrentOperation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	controller := NewController(func() (Runtime, error) {
		once.Do(func() { close(entered) })
		<-release
		return newFakeRuntime(18080), nil
	}, func(context.Context, int) error { return nil }, ServiceInfo{Port: 18080}, nil)
	startDone := make(chan error, 1)
	go func() { startDone <- controller.Start() }()
	<-entered
	if err := controller.Restart(); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent restart error = %v", err)
	}
	close(release)
	if err := <-startDone; err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestControllerReportsRuntimeFailure(t *testing.T) {
	runtime := newFakeRuntime(18080)
	events := make(chan Event, 8)
	controller := NewController(func() (Runtime, error) { return runtime, nil }, func(context.Context, int) error { return nil }, runtime.info, func(event Event) {
		events <- event
	})
	if err := controller.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForState(t, events, StateRunning)
	runtime.fail <- errors.New("serve failed")
	event := waitForState(t, events, StateFailed)
	if event.Err == nil || event.Notice != NoticeFailed {
		t.Fatalf("failure event = %+v", event)
	}
}

func TestControllerReportsFactoryFailure(t *testing.T) {
	events := make(chan Event, 4)
	controller := NewController(func() (Runtime, error) { return nil, errors.New("database unavailable") }, nil, ServiceInfo{Port: 8080}, func(event Event) {
		events <- event
	})
	if err := controller.Start(); err == nil {
		t.Fatal("expected start error")
	}
	event := waitForState(t, events, StateFailed)
	if event.Notice != NoticeFailed || event.Err == nil {
		t.Fatalf("failure event = %+v", event)
	}
}

func waitForState(t *testing.T, events <-chan Event, state State) Event {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.State == state {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for state %q", state)
		}
	}
}
