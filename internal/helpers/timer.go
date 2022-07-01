package helpers

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/internal/logger"
)

type Timer struct {
	data  []timerData
	mutex sync.Mutex
}

type timerData struct {
	time  time.Time
	name  string
	isEnd bool
}

func (t *Timer) Begin(name string) {
	if t != nil {
		t.data = append(t.data, timerData{
			name: name,
			time: time.Now(),
		})
	}
}

func (t *Timer) End(name string) {
	if t != nil {
		t.data = append(t.data, timerData{
			name:  name,
			time:  time.Now(),
			isEnd: true,
		})
	}
}

func (t *Timer) Fork() *Timer {
	if t != nil {
		return &Timer{}
	}
	return nil
}

func (t *Timer) Join(other *Timer) {
	if t != nil && other != nil {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		t.data = append(t.data, other.data...)
	}
}

func (t *Timer) Log(log logger.Log) {
	if t == nil {
		return
	}

	type pair struct {
		timerData
		index uint32
	}

	var notes []logger.MsgData
	var stack []pair
	indent := 0

	for _, item := range t.data {
		if !item.isEnd {
			top := pair{timerData: item, index: uint32(len(notes))}
			notes = append(notes, logger.MsgData{DisableMaximumWidth: true})
			stack = append(stack, top)
			indent++
		} else {
			indent--
			last := len(stack) - 1
			top := stack[last]
			stack = stack[:last]
			if item.name != top.name {
				panic("Internal error")
			}
			notes[top.index].Text = fmt.Sprintf("%s%s: %dms",
				strings.Repeat("  ", indent),
				top.name,
				item.time.Sub(top.time).Milliseconds())
		}
	}

	log.AddIDWithNotes(logger.MsgID_None, logger.Info, nil, logger.Range{},
		"Timing information (times may not nest hierarchically due to parallelism)", notes)
}
