package main

import (
	"image"
	"math"
)

// Scanned pages come off the glass a fraction of a degree out of square.
// In print that is invisible, but it is fatal for a detector that looks
// for the longest dark run within a single pixel row: at 0.5 degrees a
// grid line walks across ten pixels over the width of a 16x16 puzzle, so
// no row ever sees more than a fragment of it and the lattice search
// reports "no 17x17 lattice found at all" on a perfectly crisp page.
//
// deskew estimates the rotation from the sharpness of the horizontal ink
// profile: when the page is square, every line of text and every grid
// line falls into one row bucket and the profile is a row of tall spikes,
// which the sum of squares rewards. Born-digital pages score best at zero
// and are left untouched.

const (
	deskewRange = 2.0  // degrees searched either way
	deskewMin   = 0.15 // below this the rotation is not worth the resampling
)

// deskew rotates the bitmap upright and returns the correction applied,
// in degrees (0 = nothing done).
func (bm *bitmap) deskew() float64 {
	// The score only needs the positions of dark pixels, so collect them
	// once instead of walking the image per candidate angle.
	var xs, ys []int32
	for y := 0; y < bm.h; y += 2 {
		for x := 0; x < bm.w; x += 2 {
			if bm.lum[y*bm.w+x] < bm.ink {
				xs = append(xs, int32(x))
				ys = append(ys, int32(y))
			}
		}
	}
	if len(xs) < 1000 {
		return 0 // nearly blank page, nothing to align
	}

	off := int32(bm.w)
	hist := make([]int32, bm.h+2*bm.w)
	score := func(deg float64) float64 {
		tan := math.Tan(deg * math.Pi / 180)
		for i := range hist {
			hist[i] = 0
		}
		for i := range xs {
			b := ys[i] - int32(float64(xs[i])*tan) + off
			if b >= 0 && int(b) < len(hist) {
				hist[b]++
			}
		}
		// Score the *smoothed* profile. Rounding the shift to whole pixels
		// splits a line across two buckets at every angle except zero,
		// where the shift is exactly nothing - so an unsmoothed sum of
		// squares gives "do not rotate" a bonus no other angle can earn.
		// On a page whose text is square but whose grid is not, that bonus
		// is enough to win, and the grid stays crooked.
		var s float64
		for i := 1; i < len(hist)-1; i++ {
			v := float64(hist[i-1] + hist[i] + hist[i+1])
			s += v * v
		}
		return s
	}

	best, bestScore := 0.0, score(0)
	for deg := -deskewRange; deg <= deskewRange+1e-9; deg += 0.1 {
		if s := score(deg); s > bestScore {
			best, bestScore = deg, s
		}
	}
	for deg := best - 0.08; deg <= best+0.08+1e-9; deg += 0.02 {
		if s := score(deg); s > bestScore {
			best, bestScore = deg, s
		}
	}
	if math.Abs(best) < deskewMin {
		return 0
	}
	bm.rotate(best)
	return best
}

// crop returns an independent bitmap holding a rectangle of this one.
func (bm *bitmap) crop(x0, y0, w, h int) *bitmap {
	x0, y0 = max(x0, 0), max(y0, 0)
	w, h = min(w, bm.w-x0), min(h, bm.h-y0)
	out := &bitmap{w: w, h: h, lum: make([]uint8, w*h), ink: bm.ink, line: bm.line}
	for y := 0; y < h; y++ {
		copy(out.lum[y*w:(y+1)*w], bm.lum[(y0+y)*bm.w+x0:(y0+y)*bm.w+x0+w])
	}
	return out
}

// tiles cuts the page into overlapping windows large enough to hold a
// whole puzzle grid. A thick magazine does not lie flat on the scanner,
// so the page is not rotated but *bowed*: a single angle fits it nowhere,
// while any one region is close enough to straight. Deskewing per tile
// turns that curve into a handful of locally square images.
func (bm *bitmap) tiles() []image.Point {
	var pts []image.Point
	tw, th := bm.tileSize()
	for y := 0; y < bm.h; y += th / 2 {
		for x := 0; x < bm.w; x += tw / 2 {
			pts = append(pts, image.Pt(min(x, bm.w-tw), min(y, bm.h-th)))
			if x+tw >= bm.w {
				break
			}
		}
		if y+th >= bm.h {
			break
		}
	}
	return pts
}

// tileSize: a tile has to hold a whole puzzle grid, which never takes up
// more than about half the page in either direction.
func (bm *bitmap) tileSize() (int, int) { return bm.w * 3 / 5, bm.h * 2 / 5 }

// rotate turns the bitmap by deg degrees, sampling bilinearly and
// treating everything outside the image as paper white.
func (bm *bitmap) rotate(deg float64) {
	rad := deg * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	cx, cy := float64(bm.w)/2, float64(bm.h)/2
	out := make([]uint8, len(bm.lum))
	for y := 0; y < bm.h; y++ {
		dy := float64(y) - cy
		for x := 0; x < bm.w; x++ {
			dx := float64(x) - cx
			out[y*bm.w+x] = bm.sample(cx+dx*cos-dy*sin, cy+dx*sin+dy*cos)
		}
	}
	bm.lum = out
}

func (bm *bitmap) sample(x, y float64) uint8 {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	get := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= bm.w || y >= bm.h {
			return 255 // paper
		}
		return float64(bm.lum[y*bm.w+x])
	}
	a := get(x0, y0)*(1-fx) + get(x0+1, y0)*fx
	b := get(x0, y0+1)*(1-fx) + get(x0+1, y0+1)*fx
	return uint8(a*(1-fy) + b*fy + 0.5)
}
