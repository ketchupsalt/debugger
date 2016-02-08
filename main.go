package main

// So this is a story all about how my patience for Javascript got turned flip-upside down
// and I'd like to take a minute, just sit right there, I'll tell you how I came to write
// a terminal debugger for the Starfighter emulator CTF.
//
// In West Philadelphia born and no I'm not really going to keep doing this.
//
// This interface is very janky. I wrote it in about a day. I've barely tested it. I'm
// providing it so you can get a sense of how you might write your own.
//
// Here are the API calls that power this thing:
//
// GET /device/status                (JSON of registers and status)
// GET /device/program/apu           (JSON dump of AVR instructions)
// GET /device/memory/:offset?size=  (JSON of base64'd DMEM)
// GET /device/stdout/apu/:offset    (JSON of device output)
// POST /device/start
// POST /device/step                 Also stops the device wherever it is
// POST /device/continue
// POST /device/runto/:offset        Like a single-use breakpoint
// POST /device/restart              Really: reset
// GET /device/breakpoints           (JSON of all set breakpoints)
// PUT /device/breakpoints/:offset   Break at offset
// DELETE /device/breakpoints/:offset
// POST /vm/compile                  In: C source, Out: JSON of VM
// POST /vm/write                    In: JSON of VM, writes to flash
// POST /vm/load                     Tell the device to load from flash
// POST /vm/exec                     Tell the device to execute what it loaded
//
// Most of this should be evident from the code. Device output is an endless
// stream backed by a ring buffer; each read gives you the offset of the read
// and the bytes from that point; to poll, next read, read from offset+len(bytes).

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jroimartin/gocui"
)

// event is a message sent between components of the system
type event struct {
	kind int
	data string
	addr int
}

// receiver is a component of the system that listens to messages; this
// interface was retconned from my janky original code
//
// Components each have their own run loop, and talk to each other via
// messages sent on channels. There are plenty of thread-unsafe direct
// reads between components, too, just to keep things zesty. None of the
// events defined in the system are "bidirectional"; you never get a
// response
type receiver interface {
	deliver(e event)
	makechan()
	loop()
}

// These are the components of the UI:
var (
	// CommandLine is the command line at the bottom of the screen
	CommandLine commandLine

	// Listing is the assembly listing on the left side of the screen
	Listing listing

	// CurrentStatus is the status bar at the top of the screen
	CurrentStatus status

	// Session makes requests to the server
	Session session

	// Log is the "log" tab in the main window
	Log logbox

	// Output is the "output" tab
	Output output

	// Source is the "source" tab
	Source source

	// VM is the "vm" tab
	VM vm

	// Dump is the "dump" tab
	Dump dump

	// Stack is the "stack" tab
	Stack stack

	// Help is the "help" tab
	Help help
)

// components lists all the components the system will initialize
var components = []receiver{
	&CurrentStatus,
	&Listing,
	&Log,
	&CommandLine,
	&Output,
	&Source,
	&VM,
	&Dump,
	&Stack,
	&Help,
}

// g is the global handle to the gocui.Gui object; g.Execute() is thread-safe
var g *gocui.Gui

// statusPrint prints a message in the status bar
func statusPrint(format string, args ...interface{}) {
	withViewNamed("status", func(v *gocui.View) {
		v.Clear()
		fmt.Fprintln(v, fmt.Sprintf(format, args...))
	})
}

// Tabbar defines all the components that appear in switchable tabs in the
// main view. See tabbar.go for the code that manages this stuff. I only barely
// understand what gocui is trying to do, so that code is especially janky.
var Tabbar = tabbar{

	// options lists components in the order they appear in the tabbar
	options: []string{
		"log",
		"output",
		"source",
		"vm",
		"dump",
		"stack",
		"help",
	},

	// handlers list callbacks for each of the tabs, which mostly just
	// decide whether to autoscroll or not
	handlers: map[string]tabfn{
		"log":    renderLog,
		"output": renderOutput,
		"source": renderSource,
		"vm":     renderVm,
		"dump":   renderDump,
		"stack":  renderStack,
		"help":   renderHelp,
	},

	// selected is our current tab
	selected: 0,
}

// redraw tells the gocui loop to redraw the interface instead of waiting for
// an event gocui recognizes
func redraw() {
	g.Execute(func(g *gocui.Gui) error { return nil })
}

// Layout is our gocui Layout function, which defines the top, left, right, and
// bottom frames
func Layout(g *gocui.Gui) error {
	mx, my := g.Size()

	if top, err := g.SetView("status", 0, 0, mx-1, 2); err != nil {
		fmt.Fprintf(top, "hello\n")
	}

	if _, err := g.SetView("listing", 0, 2, mx/3, my-2); err != nil {

	}

	g.SetView("tabbar", mx/3, 2, mx-1, 4)
	Tabbar.renderView(mx/3, 4, mx-1, my-2)

	if bottom, err := g.SetView("cmdline", 0, my-2, mx-1, my); err != nil {
		bottom.Editable = true
		if err := g.SetCurrentView("cmdline"); err != nil {
			return err
		}
	}

	return nil
}

// withViewNamed executes "f" with v set to the requested view, and
// sets view focus when it does it; i barely understand what CurrentView
// means in gocui but whatever.
func withViewNamed(view string, f func(v *gocui.View)) {
	g.Execute(func(g *gocui.Gui) error {
		defer g.SetCurrentView(g.CurrentView().Name())

		new, _ := g.View(view)
		g.SetCurrentView(new.Name())
		f(new)
		return nil
	})
}

// boot initializes all the components listed in "components"
func boot() {
	for _, r := range components {
		r.makechan()
	}

	g.SetLayout(Layout)

	for _, r := range components {
		go r.loop()
	}
}

func main() {
	var user, pass string

	flag.StringVar(&user, "u", "", "Username on stockfighter.io (or env SFJB_USER")
	flag.StringVar(&pass, "p", "", "Password on stockfighter.io (or env SFJB_PASS")
	flag.Parse()

	if user == "" {
		user = os.Getenv("SFJB_USER")
	}
	if pass == "" {
		pass = os.Getenv("SFJB_PASS")
	}

	if user == "" || pass == "" {
		fmt.Fprintf(os.Stderr, "provide -u user -p password\n")
		return
	}

	Session.URL = "https://www.stockfighter.io/trainer"
	if err := Session.login(user, pass); err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %s\n", err)
		return
	}

	g = gocui.NewGui()
	if err := g.Init(); err != nil {
		return
	}
	defer g.Close()

	g.SelBgColor = gocui.ColorGreen
	g.SelFgColor = gocui.ColorBlack
	g.Cursor = true

	boot()

	setBindings()

	// gocui takes over our keyboard, so listen for SIGHUP to panic
	// the process if it hangs

	c := make(chan os.Signal)

	go func() {
		_ = <-c
		panic("signal")
	}()

	signal.Notify(c, syscall.SIGHUP)

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		return
	}
}
