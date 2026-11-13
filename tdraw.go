package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/encoding"

	"github.com/mattn/go-runewidth"
)

const (
	MODE_BOX   = "BOX"
	MODE_LINE  = "LINE"
	MODE_TEXT  = "TEXT"
	MODE_ERASE = "ERASE"

	maxInitialInputBytes = 1 << 20
	maxHistoryEntries    = 100
	canvasTopRow         = 3
	tabWidth             = 8
	verticalCellWeight   = 3
)

type lineArrowMode uint8

const (
	lineArrowheadsOn lineArrowMode = iota
	lineArrowheadsOff
)

type screenPoint struct {
	x, y int
}

type headerState struct {
	pointer    screenPoint
	mode       string
	text       textEntry
	lineArrows lineArrowMode
}

func (mode lineArrowMode) toggled() lineArrowMode {
	if mode == lineArrowheadsOn {
		return lineArrowheadsOff
	}
	return lineArrowheadsOn
}

const helpText = `tdraw - draw text-based diagrams in your terminal

Usage:
  tdraw [--help]
  tdraw > draw.txt
  some-command | tdraw > draw.txt

Piped text is placed at the top of the canvas. When you finish, the drawing is
written to stdout; redirect stdout to save it.

Mouse:
  Left-drag       Draw a box, line, or erase an area in box/line/erase mode
  Right-click     Erase the character at the pointer outside text mode
  Text mode       Left-click the canvas to choose where text starts
  Wheel           Scroll the canvas
  Shift+wheel     Scroll horizontally

Shortcuts:
  b               Switch to box mode from line or erase mode
  Esc             Finish text entry
  t               Enter text mode; left-click to choose the insertion point
  l               Enter line mode
  a               Toggle arrowheads in line mode
  e               Enter erase mode
  i               Enter text mode from line or erase mode
  u / r           Undo / redo (outside text mode)
  Enter           Start a new line in text mode
  Backspace       Delete a character or join with the previous line
  Ctrl+C / Ctrl+D Save the drawing to stdout and exit
`

var defStyle tcell.Style

func printHelpIfRequested(args []string, output io.Writer) bool {
	for _, arg := range args {
		if arg == "--help" {
			fmt.Fprint(output, helpText)
			return true
		}
	}
	return false
}

func emitStr(s tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, c := range str {
		var comb []rune
		w := runewidth.RuneWidth(c)
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}
		s.SetContent(x, y, c, comb, style)
		x += w
	}
}

func drawHeader(s tcell.Screen, state headerState) {
	width, _ := s.Size()
	for row := 0; row < canvasTopRow; row++ {
		for col := 0; col < width; col++ {
			s.SetContent(col, row, ' ', nil, defStyle)
		}
	}

	emitStr(s, 1, 0, defStyle, fmt.Sprintf("[%3d,%3d] %s | %s | %s", state.pointer.x, state.pointer.y, state.mode, modeSelectionHint(state.mode), globalHint(state.mode)))
	emitStr(s, 1, 1, defStyle, contextualHint(state.mode, state.text, state.lineArrows))
	for col := 0; col < width; col++ {
		s.SetContent(col, canvasTopRow-1, tcell.RuneHLine, nil, defStyle)
	}
}

func modeSelectionHint(mode string) string {
	switch mode {
	case MODE_BOX:
		return "l:line / t:text / e:erase"
	case MODE_LINE:
		return "b:box / t:text / e:erase"
	case MODE_ERASE:
		return "b:box / l:line / t:text"
	case MODE_TEXT:
		return "Esc:box"
	}
	return ""
}

func globalHint(mode string) string {
	if mode == MODE_TEXT {
		return "Ctrl+C/D:save"
	}
	return "u:undo / r:redo / Ctrl+C/D:save"
}

func contextualHint(mode string, text textEntry, lineArrows lineArrowMode) string {
	switch mode {
	case MODE_BOX:
		return "Left-drag: draw box / Right-click: erase character"
	case MODE_LINE:
		arrowState := "off"
		if lineArrows == lineArrowheadsOn {
			arrowState = "on"
		}
		return fmt.Sprintf("Left-drag: draw line / a: arrows %s / Right-click: erase character", arrowState)
	case MODE_ERASE:
		return "Left-drag: erase area / Right-click: erase character"
	case MODE_TEXT:
		if text.cursorSet {
			return "Esc: exit text mode / Enter: new line / Backspace: delete"
		}
		return "Left-click to select text position"
	}
	return ""
}

func readInitialInput(input io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(input, maxInitialInputBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxInitialInputBytes {
		return "", fmt.Errorf("initial input exceeds %d bytes", maxInitialInputBytes)
	}
	return string(data), nil
}

type textEntry struct {
	cursorSet    bool
	cursorX      int
	cursorY      int
	startX       int
	startY       int
	lineEnds     []int
	previewX     int
	previewY     int
	preview      canvasCell
	previewStyle tcell.Style
}

func (e *textEntry) beginSelection(canvas *canvasBuffer, x, y int, style tcell.Style) {
	e.restorePreview(canvas)
	e.cursorSet = false
	e.previewX, e.previewY = -1, -1
	e.movePreview(canvas, x, y, style)
}

func (e *textEntry) movePreview(canvas *canvasBuffer, x, y int, style tcell.Style) {
	e.restorePreview(canvas)
	width, height := canvas.Size()
	if x < 0 || x >= width || y < canvasTopRow || y >= height {
		return
	}

	main, comb, currentStyle, _ := canvas.GetContent(x, y)
	e.preview = canvasCell{main: main, comb: append([]rune(nil), comb...)}
	e.previewStyle = currentStyle
	e.previewX, e.previewY = x, y
	canvas.SetContent(x, y, '+', nil, style)
}

func (e *textEntry) selectPosition(canvas *canvasBuffer, x, y int, cursorStyle tcell.Style) bool {
	e.restorePreview(canvas)
	width, height := canvas.Size()
	if x < 0 || x >= width || y < canvasTopRow || y >= height {
		return false
	}

	e.cursorSet = true
	e.cursorX, e.cursorY = x, y
	e.startX = x
	e.startY = y
	e.lineEnds = []int{x}
	canvas.SetContent(e.cursorX, e.cursorY, '_', nil, cursorStyle)
	return true
}

func (e *textEntry) insertRune(canvas *canvasBuffer, r rune, style tcell.Style) {
	if !e.cursorSet {
		return
	}
	canvas.SetContent(e.cursorX, e.cursorY, r, nil, style)
	e.cursorX += runewidth.RuneWidth(r)
	e.lineEnds[e.currentLine()] = e.cursorX
	canvas.SetContent(e.cursorX, e.cursorY, '_', nil, style.Blink(true))
}

func (e *textEntry) startNewLine(canvas *canvasBuffer, style tcell.Style) {
	if !e.cursorSet {
		return
	}
	canvas.SetContent(e.cursorX, e.cursorY, ' ', nil, style)
	e.lineEnds = append(e.lineEnds, e.startX)
	e.cursorX = e.startX
	e.cursorY++
}

func (e *textEntry) backspace(canvas *canvasBuffer, style tcell.Style) {
	if !e.cursorSet {
		return
	}
	if e.cursorX > e.startX {
		canvas.SetContent(e.cursorX, e.cursorY, ' ', nil, style)
		e.cursorX--
		e.lineEnds[e.currentLine()] = e.cursorX
		canvas.SetContent(e.cursorX, e.cursorY, '_', nil, style.Blink(true))
		return
	}

	line := e.currentLine()
	if line == 0 {
		canvas.SetContent(e.cursorX, e.cursorY, '_', nil, style)
		return
	}

	canvas.SetContent(e.cursorX, e.cursorY, ' ', nil, style)
	e.cursorY--
	e.cursorX = e.lineEnds[line-1]
	e.lineEnds = e.lineEnds[:line]
	canvas.SetContent(e.cursorX, e.cursorY, '_', nil, style.Blink(true))
}

func (e *textEntry) currentLine() int {
	return e.cursorY - e.startY
}

func (e *textEntry) restorePreview(canvas *canvasBuffer) {
	if e.previewX < 0 || e.previewY < 0 {
		return
	}
	canvas.SetContent(e.previewX, e.previewY, e.preview.main, e.preview.comb, e.previewStyle)
	e.previewX, e.previewY = -1, -1
}

func main() {
	if printHelpIfRequested(os.Args[1:], os.Stdout) {
		return
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetOutput(os.Stdout)

	stdinInfo, e := os.Stdin.Stat()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	initialInput := ""
	if stdinInfo.Mode()&os.ModeCharDevice == 0 {
		initialInput, e = readInitialInput(os.Stdin)
		if e != nil {
			fmt.Fprintf(os.Stderr, "%v\n", e)
			os.Exit(1)
		}
	}

	encoding.Register()

	s, e := tcell.NewScreen()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	defer s.Fini()
	defStyle = tcell.StyleDefault
	s.SetStyle(defStyle)

	s.EnableMouse()
	s.Clear()
	screenWidth, screenHeight := s.Size()
	canvas := newCanvasBuffer(screenWidth, screenHeight)
	drawInput(canvas, initialInput)
	viewport := newCanvasViewport(canvas, screenWidth, screenHeight)
	history := newCanvasHistory(canvas)

	// mouse
	mx, my := -1, -1
	ox, oy := -1, -1
	bx, by := -1, -1
	// w, h := s.Size()
	mode := MODE_BOX
	lineArrows := lineArrowheadsOn
	text := textEntry{previewX: -1, previewY: -1}
	// imodeLastC := ' '

loop:
	for {
		// r, _, _, _ := s.GetContent(mx, my)
		drawHeader(s, headerState{
			pointer:    screenPoint{x: mx, y: my},
			mode:       mode,
			text:       text,
			lineArrows: lineArrows,
		})
		viewport.render(s)
		if ox >= 0 && oy >= 0 && bx >= 0 && by >= 0 {
			viewport.drawSelection(s, screenPoint{x: ox, y: oy}, screenPoint{x: bx, y: by})
		}

		s.Show()
		ev := s.PollEvent()
		st := tcell.StyleDefault
		// w, h = s.Size()

		switch ev := ev.(type) {
		case *tcell.EventResize:
			s.Sync()
			screenWidth, screenHeight := s.Size()
			viewport.resize(screenWidth, screenHeight)
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Key() == tcell.KeyCtrlD {
				s.Sync()
				_, canvasHeight := canvas.Size()
				arr := make([]string, canvasHeight-canvasTopRow)

				for y := canvasTopRow; y < canvasHeight; y++ {
					var line strings.Builder
					for x := 0; x < canvas.width; x++ {
						c, _, _, _ := canvas.GetContent(x, y)
						line.WriteRune(c)
					}
					arr[y-canvasTopRow] = line.String()
				}
				{
					n := 0
					for y := 0; y < len(arr); y++ {
						if strings.TrimSpace(arr[y]) == "" {
							n++
						} else {
							break
						}
					}
					arr = arr[n:len(arr)]
				}

				{
					n := 0
					for y := len(arr) - 1; y >= 0; y-- {
						if strings.TrimSpace(arr[y]) == "" {
							n++
						} else {
							break
						}
					}
					arr = arr[0 : len(arr)-n]
				}

				s.Clear()
				for y := 0; y < len(arr); y++ {
					fmt.Println(arr[y])
				}
				break loop
			}

			if mode != MODE_TEXT && ev.Key() == tcell.KeyRune {
				switch ev.Rune() {
				case 'u':
					history.undo(canvas)
					ox, oy, bx, by = -1, -1, -1, -1
					continue
				case 'r':
					history.redo(canvas)
					ox, oy, bx, by = -1, -1, -1, -1
					continue
				}
			}

			switch mode {
			case MODE_BOX:
				switch ev.Key() {
				default:
					switch ev.Rune() {
					case 't':
						mode = MODE_TEXT
						text.beginSelection(canvas, mx, my, st)
					case 'e':
						mode = MODE_ERASE
					case 'l':
						mode = MODE_LINE
					}
				}

			case MODE_ERASE:
				switch ev.Key() {
				default:
					switch ev.Rune() {
					case 'b':
						mode = MODE_BOX
					case 'e':
						mode = MODE_ERASE
					case 'l':
						mode = MODE_LINE
					case 't', 'i':
						mode = MODE_TEXT
						text.beginSelection(canvas, mx, my, st)
					}
				}

			case MODE_LINE:
				switch ev.Key() {
				default:
					switch ev.Rune() {
					case 'b':
						mode = MODE_BOX
					case 'e':
						mode = MODE_ERASE
					case 't', 'i':
						mode = MODE_TEXT
						text.beginSelection(canvas, mx, my, st)
					case 'a':
						lineArrows = lineArrows.toggled()
					}
				}

			case MODE_TEXT:
				switch ev.Key() {
				case tcell.KeyEscape:
					mode = MODE_BOX
					text.restorePreview(canvas)
					if text.cursorSet {
						canvas.SetContent(text.cursorX, text.cursorY, ' ', nil, st)
					}
					history.record(canvas)
				case tcell.KeyEnter:
					if !text.cursorSet {
						continue
					}
					text.startNewLine(canvas, st)
				case tcell.KeyDEL:
					if !text.cursorSet {
						continue
					}
					text.backspace(canvas, st)
				default:
					if !text.cursorSet {
						continue
					}
					text.insertRune(canvas, ev.Rune(), st)
				}
			}
		case *tcell.EventMouse:
			screenX, screenY := ev.Position()
			button := ev.Buttons()
			if button == tcell.WheelUp || button == tcell.WheelDown {
				if screenY >= canvasTopRow {
					viewport.scroll(button, ev.Modifiers())
				}
				continue
			}
			point, insideCanvas := viewport.toCanvas(screenX, screenY)
			if !insideCanvas {
				continue
			}
			x, y := point.x, point.y
			mx, my = x, y

			if mode != MODE_TEXT && button != tcell.ButtonNone && ox < 0 {
				ox, oy = x, y
			}

			switch mode {

			case MODE_ERASE:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						drawErase(canvas, ox, oy, x, y)
						history.record(canvas)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					bx, by = x, y
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					canvas.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(canvas)
				}

			case MODE_BOX:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						drawBox(canvas, ox, oy, x, y)
						history.record(canvas)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					bx, by = x, y
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					canvas.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(canvas)
				}
			case MODE_LINE:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						drawLine(canvas, screenPoint{x: ox, y: oy}, screenPoint{x: x, y: y}, lineArrows)
						history.record(canvas)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					bx, by = x, y
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					canvas.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(canvas)
				}
			case MODE_TEXT:
				if !text.cursorSet {
					if ev.Buttons() == tcell.Button1 {
						text.selectPosition(canvas, x, y, st.Blink(true))
					} else {
						text.movePreview(canvas, x, y, st)
					}
				}
			}

		}
	}
}
