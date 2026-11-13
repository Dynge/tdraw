package main

import (
	"testing"
)

func TestDrawLineFollowsDominantAxis(t *testing.T) {
	tests := []struct {
		name           string
		start, target  screenPoint
		startRune      rune
		drawnEnd       screenPoint
		endRune        rune
		unsnappedPoint screenPoint
	}{
		{
			name:           "horizontal movement is larger",
			start:          screenPoint{x: testLineOrigin, y: testLineRow},
			target:         screenPoint{x: testLineOrigin + testHorizontalLineDelta, y: testLineRow + verticalCellWeight},
			startRune:      '-',
			drawnEnd:       screenPoint{x: testLineOrigin + testHorizontalLineDelta, y: testLineRow},
			endRune:        '>',
			unsnappedPoint: screenPoint{x: testLineOrigin + testHorizontalLineDelta, y: testLineRow + verticalCellWeight},
		},
		{
			name:           "vertical movement is larger",
			start:          screenPoint{x: testVerticalStartX, y: testVerticalStartY},
			target:         screenPoint{x: testVerticalEndX, y: testVerticalEndY},
			startRune:      '|',
			drawnEnd:       screenPoint{x: testVerticalStartX, y: testVerticalEndY},
			endRune:        'v',
			unsnappedPoint: screenPoint{x: testVerticalEndX, y: testVerticalEndY},
		},
		{
			name:           "vertical movement is larger in reverse",
			start:          screenPoint{x: testVerticalEndX, y: testVerticalEndY},
			target:         screenPoint{x: testVerticalStartX, y: testVerticalStartY},
			startRune:      '|',
			drawnEnd:       screenPoint{x: testVerticalEndX, y: testVerticalStartY},
			endRune:        '^',
			unsnappedPoint: screenPoint{x: testVerticalStartX, y: testVerticalStartY},
		},
		{
			name:           "equal cell movement weighs vertical three times",
			start:          screenPoint{x: testLineOrigin, y: testLineRow},
			target:         screenPoint{x: testLineOrigin + testEqualLineDelta, y: testLineRow + testEqualLineDelta},
			startRune:      '|',
			drawnEnd:       screenPoint{x: testLineOrigin, y: testLineRow + testEqualLineDelta},
			endRune:        'v',
			unsnappedPoint: screenPoint{x: testLineOrigin + testEqualLineDelta, y: testLineRow + testEqualLineDelta},
		},
		{
			name:           "three horizontal cells equal one vertical cell",
			start:          screenPoint{x: testLineOrigin, y: testLineRow},
			target:         screenPoint{x: testLineOrigin + verticalCellWeight, y: testLineRow + testOneCellDelta},
			startRune:      '-',
			drawnEnd:       screenPoint{x: testLineOrigin + verticalCellWeight, y: testLineRow},
			endRune:        '>',
			unsnappedPoint: screenPoint{x: testLineOrigin + verticalCellWeight, y: testLineRow + testOneCellDelta},
		},
		{
			name:           "vertical weighting wins below three horizontal cells",
			start:          screenPoint{x: testLineOrigin, y: testLineRow},
			target:         screenPoint{x: testLineOrigin + verticalCellWeight - testOneCellDelta, y: testLineRow + testOneCellDelta},
			startRune:      '|',
			drawnEnd:       screenPoint{x: testLineOrigin, y: testLineRow + testOneCellDelta},
			endRune:        'v',
			unsnappedPoint: screenPoint{x: testLineOrigin + verticalCellWeight - testOneCellDelta, y: testLineRow + testOneCellDelta},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight+testLineExtraHeight)
			drawLine(screen, test.start, test.target, lineArrowheadsOn)

			assertScreenRune(t, screen, test.start.x, test.start.y, test.startRune)
			assertScreenRune(t, screen, test.drawnEnd.x, test.drawnEnd.y, test.endRune)
			assertScreenRune(t, screen, test.unsnappedPoint.x, test.unsnappedPoint.y, ' ')
		})
	}
}

func TestDrawLineWithoutArrowheads(t *testing.T) {
	tests := []struct {
		name       string
		start, end screenPoint
		lineRune   rune
	}{
		{
			name:     "horizontal line",
			start:    screenPoint{x: testLineOrigin, y: testLineRow},
			end:      screenPoint{x: testLineOrigin + testHorizontalLineDelta, y: testLineRow},
			lineRune: '-',
		},
		{
			name:     "vertical line",
			start:    screenPoint{x: testLineOrigin, y: testLineRow},
			end:      screenPoint{x: testLineOrigin, y: testLineRow + testEqualLineDelta},
			lineRune: '|',
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			screen := newTestCanvas(t, testCanvasWidth, testCanvasHeight+testLineExtraHeight)
			drawLine(screen, test.start, test.end, lineArrowheadsOff)

			assertScreenRune(t, screen, test.start.x, test.start.y, test.lineRune)
			assertScreenRune(t, screen, test.end.x, test.end.y, test.lineRune)
		})
	}
}

func TestLineArrowModeTogglesBetweenStates(t *testing.T) {
	if got := lineArrowheadsOn.toggled(); got != lineArrowheadsOff {
		t.Errorf("lineArrowheadsOn.toggled() = %v, want lineArrowheadsOff", got)
	}
	if got := lineArrowheadsOff.toggled(); got != lineArrowheadsOn {
		t.Errorf("lineArrowheadsOff.toggled() = %v, want lineArrowheadsOn", got)
	}
}
