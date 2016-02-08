package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
)

type listing struct {
	c            chan event
	program      []Instruction
	curLine      int
	hiLine       int
	lindex       map[int]int
	symdex       map[string]int
	notFollowing bool
	lastPC       int
	bps          []uint16
}

//  {
//    "mnem": "jmp",
//    "code": 61,
//    "dest": 0,
//    "src": 0,
//    "k": 238,
//    "s": 0,
//    "b": 0,
//    "offset": 0,
//    "symbol": "0000\t\u003c__vectors\u003e:\n"
//  },

type Instruction struct {
	Opcode string `json:"mnem"`
	Src    int    `json:"src"`
	Dst    int    `json:"dst"`
	K      int    `json:"k"`
	B      int    `json:"b"`
	Q      int    `json:"q"`
	S      int    `json:"s"`
	Offset int    `json:"offset"`
	Symbol string `json:"symbol"`
}

func (self *listing) init() {
	self.fetch()

	go func() {
		for len(self.lindex) == 0 {
			time.Sleep(5000 * time.Millisecond)
			self.fetch()
		}
	}()
}

func (self *listing) fetch() {
	x, _ := Session.get("/device/program/apu")
	json.Unmarshal(x.body, &self.program)
	self.notFollowing = true
	self.lindex = map[int]int{}
	self.symdex = map[string]int{}

	for i, v := range self.program {
		if sym := v.Sym(); sym != "" {
			self.symdex[sym] = v.Offset
		}

		self.lindex[v.Offset] = i
	}

	withViewNamed("listing", func(v *gocui.View) {
		v.Clear()
		_, rows := v.Size()
		for i := 0; i < rows; i++ {
			if i < len(self.program) {
				fmt.Fprintf(v, "%s\n", self.program[i].String())
			}
		}
	})
}

func (self *listing) redraw() {
	hit := func(addr uint16, set []uint16) bool {
		for _, v := range set {
			if addr == v {
				return true
			}
		}
		return false
	}

	withViewNamed("listing", func(v *gocui.View) {
		_, rows := v.Size()
		v.Clear()
		for i := 0; i < rows && i <= len(self.program); i++ {
			insn := self.program[i+self.curLine]

			sym := insn.Sym()

			if i+self.curLine == self.hiLine {
				fmt.Fprintf(v, ">> %s %s\n", insn.String(), sym)
			} else if hit(uint16(insn.Offset), self.bps) {
				fmt.Fprintf(v, "!! %s %s\n", insn.String(), sym)
			} else {
				fmt.Fprintf(v, "   %s %s\n", insn.String(), sym)
			}
		}
	})
}

func (self *listing) lineAt(addr int) {
	for i := addr; i < addr+8; i++ {
		if line, ok := self.lindex[addr]; ok {
			self.hiLine = line
			if line < 8 {
				line = 8
			}
			self.curLine = line - 8
			self.redraw()
			return
		}
	}
}

type Breakpoints struct {
	Breakpoints []int `json:"breakpoints"`
}

func allBreakpoints() (ret []uint16) {
	res, err := Session.get("/device/breakpoints")
	if !res.HTTPOK(err) {
		return
	}

	bps := &Breakpoints{}

	if err := json.Unmarshal(res.body, &bps); err != nil {
		logf("breakpoints unmarshal: %s", err)
		return
	}

	for _, v := range bps.Breakpoints {
		ret = append(ret, uint16(v))
	}

	return
}

func (self *listing) refreshBps() {
	self.bps = allBreakpoints()
	redraw()
}

func (self *listing) loop() {
	self.init()

	for {
		event := <-self.c
		switch event.kind {
		case LIST_ADDR_LIVE:
			if !self.notFollowing {
				self.lastPC = event.addr
				self.lineAt(event.addr)
			}
		case LIST_ADDR:
			self.lineAt(event.addr)
		case LIST_CENTER:
			self.lineAt(self.lastPC)
		case LIST_UP:
			if (self.curLine - 10) > 0 {
				self.curLine -= 10
			} else {
				self.curLine = 0
			}
			self.redraw()
		case LIST_DOWN:
			if (self.curLine + 10) < len(self.program) {
				self.curLine += 10
				self.redraw()
			}
		case REFRESH_BPS:
			self.refreshBps()
		}
	}
}

func (self *listing) deliver(e event) {
	self.c <- e
}

func (self *listing) makechan() {
	self.c = make(chan event)
}

type AvrIns struct {
	M   string `json:"m"`
	C   string `json:"c"`
	Src bool   `json:"source"`
	Dst bool   `json:"dest"`
	B   bool   `json:"b"`
	Q   bool   `json:"q"`
	S   bool   `json:"s"`
	K   bool   `json:"k"`
}

var avrList = []AvrIns{
	AvrIns{
		M: "ADC", C: "add with carry", Src: true, Dst: true, B: false, Q: false, S: false, K: false},
	AvrIns{
		M: "ADD", C: "add w/o carry", Src: true, Dst: true, B: false, Q: false, S: false, K: false},
	AvrIns{
		M: "ADIW", C: "add immed to word", Src: false, Dst: true, B: false, Q: false, S: false, K: true},
	AvrIns{
		M: "AND", C: "and", Src: true, Dst: true, B: false, Q: false, S: false, K: false},
	AvrIns{
		M: "ANDI", C: "and with immediate", Src: false, Dst: true, B: false, Q: false, S: false, K: true},
	AvrIns{
		M: "ASR", C: "arith shift right", Src: false, Dst: true, B: false, Q: false, S: false, K: false},
	AvrIns{
		M: "BCLR", C: "bit clear in sreg", Src: false, Dst: false, B: false, Q: false, S: true, K: false}, AvrIns{

		M: "BLD", C: "bit load from tflag to bit in register wtf", Src: false, Dst: true, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "BRBC", C: "branch if sreg bit clear", Src: false, Dst: false, B: false, Q: false, S: true, K: true}, AvrIns{

		M: "BRBS", C: "branch if sreg bit set", Src: false, Dst: false, B: false, Q: false, S: true, K: true}, AvrIns{

		M: "BRCC", C: "branch if not carry", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRCS", C: "branch if carry", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BREAK", C: "breakpoint", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "BREQ", C: "branch if eq (if z set)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRGE", C: "branch if >= (if s not set)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRHC", C: "branch if half carry clear", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRHS", C: "branch if half carry", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRID", C: "branch if interrupts enabled", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRIE", C: "branch if interrupts disabled", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRLO", C: "branch if lower (if C set)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRLT", C: "branch if less than (if S unset)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRMI", C: "branch if negative (if N set)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRNE", C: "branch if not equal (z unset)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRPL", C: "branch if plus (if N unset)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRSH", C: "branch if same or higher (C clear)", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRTC", C: "branch if T flag clear", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRTS", C: "branch if T flag set", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRVC", C: "branch if V flag cear", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BRVS", C: "branch if V flag set", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "BSET", C: "set S in SREG", Src: false, Dst: false, B: false, Q: false, S: true, K: false}, AvrIns{

		M: "BST", C: "store from reg to T flag", Src: false, Dst: true, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "CALL", C: "call", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "CBI", C: "clear bit b in ioreg A", Src: false, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "CBR", C: "ANDI the complement of K to Rd", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "CLC", C: "clear carry", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLH", C: "clear half carry", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLI", C: "clear interrupts", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLN", C: "clear N flag", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLS", C: "clear S flag", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLT", C: "clear T flag", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLV", C: "clear V", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLZ", C: "clear Z", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "COM", C: "1s comp Rd", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CP", C: "compare", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CPC", C: "compare w/ carry (Rd - Rr - C)", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CPI", C: "compare with immed", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "CPSE", C: "compare skip if equal really yes really, skip next insn if eq", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "DEC", C: "Rd -= 1", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "EICALL", C: "call to EIND:ZREG", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "EIJMP", C: "jump to EIND:ZREG", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ELPM", C: "extended load program memory", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "CLR", C: "clear reg", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "EOR", C: "XOR, tho EOR describes my mood better at this point", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ICALL", C: "call to ZREG", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "IJMP", C: "jmp to ZREG", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "IN", C: "ioaddr A -> Rd", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "INC", C: "Rd += 1", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "JMP", C: "absolute jump", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "LAC", C: "load and clear", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LAS", C: "load and set", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LAT", C: "NO MORE STOP load and toggle Z = D XOR Z, D = Z", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDI", C: "load immediate", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "LDX", C: "straight indirect load", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDXP", C: "load and then increment X", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDXM", C: "decrement X, then load", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDY", C: "D <- Y", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDYP", C: "D <- Y++", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDYM", C: "D <- --Y", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDYQ", C: "D <- Y+q", Src: false, Dst: true, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "LDDY", C: "D <- Y+q", Src: false, Dst: true, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "LDZ", C: "D <- Z", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDZP", C: "D <- Z++", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDZM", C: "D <- --Z", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LDDZ", C: "D <- Z", Src: false, Dst: true, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "LDZQ", C: "D <- Z+q", Src: false, Dst: true, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "LDS", C: "D <- (k) abs", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "LDSX", C: "D <- (k) abs short", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "LPM", C: "r0 <- imem[Z] load program memory", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LPMZ", C: "D <- imem[Z]", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LPMZP", C: "D <- imem[Z++]", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LSL", C: "logical shift left 1 bit", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "LSR", C: "logical shift right one bit", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "MOV", C: "move", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "MOVW", C: "D+1:D <- R+1:R pair copy", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "MUL", C: "unsigned mult", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "MULS", C: "signed mult", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "MULSU", C: "multiply signed unsigned because why not", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "NEG", C: "2s comp", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "NOP", C: "no-op", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "OR", C: "logical or", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ORI", C: "OR with immed", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "OUT", C: "iomem[A] <- R", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "POP", C: "pop", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "PUSH", C: "push", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "RCALL", C: "relative call, K is signed", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "RET", C: "return", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "RETI", C: "return from interrupt", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "RJMP", C: "relative jmp", Src: false, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "ROL", C: "rotate left through carry bit", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ROR", C: "rotate right through carry", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SBC", C: "subtract with carry", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SBCI", C: "subtract immed with carry", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "SBI", C: "set bit b in ioreg", Src: false, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "SBIC", C: "skip next insn if io bit clear", Src: false, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "SBIS", C: "skip next if io bit set", Src: false, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "SBIW", C: "subtract immed from word", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "SBR", C: "set bits in reg", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "SBRC", C: "skip if bit in reg clear", Src: true, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "SBRS", C: "skip if bit in reg set", Src: true, Dst: false, B: true, Q: false, S: false, K: false}, AvrIns{

		M: "SEC", C: "set carry", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SEH", C: "set half carry", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SEI", C: "set interrupts", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SEN", C: "set N flag", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SER", C: "Rd <- 0xff", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SES", C: "set S flag", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SET", C: "set T flag is how fucked AVR is where SET mnem is set T", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SEV", C: "set V", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SEZ", C: "set Z", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SLEEP", C: "sleep", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SPM", C: "ultra complicated store prog mem", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "STX", C: "X <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ST X+", C: "X++ <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ST X-", C: "--X <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ST Y", C: "Y <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ST Y+", C: "Y++ <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "ST Y-", C: "--Y <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "STD Y+", C: "store Y", Src: true, Dst: false, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "STYQ", C: "Y+q <- Rr", Src: true, Dst: false, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "STZ", C: "Z <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "STZP", C: "Z++ <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "STZM", C: "--Z <- Rr", Src: true, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "STDZ", C: "store Z", Src: true, Dst: false, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "STZQ", C: "Z+q <- Rr", Src: true, Dst: false, B: false, Q: true, S: false, K: false}, AvrIns{

		M: "ST", C: "(X) <- source", Src: true, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "STS", C: "(k) <- source", Src: true, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "STSX", C: "(k) <- source", Src: true, Dst: false, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "SUB", C: "subtract without carry", Src: true, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "SUBI", C: "subtract immed", Src: false, Dst: true, B: false, Q: false, S: false, K: true}, AvrIns{

		M: "SWAP", C: "swap nybbles", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "TST", C: "test for zero or minus; REG & REG", Src: false, Dst: true, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "WDR", C: "watchdog reset", Src: false, Dst: false, B: false, Q: false, S: false, K: false}, AvrIns{

		M: "XCH", C: "Z and Rd switch", Src: false, Dst: true, B: false, Q: false, S: false, K: false},
}

var avrTable = map[string]AvrIns{}

func init() {
	for _, v := range avrList {
		avrTable[v.M] = v
	}
}

func (self *Instruction) Sym() string {
	if self.Symbol == "" {
		return ""
	}

	toks := strings.Split(self.Symbol, "<")
	if len(toks) < 2 {
		return ""
	}

	toks = strings.Split(toks[1], ">")

	return toks[0]
}

func (self *Instruction) String() string {
	r, ok := avrTable[strings.ToUpper(self.Opcode)]
	if !ok {
		return fmt.Sprintf("%0.4x: [%s]", self.Offset, strings.ToUpper(self.Opcode))
	}

	out := &bytes.Buffer{}
	fmt.Fprintf(out, "%0.4x: ", self.Offset)

	fmt.Fprintf(out, "%s ", r.M)
	if r.K {
		fmt.Fprintf(out, "%d ", self.K)
	}
	if r.Dst {
		fmt.Fprintf(out, "r%d ", self.Dst)
	}
	if r.Src {
		fmt.Fprintf(out, "r%d ", self.Src)
	}
	if r.S {
		fmt.Fprintf(out, "%d ", self.S)
	}
	if r.B {
		fmt.Fprintf(out, "%d ", self.B)
	}
	if r.Q {
		fmt.Fprintf(out, "%d ", self.Q)
	}
	return out.String()
}
