tdraw
---
Inspired by [Explaining Code using ASCII Art](https://blog.regehr.org/archives/1653).<br/>
Supports box / line drawing, text input, freehand character drawing and erasing.

![tdraw](./tdraw.gif)

#### Install
```
go install github.com/Dynge/tdraw@latest
```
or download from https://github.com/Dynge/tdraw/releases

#### Usage
```
tdraw > draw.txt
```

You can also pipe text into `tdraw` to prefill the canvas:

```
some-command | tdraw > draw.txt
```

The input is placed at the top of the drawing area. Text beyond the terminal
size is clipped, and piped input is limited to 1 MiB.

Use the mouse wheel to scroll the canvas vertically, or Shift+wheel to scroll
horizontally. The headers stay fixed while the canvas moves.

```
// tdraw has 5 modes.
b: Box(+Undirected line, default)
ESC: End text entry
l: Line
a: Toggle line arrowheads (on by default)
t: Text
e: Erase
f: Freehand (type a printable ASCII character to select the brush; click or drag to draw)
ESC: Exit freehand mode
u: Undo the last completed drawing, erase, freehand stroke, or text-entry session
r: Redo

CTRL-C/CTRL-D: exit

// It might not work if terminal emulator consumes right click(iterm2)
MouseR: Eraser 
```
