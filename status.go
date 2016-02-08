package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jroimartin/gocui"
)

var statuses = []string{
	"???",
	"off",
	"ON",
	">>FAULT<<",
	"<BREAK>",
}

type status struct {
	c    chan event
	stat StatMsg
}

func (self *status) init() {
	self.update()

	go func() {
		for {
			time.Sleep(2 * time.Second)
			self.c <- event{kind: STAT_UPDATE}
		}
	}()
}

func (self *status) update() {
        fail := false

	x, err := Session.get("/device/status")
	if err != nil { 
 		fail = true
	}	
	err = json.Unmarshal(x.body, &self.stat)
	if err != nil { 
 		fail = true
	}

	if fail { 
		withViewNamed("status", func(v *gocui.View) {
			v.Clear()
			fmt.Fprintf(v, "can't reach emulator")
		})
		return
	}	

	withViewNamed("status", func(v *gocui.View) {
		v.Clear()
		s := statuses[0]
		if self.stat.Status > 0 && self.stat.Status < len(statuses) {
			s = statuses[self.stat.Status]
		}
		fmt.Fprintf(v, "status: %s pc:%0.4x [%s] ", s, self.stat.Cpu.Pc, self.stat.Cpu.Sr)

		for i, reg := range self.stat.Cpu.Registers {
			switch i {
			case 8, 16, 24:
				fmt.Fprintf(v, " %.2d:", i)
			}
			fmt.Fprintf(v, "%s", reg)
		}
		fmt.Fprintf(v, "\n")
	})

	Listing.deliver(event{kind: LIST_ADDR_LIVE, addr: self.stat.Cpu.Pc})
	Dump.deliver(event{kind: FETCH_LIVE, addr: self.stat.Cpu.Pc})
	Stack.deliver(event{kind: FETCH_LIVE, addr: self.stat.Cpu.Pc})
}

func (self *status) loop() {
	self.init()

	for {
		event := <-self.c
		switch event.kind {
		case STAT_UPDATE:
			self.update()
		}
	}
}

func (self *status) deliver(e event) {
	self.c <- e
}

func (self *status) makechan() {
	self.c = make(chan event)
}

func updateStatus() {
	CurrentStatus.deliver(event{kind: STAT_UPDATE})
}

type ApuState struct {
	Pc        int      `json:"pc"`
	Sp        string   `json:"sp"`
	Sr        string   `json:"sr_string"`
	Cycles    int      `json:"cycles"`
	Registers []string `json:"registers"`
}

type StatMsg struct {
	Cpu    ApuState `json:"apu_state"`
	Status int      `json:"status"`
}

// {
//   "ok": true,
//   "address": "xN733aDULSONZTv6BazgA63eTwzyXdcDjTWj4t9f",
//   "runcount": 1,
//   "apu_state": {
//     "pc": 0,
//     "sp": "0000",
//     "sr": 63,
//     "sr_string": "ithsvnzc",
//     "pc_string": "0000",
//     "cycles": 0,
//     "current_insn": "",
//     "registers": [
//       "00",
//       "f0",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00",
//       "00"
//     ]
//   },
//   "status": 1,
//   "asnSleeping": [
//     false,
//     false,
//     false
//   ]
// }
