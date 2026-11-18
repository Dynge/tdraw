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
	testClipHeight          = 5
	testHeaderWidth         = 80
	testHeaderPointerX      = 3
	testHeaderPointerY      = 4
	testLastColumn          = testClipWidth - 1
	testNextCanvasRow       = testCanvasRow + 1
	testPreviewColumn       = 2
	testSecondColumn        = 2
	testViewportWidth       = 4
	testViewportHeight      = 5
	testViewportPointerX    = 1
	testViewportPointerY    = 2
	testWideRuneCellWidth   = 2
	testAlphabetLength      = 26
	testLineOrigin          = 1
	testLineRow             = canvasTopRow + 1
	testEqualLineDelta      = 4
	testOneCellDelta        = 1
	testHorizontalLineDelta = 10
	testVerticalStartX      = 2
	testVerticalStartY      = canvasTopRow + 1
	testVerticalEndX        = 5
	testVerticalEndY        = canvasTopRow + 8
	testLineExtraHeight     = 8
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
		"Undo / redo (outside text and freehand modes)",
		"b               Switch to box mode from line or erase mode",
		"a               Toggle arrowheads in line mode",
		"f               Enter freehand mode",
		"Left-click or drag to draw with the selected ASCII character",
		"Esc             Finish text entry",
		"Left-click the canvas to choose where text starts",
		"Backspace       Delete a character or join with the previous line",
		"Ctrl+V          Paste clipboard text at the top-left of the canvas",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help output does not contain %q", want)
		}
	}

	if strings.Contains(output.String(), "Esc             Return to box mode") {
		t.Error("help output still describes Escape as the box-mode shortcut")
	}
}

func TestPreparePastedInputTrimsTrailingSpacesOnEachLine(t *testing.T) {
	input, err := preparePastedInput("    input      \r\n  next row  \n")
	if err != nil {
		t.Fatalf("preparePastedInput returned an error: %v", err)
	}
	if want := "    input\n  next row\n"; input != want {
		t.Errorf("prepared clipboard input = %q, want %q", input, want)
	}
}

func TestPreparePastedInputRejectsOversizedClipboard(t *testing.T) {
	input := strings.Repeat("x", maxPasteInputBytes+1)
	if _, err := preparePastedInput(input); err == nil {
		t.Fatal("preparePastedInput accepted oversized clipboard text")
	}
}

func TestDrawPastedInputStartsAtCanvasTopLeftAndCanBeUndone(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	canvas.SetContent(0, testCanvasRow, 'x', nil, tcell.StyleDefault)
	canvas.SetContent(1, testCanvasRow, 'y', nil, tcell.StyleDefault)
	history := newCanvasHistory(canvas)
	input, err := preparePastedInput("ab\ncd")
	if err != nil {
		t.Fatalf("preparePastedInput returned an error: %v", err)
	}

	drawPastedInput(canvas, history, input)

	assertScreenRune(t, canvas, 0, testCanvasRow, 'a')
	assertScreenRune(t, canvas, 1, testCanvasRow, 'b')
	assertScreenRune(t, canvas, 0, testNextCanvasRow, 'c')
	assertScreenRune(t, canvas, 1, testNextCanvasRow, 'd')

	history.undo(canvas)
	assertScreenRune(t, canvas, 0, testCanvasRow, 'x')
	assertScreenRune(t, canvas, 1, testCanvasRow, 'y')
	assertScreenRune(t, canvas, 0, testNextCanvasRow, ' ')

	history.redo(canvas)
	assertScreenRune(t, canvas, 0, testCanvasRow, 'a')
	assertScreenRune(t, canvas, 0, testNextCanvasRow, 'c')
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
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)

	drawInput(canvas, "ab\tc\r\nxy")

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
		got, _, _, _ := canvas.GetContent(cell.x, cell.y)
		if got != cell.want {
			t.Errorf("canvas content at (%d,%d) = %q, want %q", cell.x, cell.y, got, cell.want)
		}
	}
}

func TestDrawInputClipsToCanvas(t *testing.T) {
	canvas := newTestCanvas(t, testClipWidth, testClipHeight)

	drawInput(canvas, "abcdef\n123456\noutside")

	for _, cell := range []struct {
		x, y int
		want rune
	}{
		{x: testLastColumn, y: testCanvasRow, want: 'd'},
		{x: 0, y: testNextCanvasRow, want: '1'},
		{x: testLastColumn, y: testNextCanvasRow, want: '4'},
	} {
		got, _, _, _ := canvas.GetContent(cell.x, cell.y)
		if got != cell.want {
			t.Errorf("canvas content at (%d,%d) = %q, want %q", cell.x, cell.y, got, cell.want)
		}
	}
}

func TestCanvasHistoryUndoAndRedo(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	canvas.SetContent(1, testCanvasRow, 'a', nil, tcell.StyleDefault)
	history := newCanvasHistory(canvas)

	canvas.SetContent(1, testCanvasRow, 'b', nil, tcell.StyleDefault)
	history.record(canvas)

	history.undo(canvas)
	assertScreenRune(t, canvas, 1, testCanvasRow, 'a')

	history.redo(canvas)
	assertScreenRune(t, canvas, 1, testCanvasRow, 'b')
}

func TestCanvasHistoryNewActionClearsRedo(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	history := newCanvasHistory(canvas)

	canvas.SetContent(1, testCanvasRow, 'a', nil, tcell.StyleDefault)
	history.record(canvas)
	history.undo(canvas)

	canvas.SetContent(testSecondColumn, testCanvasRow, 'b', nil, tcell.StyleDefault)
	history.record(canvas)
	history.redo(canvas)

	assertScreenRune(t, canvas, 1, testCanvasRow, ' ')
	assertScreenRune(t, canvas, testSecondColumn, testCanvasRow, 'b')
}

func TestCanvasHistoryIsBounded(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	history := newCanvasHistory(canvas)

	for i := 0; i < maxHistoryEntries+1; i++ {
		canvas.SetContent(0, testCanvasRow, rune('a'+i%testAlphabetLength), nil, tcell.StyleDefault)
		history.record(canvas)
	}

	if got, want := len(history.snapshots), maxHistoryEntries+1; got != want {
		t.Fatalf("history entries = %d, want %d", got, want)
	}
}

func TestTextEntryPreviewFollowsPointerAndRestoresCanvas(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	canvas.SetContent(1, testCanvasRow, 'x', nil, tcell.StyleDefault)
	entry := textEntry{previewX: -1, previewY: -1}

	entry.beginSelection(canvas, 1, testCanvasRow, tcell.StyleDefault)
	assertScreenRune(t, canvas, 1, testCanvasRow, '+')

	entry.movePreview(canvas, testPreviewColumn, testCanvasRow, tcell.StyleDefault)
	assertScreenRune(t, canvas, 1, testCanvasRow, 'x')
	assertScreenRune(t, canvas, testPreviewColumn, testCanvasRow, '+')

	if !entry.selectPosition(canvas, testPreviewColumn, testCanvasRow, tcell.StyleDefault.Blink(true)) {
		t.Fatal("selectPosition rejected a canvas position")
	}
	if !entry.cursorSet {
		t.Fatal("selectPosition did not activate the text cursor")
	}
	assertScreenRune(t, canvas, testPreviewColumn, testCanvasRow, '_')
}

func TestTextEntryCannotSelectHeaderPosition(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}

	if entry.selectPosition(canvas, 1, 0, tcell.StyleDefault) {
		t.Fatal("selectPosition accepted a header position")
	}
	if entry.cursorSet {
		t.Fatal("text cursor became active after an invalid selection")
	}
}

func TestFreehandEntrySelectsPrintableASCIICharacters(t *testing.T) {
	entry := newFreehandEntry()
	if got, want := entry.character, '*'; got != want {
		t.Fatalf("initial freehand character = %q, want %q", got, want)
	}

	for character := rune(' '); character <= '~'; character++ {
		if !entry.selectCharacter(character) {
			t.Fatalf("selectCharacter rejected printable ASCII %q", character)
		}
		if got := entry.character; got != character {
			t.Errorf("selected character = %q, want %q", got, character)
		}
	}

	for _, character := range []rune{'\n', '\x7f', '界'} {
		if entry.selectCharacter(character) {
			t.Errorf("selectCharacter accepted non-printable ASCII character %q", character)
		}
		if got, want := entry.character, '~'; got != want {
			t.Errorf("character after rejected selection = %q, want %q", got, want)
		}
	}
}

func TestFreehandEntryDrawsSelectedCharacterOnCanvas(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := newFreehandEntry()
	entry.selectCharacter('#')

	if !entry.draw(canvas, testSecondColumn, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("draw rejected a canvas position")
	}
	assertScreenRune(t, canvas, testSecondColumn, testCanvasRow, '#')

	if entry.draw(canvas, testSecondColumn, canvasTopRow-1, tcell.StyleDefault) {
		t.Fatal("draw accepted a header position")
	}
	if entry.draw(canvas, testCanvasWidth, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("draw accepted a position beyond the canvas width")
	}
}

func TestDrawHeaderShowsModeAndContextHints(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		textCursorSet  bool
		lineArrows     lineArrowMode
		wantGlobal     string
		wantContext    string
		unwantedGlobal string
	}{
		{
			name:           "box mode",
			mode:           MODE_BOX,
			wantGlobal:     "l:line / t:text / e:erase / f:freehand",
			wantContext:    "Left-drag: draw box",
			unwantedGlobal: "b:box",
		},
		{
			name:        "line mode",
			mode:        MODE_LINE,
			lineArrows:  lineArrowheadsOn,
			wantGlobal:  "b:box / t:text / e:erase / f:freehand",
			wantContext: "a: arrows on",
		},
		{
			name:        "line mode without arrowheads",
			mode:        MODE_LINE,
			lineArrows:  lineArrowheadsOff,
			wantGlobal:  "b:box / t:text / e:erase",
			wantContext: "a: arrows off",
		},
		{
			name:        "erase mode",
			mode:        MODE_ERASE,
			wantGlobal:  "b:box / l:line / t:text / f:freehand",
			wantContext: "Left-drag: erase area",
		},
		{
			name:           "text selection",
			mode:           MODE_TEXT,
			wantGlobal:     "Esc:box",
			wantContext:    "Left-click to select text position",
			unwantedGlobal: "u:undo",
		},
		{
			name:          "text entry",
			mode:          MODE_TEXT,
			textCursorSet: true,
			wantGlobal:    "Esc:box",
			wantContext:   "Esc: exit text mode",
		},
		{
			name:        "freehand",
			mode:        MODE_FREEHAND,
			wantGlobal:  "Esc:box",
			wantContext: "Character: '*' / Type ASCII to change",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			screen := newTestScreen(t, testHeaderWidth, testCanvasHeight)
			drawHeader(screen, headerState{
				pointer:    screenPoint{x: testHeaderPointerX, y: testHeaderPointerY},
				mode:       test.mode,
				text:       textEntry{cursorSet: test.textCursorSet},
				freehand:   newFreehandEntry(),
				lineArrows: test.lineArrows,
			})

			global := screenLine(screen, 0)
			if !strings.Contains(global, test.wantGlobal) {
				t.Errorf("global header = %q, want it to contain %q", global, test.wantGlobal)
			}
			if test.unwantedGlobal != "" && strings.Contains(global, test.unwantedGlobal) {
				t.Errorf("global header = %q, should not contain %q", global, test.unwantedGlobal)
			}

			context := screenLine(screen, 1)
			if !strings.Contains(context, test.wantContext) {
				t.Errorf("context header = %q, want it to contain %q", context, test.wantContext)
			}
		})
	}
}

func TestDrawHeaderClearsPreviousHints(t *testing.T) {
	screen := newTestScreen(t, testHeaderWidth, testCanvasHeight)
	drawHeader(screen, headerState{pointer: screenPoint{x: testHeaderPointerX, y: testHeaderPointerY}, mode: MODE_ERASE})
	drawHeader(screen, headerState{pointer: screenPoint{x: testHeaderPointerX, y: testHeaderPointerY}, mode: MODE_BOX})

	global := screenLine(screen, 0)
	if strings.Contains(global, "b:box") {
		t.Errorf("global header retained an old mode hint: %q", global)
	}

	context := screenLine(screen, 1)
	if strings.Contains(context, "erase area") {
		t.Errorf("context header retained an old mode hint: %q", context)
	}
}

func TestCanvasViewportScrollsAndRendersBackingCanvas(t *testing.T) {
	canvas := newTestCanvas(t, testHeaderWidth, testCanvasHeight)
	canvas.SetContent(testSecondColumn, testCanvasRow, 'a', nil, tcell.StyleDefault)
	viewport := newCanvasViewport(canvas, testHeaderWidth, testCanvasHeight)

	if !viewport.scroll(tcell.WheelDown, 0) {
		t.Fatal("vertical wheel did not scroll the canvas")
	}
	if got, want := viewport.offset.y, canvasScrollStep; got != want {
		t.Errorf("vertical offset = %d, want %d", got, want)
	}
	point, ok := viewport.toCanvas(testSecondColumn, canvasTopRow)
	if !ok {
		t.Fatal("canvas viewport rejected its first canvas row")
	}
	if got, want := point.y, canvasTopRow+canvasScrollStep; got != want {
		t.Errorf("translated canvas row = %d, want %d", got, want)
	}

	screen := newTestScreen(t, testHeaderWidth, testCanvasHeight)
	drawHeader(screen, headerState{pointer: screenPoint{x: testViewportPointerX, y: testViewportPointerY}, mode: MODE_BOX})
	viewport.render(screen)

	assertScreenRune(t, screen, testSecondColumn, canvasTopRow, ' ')
	canvas.SetContent(testSecondColumn, canvasTopRow+canvasScrollStep, 'b', nil, tcell.StyleDefault)
	viewport.render(screen)
	assertScreenRune(t, screen, testSecondColumn, canvasTopRow, 'b')
	assertScreenRune(t, screen, 0, canvasTopRow-1, tcell.RuneHLine)
	if !strings.Contains(screenLine(screen, 0), "l:line / t:text / e:erase") {
		t.Errorf("header changed during canvas scrolling: %q", screenLine(screen, 0))
	}

	if !viewport.scroll(tcell.WheelUp, 0) {
		t.Fatal("wheel up did not return the canvas viewport")
	}
	if got := viewport.offset.y; got != 0 {
		t.Errorf("vertical offset after returning = %d, want 0", got)
	}
}

func TestCanvasViewportShiftWheelScrollsHorizontally(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	viewport := newCanvasViewport(canvas, testViewportWidth, testViewportHeight)

	if !viewport.scroll(tcell.WheelDown, tcell.ModShift) {
		t.Fatal("Shift+wheel did not scroll horizontally")
	}
	if got, want := viewport.offset.x, canvasScrollStep; got != want {
		t.Errorf("horizontal offset = %d, want %d", got, want)
	}
	if got := viewport.offset.y; got != 0 {
		t.Errorf("vertical offset after Shift+wheel = %d, want 0", got)
	}

	if !viewport.scroll(tcell.WheelUp, tcell.ModShift) {
		t.Fatal("Shift+wheel up did not return the viewport to the left edge")
	}
	if got := viewport.offset.x; got != 0 {
		t.Errorf("horizontal offset after returning = %d, want 0", got)
	}
	if viewport.scroll(tcell.WheelUp, tcell.ModShift) {
		t.Fatal("Shift+wheel up scrolled beyond the left edge")
	}
	if viewport.scroll(tcell.Button1, tcell.ModShift) {
		t.Fatal("non-wheel button changed the viewport")
	}
}

func TestCanvasViewportPreservesWideCharacters(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	canvas.SetContent(testPreviewColumn, testCanvasRow, '界', nil, tcell.StyleDefault)
	viewport := newCanvasViewport(canvas, testCanvasWidth, testCanvasHeight)
	screen := newTestScreen(t, testCanvasWidth, testCanvasHeight)

	viewport.render(screen)

	main, _, _, width := screen.GetContent(testPreviewColumn, testCanvasRow)
	if main != '界' {
		t.Errorf("rendered wide character = %q, want %q", main, '界')
	}
	if width != testWideRuneCellWidth {
		t.Errorf("rendered wide character width = %d, want %d", width, testWideRuneCellWidth)
	}
}

func TestTextEntryBackspaceAtEmptyLineStartReturnsToPreviousLine(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}
	if !entry.selectPosition(canvas, testPreviewColumn, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("selectPosition rejected a canvas position")
	}

	entry.insertRune(canvas, 'a', tcell.StyleDefault)
	entry.startNewLine(canvas, tcell.StyleDefault)
	entry.insertRune(canvas, 'b', tcell.StyleDefault)
	entry.insertRune(canvas, 'c', tcell.StyleDefault)
	entry.backspace(canvas, tcell.StyleDefault)
	entry.backspace(canvas, tcell.StyleDefault)
	entry.backspace(canvas, tcell.StyleDefault)

	if got, want := entry.cursorX, testPreviewColumn+testOneCellDelta; got != want {
		t.Errorf("cursorX after deleting newline = %d, want %d", got, want)
	}
	if got, want := entry.cursorY, testCanvasRow; got != want {
		t.Errorf("cursorY after deleting newline = %d, want %d", got, want)
	}
	if got, want := len(entry.lineEnds), 1; got != want {
		t.Errorf("tracked line ends = %d, want %d", got, want)
	}
	assertScreenRune(t, canvas, testPreviewColumn, testCanvasRow, 'a')
	assertScreenRune(t, canvas, testPreviewColumn+testOneCellDelta, testCanvasRow, '_')
	assertScreenRune(t, canvas, testPreviewColumn, testNextCanvasRow, ' ')
	assertScreenRune(t, canvas, testPreviewColumn+testOneCellDelta, testNextCanvasRow, ' ')
}

func TestTextEntryBackspaceDoesNotCrossInitialLineStart(t *testing.T) {
	canvas := newTestCanvas(t, testCanvasWidth, testCanvasHeight)
	entry := textEntry{previewX: -1, previewY: -1}
	if !entry.selectPosition(canvas, testPreviewColumn, testCanvasRow, tcell.StyleDefault) {
		t.Fatal("selectPosition rejected a canvas position")
	}

	entry.backspace(canvas, tcell.StyleDefault)

	if got, want := entry.cursorX, testPreviewColumn; got != want {
		t.Errorf("cursorX at initial line start = %d, want %d", got, want)
	}
	if got, want := entry.cursorY, testCanvasRow; got != want {
		t.Errorf("cursorY at initial line start = %d, want %d", got, want)
	}
	assertScreenRune(t, canvas, testPreviewColumn, testCanvasRow, '_')
}

func newTestCanvas(t *testing.T, width, height int) *canvasBuffer {
	t.Helper()
	return newCanvasBuffer(width, height)
}

func newTestScreen(t *testing.T, width, height int) tcell.Screen {
	t.Helper()
	screen := tcell.NewSimulationScreen("")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init returned an error: %v", err)
	}
	screen.SetSize(width, height)
	t.Cleanup(screen.Fini)
	return screen
}

type contentReader interface {
	GetContent(x, y int) (mainc rune, combc []rune, style tcell.Style, width int)
}

func assertScreenRune(t *testing.T, screen contentReader, x, y int, want rune) {
	t.Helper()
	got, _, _, _ := screen.GetContent(x, y)
	if got != want {
		t.Errorf("screen content at (%d,%d) = %q, want %q", x, y, got, want)
	}
}

func screenLine(screen tcell.Screen, row int) string {
	width, _ := screen.Size()
	var line strings.Builder
	for col := 0; col < width; col++ {
		main, _, _, _ := screen.GetContent(col, row)
		line.WriteRune(main)
	}
	return strings.TrimRight(line.String(), " ")
}
