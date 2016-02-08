package main

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

type vm struct {
	c        chan event
	contents []byte
	written  bool
	opcodes  []Opcode
}

func (self *vm) deliver(e event) {
	self.c <- e
}

func (self *vm) makechan() {
	self.c = make(chan event)
}

func (self *vm) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = false
	}

	if self.written {
		return
	}

	for i, op := range self.opcodes {
		if op.Arg != nil {
			fmt.Fprintf(v, "%.4d: %s %d\n", i, op.Code, *op.Arg)
		} else {
			fmt.Fprintf(v, "%.4d: %s\n", i, op.Code)
		}
	}

	self.written = true
}

func (self *vm) init() {
}

func (self *vm) load() {
	self.opcodes = Source.compiled.Opcodes
	self.written = false
}

func (self *vm) loop() {
	self.init()

	for {
		e := <-self.c
		switch e.kind {
		case LOAD:
			self.load()
		}

		redraw()
	}
}
