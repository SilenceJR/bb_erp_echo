package servertray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bb_erp_echo/internal/app"
	"bb_erp_echo/internal/config"
)

var (
	ErrBusy     = errors.New("server tray operation is already in progress")
	ErrStopping = errors.New("server tray is stopping")
)

type State string

const (
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateRestarting State = "restarting"
	StateFailed     State = "failed"
	StateStopping   State = "stopping"
	StateStopped    State = "stopped"
)

type Notice string

const (
	NoticeNone      Notice = ""
	NoticeStarted   Notice = "started"
	NoticeRestarted Notice = "restarted"
	NoticeFailed    Notice = "failed"
)

type ServiceInfo struct {
	Version    string
	Port       int
	LogDir     string
	ServiceDir string
}

type Event struct {
	State  State
	Info   ServiceInfo
	Notice Notice
	Err    error
}

type Runtime interface {
	RunContext(context.Context, chan<- struct{}) error
	Shutdown(context.Context) error
	Info() ServiceInfo
}

type Factory func() (Runtime, error)
type Probe func(context.Context, int) error

type Controller struct {
	factory         Factory
	probe           Probe
	onEvent         func(Event)
	startupTimeout  time.Duration
	shutdownTimeout time.Duration

	opMu    sync.Mutex
	mu      sync.Mutex
	busy    bool
	exiting bool
	current *runningService
	info    ServiceInfo
}

type runningService struct {
	runtime Runtime
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	err     error
}

func NewController(factory Factory, probe Probe, initial ServiceInfo, onEvent func(Event)) *Controller {
	if factory == nil {
		factory = NewERPRuntime
	}
	if probe == nil {
		probe = WaitReady
	}
	return &Controller{
		factory:         factory,
		probe:           probe,
		onEvent:         onEvent,
		startupTimeout:  30 * time.Second,
		shutdownTimeout: 12 * time.Second,
		info:            initial,
	}
}

func (c *Controller) Start() error {
	if !c.beginOperation() {
		return c.operationError()
	}
	defer c.endOperation()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.emit(Event{State: StateStarting, Info: c.snapshotInfo()})
	return c.startLocked(false)
}

func (c *Controller) Restart() error {
	if !c.beginOperation() {
		return c.operationError()
	}
	defer c.endOperation()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.emit(Event{State: StateRestarting, Info: c.snapshotInfo()})
	if err := c.stopCurrentLocked(c.shutdownTimeout); err != nil {
		return c.fail(err)
	}
	if c.isExiting() {
		return ErrStopping
	}
	return c.startLocked(true)
}

func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	c.exiting = true
	c.mu.Unlock()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.emit(Event{State: StateStopping, Info: c.snapshotInfo()})
	var err error
	if current := c.detachCurrent(); current != nil {
		current.cancel()
		select {
		case <-current.done:
			err = current.result()
		case <-ctx.Done():
			err = fmt.Errorf("stop ERP service: %w", ctx.Err())
		}
	}
	c.emit(Event{State: StateStopped, Info: c.snapshotInfo(), Err: err})
	return err
}

func (c *Controller) startLocked(restarted bool) error {
	runtime, err := c.factory()
	if err != nil {
		return c.fail(fmt.Errorf("initialize ERP service: %w", err))
	}
	info := runtime.Info()
	c.setInfo(info)
	if c.isExiting() {
		ctx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
		defer cancel()
		return errors.Join(ErrStopping, runtime.Shutdown(ctx))
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	running := &runningService{runtime: runtime, cancel: cancelRun, done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		err := runtime.RunContext(runCtx, ready)
		running.mu.Lock()
		running.err = err
		running.mu.Unlock()
		close(running.done)
	}()

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), c.startupTimeout)
	defer cancelStartup()
	select {
	case <-ready:
	case <-running.done:
		return c.fail(unexpectedRunError(running.result()))
	case <-startupCtx.Done():
		cancelRun()
		<-running.done
		return c.fail(fmt.Errorf("wait for ERP service startup: %w", startupCtx.Err()))
	}
	if err := c.probe(startupCtx, info.Port); err != nil {
		cancelRun()
		<-running.done
		return c.fail(fmt.Errorf("verify ERP readiness: %w", err))
	}

	c.mu.Lock()
	if c.exiting {
		c.mu.Unlock()
		cancelRun()
		<-running.done
		return ErrStopping
	}
	c.current = running
	c.mu.Unlock()
	notice := NoticeStarted
	if restarted {
		notice = NoticeRestarted
	}
	c.emit(Event{State: StateRunning, Info: info, Notice: notice})
	go c.monitor(running)
	return nil
}

func (c *Controller) stopCurrentLocked(timeout time.Duration) error {
	current := c.detachCurrent()
	if current == nil {
		return nil
	}
	current.cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-current.done:
		return current.result()
	case <-timer.C:
		return errors.New("ERP service shutdown timed out")
	}
}

func (c *Controller) monitor(running *runningService) {
	<-running.done
	c.mu.Lock()
	if c.current != running {
		c.mu.Unlock()
		return
	}
	c.current = nil
	exiting := c.exiting
	info := c.info
	c.mu.Unlock()
	if !exiting {
		c.emit(Event{State: StateFailed, Info: info, Notice: NoticeFailed, Err: unexpectedRunError(running.result())})
	}
}

func (c *Controller) fail(err error) error {
	c.emit(Event{State: StateFailed, Info: c.snapshotInfo(), Notice: NoticeFailed, Err: err})
	return err
}

func unexpectedRunError(err error) error {
	if err == nil {
		return errors.New("ERP service stopped unexpectedly")
	}
	return err
}

func (c *Controller) beginOperation() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy || c.exiting {
		return false
	}
	c.busy = true
	return true
}

func (c *Controller) endOperation() {
	c.mu.Lock()
	c.busy = false
	c.mu.Unlock()
}

func (c *Controller) operationError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exiting {
		return ErrStopping
	}
	return ErrBusy
}

func (c *Controller) isExiting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exiting
}

func (c *Controller) detachCurrent() *runningService {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.current
	c.current = nil
	return current
}

func (c *Controller) setInfo(info ServiceInfo) {
	c.mu.Lock()
	c.info = info
	c.mu.Unlock()
}

func (c *Controller) snapshotInfo() ServiceInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.info
}

func (c *Controller) emit(event Event) {
	if c.onEvent != nil {
		c.onEvent(event)
	}
}

func (r *runningService) result() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

type erpRuntime struct{ app *app.App }

func NewERPRuntime() (Runtime, error) {
	erp, err := app.New()
	if err != nil {
		return nil, err
	}
	return &erpRuntime{app: erp}, nil
}

func (r *erpRuntime) RunContext(ctx context.Context, ready chan<- struct{}) error {
	return r.app.RunContext(ctx, ready)
}

func (r *erpRuntime) Shutdown(ctx context.Context) error { return r.app.Shutdown(ctx) }

func (r *erpRuntime) Info() ServiceInfo {
	return ServiceInfo{
		Version:    r.app.Config.App.Version,
		Port:       r.app.Config.HTTP.Port,
		LogDir:     absolutePath(r.app.Config.Log.Dir),
		ServiceDir: executableDir(),
	}
}

func InitialInfo() ServiceInfo {
	info := ServiceInfo{Version: "dev", Port: 8080, LogDir: absolutePath("logs"), ServiceDir: executableDir()}
	cfg, err := config.Load()
	if err == nil {
		info.Version = cfg.App.Version
		info.Port = cfg.HTTP.Port
		info.LogDir = absolutePath(cfg.Log.Dir)
	}
	return info
}

func executableDir() string {
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(executable)
}

func absolutePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
