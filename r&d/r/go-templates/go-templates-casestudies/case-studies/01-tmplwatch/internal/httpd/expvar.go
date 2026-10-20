package httpd

import "sync/atomic"

// current holds the State shown under expvar "tmplwatch".
var current atomicState

type atomicState struct{ v atomic.Value }

type stateBox struct{ s State }

func (a *atomicState) Store(s State) { a.v.Store(stateBox{s}) }

func (a *atomicState) Load() any {
	b, ok := a.v.Load().(stateBox)
	if !ok || b.s == nil {
		return nil
	}
	return b.s.Status()
}
