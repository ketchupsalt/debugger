package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jroimartin/gocui"
)

type stack struct {
	c        chan event
	live     chan bool
	demand   chan bool
	contents []byte
	written  bool
	sx, sy   int
	lastsp   uint16
	lastaddr uint16
	lastpc   uint16
	bump     int
}

func (self *stack) deliver(e event) {
	self.c <- e
}

func (self *stack) makechan() {
	self.c = make(chan event)
}

func (self *stack) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = false
	}

	if self.written {
		return
	}

	v.Clear()

	self.sx, self.sy = v.Size()

	blob := self.contents[self.bump:]

	for off, b := range blob {
		cur := self.lastaddr + uint16(off) + uint16(self.bump)
		if off%2 == 0 {
			if cur == self.lastsp {
				fmt.Fprintf(v, "%0.4x:>>    ", cur)
			} else {
				fmt.Fprintf(v, "%0.4x:      ", cur)
			}

			fmt.Fprintf(v, "%0.2x", b)
		} else {
			fmt.Fprintf(v, "%0.2x\n", b)
		}
	}

	self.written = true
}

func (self *stack) update() {
	size := self.sy * 2

	a, _ := strconv.ParseUint(CurrentStatus.stat.Cpu.Sp, 16, 16)
	self.lastsp = uint16(a)
	if self.lastsp != 0 {
		if self.lastsp > uint16(size) {
			self.lastaddr = self.lastsp - (uint16(size) / 2)
			if self.lastpc%2 == 0 {
				self.lastaddr += 1
			}
		} else {
			self.lastaddr = 0
		}

		self.contents = peek(self.lastaddr, size)
		self.written = false
		redraw()
	}
}

func (self *stack) init() {
	time.Sleep(2 * time.Second)

	v, _ := g.View("tabview")
	self.sx, self.sy = v.Size()

	// rate limit live updates
	self.live = make(chan bool)
	go func() {
		cts := true
		for {
			select {
			case <-self.live:
				if cts {
					self.update()
					cts = false
				}
			case <-time.After(5 * time.Second):
				cts = true
			}
		}
	}()

	// debounce keystroke updates
	self.demand = make(chan bool)
	go func() {
		hit := false
		fire := false
		for {
			select {
			case hit = <-self.demand:
				fire = false
			case <-time.After(500 * time.Millisecond):
				if !fire {
					fire = true
				} else if hit && fire {
					self.update()
					hit = false
					fire = false
				}
			}
		}
	}()

	self.update()
}

func (self *stack) loop() {
	self.init()

	for {
		e := <-self.c
		switch e.kind {
		case FETCH_LIVE:
			if uint16(e.addr) != self.lastpc {
				self.lastpc = uint16(e.addr)
				self.live <- true
			}
		case STACK_BUMP:
			self.bump = (self.bump + 1) % 2
			self.written = false
			redraw()
		}
	}
}
