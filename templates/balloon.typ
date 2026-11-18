#import "@preview/tiaoma:0.3.0": code128

// --- Inputs ---
#let date-time = sys.inputs.at("datetime", default: "14-02-2026 10:00")
#let ticket-id = sys.inputs.at("ticket_id", default: "41")
#let problem-letter = sys.inputs.at("problem", default: "C")
#let uni-name = sys.inputs.at("team_name", default: "Eindhoven University of Technology")
#let team-number = sys.inputs.at("team_id", default: "1")
#let balloons-input = sys.inputs.at("balloons", default: "A,B,C,D,E,F,G,H,I,J,K,L")
#let delivered = sys.inputs.at("delivered", default: "A,B").split(",").map(s => s.trim()).filter(s => s != "")
#let in-delivery = sys.inputs.at("in_delivery", default: "C").split(",").map(s => s.trim()).filter(s => s != "")
#let is-first-solve = sys.inputs.at("first_solve", default: "false") == "true"
#let scan-url = sys.inputs.at("scan_url", default: "")
#let map-path = sys.inputs.at("map_path", default: "")
#let page-width-mm = float(sys.inputs.at("page_width_mm", default: "80"))

// --- Page setup ---
#set page(
  width: page-width-mm * 1mm,
  height: auto,
  margin: (x: 2mm, y: 6mm),
)
#set text(
  font: ("Arial", "Liberation Sans", "Helvetica", "DejaVu Sans"),
  size: 11pt,
  weight: "bold",
  fill: black,
)

#show: body => {
  set par(spacing: 0pt, leading: 0.5em)
  set block(above: 0pt, below: 0pt)
  body
}

// --- Helpers ---
#let h-line() = line(length: 100%, stroke: 1.2pt + black)

#let glyph-box-w = 20pt
#let glyph-box-h = 38pt

// `solved` filters out unsolved balloons before draw-balloon is called, so
// status is always "delivered" or "in-delivery".
#let draw-balloon(lbl, status: "delivered", is-current: false) = {
  let is-delivered = status == "delivered"
  let fill-color = if is-delivered { black } else { white }
  let text-color = if is-delivered { white } else { black }
  let border = if is-delivered { 0.8pt + black } else { 1.5pt + black }

  box(width: glyph-box-w, height: glyph-box-h)[
    #align(center)[
      #circle(radius: 10pt, fill: fill-color, stroke: border)[
        #set align(center + horizon)
        #text(fill: text-color, size: 9pt, weight: "bold")[#lbl]
      ]
      #v(-3pt)
      #if is-current [
        #v(2pt)
        #polygon(
          fill: black,
          (0pt, 0pt),
          (-4pt, 5pt),
          (4pt, 5pt),
        )
      ]
    ]
  ]
}

#let gap = 4pt
#let avail = (page-width-mm * 1mm - 4mm).pt()
#let per-row = calc.max(1, calc.floor((avail + gap.pt()) / (glyph-box-w.pt() + gap.pt())))

#let solved = (
  balloons-input.split(",").map(b => b.trim()).filter(b => b != "" and (b in delivered or b in in-delivery))
)

#let balloon-rows = (
  range(0, solved.len(), step: per-row).map(i => solved.slice(i, calc.min(i + per-row, solved.len())))
)

// --- Layout ---
#align(center)[
  #grid(
    columns: (1fr, 1fr),
    align(left)[#text(size: 8pt, weight: "regular")[#date-time]],
    align(right)[#text(size: 8pt, weight: "regular")[id: #ticket-id]],
  )

  #v(1mm)
  #h-line()
  #v(2mm)

  #if is-first-solve [
    #block(
      width: 100%,
      inset: (x: 3mm, y: 2mm),
      fill: black,
      radius: 2pt,
      text(
        size: 16pt,
        weight: "bold",
        fill: white,
        tracking: 2pt,
      )[★ FIRST SOLVE ★],
    )
    #v(1mm)
  ]

  #text(
    size: 110pt,
    weight: "bold",
    top-edge: "cap-height",
    bottom-edge: "baseline",
  )[#problem-letter]

  #v(2mm)
  #h-line()
  #v(2mm)

  #text(size: 13pt)[#uni-name]
  #v(2mm)
  #text(weight: "regular")[Location #team-number]

  #if map-path != "" [
    #v(2mm)
    #image(map-path)
  ]

  #v(2mm)
  #h-line()
  #v(2mm)

  #stack(
    dir: ttb,
    spacing: 3pt,
    ..balloon-rows.map(row => align(center, box(stack(
      dir: ltr,
      spacing: gap,
      ..row.map(b => {
        let status = if b in delivered { "delivered" } else { "in-delivery" }
        draw-balloon(b, status: status, is-current: b == problem-letter)
      }),
    )))),
  )

  #v(2mm)
  #h-line()

  #if scan-url != "" [
    #v(2mm)
    #code128(
      ticket-id,
    )
    #v(1mm)
    #text(size: 8pt, weight: "regular")[Scan to mark delivered]
  ]
]
