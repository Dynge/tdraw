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
	canvasTopRow         = 2
	tabWidth             = 8
	verticalCellWeight   = 3
)

var defStyle tcell.Style

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

func drawHeader(s tcell.Screen, x, y int, mode string) {
	emitStr(s, 1, 0, defStyle, fmt.Sprintf("[%3d,%3d] %s | Esc:box / t:text / l:line / e:erase / u:undo / r:redo / Ctrl+C:quit/save", x, y, mode))
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

type inputCanvas struct {
	screen        tcell.Screen
	width, height int
	x, y          int
	lastX, lastY  int
}

type canvasCell struct {
	main rune
	comb []rune
}

type canvasSnapshot struct {
	width, height int
	cells         []canvasCell
}

type canvasHistory struct {
	snapshots []canvasSnapshot
	current   int
}

func newCanvasHistory(screen tcell.Screen) *canvasHistory {
	return &canvasHistory{
		snapshots: []canvasSnapshot{captureCanvas(screen)},
	}
}

func captureCanvas(screen tcell.Screen) canvasSnapshot {
	width, screenHeight := screen.Size()
	height := screenHeight - canvasTopRow
	if height < 0 {
		height = 0
	}
	snapshot := canvasSnapshot{
		width:  width,
		height: height,
		cells:  make([]canvasCell, width*height),
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			main, comb, _, _ := screen.GetContent(x, y+canvasTopRow)
			snapshot.cells[y*width+x] = canvasCell{
				main: main,
				comb: append([]rune(nil), comb...),
			}
		}
	}
	return snapshot
}

func (h *canvasHistory) record(screen tcell.Screen) {
	snapshot := captureCanvas(screen)
	if canvasSnapshotsEqual(h.snapshots[h.current], snapshot) {
		return
	}

	h.snapshots = h.snapshots[:h.current+1]
	h.snapshots = append(h.snapshots, snapshot)
	if len(h.snapshots) > maxHistoryEntries+1 {
		h.snapshots = h.snapshots[1:]
	}
	h.current = len(h.snapshots) - 1
}

func canvasSnapshotsEqual(left, right canvasSnapshot) bool {
	if left.width != right.width || left.height != right.height {
		return false
	}
	for i := range left.cells {
		if left.cells[i].main != right.cells[i].main || len(left.cells[i].comb) != len(right.cells[i].comb) {
			return false
		}
		for j := range left.cells[i].comb {
			if left.cells[i].comb[j] != right.cells[i].comb[j] {
				return false
			}
		}
	}
	return true
}

func (h *canvasHistory) undo(screen tcell.Screen) {
	if h.current == 0 {
		return
	}
	h.current--
	restoreCanvas(screen, h.snapshots[h.current])
}

func (h *canvasHistory) redo(screen tcell.Screen) {
	if h.current >= len(h.snapshots)-1 {
		return
	}
	h.current++
	restoreCanvas(screen, h.snapshots[h.current])
}

func restoreCanvas(screen tcell.Screen, snapshot canvasSnapshot) {
	width, screenHeight := screen.Size()
	height := screenHeight - canvasTopRow
	if height < 0 {
		height = 0
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvasCell{main: ' '}
			if x < snapshot.width && y < snapshot.height {
				cell = snapshot.cells[y*snapshot.width+x]
			}
			screen.SetContent(x, y+canvasTopRow, cell.main, cell.comb, tcell.StyleDefault)
		}
	}
}

func drawInput(s tcell.Screen, input string) {
	width, height := s.Size()
	canvas := inputCanvas{
		screen: s,
		width:  width,
		height: height,
		y:      canvasTopRow,
		lastX:  -1,
		lastY:  -1,
	}
	for _, r := range input {
		canvas.drawRune(r)
	}
}

func (c *inputCanvas) drawRune(r rune) {
	switch r {
	case '\n':
		c.x = 0
		c.y++
		c.lastX, c.lastY = -1, -1
		return
	case '\r':
		return
	case '\t':
		c.drawSpaces(tabWidth - c.x%tabWidth)
		return
	}

	runeWidth := runewidth.RuneWidth(r)
	if runeWidth < 0 {
		return
	}
	if runeWidth == 0 {
		c.drawCombiningRune(r)
		return
	}
	if c.x+runeWidth > c.width {
		c.x = c.width
		return
	}

	if c.y < c.height {
		c.screen.SetContent(c.x, c.y, r, nil, tcell.StyleDefault)
		c.lastX, c.lastY = c.x, c.y
	}
	c.x += runeWidth
}

func (c *inputCanvas) drawSpaces(count int) {
	for i := 0; i < count; i++ {
		if c.x < c.width && c.y < c.height {
			c.screen.SetContent(c.x, c.y, ' ', nil, tcell.StyleDefault)
			c.lastX, c.lastY = c.x, c.y
		}
		c.x++
	}
}

func (c *inputCanvas) drawCombiningRune(r rune) {
	if c.lastX < 0 || c.lastY < 0 {
		return
	}
	mainc, combc, style, _ := c.screen.GetContent(c.lastX, c.lastY)
	c.screen.SetContent(c.lastX, c.lastY, mainc, append(combc, r), style)
}

func drawLine(s tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, r rune) {
	// do not draw point
	if x1 == x2 && y1 == y2 {
		return
	}

	dx := x2 - x1
	if dx < 0 {
		dx = -dx
	}
	dy := y2 - y1
	if dy < 0 {
		dy = -dy
	}
	if dx >= verticalCellWeight*dy {
		y2 = y1
	} else {
		x2 = x1
	}

	cross := false
	if c, _, _, _ := s.GetContent(x1, y1); c != ' ' {
		cross = true
	}

	if x1 == x2 {
		if y1 < y2 {
			for row := y1; row <= y2; row++ {
				s.SetContent(x1, row, '|', nil, tcell.StyleDefault)
			}
			s.SetContent(x1, y2, 'v', nil, tcell.StyleDefault)

		} else {
			for row := y2; row <= y1; row++ {
				s.SetContent(x1, row, '|', nil, tcell.StyleDefault)
			}
			s.SetContent(x1, y2, '^', nil, tcell.StyleDefault)
		}
	} else if y1 == y2 {
		if x1 < x2 {
			for col := x1; col <= x2; col++ {
				s.SetContent(col, y1, '-', nil, tcell.StyleDefault)
			}
			s.SetContent(x2, y1, '>', nil, tcell.StyleDefault)
		} else {
			for col := x2; col <= x1; col++ {
				s.SetContent(col, y1, '-', nil, tcell.StyleDefault)
			}
			s.SetContent(x2, y1, '<', nil, tcell.StyleDefault)
		}
	}

	if cross {
		s.SetContent(x1, y1, '+', nil, tcell.StyleDefault)
	}

	return
}

func drawErase(s tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, r rune) {
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 < x1 {
		x1, x2 = x2, x1
	}

	for col := x1; col <= x2; col++ {
		for row := y1; row <= y2; row++ {
			s.SetContent(col, row, ' ', nil, tcell.StyleDefault)
		}
	}
}

func drawBox(s tcell.Screen, x1, y1, x2, y2 int, style tcell.Style, r rune) {
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 < x1 {
		x1, x2 = x2, x1
	}

	// do not draw point
	if x1 == x2 && y1 == y2 {
		return
	}

	for col := x1 + 1; col < x2; col++ {
		// RuneHLine    = '─'
		s.SetContent(col, y1, tcell.RuneHLine, nil, tcell.StyleDefault)
		s.SetContent(col, y2, tcell.RuneHLine, nil, tcell.StyleDefault)
	}
	for row := y1 + 1; row < y2; row++ {
		// RuneVLine    = '│'
		s.SetContent(x1, row, tcell.RuneVLine, nil, tcell.StyleDefault)
		s.SetContent(x2, row, tcell.RuneVLine, nil, tcell.StyleDefault)
	}

	if y1 == y2 {
		s.SetContent(x1, y1, tcell.RuneHLine, nil, tcell.StyleDefault)
		s.SetContent(x2, y2, tcell.RuneHLine, nil, tcell.StyleDefault)
		return
	}

	if x1 == x2 {
		s.SetContent(x1, y1, tcell.RuneVLine, nil, tcell.StyleDefault)
		s.SetContent(x2, y2, tcell.RuneVLine, nil, tcell.StyleDefault)
		return
	}

	if y1 != y2 && x1 != x2 {
		// Only add corners if we need to
		s.SetContent(x1, y1, tcell.RuneULCorner, nil, tcell.StyleDefault)
		s.SetContent(x2, y1, tcell.RuneURCorner, nil, tcell.StyleDefault)
		s.SetContent(x1, y2, tcell.RuneLLCorner, nil, tcell.StyleDefault)
		s.SetContent(x2, y2, tcell.RuneLRCorner, nil, tcell.StyleDefault)
	}
}

func drawSelect(s tcell.Screen, x1, y1, x2, y2 int, sel bool) {
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	for row := y1; row <= y2; row++ {
		for col := x1; col <= x2; col++ {
			mainc, combc, style, width := s.GetContent(col, row)
			if style == tcell.StyleDefault {
				style = defStyle
			}
			style = style.Reverse(sel)
			s.SetContent(col, row, mainc, combc, style)
			col += width - 1
		}
	}
}

func main() {
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
	drawInput(s, initialInput)
	history := newCanvasHistory(s)

	// mouse
	mx, my := -1, -1
	ox, oy := -1, -1
	bx, by := -1, -1
	// w, h := s.Size()
	lchar := '*'

	mode := MODE_BOX
	imodeCurX, imodeCurY := 0, 0
	imodeStartX, _ := 0, 0
	// imodeLastC := ' '

loop:
	for {
		// r, _, _, _ := s.GetContent(mx, my)
		drawHeader(s, mx, my, mode)

		s.Show()
		ev := s.PollEvent()
		st := tcell.StyleDefault
		up := tcell.StyleDefault.
			Background(tcell.ColorBlue).
			Foreground(tcell.ColorBlack)
		// w, h = s.Size()

		// always clear any old selection box
		if ox >= 0 && oy >= 0 && bx >= 0 {
			drawSelect(s, ox, oy, bx, by, false)
		}

		switch ev := ev.(type) {
		case *tcell.EventResize:
			s.Sync()
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Key() == tcell.KeyCtrlD {
				s.Sync()
				sizeX, sizeY := s.Size()
				arr := make([]string, sizeY)

				for y := 2; y < sizeY; y++ {
					for x := 0; x < sizeX; x++ {
						c, _, _, _ := s.GetContent(x, y)
						arr[y] = arr[y] + string(c)
					}
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
					history.undo(s)
					ox, oy, bx, by = -1, -1, -1, -1
					continue
				case 'r':
					history.redo(s)
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
						imodeCurX, imodeCurY = mx, my
						imodeStartX, _ = mx, my
						s.SetContent(imodeCurX, imodeCurY, '_', nil, st.Blink(true))
					case 'e':
						mode = MODE_ERASE
					case 'l':
						mode = MODE_LINE
					}
				}

			case MODE_ERASE:
				switch ev.Key() {
				case tcell.KeyEscape:
					mode = MODE_BOX
				default:
					switch ev.Rune() {
					case 'e':
						mode = MODE_ERASE
					case 'l':
						mode = MODE_LINE
					case 't', 'i':
						mode = MODE_TEXT
						imodeCurX, imodeCurY = mx, my
						imodeStartX, _ = mx, my
						s.SetContent(imodeCurX, imodeCurY, '_', nil, st.Blink(true))
					}
				}

			case MODE_LINE:
				switch ev.Key() {
				case tcell.KeyEscape:
					mode = MODE_BOX
				default:
					switch ev.Rune() {
					case 'e':
						mode = MODE_ERASE
					case 't', 'i':
						mode = MODE_TEXT
						imodeCurX, imodeCurY = mx, my
						imodeStartX, _ = mx, my
						s.SetContent(imodeCurX, imodeCurY, '_', nil, st.Blink(true))
					}
				}

			case MODE_TEXT:
				switch ev.Key() {
				case tcell.KeyEscape:
					mode = MODE_BOX
					s.SetContent(imodeCurX, imodeCurY, ' ', nil, st)
					history.record(s)
				case tcell.KeyEnter:
					s.SetContent(imodeCurX, imodeCurY, ' ', nil, st)
					imodeCurX = imodeStartX
					imodeCurY = imodeCurY + 1
				case tcell.KeyDEL:
					if imodeCurX > imodeStartX {
						s.SetContent(imodeCurX, imodeCurY, ' ', nil, st)
						s.SetContent(imodeCurX-1, imodeCurY, '_', nil, st.Blink(true))
						imodeCurX -= 1
					} else {
						s.SetContent(imodeCurX, imodeCurY, '_', nil, st)
					}
				default:
					s.SetContent(imodeCurX, imodeCurY, ev.Rune(), nil, st)
					imodeCurX += runewidth.RuneWidth(ev.Rune())
					s.SetContent(imodeCurX, imodeCurY, '_', nil, st.Blink(true))
				}
			}
		case *tcell.EventMouse:
			x, y := ev.Position()
			mx, my = x, y
			button := ev.Buttons()

			if button != tcell.ButtonNone && ox < 0 {
				ox, oy = x, y
			}

			switch mode {

			case MODE_ERASE:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						bg := tcell.Color((lchar - '0') * 2)
						drawErase(s, ox, oy, x, y, up.Background(bg), lchar)
						history.record(s)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					ch := ' '
					bx, by = x, y
					lchar = rune(ch)
					if ox >= 0 && bx >= 0 {
						drawSelect(s, ox, oy, bx, by, true)
					}
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					s.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(s)
				}

			case MODE_BOX:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						bg := tcell.Color((lchar - '0') * 2)
						drawBox(s, ox, oy, x, y, up.Background(bg), lchar)
						history.record(s)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					ch := ' '
					bx, by = x, y
					lchar = rune(ch)
					if ox >= 0 && bx >= 0 {
						drawSelect(s, ox, oy, bx, by, true)
					}
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					s.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(s)
				}
			case MODE_LINE:
				switch ev.Buttons() {
				case tcell.ButtonNone:
					if ox >= 0 {
						bg := tcell.Color((lchar - '0') * 2)
						drawLine(s, ox, oy, x, y, up.Background(bg), lchar)
						history.record(s)
						ox, oy = -1, -1
						bx, by = -1, -1
					}
				case tcell.Button1:
					ch := ' '
					bx, by = x, y
					lchar = rune(ch)
					if ox >= 0 && bx >= 0 {
						drawSelect(s, ox, oy, bx, by, true)
					}
				case tcell.Button3:
					ox, oy = -1, -1
					bx, by = -1, -1
					s.SetContent(x, y, ' ', nil, tcell.StyleDefault)
					history.record(s)
				}
			}

		}
	}
}
