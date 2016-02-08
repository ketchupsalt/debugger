package main

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

type logbox struct {
	c       chan event
	log     []string
	written int
}

func (self *logbox) deliver(e event) {
	self.c <- e
}

func (self *logbox) makechan() {
	self.c = make(chan event, 20)
}

func (self *logbox) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = 0
	}

	for i := self.written; i < len(self.log); i++ {
		fmt.Fprintln(v, self.log[i])
	}

	self.written = len(self.log)
}

func (self *logbox) logLine(line string) {
	self.log = append(self.log, line)
	v, _ := g.View("tabview")
	v.Autoscroll = true
}

func (self *logbox) clear() {
	self.log = []string{}
	self.written = 0
	v, _ := g.View("tabview")
	v.Clear()
	redraw()
}

func (self *logbox) loop() {
	for {
		e := <-self.c
		switch e.kind {
		case CLEAR:
			self.clear()
		case LINE:
			self.logLine(e.data)
		}
	}
}

func logf(format string, args ...interface{}) {
	Log.deliver(event{kind: LINE, data: fmt.Sprintf(format, args...)})
}

func logError(source string, err error) {
	if err != nil {
		logf("%s error: %s", source, err)
	}
}
