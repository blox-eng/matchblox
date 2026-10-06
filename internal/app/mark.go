package app

import (
	"image/color"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// The start screen draws the site's matchbox in half blocks: each cell is
// two pixels, the top one in the foreground and the bottom one in the
// background.
const markW, markH = 40, 24

type cell struct{ top, bottom color.Color }

type raster [markH][markW]color.Color

const (
	isoS  = 5.0 // pixels for one isometric unit
	isoOX = 11.0
	isoOY = 7.0
)

var cos30 = math.Cos(math.Pi / 6)

// iso projects x (right-down), y (left-down) and z (up) to pixels.
func iso(x, y, z float64) (float64, float64) {
	return isoOX + (x-y)*cos30*isoS, isoOY + (x+y)*0.5*isoS - z*isoS
}

// fill paints every pixel whose centre in tells is inside.
func (r *raster) fill(c color.Color, in func(px, py float64) bool) {
	for y := 0; y < markH; y++ {
		for x := 0; x < markW; x++ {
			if in(float64(x)+0.5, float64(y)+0.5) {
				r[y][x] = c
			}
		}
	}
}

func (r *raster) poly(c color.Color, pts ...[3]float64) {
	xs, ys := make([]float64, len(pts)), make([]float64, len(pts))
	for i, p := range pts {
		xs[i], ys[i] = iso(p[0], p[1], p[2])
	}
	r.fill(c, func(px, py float64) bool {
		in := false
		for i, j := 0, len(xs)-1; i < len(xs); j, i = i, i+1 {
			if (ys[i] > py) != (ys[j] > py) && px < (xs[j]-xs[i])*(py-ys[i])/(ys[j]-ys[i])+xs[i] {
				in = !in
			}
		}
		return in
	})
}

// box draws the three faces a viewer sees: left (y+d), right (x+w), top.
func (r *raster) box(x, y, z, w, d, h float64, top, left, right color.Color) {
	r.poly(left, [3]float64{x, y + d, z}, [3]float64{x + w, y + d, z}, [3]float64{x + w, y + d, z + h}, [3]float64{x, y + d, z + h})
	r.poly(right, [3]float64{x + w, y, z}, [3]float64{x + w, y + d, z}, [3]float64{x + w, y + d, z + h}, [3]float64{x + w, y, z + h})
	r.poly(top, [3]float64{x, y, z + h}, [3]float64{x + w, y, z + h}, [3]float64{x + w, y + d, z + h}, [3]float64{x, y + d, z + h})
}

func (r *raster) line(x0, y0, x1, y1, width float64, c color.Color) {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	r.fill(c, func(px, py float64) bool {
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/l2))
		}
		return math.Hypot(px-(x0+t*dx), py-(y0+t*dy)) <= width/2
	})
}

func (r *raster) ellipse(cx, cy, rx, ry float64, c color.Color) {
	r.fill(c, func(px, py float64) bool {
		return (px-cx)*(px-cx)/(rx*rx)+(py-cy)*(py-cy)/(ry*ry) <= 1
	})
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }
func easeOut(x float64) float64 { return 1 - math.Pow(1-clamp01(x), 3) }

// The timeline of the start screen, in ms.
const (
	fallEnd   = 300.0
	openEnd   = 380.0
	strikeEnd = 480.0
	standEnd  = 560.0
)

// markFrame is the start screen at time t: the blocks fall into a box, the
// drawer opens, the match strikes on the side and stands up. In a dark
// terminal it burns; in a light one it stays unlit.
func markFrame(t time.Duration, dark bool) [][]cell {
	ms := float64(t) / float64(time.Millisecond)
	th := newStyles(dark).theme
	mk := darkMark
	if !dark {
		mk = lightMark
	}
	var r raster
	if ms < fallEnd {
		cells := [][2]float64{{0, 0}, {1, 0}, {0, 1}, {2, 0}, {1, 1}, {2, 1}}
		for k, c := range cells {
			start := float64(k) * 28
			if ms < start {
				continue
			}
			p := clamp01((ms - start) / 150)
			r.box(c[0], c[1], (1-p*p)*4, 1, 1, 1, mk.Top, mk.Left, mk.Right)
		}
		return r.cells()
	}
	r.box(0, 0, 0, 3, 2, 1, mk.Top, mk.Left, mk.Right)
	r.poly(mk.Striker, [3]float64{0.15, 2, 0.2}, [3]float64{2.85, 2, 0.2}, [3]float64{2.85, 2, 0.8}, [3]float64{0.15, 2, 0.8})
	out := 1.4 * easeOut((ms-fallEnd)/(openEnd-fallEnd))
	if out > 0.01 {
		r.box(3, 0.12, 0.08, out, 1.76, 0.8, mk.Tray, mk.Left, mk.Right)
	}
	const length = 2.0 * isoS
	s0x, s0y := iso(0.3, 2, 0.5)
	s1x, s1y := iso(2.8, 2, 0.5)
	ux, uy := iso(4.9, 0.7, 0)
	tail := func(hx, hy float64) (float64, float64) { return hx - length*0.9, hy + length*0.35 }
	var hx, hy, tx, ty float64
	switch {
	case ms < openEnd:
		hx, hy = iso(3+out-0.25, 1, 0.9)
		tx, ty = iso(3.1, 1, 0.9)
	case ms < strikeEnd:
		k := math.Pow((ms-openEnd)/(strikeEnd-openEnd), 1.6)
		hx, hy = s0x+(s1x-s0x)*k, s0y+(s1y-s0y)*k
		tx, ty = tail(hx, hy)
	default:
		k := easeOut((ms - strikeEnd) / (standEnd - strikeEnd))
		hx, hy = s1x+(ux-s1x)*k, s1y+(uy-length-s1y)*k
		bx, by := tail(s1x, s1y)
		tx, ty = bx+(ux-bx)*k, by+(uy-by)*k
	}
	r.line(tx, ty, hx, hy, 1.3, mk.Stick)
	r.ellipse(hx, hy, 1.3, 1.3, th.Accent)
	if dark && ms >= strikeEnd {
		size := 0.6 * isoS * easeOut((ms-strikeEnd)/(standEnd-strikeEnd))
		if size > 0.3 {
			r.ellipse(hx, hy-1-size*0.95, size*0.55, size*0.95, mk.Flame)
			r.ellipse(hx, hy-1-size*0.6, size*0.3, size*0.55, mk.Core)
		}
	}
	return r.cells()
}

func (r *raster) cells() [][]cell {
	out := make([][]cell, markH/2)
	for y := range out {
		out[y] = make([]cell, markW)
		for x := range out[y] {
			out[y][x] = cell{r[2*y][x], r[2*y+1][x]}
		}
	}
	return out
}

// drawMark renders a frame as lines of half blocks.
func drawMark(f [][]cell) []string {
	lines := make([]string, len(f))
	for y, row := range f {
		var b strings.Builder
		for _, c := range row {
			switch {
			case c.top != nil && c.bottom != nil:
				b.WriteString(lipgloss.NewStyle().Foreground(c.top).Background(c.bottom).Render("▀"))
			case c.top != nil:
				b.WriteString(lipgloss.NewStyle().Foreground(c.top).Render("▀"))
			case c.bottom != nil:
				b.WriteString(lipgloss.NewStyle().Foreground(c.bottom).Render("▄"))
			default:
				b.WriteByte(' ')
			}
		}
		lines[y] = b.String()
	}
	return lines
}
