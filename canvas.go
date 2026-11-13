package main

import (
	"github.com/gdamore/tcell"
	"github.com/mattn/go-runewidth"
)

const (
	maxCanvasCells   = 1 << 20
	canvasScrollStep = 3
)

type canvasCell struct {
	main  rune
	comb  []rune
	style tcell.Style
}

type canvasSnapshot struct {
	cells map[screenPoint]canvasCell
}

type canvasBuffer struct {
	width, height int
	cells         map[screenPoint]canvasCell
}

func newCanvasBuffer(width, height int) *canvasBuffer {
	return &canvasBuffer{
		width:  width,
		height: height,
		cells:  make(map[screenPoint]canvasCell),
	}
}

func (c *canvasBuffer) Size() (int, int) {
	return c.width, c.height
}

func (c *canvasBuffer) GetContent(x, y int) (rune, []rune, tcell.Style, int) {
	cell, ok := c.cells[screenPoint{x: x, y: y}]
	if !ok {
		return ' ', nil, tcell.StyleDefault, 1
	}
	width := runewidth.RuneWidth(cell.main)
	if width < 1 {
		width = 1
	}
	return cell.main, cell.comb, cell.style, width
}

func (c *canvasBuffer) SetContent(x, y int, main rune, comb []rune, style tcell.Style) {
	if x < 0 || x >= c.width || y < canvasTopRow || y >= c.height {
		return
	}
	point := screenPoint{x: x, y: y}
	if main == ' ' && len(comb) == 0 {
		delete(c.cells, point)
		return
	}
	c.cells[point] = canvasCell{
		main:  main,
		comb:  append([]rune(nil), comb...),
		style: style,
	}
}

func (c *canvasBuffer) resize(width, height int) bool {
	if width < c.width {
		width = c.width
	}
	if height < c.height {
		height = c.height
	}
	if width <= 0 || height <= 0 || int64(width)*int64(height) > maxCanvasCells {
		return false
	}
	c.width, c.height = width, height
	return true
}

type canvasHistory struct {
	snapshots []canvasSnapshot
	current   int
}

func newCanvasHistory(canvas *canvasBuffer) *canvasHistory {
	return &canvasHistory{
		snapshots: []canvasSnapshot{captureCanvas(canvas)},
	}
}

func captureCanvas(canvas *canvasBuffer) canvasSnapshot {
	snapshot := canvasSnapshot{cells: make(map[screenPoint]canvasCell, len(canvas.cells))}
	for point, cell := range canvas.cells {
		snapshot.cells[point] = canvasCell{main: cell.main, comb: append([]rune(nil), cell.comb...)}
	}
	return snapshot
}

func (h *canvasHistory) record(canvas *canvasBuffer) {
	snapshot := captureCanvas(canvas)
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
	if len(left.cells) != len(right.cells) {
		return false
	}
	for point, leftCell := range left.cells {
		rightCell, ok := right.cells[point]
		if !ok || leftCell.main != rightCell.main || len(leftCell.comb) != len(rightCell.comb) {
			return false
		}
		for j := range leftCell.comb {
			if leftCell.comb[j] != rightCell.comb[j] {
				return false
			}
		}
	}
	return true
}

func (h *canvasHistory) undo(canvas *canvasBuffer) {
	if h.current == 0 {
		return
	}
	h.current--
	restoreCanvas(canvas, h.snapshots[h.current])
}

func (h *canvasHistory) redo(canvas *canvasBuffer) {
	if h.current >= len(h.snapshots)-1 {
		return
	}
	h.current++
	restoreCanvas(canvas, h.snapshots[h.current])
}

func restoreCanvas(canvas *canvasBuffer, snapshot canvasSnapshot) {
	canvas.cells = make(map[screenPoint]canvasCell, len(snapshot.cells))
	for point, cell := range snapshot.cells {
		canvas.cells[point] = canvasCell{
			main:  cell.main,
			comb:  append([]rune(nil), cell.comb...),
			style: tcell.StyleDefault,
		}
	}
}

type canvasViewport struct {
	canvas *canvasBuffer
	offset screenPoint
	width  int
	height int
}

func newCanvasViewport(canvas *canvasBuffer, width, height int) *canvasViewport {
	return &canvasViewport{canvas: canvas, width: width, height: height}
}

func (v *canvasViewport) resize(width, height int) {
	v.width, v.height = width, height
	if v.offset.x+width > v.canvas.width {
		v.offset.x = maxInt(0, v.canvas.width-width)
	}
	if v.offset.y+height > v.canvas.height {
		v.offset.y = maxInt(0, v.canvas.height-height)
	}
	v.ensureVisible()
}

func (v *canvasViewport) ensureVisible() bool {
	return v.canvas.resize(v.offset.x+v.width, v.offset.y+v.height)
}

func (v *canvasViewport) scroll(button tcell.ButtonMask, modifiers tcell.ModMask) bool {
	horizontal := modifiers&tcell.ModShift != 0
	delta := 0
	switch button {
	case tcell.WheelUp:
		delta = -canvasScrollStep
	case tcell.WheelDown:
		delta = canvasScrollStep
	default:
		return false
	}

	next := v.offset
	if horizontal {
		next.x += delta
		if next.x < 0 {
			next.x = 0
		}
	} else {
		next.y += delta
		if next.y < 0 {
			next.y = 0
		}
	}
	if next == v.offset {
		return false
	}

	previous := v.offset
	v.offset = next
	if !v.ensureVisible() {
		v.offset = previous
		return false
	}
	return true
}

func (v *canvasViewport) toCanvas(x, y int) (screenPoint, bool) {
	if x < 0 || x >= v.width || y < canvasTopRow || y >= v.height {
		return screenPoint{}, false
	}
	return screenPoint{x: x + v.offset.x, y: y + v.offset.y}, true
}

func (v *canvasViewport) drawSelection(screen tcell.Screen, start, end screenPoint) {
	width, height := screen.Size()
	x1, y1 := start.x-v.offset.x, start.y-v.offset.y
	x2, y2 := end.x-v.offset.x, end.y-v.offset.y
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if x2 < 0 || x1 >= width || y2 < canvasTopRow || y1 >= height {
		return
	}
	x1 = maxInt(0, x1)
	x2 = minInt(width-1, x2)
	y1 = maxInt(canvasTopRow, y1)
	y2 = minInt(height-1, y2)
	drawSelect(screen, x1, y1, x2, y2)
}

func (v *canvasViewport) render(screen tcell.Screen) {
	width, height := screen.Size()
	for y := canvasTopRow; y < height; y++ {
		for x := 0; x < width; x++ {
			main, comb, style, cellWidth := v.canvas.GetContent(x+v.offset.x, y+v.offset.y)
			screen.SetContent(x, y, main, comb, style)
			x += cellWidth - 1
		}
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
