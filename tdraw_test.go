package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gdamore/tcell"
)

const (
	testCanvasWidth         = 12
	testCanvasHeight        = 5
	testCanvasRow           = canvasTopRow
	testClipWidth           = 4
	testClipHeight          = 4
	testHeaderWidth         = 80
	testLastColumn          = testClipWidth - 1
	testNextCanvasRow       = testCanvasRow + 1
	testPreviewColumn       = 2
	testLineOrigin          = 1
	testEqualLineDelta      = 4
	testOneCellDelta        = 1
	testHorizontalLineDelta = 10
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

func TestPrintHelpIfRequested(t *testing.T) {
	var output strings.Builder
	if !printHelpIfRequested([]string{"--help"}, &output) {
		t.Fatal("printHelpIfRequested did not recognize --help")
	}

	for _, want := range []string{
		"Usage:",
		"some-command | tdraw > draw.txt",
		"Left-drag",
		"Right-click",
		"Ctrl+C / Ctrl+D",
		"Undo / redo",
		"b               Switch to box mode from line or erase mode",
		"Esc             Finish text entry",
		"Left-click the canvas to choose where text starts",
		"Backspace       Delete a character or join with the previous line",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help output does not contain %q", want)
		}
	}

	if strings.Contains(output.String(), "Esc             Return to box mode") {
		t.Error("help output still describes Escape as the box-mode shortcut")
	}
}

func TestPrintHelpIfNotRequested(t *testing.T) {
	var output strings.Builder
	if printHelpIfRequested([]string{"--other"}, &output) {
		t.Fatal("printHelpIfRequested recognized an unrelated argument")
	}
	if output.Len() != 0 {
		t.Errorf("help output = %q, want empty output", output.String())
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

func TestCanvasHistoryUndoAndRedo(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	screen.SetContent(1, testCanvasRow, 'a', nil, tcell.StyleDefault)
	history := newCanvasHistory(screen)

	screen.SetContent(1, testCanvasRow, 'b', nil, tcell.StyleDefault)
	history.record(screen)

	history.undo(screen)
	assertScreenRune(t, screen, 1, testCanvasRow, 'a')

	history.redo(screen)
	assertScreenRune(t, screen, 1, testCanvasRow, 'b')
}

func TestCanvasHistoryNewActionClearsRedo(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	history := newCanvasHistory(screen)

	screen.SetContent(1, testCanvasRow, 'a', nil, tcell.StyleDefault)
	history.record(screen)
	history.undo(screen)

	screen.SetContent(2, testCanvasRow, 'b', nil, tcell.StyleDefault)
	history.record(screen)
	history.redo(screen)

	assertScreenRune(t, screen, 1, testCanvasRow, ' ')
	assertScreenRune(t, screen, 2, testCanvasRow, 'b')
}

func TestCanvasHistoryIsBounded(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	history := newCanvasHistory(screen)

	for i := 0; i < maxHistoryEntries+1; i++ {
		screen.SetContent(0, testCanvasRow, rune('a'+i%26), nil, tcell.StyleDefault)
		history.record(screen)
	}

	if got, want := len(history.snapshots), maxHistoryEntries+1; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
}

func TestTextEntryPreviewFollowsPointerAndRestoresCanvas(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	screen.SetContent(1, testCanvasRow, 'x', nil, tcell.StyleDefault)
	entry := textEntry{previewX: -1, previewY: -1}

	entry.beginSelection(screen, 1, testCanvasRow, tcell.StyleDefault)
	assertScreenRune(t, screen, 1, testCanvasRow, '+')

	entry.movePreview(screen, testPreviewColumn, testCanvasRow, tcell.StyleDefault)
	assertScreenRune(t, screen, 1, testCanvasRow, 'x')
	assertScreenRune(t, screen, testPreviewColumn, testCanvasRow, '+')

	if !entry.selectPosition(screen, testPreviewColumn, testCanvasRow, tcell.StyleDefault.Blink(true)) {
		t.Fatal("selectPosition rejected a canvas position")
	}
	if !entry.cursorSet {
		t.Fatal("selectPosition did not activate the text cursor")
	}
	assertScreenRune(t, screen, testPreviewColumn, testCanvasRow, '_')
}

func TestTextEntryCannotSelectHeaderPosition(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}

	if entry.selectPosition(screen, 1, 0, tcell.StyleDefault) {
		t.Fatal("selectPosition accepted a header position")
	}
	if entry.cursorSet {
		t.Fatal("text cursor became active after an invalid selection")
	}
}

func TestTextEntryBackspaceAtEmptyLineStartReturnsToPreviousLine(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}
	if !entry.selectPosition(screen, testPreviewColumn, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("selectPosition rejected a canvas position")
	}

	entry.insertRune(screen, 'a', tcell.StyleDefault)
	entry.startNewLine(screen, tcell.StyleDefault)
	entry.insertRune(screen, 'b', tcell.StyleDefault)
	entry.insertRune(screen, 'c', tcell.StyleDefault)
	entry.backspace(screen, tcell.StyleDefault)
	entry.backspace(screen, tcell.StyleDefault)
	entry.backspace(screen, tcell.StyleDefault)

	if got, want := entry.cursorX, testPreviewColumn+testOneCellDelta; got != want {
		t.Errorf("cursorX after deleting newline = %d, want %d", got, want)
	}
	if got, want := entry.cursorY, testCanvasRow; got != want {
		t.Errorf("cursorY after deleting newline = %d, want %d", got, want)
	}
	if got, want := len(entry.lineEnds), 1; got != want {
		t.Errorf("tracked line ends = %d, want %d", got, want)
	}
	assertScreenRune(t, screen, testPreviewColumn, testCanvasRow, 'a')
	assertScreenRune(t, screen, testPreviewColumn+testOneCellDelta, testCanvasRow, '_')
	assertScreenRune(t, screen, testPreviewColumn, testNextCanvasRow, ' ')
	assertScreenRune(t, screen, testPreviewColumn+testOneCellDelta, testNextCanvasRow, ' ')
}

func TestTextEntryBackspaceDoesNotCrossInitialLineStart(t *testing.T) {
	screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}
	if !entry.selectPosition(screen, testPreviewColumn, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("selectPosition rejected a canvas position")
	}

	entry.backspace(screen, tcell.StyleDefault)

	if got, want := entry.cursorX, testPreviewColumn; got != want {
		t.Errorf("cursorX at initial line start = %d, want %d", got, want)
	}
	if got, want := entry.cursorY, testCanvasRow; got != want {
		t.Errorf("cursorY at initial line start = %d, want %d", got, want)
	}
	assertScreenRune(t, screen, testPreviewColumn, testCanvasRow, '_')
}

func newTestCanvas(t *testing.T, width, height int) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init returned an error: %v", err)
	}
	screen.SetSize(width, height)
	t.Cleanup(screen.Fini)
	return screen
}

func assertScreenRune(t *testing.T, screen tcell.Screen, x, y int, want rune) {
	t.Helper()
	got, _, _, _ := screen.GetContent(x, y)
	if got != want {
		t.Errorf("screen content at (%d,%d) = %q, want %q", x, y, got, want)
	}
}

func TestDrawLineFollowsDominantAxis(t *testing.T) {
	tests := []struct {
		name           string
		x1, y1, x2, y2 int
		start          rune
		endX, endY     int
		end            rune
		unsnappedX     int
		unsnappedY     int
	}{
		{
			name:       "horizontal movement is larger",
			x1:         testLineOrigin,
			y1:         testLineOrigin,
			x2:         testLineOrigin + testHorizontalLineDelta,
			y2:         testLineOrigin + verticalCellWeight,
			start:      '-',
			endX:       testLineOrigin + testHorizontalLineDelta,
			endY:       testLineOrigin,
			end:        '>',
			unsnappedX: testLineOrigin + testHorizontalLineDelta,
			unsnappedY: testLineOrigin + verticalCellWeight,
		},
		{
			name:       "vertical movement is larger",
			x1:         2,
			y1:         1,
			x2:         5,
			y2:         8,
			start:      '|',
			endX:       2,
			endY:       8,
			end:        'v',
			unsnappedX: 5,
			unsnappedY: 8,
		},
		{
			name:       "vertical movement is larger in reverse",
			x1:         5,
			y1:         8,
			x2:         2,
			y2:         1,
			start:      '|',
			endX:       5,
			endY:       1,
			end:        '^',
			unsnappedX: 2,
			unsnappedY: 1,
		},
		{
			name:       "equal cell movement weighs vertical three times",
			x1:         testLineOrigin,
			y1:         testLineOrigin,
			x2:         testLineOrigin + testEqualLineDelta,
			y2:         testLineOrigin + testEqualLineDelta,
			start:      '|',
			endX:       testLineOrigin,
			endY:       testLineOrigin + testEqualLineDelta,
			end:        'v',
			unsnappedX: testLineOrigin + testEqualLineDelta,
			unsnappedY: testLineOrigin + testEqualLineDelta,
		},
		{
			name:       "three horizontal cells equal one vertical cell",
			x1:         testLineOrigin,
			y1:         testLineOrigin,
			x2:         testLineOrigin + verticalCellWeight,
			y2:         testLineOrigin + testOneCellDelta,
			start:      '-',
			endX:       testLineOrigin + verticalCellWeight,
			endY:       testLineOrigin,
			end:        '>',
			unsnappedX: testLineOrigin + verticalCellWeight,
			unsnappedY: testLineOrigin + testOneCellDelta,
		},
		{
			name:       "vertical weighting wins below three horizontal cells",
			x1:         testLineOrigin,
			y1:         testLineOrigin,
			x2:         testLineOrigin + verticalCellWeight - testOneCellDelta,
			y2:         testLineOrigin + testOneCellDelta,
			start:      '|',
			endX:       testLineOrigin,
			endY:       testLineOrigin + testOneCellDelta,
			end:        'v',
			unsnappedX: testLineOrigin + verticalCellWeight - testOneCellDelta,
			unsnappedY: testLineOrigin + testOneCellDelta,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("")
			if err := screen.Init(); err != nil {
				t.Fatalf("screen.Init returned an error: %v", err)
			}
			defer screen.Fini()
			screen.SetSize(testCanvasWidth, testCanvasHeight+5)

			drawLine(screen, test.x1, test.y1, test.x2, test.y2, tcell.StyleDefault, '*')

			for _, cell := range []struct {
				x, y int
				want rune
			}{
				{x: test.x1, y: test.y1, want: test.start},
				{x: test.endX, y: test.endY, want: test.end},
				{x: test.unsnappedX, y: test.unsnappedY, want: ' '},
			} {
				got, _, _, _ := screen.GetContent(cell.x, cell.y)
				if got != cell.want {
					t.Errorf("screen content at (%d,%d) = %q, want %q", cell.x, cell.y, got, cell.want)
				}
			}
		})
	}
}
