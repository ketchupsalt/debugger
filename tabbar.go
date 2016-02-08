package main

import (
	"fmt"
	"strings"

	"github.com/jroimartin/gocui"
)

type tabfn func(v *gocui.View, refresh bool)

type tabbar struct {
	options        []string
	handlers       map[string]tabfn
	selected, last int
}

func (self *tabbar) renderBar(v *gocui.View) {
	v.Clear()

	for i, vs := range self.options {
		if i == self.selected {
			fmt.Fprintf(v, " [%s]", strings.ToUpper(vs))
		} else {
			fmt.Fprintf(v, " %s", vs)
		}
	}
}

func (self *tabbar) nextTab() {
	self.selected = (self.selected + 1) % len(self.options)
}

func (self *tabbar) switchTo(which string) {
	for i, v := range self.options {
		if v == which {
			self.selected = i
			redraw()
			return
		}
	}
}

func (self *tabbar) renderView(sx, sy, ex, ey int) {
	bar, _ := g.View("tabbar")
	self.renderBar(bar)

	v, err := g.SetView("tabview", sx, sy, ex, ey)
	if err != nil {
		v.Editable = true
	}

	refresh := false
	if self.selected != self.last {
		v.Clear()
		v.SetOrigin(0, 0)
		refresh = true
		self.last = self.selected
	}

	self.handlers[self.options[self.selected]](v, refresh)
}

// BUG(tqbf): obviously clean this up

func renderLog(v *gocui.View, refresh bool) {
	v.Wrap = true
	Log.draw(v, refresh)
}

func renderOutput(v *gocui.View, refresh bool) {
	v.Wrap = true
	Output.draw(v, refresh)
}

func renderSource(v *gocui.View, refresh bool) {
	v.Wrap = true
	v.Autoscroll = false
	Source.draw(v, refresh)
}

func renderVm(v *gocui.View, refresh bool) {
	v.Wrap = true
	v.Autoscroll = false
	VM.draw(v, refresh)
}

func renderDump(v *gocui.View, refresh bool) {
	v.Wrap = true
	v.Autoscroll = false
	Dump.draw(v, refresh)
}

func renderStack(v *gocui.View, refresh bool) {
	v.Wrap = true
	v.Autoscroll = false
	Stack.draw(v, refresh)
}

func renderHelp(v *gocui.View, refresh bool) {
	v.Wrap = true
	v.Autoscroll = false
	Help.draw(v, refresh)
}

func (self *tabbar) current() string {
	return self.options[self.selected]
}

func (self *tabbar) bottom() {
	g.Execute(func(g *gocui.Gui) error {
		v, _ := g.View("tabview")
		cx, _ := v.Cursor()
		_, sy := v.Size()
		lines := strings.Count(v.Buffer(), "\n")
		if lines > sy {
			if err := v.SetCursor(cx, lines-(sy-1)); err != nil {
				ox, _ := v.Origin()
				if err := v.SetOrigin(ox, lines-(sy-1)); err != nil {
					return err
				}
			}
		}
		redraw()
		return nil
	})
}

func (self *tabbar) down() {
	if self.current() == "dump" {
		Dump.deliver(event{kind: DOWN})
		return
	}

	g.Execute(func(g *gocui.Gui) error {
		v, _ := g.View("tabview")
		cx, cy := v.Cursor()
		_, sy := v.Size()
		v.Autoscroll = false
		if err := v.SetCursor(cx, cy+sy+10); err != nil {
			ox, oy := v.Origin()
			if err := v.SetOrigin(ox, oy+10); err != nil {
				return err
			}
		}
		redraw()
		return nil
	})
}

func (self *tabbar) up() {
	if self.current() == "dump" {
		Dump.deliver(event{kind: UP})
		return
	}

	g.Execute(func(*gocui.Gui) error {
		v, _ := g.View("tabview")
		ox, oy := v.Origin()
		cx, cy := v.Cursor()
		v.Autoscroll = false
		if err := v.SetCursor(cx, cy-10); err != nil && oy > 0 {
			if err := v.SetOrigin(ox, oy-10); err != nil {
				return err
			}
		}
		redraw()
		return nil
	})
}
