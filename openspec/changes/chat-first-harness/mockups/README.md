# Mockups

These are the source files for the "Nacre chat-first" design canvas (https://claude.ai/artifact/KqZKiA9MbzBRJ71DNN8VyE).

- `canvas.json`: lists the artboards and where each sits on the canvas.
- `*.dc.html`: one file per artboard. Cover-screen artboards are 412×915 and unfolded ones are 840×740. `CoverPalette` is interactive; the `Ocean*` boards mount other artboards inside the Ocean palette.
- `m3e.css`: the shared Material 3 Expressive tokens (light and dark), components, and the palettes.
- `gen_palettes.py`: generates the palettes from HCT tonal palettes and checks their contrast (needs the `materialyoucolor` package).
- `design-brief.md`: the plan as it was reviewed (see ../review.md).

The artboards load the canvas runtime (`support.js`), so they render only inside the canvas, not as plain HTML files.
