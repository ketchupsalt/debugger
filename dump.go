package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jroimartin/gocui"
)

type dump struct {
	c        chan event
	live     chan bool
	demand   chan bool
	contents []byte
	written  bool
	addr     uint16
	sx, sy   int
	lastpc   uint16
}

func (self *dump) deliver(e event) {
	self.c <- e
}

// peek returns either nil or a slice of bytes retrieved from
// the /device/memory API endpoint, logging errors to the console log
func peek(addr uint16, size int) (ret []byte) {
	if size > 2048 {
		size = 2048
	}

	res, err := Session.get(fmt.Sprintf("/device/memory/%d?size=%d", addr, size))
	if !res.OK(err) {
		return
	}

	type MemoryMsg struct {
		Offset  int    `json:"offset"`
		Bytes64 string `json:"bytes64"`
	}

	msg := &MemoryMsg{}

	if err := json.Unmarshal(res.body, msg); err != nil {
		logf("can't unmarshal: %s", err)
		return
	}

	buf, _ := base64.StdEncoding.DecodeString(msg.Bytes64)
	return buf
}

func (self *dump) makechan() {
	self.c = make(chan event)
}

// draw a simple hex dump, filling available space
func (self *dump) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = false
	}

	if self.written {
		return
	}

	v.Clear()

	self.sx, self.sy = v.Size()

	xpos := 0
	ypos := 0

	p, _ := fmt.Fprintf(v, "%0.4X:   ", self.addr)
	xpos += p
	bytesThisLine := 0

	for off, b := range self.contents {
		need := 3
		pad := " "
		if bytesThisLine != 0 && bytesThisLine%4 == 0 {
			need += 2
			pad = "   "
		}
		if xpos < (self.sx - need) {
			bytesThisLine++
			p, _ = fmt.Fprintf(v, "%.2x%s", b, pad)
			xpos += p
		} else {
			xpos = 0
			bytesThisLine = 1
			ypos += 1

			v.Write([]byte("\n"))

			if ypos == self.sy {
				break
			}

			p, _ = fmt.Fprintf(v, "%0.4X:   %0.2x ", self.addr+uint16(off), b)
			xpos += p
		}
	}

	v.Write([]byte("\n"))

	self.written = true
}

// update re-fetches memory when dump.addr changes, then asks to be redrawn
func (self *dump) update() {
	size := 16 * 8
	if self.sx != 0 {
		size = (self.sx * self.sy) / 2
	}
	self.contents = peek(self.addr, size)
	self.written = false
	redraw()
}

func (self *dump) init() {
	time.Sleep(2 * time.Second)

	v, _ := g.View("tabview")
	self.sx, self.sy = v.Size()

	// memory fetch requests can get expensive; don't issue them on every
	// keystroke or status update

	// rate limit live updates: re-fetch once every 5 seconds
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

func (self *dump) loop() {
	self.init()

	for {
		e := <-self.c
		switch e.kind {
		// set a new address to dump
		case FETCH:
			self.addr = uint16(e.addr)
			self.update()

			// set a new address from status update (rate limited)
		case FETCH_LIVE:
			if uint16(e.addr) != self.lastpc {
				self.lastpc = uint16(e.addr)
				self.live <- true
			}

			// page up
		case UP:
			if (self.addr - (16 * 8)) > 0 {
				self.addr -= (16 * 8)
				self.demand <- true
			}

			// page down
		case DOWN:
			if (self.addr + (16 * 8)) < 0xffff {
				self.addr += (16 * 8)
				self.demand <- true
			}
		}
	}
}
