package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/mattn/go-runewidth"
)

const maxPasteInputBytes = 1 << 20

func preparePastedInput(input string) (string, error) {
	if len(input) > maxPasteInputBytes {
		return "", fmt.Errorf("clipboard text exceeds %d bytes", maxPasteInputBytes)
	}

	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.Join(lines, "\n"), nil
}

func drawPastedInput(canvas *canvasBuffer, history *canvasHistory, input string) {
	if input == "" {
		return
	}
	drawInput(canvas, input)
	history.record(canvas)
}

type inputCanvas struct {
	canvas        *canvasBuffer
	width, height int
	x, y          int
	lastX, lastY  int
}

func drawInput(canvas *canvasBuffer, input string) {
	width, height := canvas.Size()
	inputCanvas := inputCanvas{
		canvas: canvas,
		width:  width,
		height: height,
		y:      canvasTopRow,
		lastX:  -1,
		lastY:  -1,
	}
	for _, r := range input {
		inputCanvas.drawRune(r)
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
		c.canvas.SetContent(c.x, c.y, r, nil, tcell.StyleDefault)
		c.lastX, c.lastY = c.x, c.y
	}
	c.x += runeWidth
}

func (c *inputCanvas) drawSpaces(count int) {
	for i := 0; i < count; i++ {
		if c.x < c.width && c.y < c.height {
			c.canvas.SetContent(c.x, c.y, ' ', nil, tcell.StyleDefault)
			c.lastX, c.lastY = c.x, c.y
		}
		c.x++
	}
}

func (c *inputCanvas) drawCombiningRune(r rune) {
	if c.lastX < 0 || c.lastY < 0 {
		return
	}
	mainc, combc, style, _ := c.canvas.GetContent(c.lastX, c.lastY)
	c.canvas.SetContent(c.lastX, c.lastY, mainc, append(combc, r), style)
}

func drawLine(s *canvasBuffer, start, end screenPoint, arrowMode lineArrowMode) {
	// do not draw point
	if start == end {
		return
	}

	dx := end.x - start.x
	if dx < 0 {
		dx = -dx
	}
	dy := end.y - start.y
	if dy < 0 {
		dy = -dy
	}
	if dx >= verticalCellWeight*dy {
		end.y = start.y
	} else {
		end.x = start.x
	}

	cross := false
	if c, _, _, _ := s.GetContent(start.x, start.y); c != ' ' {
		cross = true
	}

	if start.x == end.x {
		drawVerticalLine(s, start, end, arrowMode)
	} else {
		drawHorizontalLine(s, start, end, arrowMode)
	}

	if cross {
		s.SetContent(start.x, start.y, '+', nil, tcell.StyleDefault)
	}
}

func drawVerticalLine(s *canvasBuffer, start, end screenPoint, arrowMode lineArrowMode) {
	lineEnd := '|'
	if arrowMode == lineArrowheadsOn {
		if start.y < end.y {
			lineEnd = 'v'
		} else {
			lineEnd = '^'
		}
	}

	firstRow, lastRow := start.y, end.y
	if lastRow < firstRow {
		firstRow, lastRow = lastRow, firstRow
	}
	for row := firstRow; row <= lastRow; row++ {
		s.SetContent(start.x, row, '|', nil, tcell.StyleDefault)
	}
	s.SetContent(end.x, end.y, lineEnd, nil, tcell.StyleDefault)
}

func drawHorizontalLine(s *canvasBuffer, start, end screenPoint, arrowMode lineArrowMode) {
	lineEnd := '-'
	if arrowMode == lineArrowheadsOn {
		if start.x < end.x {
			lineEnd = '>'
		} else {
			lineEnd = '<'
		}
	}

	firstColumn, lastColumn := start.x, end.x
	if lastColumn < firstColumn {
		firstColumn, lastColumn = lastColumn, firstColumn
	}
	for col := firstColumn; col <= lastColumn; col++ {
		s.SetContent(col, start.y, '-', nil, tcell.StyleDefault)
	}
	s.SetContent(end.x, end.y, lineEnd, nil, tcell.StyleDefault)
}

func drawErase(s *canvasBuffer, x1, y1, x2, y2 int) {
	x1, y1, x2, y2 = normalizeBounds(x1, y1, x2, y2)

	for col := x1; col <= x2; col++ {
		for row := y1; row <= y2; row++ {
			s.SetContent(col, row, ' ', nil, tcell.StyleDefault)
		}
	}
}

func drawBox(s *canvasBuffer, x1, y1, x2, y2 int) {
	x1, y1, x2, y2 = normalizeBounds(x1, y1, x2, y2)

	if x1 == x2 && y1 == y2 {
		return
	}
	if x1 == x2 {
		drawVerticalBorder(s, x1, y1, y2)
		return
	}
	if y1 == y2 {
		drawHorizontalBorder(s, y1, x1, x2)
		return
	}

	drawBoxSides(s, x1, y1, x2, y2)
	s.SetContent(x1, y1, tcell.RuneULCorner, nil, tcell.StyleDefault)
	s.SetContent(x2, y1, tcell.RuneURCorner, nil, tcell.StyleDefault)
	s.SetContent(x1, y2, tcell.RuneLLCorner, nil, tcell.StyleDefault)
	s.SetContent(x2, y2, tcell.RuneLRCorner, nil, tcell.StyleDefault)
}

func normalizeBounds(x1, y1, x2, y2 int) (int, int, int, int) {
	if y2 < y1 {
		y1, y2 = y2, y1
	}
	if x2 < x1 {
		x1, x2 = x2, x1
	}
	return x1, y1, x2, y2
}

func drawVerticalBorder(s *canvasBuffer, x, y1, y2 int) {
	for row := y1; row <= y2; row++ {
		s.SetContent(x, row, tcell.RuneVLine, nil, tcell.StyleDefault)
	}
}

func drawHorizontalBorder(s *canvasBuffer, y, x1, x2 int) {
	for col := x1; col <= x2; col++ {
		s.SetContent(col, y, tcell.RuneHLine, nil, tcell.StyleDefault)
	}
}

func drawBoxSides(s *canvasBuffer, x1, y1, x2, y2 int) {
	for col := x1 + 1; col < x2; col++ {
		s.SetContent(col, y1, tcell.RuneHLine, nil, tcell.StyleDefault)
		s.SetContent(col, y2, tcell.RuneHLine, nil, tcell.StyleDefault)
	}
	for row := y1 + 1; row < y2; row++ {
		s.SetContent(x1, row, tcell.RuneVLine, nil, tcell.StyleDefault)
		s.SetContent(x2, row, tcell.RuneVLine, nil, tcell.StyleDefault)
	}
}

func drawSelect(s tcell.Screen, x1, y1, x2, y2 int) {
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
			style = style.Reverse(true)
			s.SetContent(col, row, mainc, combc, style)
			col += width - 1
		}
	}
}
