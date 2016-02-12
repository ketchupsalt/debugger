package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jroimartin/gocui"
)

// The command line parser. Most of what you're looking for is in the
// "parse" function, which is just a big switch statement. As you'll see,
// there's nothing complicated about how we respond to commands: do
// something, and then use "logf" to print output to the console.

type commandLine struct {
	c           chan event
	history     []string
	historyPos  int
	savedLine   string
	lastCommand string
}

func (self *commandLine) deliver(e event) {
	self.c <- e
}

func (self *commandLine) makechan() {
	self.c = make(chan event)
}

const (
	I8 = iota
	I16
	I32
	S
	R
)

var (
	rxr8  = regexp.MustCompile("(r|read|x)/(8|b|byte)")
	rxr16 = regexp.MustCompile("(r|read|x)/(16|w|word)")
	rxrs  = regexp.MustCompile("(r|read|x)/(s|str|string)")
	rxrm  = regexp.MustCompile("(r|read|x)/(m|mem|memory)")

	rxtwi = regexp.MustCompile("@(r|$)([0-9]+):r?([0-9]+)")
	rxtw  = regexp.MustCompile("(r|$)([0-9]+):r?([0-9]+)")
	rxtri = regexp.MustCompile("@(r|$)([0-9]+)")
	rxtr  = regexp.MustCompile("(r|$)([0-9]+)")
	rxta  = regexp.MustCompile("@([0-9a-fA-F]+)")

	rxrep = regexp.MustCompile("^([0-9]+)x\\s+")
)

func (self *commandLine) read(line string) {
	terms := strings.Split(line, " ")
	if len(terms) == 1 {
		logf("can't parse %s", line)
		return
	}

	kind := I8

	if m := rxr8.FindStringSubmatch(terms[0]); m != nil {
		kind = I8
	} else if m := rxr16.FindStringSubmatch(terms[0]); m != nil {
		kind = 16
	} else if m := rxrs.FindStringSubmatch(terms[0]); m != nil {
		kind = S
	} else if m := rxrm.FindStringSubmatch(terms[0]); m != nil {
		kind = R
	}

	wreg := -1
	reg := -1
	addr := uint64(0xffffff)
	indir := false

	if m := rxtwi.FindStringSubmatch(terms[1]); m != nil {
		indir = true
		wreg, _ = strconv.Atoi(m[2])
		if wreg%2 != 0 {
			wreg++
		}

		if wreg > 30 {
			logf("can't parse reg:reg %s", terms[1])
			return
		}
	} else if m := rxtw.FindStringSubmatch(terms[1]); m != nil {
		wreg, _ = strconv.Atoi(m[2])
		if wreg%2 != 0 {
			wreg++
		}

		if wreg > 31 {
			logf("can't parse reg:reg %s", terms[1])
			return
		}
	} else if m := rxtri.FindStringSubmatch(terms[1]); m != nil {
		indir = true
		wreg, _ = strconv.Atoi(m[2])
		if wreg%2 != 0 {
			wreg++
		}

		if wreg > 31 {
			logf("can't parse reg:reg %s", terms[1])
			return
		}
	} else if m := rxtr.FindStringSubmatch(terms[1]); m != nil {
		reg, _ = strconv.Atoi(m[2])
		if reg > 31 {
			logf("can't parse reg:reg %s", terms[1])
			return
		}
	} else if m := rxta.FindStringSubmatch(terms[1]); m != nil {
		addr, _ = strconv.ParseUint(m[1], 16, 16)
		if addr > 0xffff {
			logf("can't parse address: %s", terms[1])
		}
	}

	if reg != -1 {
		logf("r%d: %s", reg, CurrentStatus.stat.Cpu.Registers[reg])
		return
	}

	if wreg != -1 {
		val := CurrentStatus.stat.Cpu.Registers[wreg+1] + CurrentStatus.stat.Cpu.Registers[wreg]

		if !indir {
			logf("r%d:%d: %s", wreg, wreg+1, val)
			return
		}

		addr, _ = strconv.ParseUint(val, 16, 16)
	}

	if addr > 0xffff {
		logf("can't parse %s", terms[1])
		return
	}

	var blob []byte

	switch kind {
	case I8:
		if blob = peek(uint16(addr), 1); blob != nil {
			logf("value at %0.4x: %0.2x", addr, blob[0])
		} else {
			logf("can't read %0.4x", addr)
		}
	case I16:
		if blob = peek(uint16(addr), 2); blob != nil {
			logf("value at %0.4x: %0.2x%0.2x", addr, blob[0], blob[1])
		} else {
			logf("can't read %0.4x", addr)
		}
	case S:
		if blob = peek(uint16(addr), 64); blob != nil {
			off := bytes.Index(blob, []byte("\x00"))
			if off != -1 {
				blob = blob[0:off]
			}

			for i := 0; i < len(blob); i++ {
				if blob[i] < 32 || blob[i] > 126 {
					blob[i] = '?'
				}
			}

			logf("value at %0.4x: %s", addr, string(blob))
		} else {
			logf("can't read %0.4x", addr)
		}
	case R:
		if blob = peek(uint16(addr), 32); blob != nil {
			out := &bytes.Buffer{}
			for _, b := range blob {
				fmt.Fprintf(out, "%0.2x", b)
			}

			logf("value at %0.4x: %s", addr, out.String())
		} else {
			logf("can't read %0.4x", addr)
		}
	}
}

func (self *commandLine) parse(line string) {
	for _, term := range strings.Split(line, ";") {
		term = strings.Trim(term, " \t")
		repeat := 1
		if m := rxrep.FindStringSubmatch(term); m != nil {
			repeat, _ = strconv.Atoi(m[1])
			term = strings.Trim(term[len(m[1])+2:], " \t")
		}

		if !self.parseTerm(term) {
			logf("bad command '%s'", term)
			break
		} else if term != "" {
			self.lastCommand = term
		}

		for i := 0; i < repeat-1; i++ {
			self.parseTerm(term)
		}
	}
}

func (self *commandLine) parseTerm(line string) (success bool) {
	success = true

	if line == "" && self.lastCommand != "" {
		line = self.lastCommand
	}

	switch {
	case strings.HasPrefix(line, "read/"):
		fallthrough
	case strings.HasPrefix(line, "r/"):
		fallthrough
	case strings.HasPrefix(line, "x/"):
		self.read(line)
		redraw()
		return
	}

	toks := strings.Split(line, " ")
	switch toks[0] {
	case "clr", "cls":
		Log.deliver(event{kind: CLEAR})
	case "vmload":
		Session.post("/vm/load", "")
		logf("requesting code load")
	case "vmexec":
		Session.post("/vm/exec", "")
		logf("requesting code execution")
	case "flash":
		Source.flash()
	case "bump":
		Stack.deliver(event{kind: STACK_BUMP})
	case "dump":
		if len(toks) > 1 {
			addr, _ := strconv.ParseUint(toks[1], 16, 16)

			if toks[1] == "stack" || toks[1] == "sp" {
				addr, _ = strconv.ParseUint(CurrentStatus.stat.Cpu.Sp, 16, 16)
				if addr > (16 * 8) {
					addr -= (16 * 8)
				} else {
					addr = 0
				}
			}

			Dump.deliver(event{kind: FETCH, addr: int(addr)})
		}

	case "compile":
		Source.deliver(event{kind: COMPILE})
	case "load":
		if len(toks) > 1 {
			Source.deliver(event{kind: LOAD, data: toks[1]})
		}
	case "functions":
		if len(toks) > 1 {
			logf("All functions matching %s:", toks[1])
			logf("------------------------------------")

			for k, v := range Listing.symdex {
				if match, _ := regexp.MatchString(toks[1], k); match {
					logf("%s %0.4x", k, v)
				}
			}

			logf("")
		} else {
			logf("All functions:")
			logf("--------------")
			for k, v := range Listing.symdex {
				logf("%s %0.4x", k, v)
			}
		}

		logf("")
	case "follow":
		Listing.notFollowing = false
	case "nofollow":
		Listing.notFollowing = true
	case "breakpoints":
		bps := allBreakpoints()
		logf("All breakpoints:")
		logf("----------------")
		for i, addr := range bps {
			logf("%.3d.  %0.4x", i, addr)
		}
		logf("")

	case "uptime":
		res, err := Session.get("/uptime")
		if res.HTTPOK(err) {
			logf(string(res.body))
		}

	case "runto", "rt":
		if len(toks) > 1 {
			addr, _ := strconv.ParseUint(toks[1], 16, 16)
			res, err := Session.post(fmt.Sprintf("/device/runto/%d", addr), "")
			if res.HTTPOK(err) {
				logf("running to %0.4x", addr)
			}
		}
	case "break", "b":
		if len(toks) > 1 {
			addr, err := strconv.ParseUint(toks[1], 16, 16)
			if err != nil {
				addr = uint64(Listing.symdex[toks[1]])
				if addr == 0 {
					logf("no symbol matching %s", toks[1])
					return
				}
			}

			res, err := Session.put(fmt.Sprintf("/device/breakpoints/%d", addr), "")
			if res.HTTPOK(err) {
				logf("breakpoint added at %0.4x", addr)
				Listing.deliver(event{kind: REFRESH_BPS})
				updateStatus()
			}
		}
	case "clear":
		if len(toks) > 1 {
			addr, err := strconv.ParseUint(toks[1], 16, 16)
			if err != nil {
				addr = uint64(Listing.symdex[toks[1]])
				if addr == 0 {
					logf("no symbol matching %s", toks[1])
					return
				}
			}

			res, err := Session.del(fmt.Sprintf("/device/breakpoints/%d", addr))
			if res.HTTPOK(err) {
				logf("cleared all breakpoints at %0.4x", addr)
				Listing.deliver(event{kind: REFRESH_BPS})
				updateStatus()
			}
		}
	case "echo":
		if len(toks) > 1 {
			logf("%s", strings.Join(toks[1:], " "))
		}
		return
	case "restart":
		res, err := Session.post("/device/restart", "")
		if res.OK(err) {
			logf("restarting device")
			updateStatus()
		}
	case "continue", "cont", "c":
		res, err := Session.post("/device/continue", "")
		if res.OK(err) {
			updateStatus()
		}
	case "step", "s":
		res, err := Session.post("/device/step", "")
		if res.OK(err) {
			updateStatus()
		}
	case "start":
		Listing.notFollowing = false
		res, err := Session.post("/device/start", "")
		if res.OK(err) {
			logf("started device")
			updateStatus()
		}
	case "update":
		updateStatus()
	case "list", "l":
		if len(toks) > 1 {
			addr, err := strconv.ParseUint(toks[1], 16, 16)
			if err != nil {
				addr = uint64(Listing.symdex[toks[1]])
				if addr == 0 {
					logf("no symbol matching %s", toks[1])
					return
				}
			}
			Listing.deliver(event{kind: LIST_ADDR, addr: int(addr)})
		}
		return
	default:
		success = false
	}

	return
}

func (self *commandLine) flush() {
	withViewNamed("cmdline", func(v *gocui.View) {
		line := strings.Trim(v.Buffer(), " \t\n")
		self.history = append(self.history, line)
		self.historyPos = len(self.history) - 1
		v.Clear()
		self.parse(line)
	})
}

func (self *commandLine) fromHistory() {
	line := self.history[self.historyPos]

	//withViewNamed( "cmdline", func(v *gocui.View) {
	// 	v.Clear()
	//})
	withViewNamed("cmdline", func(v *gocui.View) {
		v.Clear()
		fmt.Fprintf(v, "%s", strings.Trim(line, " \t\n"))
		v.MoveCursor(-2, 0, false) // this is hack around some weirdness in Editable
	})
}

func (self *commandLine) loop() {
	for {
		event := <-self.c
		switch event.kind {
		case FLUSH:
			self.flush()
		case HIST_UP:
			if len(self.history) > 0 {
				self.historyPos -= 1
				if self.historyPos < 0 {
					self.historyPos = 0
				}
				self.fromHistory()
			}
		case HIST_DOWN:
			if len(self.history) > 0 {
				self.historyPos += 1
				if self.historyPos >= len(self.history) {
					self.historyPos = len(self.history) - 1
				}
				self.fromHistory()
			}
		case MODE_IN:
			v, _ := g.View("cmdline")
			self.savedLine = strings.Trim(v.Buffer(), " \t\n")
			v.Clear()
			v.Editable = false
			fmt.Fprintf(v, "[S]tart [s]tep [c]ontinue [R]estart [h]elp")
		case MODE_OUT:
			v, _ := g.View("cmdline")
			v.Clear()
			fmt.Fprintf(v, self.savedLine)
			v.Editable = true
		case COMMAND:
			self.parse(event.data)
		}

		redraw()
	}
}
