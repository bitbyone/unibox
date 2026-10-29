package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/tobola/unibox/internal/unibox"
)

// taskSpinner says on the terminal what the manager does while a task is
// readied, which takes seconds of sbx's: a spinner and the step, cleared
// before the manager writes anything and whenever it hands the terminal
// to sbx or an agent.
type taskSpinner struct {
	mu    sync.Mutex
	out   io.Writer
	what  string
	drawn bool
	quit  chan struct{}
	done  chan struct{}
}

func (s *taskSpinner) clear() {
	if s.drawn {
		fmt.Fprint(s.out, "\r\x1b[K")
		s.drawn = false
	}
}

// set is the manager's Progress: what it does now, "" for nothing.
func (s *taskSpinner) set(what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.what = what
	if what == "" {
		s.clear()
	}
}

func (s *taskSpinner) spin() {
	defer close(s.done)
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		select {
		case <-s.quit:
			return
		case <-tick.C:
		}
		s.mu.Lock()
		if s.what != "" {
			fmt.Fprintf(s.out, "\r\x1b[K%c %s", frames[i%len(frames)], s.what)
			s.drawn = true
		}
		s.mu.Unlock()
	}
}

func (s *taskSpinner) stop() {
	close(s.quit)
	<-s.done
	s.mu.Lock()
	s.clear()
	s.mu.Unlock()
}

// spinnerWriter clears the spinner before what is written through it.
type spinnerWriter struct {
	s *taskSpinner
	w io.Writer
}

func (w spinnerWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	w.s.clear()
	return w.w.Write(p)
}

// runTask runs a task's session as the manager does, with a spinner saying
// what is under way until the session takes the terminal.
func (o *options) runTask(ctx context.Context, req unibox.Request, extra []string) error {
	f, ok := o.log.(*os.File)
	if !ok || o.json || !term.IsTerminal(int(f.Fd())) {
		return o.manager.Run(ctx, req, o.input, o.output, o.log, extra)
	}
	s := &taskSpinner{out: f, quit: make(chan struct{}), done: make(chan struct{})}
	log := o.manager.Log
	o.manager.Log, o.manager.Progress = spinnerWriter{s, log}, s.set
	go s.spin()
	defer func() {
		s.stop()
		o.manager.Log, o.manager.Progress = log, nil
	}()
	return o.manager.Run(ctx, req, o.input, o.output, o.log, extra)
}
