package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jroimartin/gocui"
)

type output struct {
	c         chan event
	contents  bytes.Buffer
	written   int
	lastFetch int
	lastRun   int
}

func (self *output) deliver(e event) {
	self.c <- e
}

func (self *output) makechan() {
	self.c = make(chan event)
}

func (self *output) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = 0
	}

	if self.written == self.contents.Len() {
		return
	}

	buf := self.contents.Bytes()
	v.Write(buf[self.written:])

	self.written = self.contents.Len()
}

func (self *output) init() {
	go func() {
		time.Sleep(2 * time.Second)
		for {
			time.Sleep(1 * time.Second)
			self.deliver(event{kind: FETCH})
		}
	}()
}

//{
//  "ok": true,
//  "runcount": 1,
//  "iov": {
//    "offset": 41,
//    "base64bytes": "IE9OTFkgVU5BVVRIT1JJWkVEIFVTRSBQUk9ISUJJVEVECihjKSAxOTkzLTIwMTUgMzU9RyBUZWNobm9sb2dpZXMgLyBBbGwgUmlnaHRzIFJlc2VydmVkCi0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLQoKW3J1bm5pbmddCgo="
//  }
//}

type Iov struct {
	Offset   int    `json:"offset"`
	B64bytes string `json:"base64bytes"`
}

type OutMsg struct {
	Ok  bool `json:"ok"`
	Rc  int  `json:"runcount"`
	Iov Iov  `json:"iov"`
}

func (self *output) fetch() {
	r, err := Session.get(fmt.Sprintf("/device/stdout/apu/%d", self.lastFetch))
	if !r.HTTPOK(err) {
		return
	}

	msg := &OutMsg{}
	if err := json.Unmarshal(r.body, msg); err != nil {
		return
	}

	if !msg.Ok {
		return
	}

	raw, _ := base64.StdEncoding.DecodeString(msg.Iov.B64bytes)

	self.lastFetch = msg.Iov.Offset + len(raw)

	if self.lastRun != msg.Rc {
		logf("emulator has restarted")
		self.lastRun = msg.Rc
		self.lastFetch = 0
		return
	}

	self.contents.Write(raw)
	v, _ := g.View("tabview")
	v.Autoscroll = true
}

func (self *output) loop() {
	self.init()

	for {
		e := <-self.c
		switch e.kind {
		case FETCH:
			self.fetch()
		}
	}
}
