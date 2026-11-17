// Command cxplayground is the CX Playground app: a list of CX apps, each
// identified by the hash of its manifest, and a runner that gives the
// selected program a canvas, touch and key events and the soft keyboard.
package main

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	"github.com/0magnet/gio/app"
	"github.com/0magnet/gio/f32"
	"github.com/0magnet/gio/font"
	"github.com/0magnet/gio/font/gofont"
	"github.com/0magnet/gio/io/event"
	"github.com/0magnet/gio/io/key"
	"github.com/0magnet/gio/io/pointer"
	"github.com/0magnet/gio/layout"
	"github.com/0magnet/gio/op"
	"github.com/0magnet/gio/op/clip"
	"github.com/0magnet/gio/op/paint"
	"github.com/0magnet/gio/text"
	"github.com/0magnet/gio/unit"
	"github.com/0magnet/gio/widget"
	"github.com/0magnet/gio/widget/material"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// The app bundle is built by `cxmanifest bundle` from apps/.
//
//go:embed bundle
var bundle embed.FS

var (
	bg      = color.NRGBA{R: 0x12, G: 0x14, B: 0x1c, A: 0xff}
	panel   = color.NRGBA{R: 0x1d, G: 0x21, B: 0x2e, A: 0xff}
	fg      = color.NRGBA{R: 0xec, G: 0xee, B: 0xf4, A: 0xff}
	dim     = color.NRGBA{R: 0x9a, G: 0xa1, B: 0xb5, A: 0xff}
	accent  = color.NRGBA{R: 0x4c, G: 0xc2, B: 0xff, A: 0xff}
	hashCol = color.NRGBA{R: 0xe8, G: 0xb8, B: 0x4a, A: 0xff}
	mono    = font.Font{Typeface: "Go Mono"}
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("CX Playground"))
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type ui struct {
	w     *app.Window
	th    *material.Theme
	src   store.Source
	apps  []store.App
	errs  []error
	list  widget.List
	rows  []widget.Clickable
	back  widget.Clickable
	scrl  widget.List
	runID int

	// Set while a program is selected.
	cur      *store.App
	host     *host
	out      *console
	done     chan struct{}
	result   string
	started  time.Time
	canvas   int // event tag
	focused  bool
	edit     []rune // what the IME believes the text field holds
	leaveReq bool
}

func run(w *app.Window) error {
	sub, err := fs.Sub(bundle, "bundle")
	if err != nil {
		return err
	}
	u := &ui{w: w, src: store.FSSource{FS: sub}}
	u.th = material.NewTheme()
	u.th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	u.th.Palette.Fg, u.th.Palette.Bg, u.th.Palette.ContrastBg = fg, bg, accent
	u.list.Axis = layout.Vertical
	u.scrl.Axis = layout.Vertical
	u.apps, u.errs, err = store.List(u.src)
	if err != nil {
		return err
	}
	for _, a := range u.apps {
		log.Printf("cxplayground: app %s manifest verified: %s (%d files)", a.Hash, a.Manifest.Name, len(a.Manifest.Files))
	}
	for _, e := range u.errs {
		log.Printf("cxplayground: %v", e)
	}
	u.rows = make([]widget.Clickable, len(u.apps))

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.Fill(gtx.Ops, bg)
			if u.cur == nil {
				u.layoutList(gtx)
			} else {
				u.layoutRun(gtx)
			}
			e.Frame(gtx.Ops)
			if u.host != nil {
				u.host.frameShown()
			}
		}
	}
}

func (u *ui) label(gtx layout.Context, f font.Font, sp unit.Sp, c color.NRGBA, s string) layout.Dimensions {
	l := material.Label(u.th, sp, s)
	l.Font, l.Color = f, c
	return l.Layout(gtx)
}

func (u *ui) layoutList(gtx layout.Context) {
	for i := range u.rows {
		if u.rows[i].Clicked(gtx) {
			u.start(&u.apps[i])
		}
	}
	inset := layout.UniformInset(unit.Dp(16))
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, font.Font{Weight: font.Bold}, 28, fg, "CX Playground")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := fmt.Sprintf("%d apps, each named by the SHA-256 of its manifest, verified chunk by chunk", len(u.apps))
						if len(u.errs) > 0 {
							s += fmt.Sprintf(", %d failed verification", len(u.errs))
						}
						return u.label(gtx, font.Font{}, 14, dim, s)
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(u.th, &u.list).Layout(gtx, len(u.apps), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Left: 12, Right: 12, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.row(gtx, i)
				})
			})
		}),
	)
}

func (u *ui) row(gtx layout.Context, i int) layout.Dimensions {
	a := u.apps[i]
	return u.rows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		m := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			kind := a.Manifest.Kind
			if kind == "" {
				kind = "console"
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, font.Font{Weight: font.Bold}, 19, fg, a.Manifest.Name)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, font.Font{}, 14, dim, kind+" · "+a.Manifest.Description)
				}),
				layout.Rigid(layout.Spacer{Height: 4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.label(gtx, mono, 11, hashCol, string(a.Hash))
				}),
			)
		})
		call := m.Stop()
		rr := gtx.Dp(8)
		defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, rr).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, panel)
		call.Add(gtx.Ops)
		return dims
	})
}

// start loads, verifies and runs an app on its own goroutine.
func (u *ui) start(a *store.App) {
	u.cur, u.result, u.leaveReq, u.focused, u.edit = a, "", false, false, nil
	u.runID++
	u.host = newHost(u.w.Invalidate)
	u.out = &console{invalidate: u.w.Invalidate}
	u.done = make(chan struct{})
	u.started = time.Now()
	h, out, done := u.host, u.out, u.done
	go func() {
		defer close(done)
		defer u.w.Invalidate()
		files, err := store.Load(u.src, *a)
		if err != nil {
			fmt.Fprintf(out, "verification failed: %v\n", err)
			return
		}
		log.Printf("cxplayground: running %s (%s)", a.Manifest.Name, a.Hash)
		err = cxrun.Run(files, a.Manifest.Entry, h, out)
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		}
		log.Printf("cxplayground: %s finished: %v", a.Manifest.Name, err)
	}()
}

func (u *ui) finished() bool {
	select {
	case <-u.done:
		return true
	default:
		return false
	}
}

// leave asks the program to stop and returns to the list once it has.
func (u *ui) leave() {
	u.leaveReq = true
	u.host.closed.Store(true)
}

func (u *ui) layoutRun(gtx layout.Context) {
	if u.back.Clicked(gtx) {
		u.leave()
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameBack})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			u.leave()
		}
	}
	done := u.finished()
	if done && u.result == "" {
		u.result = fmt.Sprintf("finished in %d ms", time.Since(u.started).Milliseconds())
	}
	if done && u.leaveReq {
		gtx.Execute(key.SoftKeyboardCmd{Show: false})
		u.cur, u.host = nil, nil
		u.w.Invalidate()
		u.layoutList(gtx)
		return
	}
	cmds, drew := u.host.takeFrame()
	gfx := u.cur.Manifest.Kind == "gfx" && drew && !done

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.topBar(gtx, done)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if gfx || (u.cur.Manifest.Kind == "gfx" && !done) {
				return u.layoutCanvas(gtx, cmds)
			}
			return u.layoutConsole(gtx)
		}),
	)
}

func (u *ui) topBar(gtx layout.Context, done bool) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(u.th, &u.back, "‹ Apps")
				b.Background = panel
				return b.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: 12}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return u.label(gtx, font.Font{Weight: font.Bold}, 17, fg, u.cur.Manifest.Name)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := "running"
						if done {
							s = u.result
						} else if u.leaveReq {
							s = "stopping when the program returns"
						}
						return u.label(gtx, font.Font{}, 13, dim, s)
					}),
				)
			}),
		)
	})
}

func (u *ui) layoutConsole(gtx layout.Context) layout.Dimensions {
	lines := strings.Split(strings.TrimRight(u.out.String(), "\n"), "\n")
	return material.List(u.th, &u.scrl).Layout(gtx, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Left: 12, Right: 12, Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return u.label(gtx, mono, 14, fg, lines[i])
		})
	})
}

// layoutCanvas routes input to the program and draws its last frame.
func (u *ui) layoutCanvas(gtx layout.Context, cmds []cxrun.Cmd) layout.Dimensions {
	size := gtx.Constraints.Max
	u.host.setSize(size)
	tag := &u.canvas
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)

	// The canvas holds key focus so hardware keys and IME text reach the
	// program whether or not it opened the soft keyboard.
	if !u.focused {
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
	if show, ok := u.host.takeKeyboard(); ok {
		gtx.Execute(key.SoftKeyboardCmd{Show: show})
	}
	for {
		ev, ok := gtx.Event(
			pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Drag | pointer.Release},
			key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Name: key.NameDeleteBackward},
			key.Filter{Focus: tag, Name: key.NameReturn},
			key.Filter{Focus: tag, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			k := map[pointer.Kind]int{pointer.Press: cxrun.EvTouchDown, pointer.Drag: cxrun.EvTouchMove, pointer.Release: cxrun.EvTouchUp}[e.Kind]
			if k != 0 {
				u.host.push(cxrun.Event{Kind: k, X: e.Position.X, Y: e.Position.Y})
			}
		case key.EditEvent:
			u.applyEdit(gtx, tag, e.Range, e.Text)
		case key.Event:
			if e.State != key.Press {
				break
			}
			switch e.Name {
			case key.NameDeleteBackward:
				n := len(u.edit)
				u.applyEdit(gtx, tag, key.Range{Start: max(n-1, 0), End: n}, "")
				if n == 0 {
					u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyBackspace})
				}
			case key.NameReturn, key.NameEnter:
				u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyEnter})
			}
		case key.FocusEvent:
			u.focused = e.Focus
		}
	}
	draw(gtx, u.th.Shaper, cmds)
	return layout.Dimensions{Size: size}
}

func rgba(c cxrun.Cmd) color.NRGBA {
	b := func(f float32) uint8 {
		return uint8(min(max(f, 0), 1)*255 + 0.5)
	}
	return color.NRGBA{R: b(c.R), G: b(c.G), B: b(c.B), A: b(c.A)}
}

// draw renders a display list with Gio's GPU renderer.
func draw(gtx layout.Context, shaper *text.Shaper, cmds []cxrun.Cmd) {
	for _, c := range cmds {
		col := rgba(c)
		p := c.P
		switch c.Kind {
		case cxrun.CmdClear:
			paint.Fill(gtx.Ops, col)
		case cxrun.CmdRect:
			r := image.Rect(int(p[0]), int(p[1]), int(p[0]+p[2]), int(p[1]+p[3]))
			paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
		case cxrun.CmdCircle:
			r := image.Rect(int(p[0]-p[2]), int(p[1]-p[2]), int(p[0]+p[2]), int(p[1]+p[2]))
			paint.FillShape(gtx.Ops, col, clip.Ellipse(r).Op(gtx.Ops))
		case cxrun.CmdLine:
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(p[0], p[1]))
			path.LineTo(f32.Pt(p[2], p[3]))
			paint.FillShape(gtx.Ops, col, clip.Stroke{Path: path.End(), Width: max(c.Width, 1)}.Op())
		case cxrun.CmdTri:
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(p[0], p[1]))
			path.LineTo(f32.Pt(p[2], p[3]))
			path.LineTo(f32.Pt(p[4], p[5]))
			path.Close()
			paint.FillShape(gtx.Ops, col, clip.Outline{Path: path.End()}.Op())
		case cxrun.CmdText:
			t := op.Offset(image.Pt(int(p[0]), int(p[1]))).Push(gtx.Ops)
			m := op.Record(gtx.Ops)
			paint.ColorOp{Color: col}.Add(gtx.Ops)
			mat := m.Stop()
			g := gtx
			g.Constraints = layout.Constraints{Max: image.Pt(max(gtx.Constraints.Max.X-int(p[0]), 1), max(gtx.Constraints.Max.Y, 1))}
			sp := unit.Sp(c.Width / gtx.Metric.PxPerSp)
			widget.Label{}.Layout(g, shaper, font.Font{}, sp, c.Text, mat)
			t.Pop()
		}
	}
}

// applyEdit replaces r in the mirrored text field with s, as the IME asked,
// and sends the program the difference as backspaces and runes. IMEs
// rewrite their composing text, so appending EditEvent.Text drops input.
func (u *ui) applyEdit(gtx layout.Context, tag event.Tag, r key.Range, s string) {
	old := u.edit
	start, end := min(r.Start, r.End), max(r.Start, r.End)
	start, end = min(max(start, 0), len(old)), min(max(end, 0), len(old))
	next := append(append(append([]rune{}, old[:start]...), []rune(s)...), old[end:]...)
	p := 0
	for p < len(old) && p < len(next) && old[p] == next[p] {
		p++
	}
	for range len(old) - p {
		u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyBackspace})
	}
	for _, c := range next[p:] {
		code := int32(c)
		if c == '\n' {
			code = cxrun.KeyEnter
		}
		u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: code})
	}
	u.edit = next
	n := len(next)
	gtx.Execute(key.SnippetCmd{Tag: tag, Snippet: key.Snippet{Range: key.Range{End: n}, Text: string(next)}})
	gtx.Execute(key.SelectionCmd{Tag: tag, Range: key.Range{Start: n, End: n}, Caret: key.Caret{}})
}
