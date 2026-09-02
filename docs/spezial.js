/* Shared drawing helper for the Spezialrätsel pages.
 *
 * drawRaster() turns a puzzle exactly as it is stored in puzzles/ - one
 * line per raster row, a blank where no grid covers the position - into an
 * SVG. Everything the pages show is drawn from that same text, so a
 * diagram cannot drift away from the file it illustrates.
 */

const SVGNS = "http://www.w3.org/2000/svg";

function el(name, attrs, parent) {
  const n = document.createElementNS(SVGNS, name);
  for (const k in attrs) n.setAttribute(k, attrs[k]);
  if (parent) parent.appendChild(n);
  return n;
}

/* opts:
 *   rows      array of strings, ' ' = no cell, '.' = empty cell
 *   cell      pixel size of one cell
 *   box       box size in cells (4 for a hexadoku), 0 for none
 *   grids     [{r, c, n, color, label}] outlines to draw on top
 *   tint      (r, c, ch) -> fill colour or null
 *   glyphs    draw the clue characters (default true)
 *   pad       outer padding
 */
function drawRaster(opts) {
  const rows = opts.rows,
    cs = opts.cell || 14,
    box = opts.box === undefined ? 4 : opts.box,
    pad = opts.pad === undefined ? 10 : opts.pad,
    glyphs = opts.glyphs !== false,
    h = rows.length,
    w = Math.max(...rows.map((r) => r.length));

  const svg = el("svg", {
    viewBox: `0 0 ${w * cs + 2 * pad} ${h * cs + 2 * pad}`,
    width: w * cs + 2 * pad,
    role: "img",
  });
  if (opts.title) el("title", {}, svg).textContent = opts.title;
  const g = el("g", { transform: `translate(${pad},${pad})` }, svg);

  const has = (r, c) =>
    r >= 0 && r < h && c >= 0 && c < rows[r].length && rows[r][c] !== " ";

  // cell backgrounds
  for (let r = 0; r < h; r++) {
    for (let c = 0; c < rows[r].length; c++) {
      if (!has(r, c)) continue;
      const ch = rows[r][c];
      const fill = (opts.tint && opts.tint(r, c, ch)) || "var(--cell-bg)";
      el(
        "rect",
        {
          x: c * cs,
          y: r * cs,
          width: cs,
          height: cs,
          fill: fill,
          stroke: "var(--grid-line)",
          "stroke-width": 0.5,
        },
        g
      );
    }
  }

  // box rules: a segment wherever a box boundary separates two cells
  if (box > 0) {
    const line = (x1, y1, x2, y2) =>
      el(
        "line",
        {
          x1: x1,
          y1: y1,
          x2: x2,
          y2: y2,
          stroke: "var(--grid-heavy)",
          "stroke-width": 1.1,
          "stroke-linecap": "square",
        },
        g
      );
    for (let r = 0; r < h; r++) {
      for (let c = 0; c <= w; c++) {
        if (c % box === 0 && (has(r, c) || has(r, c - 1)))
          line(c * cs, r * cs, c * cs, (r + 1) * cs);
      }
    }
    for (let c = 0; c < w; c++) {
      for (let r = 0; r <= h; r++) {
        if (r % box === 0 && (has(r, c) || has(r - 1, c)))
          line(c * cs, r * cs, (c + 1) * cs, r * cs);
      }
    }
  }

  // clue characters
  if (glyphs) {
    for (let r = 0; r < h; r++) {
      for (let c = 0; c < rows[r].length; c++) {
        const ch = rows[r][c];
        if (ch === " " || ch === ".") continue;
        el(
          "text",
          {
            x: c * cs + cs / 2,
            y: r * cs + cs / 2,
            "text-anchor": "middle",
            "dominant-baseline": "central",
            "font-size": cs * 0.62,
            fill: "var(--ink)",
          },
          g
        ).textContent = ch;
      }
    }
  }

  // grid outlines on top
  for (const gr of opts.grids || []) {
    el(
      "rect",
      {
        x: gr.c * cs,
        y: gr.r * cs,
        width: gr.n * cs,
        height: gr.n * cs,
        fill: "none",
        stroke: gr.color,
        "stroke-width": 2.2,
      },
      g
    );
    if (gr.label)
      el(
        "text",
        {
          x: gr.c * cs + 4,
          y: gr.r * cs + 4,
          "text-anchor": "start",
          "dominant-baseline": "hanging",
          "font-size": cs * 0.9,
          "font-weight": 700,
          fill: gr.color,
        },
        g
      ).textContent = gr.label;
  }
  return svg;
}

/* Put an SVG into the element with the given id, replacing what is there. */
function mount(id, svg) {
  const host = document.getElementById(id);
  if (!host) return;
  host.textContent = "";
  host.appendChild(svg);
}

/* colour with an alpha suffix that works for both themes: the diagram
 * palette is solid, so tints are drawn as the colour at low opacity via a
 * separate rect fill-opacity is not available here - use colour-mix. */
function tint(varName, pct) {
  return `color-mix(in srgb, ${varName} ${pct}%, var(--cell-bg))`;
}
