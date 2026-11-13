package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gdamore/tcell"
)

const (
	testCanvasWidth   = 12
	testCanvasHeight  = 5
	testCanvasRow     = canvasTopRow
	testClipWidth     = 4
	testClipHeight    = 4
	testHeaderWidth   = 80
	testLastColumn    = testClipWidth - 1
	testNextCanvasRow = testCanvasRow + 1
)

func TestReadInitialInputFromPipe(t *testing.T) {
	input, err := readInitialInput(strings.NewReader("first line\nsecond line\n"))
	if err != nil {
		t.Fatalf("readInitialInput returned an error: %v", err)
	}
	if want := "first line\nsecond line\n"; input != want {
		t.Errorf("readInitialInput = %q, want %q", input, want)
	}
}

func TestReadInitialInputRejectsOversizedInput(t *testing.T) {
	input := bytes.NewReader(bytes.Repeat([]byte{'x'}, maxInitialInputBytes+1))
	_, err := readInitialInput(input)
	if err == nil {
		t.Fatal("readInitialInput accepted oversized input")
	}
}

func TestDrawInputPlacesTextOnCanvas(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init returned an error: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(testCanvasWidth, testCanvasHeight)

	drawInput(screen, "ab\tc\r\nxy")

	for _, cell := range []struct {
		x, y int
		want rune
	}{
		{x: 0, y: testCanvasRow, want: 'a'},
		{x: 1, y: testCanvasRow, want: 'b'},
		{x: tabWidth, y: testCanvasRow, want: 'c'},
		{x: 0, y: testNextCanvasRow, want: 'x'},
		{x: 1, y: testNextCanvasRow, want: 'y'},
	} {
		got, _, _, _ := screen.GetContent(cell.x, cell.y)
		if got != cell.want {
			t.Errorf("screen content at (%d,%d) = %q, want %q", cell.x, cell.y, got, cell.want)
		}
	}
}

func TestDrawInputClipsToCanvas(t *testing.T) {
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init returned an error: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(testClipWidth, testClipHeight)

	drawInput(screen, "abcdef\n123456\noutside")

	for _, cell := range []struct {
		x, y int
		want rune
	}{
		{x: testLastColumn, y: testCanvasRow, want: 'd'},
		{x: 0, y: testNextCanvasRow, want: '1'},
		{x: testLastColumn, y: testNextCanvasRow, want: '4'},
	} {
		got, _, _, _ := screen.GetContent(cell.x, cell.y)
		if got != cell.want {
			t.Errorf("screen content at (%d,%d) = %q, want %q", cell.x, cell.y, got, cell.want)
		}
	}
}
