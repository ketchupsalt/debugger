package main

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"

	"github.com/jroimartin/gocui"
)

type source struct {
	c        chan event
	contents []byte
	written  bool
	compiled CompileMsg
}

func (self *source) deliver(e event) {
	self.c <- e
}

func (self *source) makechan() {
	self.c = make(chan event)
}

func (self *source) draw(v *gocui.View, refresh bool) {
	if refresh {
		self.written = false
	}

	if self.written {
		return
	}

	v.Write(self.contents)

	self.written = true
}

func (self *source) init() {
}

type Opcode struct {
	Code string `json:"code"`
	Arg  *int   `json:"arg"`
}

type CompileMsg struct {
	Opcodes []Opcode `json:"opcodes"`
	Ep      int      `json:"ep"`
	Raw64   string   `json:"raw"`
	BSS64   string   `json:"bss"`
	raw     []byte
	bss     []byte
}

func (self *source) compile() {
	if self.contents == nil || len(self.contents) == 0 {
		logf("no source loaded")
	}

	res, err := Session.post("/vm/compile", string(self.contents))
	if !res.OK(err) {
		return
	}

	if err := json.Unmarshal(res.body, &self.compiled); err != nil {
		logf("can't unmarshal: %s", err)
	}

	self.compiled.raw, _ = base64.StdEncoding.DecodeString(self.compiled.Raw64)
	self.compiled.bss, _ = base64.StdEncoding.DecodeString(self.compiled.BSS64)

	logf("compiled to %d opcodes", len(self.compiled.Opcodes))

	VM.deliver(event{kind: LOAD})
}

func (self *source) load(path string) {
	var err error
	self.contents, err = ioutil.ReadFile(path)
	logf("loaded %d bytes of source", len(self.contents))
	self.written = false
	if err != nil {
		logf("can't load: %s", err)
	}
}

func (self *source) loop() {
	self.init()

	for {
		e := <-self.c
		switch e.kind {
		case LOAD:
			self.load(e.data)
		case COMPILE:
			self.compile()
		}

		redraw()
	}
}

func (self *source) flash() {
	type WriteMsg struct {
		Raw64 string `json:"raw"`
		Bss64 string `json:"bss"`
		Ep    int    `json:"ep"`
	}

	wm := &WriteMsg{}

	if self.compiled.raw == nil || self.compiled.bss == nil {
		logf("no compiled code to flash")
		return
	}

	wm.Raw64 = base64.StdEncoding.EncodeToString(self.compiled.raw)
	wm.Bss64 = base64.StdEncoding.EncodeToString(self.compiled.bss)
	wm.Ep = self.compiled.Ep

	buf, _ := json.Marshal(wm)

	res, err := Session.post("/vm/write", string(buf))
	if !res.OK(err) {
		return
	}

	logf("flashed device")
}
