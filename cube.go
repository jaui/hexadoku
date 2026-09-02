// Hexadocube: six hexadokus on the faces of a cube (Elektor 7-8/2010,
// designer Claude Ghyselen). The page prints the cube unfolded - the
// magazine calls it the development - as six 16x16 grids on one raster:
//
//	                         +----+
//	                         | 4  |     the four grids of the middle row
//	+----+----+----+----+----+----+     wrap around the cube, face 4 is
//	| 0  | 1  | 2  | 3  |               its top and face 5 its bottom
//	+----+----+----+----+----+
//	                         | 5  |
//	                         +----+
//
// A face is linked to each of its four neighbours by the cells lying on
// the cube edge between them: those cells are the same cell seen from two
// sides, so they must carry the same value, and the magazine states the
// rule that way ("a character found on face 1 along the edge with face 2
// will have to be copied across into the box on face 2 on the other side
// of the boundary"). At a cube corner three faces meet and three cells
// coincide. The border ring is left empty in the printed puzzle, which is
// why one face on its own is not a hexadoku with 84 clues but a fragment.
//
// Rather than tabulate the twelve edge identifications and their eight
// orientations by hand, the faces are folded up: each one gets an origin
// and two axes in 3D, propagated from its neighbour in the development,
// and a cell is placed at O + r*V + c*U on the surface lattice of a cube
// of side 15. Cells that end up at the same lattice point are the same
// cell - the whole identification falls out of integer coordinates, in
// the right orientation, with nothing to get wrong per edge.
//
//	6*256 - 12*16 + 8 = 1352 cells   (12 edges of 16 shared, 8 corners
//	6*48  - 12      =  276 units      counted three times instead of two)
//
// The 288 units of the six grids collapse to 276 for the same reason: the
// boundary line of a face *is* the boundary line of its neighbour, one
// unit declared twice, so the dedup in addUnit merges the twelve of them.
package main

// side is the edge length of the cube in lattice steps: a face spans 16
// lattice points from 0 to 15 along each axis, and two faces meeting at a
// cube edge share the 16 points on it.
const side = maxSize - 1

type vec3 struct{ x, y, z int8 }

func (a vec3) add(b vec3) vec3 { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a vec3) sub(b vec3) vec3 { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a vec3) mul(s int8) vec3 { return vec3{a.x * s, a.y * s, a.z * s} }

func (a vec3) cross(b vec3) vec3 {
	return vec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}

// face is one grid of the development: where it sits on the page (in
// units of whole faces) and how it lies on the cube once folded. Cell
// (r, c) of the face is the lattice point o + v*r + u*c.
type face struct {
	netRow, netCol int
	o, u, v        vec3
	placed         bool
}

func (f *face) point(r, c int) vec3 {
	return f.o.add(f.v.mul(int8(r))).add(f.u.mul(int8(c)))
}

// normal is the outward normal of the folded face, u x v.
func (f *face) normal() vec3 { return f.u.cross(f.v) }

// hexadocubeNet folds the printed development. Face 0 is put down as the
// z = 15 face with u pointing along +x and v along +y; every other face
// is derived from an already placed neighbour by rotating a quarter turn
// about the edge they share. Crossing to the right turns u into -normal
// and leaves v alone, crossing downwards turns v into -normal and leaves
// u alone; the reverse directions undo that, and the origin always moves
// so that the two faces keep the shared line of 16 lattice points.
func hexadocubeNet() []face {
	faces := []face{
		{netRow: 1, netCol: 0}, {netRow: 1, netCol: 1},
		{netRow: 1, netCol: 2}, {netRow: 1, netCol: 3},
		{netRow: 0, netCol: 3}, {netRow: 2, netCol: 3},
	}
	faces[0].o = vec3{0, 0, side}
	faces[0].u = vec3{1, 0, 0}
	faces[0].v = vec3{0, 1, 0}
	faces[0].placed = true

	at := func(r, c int) *face {
		for i := range faces {
			if faces[i].netRow == r && faces[i].netCol == c {
				return &faces[i]
			}
		}
		return nil
	}
	for again := true; again; {
		again = false
		for i := range faces {
			f := &faces[i]
			if !f.placed {
				continue
			}
			n := f.normal()
			// (neighbour, its axes, its origin) for the four directions
			type step struct {
				dr, dc int
				u, v   vec3
				o      vec3
			}
			for _, s := range []step{
				{0, 1, n.mul(-1), f.v, f.o.add(f.u.mul(side))},
				{0, -1, n, f.v, f.o.sub(n.mul(side))},
				{1, 0, f.u, n.mul(-1), f.o.add(f.v.mul(side))},
				{-1, 0, f.u, n, f.o.sub(n.mul(side))},
			} {
				g := at(f.netRow+s.dr, f.netCol+s.dc)
				if g == nil || g.placed {
					continue
				}
				g.o, g.u, g.v = s.o, s.u, s.v
				g.placed = true
				again = true
			}
		}
	}
	return faces
}

// hexadocubeVariant builds the six-face layout. The development is the
// text raster: four faces wide, three tall, blanks where the net has no
// face. A cell on a cube edge is drawn twice, once on each face, so the
// raster has 6*256 = 1536 characters for 1352 cells - the file keeps the
// printed picture, and the solver sees the folded cube.
func hexadocubeVariant() *variant {
	const n, b = maxSize, 4
	v := newVariant("hexadocube", n, 4*n, 3*n)
	faces := hexadocubeNet()
	seen := make(map[vec3]int, 1352)
	for _, f := range faces {
		for r := 0; r < n; r++ {
			for c := 0; c < n; c++ {
				p := f.point(r, c)
				k, ok := seen[p]
				if !ok {
					k = v.ncells
					seen[p] = k
					v.pos[k] = int32((f.netRow*n+r)*v.width + f.netCol*n + c)
					v.ncells++
				}
				v.at[(f.netRow*n+r)*v.width+f.netCol*n+c] = int32(k + 1)
			}
		}
	}
	for _, f := range faces {
		v.addGrid(f.netRow*n, f.netCol*n, n, b)
	}
	v.finish()
	return v
}
