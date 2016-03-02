package main

import "github.com/jroimartin/gocui"

type binding struct {
	v string
	k interface{}
	m gocui.Modifier
	h gocui.KeybindingHandler
}

// The list of all events recognized by the system

const (
	FLUSH = iota
	HIST_UP
	HIST_DOWN
	LINE
	LIST_UP
	LIST_DOWN
	LIST_CENTER
	LIST_ADDR
	LIST_ADDR_LIVE
	STAT_UPDATE
	FETCH
	FETCH_LIVE
	MODE_IN
	MODE_OUT
	COMMAND
	REFRESH_BPS
	LOAD
	COMPILE
	UP
	DOWN
	STACK_BUMP
	CLEAR
	SAVE
)

var modal = 0

// switchMode is called on CTR-X, and determines whether the command line
// is editable, or whether we can instead recognize unmodified individual
// hotkeys
func switchMode(v *gocui.View) {
	modal = (modal + 1) % 2
	if modal == 0 {
		CommandLine.deliver(event{kind: MODE_OUT})
	} else {
		CommandLine.deliver(event{kind: MODE_IN})
	}
}

// genericKey defines most of our key bindings
func genericKey(v *gocui.View, kv interface{}) {
	cmd := func(str string) { CommandLine.deliver(event{kind: COMMAND, data: str}) }

	k, ok := kv.(gocui.Key)
	if !ok {
		if modal == 1 {
			switch kv.(rune) {
			case 'S':
				cmd("start")
			case 's':
				cmd("step")
			case 'c':
				cmd("continue")
			case 'R':
				cmd("restart")
			case 'u':
				cmd("update")
			}
		}
	}

	if k == gocui.KeyEsc || k == gocui.KeyCtrlX {
		switchMode(v)
		return
	}

	if modal == 1 {
		switch k {
		case gocui.KeyArrowDown:
			Tabbar.down()
		case gocui.KeyArrowUp:
			Tabbar.up()
		}
	} else {
		switch k {
		case gocui.KeyArrowDown:
			CommandLine.deliver(event{kind: HIST_DOWN})
		case gocui.KeyArrowUp:
			CommandLine.deliver(event{kind: HIST_UP})
		}
	}

	switch k {
	case gocui.KeyCtrlD:
		Listing.deliver(event{kind: LIST_DOWN})
	case gocui.KeyCtrlU:
		Listing.deliver(event{kind: LIST_UP})
	case gocui.KeyArrowLeft:
		Listing.deliver(event{kind: LIST_CENTER})
	case gocui.KeyArrowRight:
		Tabbar.bottom()
	case gocui.KeyPgup:
		Tabbar.up()
	case gocui.KeyPgdn:
		Tabbar.down()
	case gocui.KeyEnter:
		CommandLine.deliver(event{kind: FLUSH})
	case gocui.KeyCtrlB:
		cmd("bump")
	case gocui.KeyCtrlQ:
		Tabbar.switchTo("log")
	case gocui.KeyCtrlW:
		Tabbar.switchTo("output")
	case gocui.KeyCtrlE:
		Tabbar.switchTo("source")
	case gocui.KeyCtrlR:
		Tabbar.switchTo("vm")
	case gocui.KeyCtrlT:
		Tabbar.switchTo("dump")
	case gocui.KeyCtrlY:
		Tabbar.switchTo("stack")
	case gocui.KeyCtrlH:
		Tabbar.switchTo("help")
	}

}

func genGenericKey(k interface{}) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		genericKey(v, k)
		return nil
	}
}

var bindings = []binding{
	{"", gocui.KeyCtrlC, gocui.ModNone, quit},
	{"", gocui.KeyTab, gocui.ModNone, keyNextTab},
}

var genericKeys = []interface{}{
	gocui.KeyEsc,
	gocui.KeyEnter,
	gocui.KeyArrowUp,
	gocui.KeyArrowDown,
	gocui.KeyArrowLeft,
	gocui.KeyArrowRight,
	gocui.KeyPgup,
	gocui.KeyPgdn,
	gocui.KeyCtrlX,
	gocui.KeyCtrlD,
	gocui.KeyCtrlQ,
	gocui.KeyCtrlW,
	gocui.KeyCtrlE,
	gocui.KeyCtrlR,
	gocui.KeyCtrlT,
	gocui.KeyCtrlY,
	gocui.KeyCtrlU,
	gocui.KeyCtrlH,
	gocui.KeyCtrlB,
	'S', 's', 'R', 'c', 'u', 'h',
}

func setBindings() {
	for _, v := range bindings {
		if err := g.SetKeybinding(v.v, v.k, v.m, v.h); err != nil {
			return
		}
	}

	for _, v := range genericKeys {
		g.SetKeybinding("", v, gocui.ModNone, genGenericKey(v))
	}
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func keyNextTab(g *gocui.Gui, v *gocui.View) error {
	Tabbar.nextTab()
	redraw()
	return nil
}
