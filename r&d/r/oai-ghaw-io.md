# The Complete Ebitengine Reference Guide: 2D Game & Tool Development in Go (2026)

## TL;DR
- **Ebitengine v2 (current stable v2.9.0, released October 8, 2025) is the mature, production-proven choice for 2D games and pixel-art tools in Go**: you implement one three-method `Game` interface (`Update`, `Draw`, `Layout`), and the same code ships to Windows/macOS/Linux/FreeBSD, browsers (WebAssembly), Android/iOS, and Nintendo Switch. Commercial hits like Odencat's *Fishing Paradiso* (1,000,000+ downloads on Google Play) and *Meg's Monster* prove it at scale.
- **The ecosystem is now deep enough to build any of your target styles without leaving Go**: ebitenui/furex for GUI, donburi/ark for ECS, stagehand/bamenn for scenes, resolv/cp for physics, go-tiled/ldtkgo for maps, quasilyte/pathing for A*, go-steamworks for Steam, and Kage (Go-syntax shaders) for effects. For asset creation, Aseprite ($19.99 one-time) plus free LibreSprite/Piskel/Pixelorama, Tiled/LDtk for maps, and CC0 libraries (Kenney, OpenGameArt, itch.io) cover the pipeline.
- **Biggest gotchas to plan around**: non-Windows/Wasm targets need CGo so cross-compilation is effectively impossible without native toolchains (Docker/CI per-OS is the practical answer); the Steamworks Go binding implements only a small slice of the SDK and needs amd64 (no macOS arm64); and Ebitengine has no visual editor or automatic Z-buffer — you sort draw order yourself.

## Key Findings

1. **The engine is deliberately tiny and stable.** The whole game is one interface. `Update` runs at a fixed tick rate (default 60 TPS); `Draw` runs per rendered frame; `Layout` maps the outside window size to a logical screen size. Per the official "Ebitengine in 2025" post, the project slowed to a single 2025 release (v2.9): "while we released minor version updates twice a year until 2024, this year, 2025, saw only one release: v2.9" — reflecting maturity, not abandonment.
2. **Pick libraries, not a monolith.** Ebitengine ships graphics/audio/input/shaders; everything else (ECS, UI, physics, tilemaps, pathing, scenes) is a well-maintained third-party package listed in `sedyh/awesome-ebitengine`.
3. **Style is a rendering-and-math choice, not an engine choice.** Top-down, isometric, side-scroller, and terminal-UI aesthetics are all just different coordinate transforms and draw orders on the same `Draw(screen)` canvas.
4. **Shipping is the hard part, not coding.** CGo, per-platform packaging, and Steam depot config are where most time goes; budget for CI runners on each OS.

## Details

### 1. Ebitengine core: the Game interface & loop

Every Ebitengine program implements:

```go
type Game interface {
    Update() error                              // logic, ~60 ticks/sec
    Draw(screen *ebiten.Image)                  // rendering, per frame
    Layout(outsideW, outsideH int) (int, int)   // logical screen size
}
```

Minimal program:

```go
package main

import (
    "log"
    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

type Game struct{}

func (g *Game) Update() error { return nil }
func (g *Game) Draw(screen *ebiten.Image) {
    ebitenutil.DebugPrint(screen, "Hello, World!")
}
func (g *Game) Layout(w, h int) (int, int) { return 320, 240 }

func main() {
    ebiten.SetWindowSize(640, 480)
    ebiten.SetWindowTitle("My Game")
    if err := ebiten.RunGame(&Game{}); err != nil {
        log.Fatal(err)
    }
}
```

Install: `go mod init mygame && go get github.com/hajimehoshi/ebiten/v2 && go run .`

**Loop architecture (diagram).** The engine drives:
```
RunGame(game)
  └─ main loop (locked to main OS thread)
       ├─ every tick  → Update()  [fixed timestep, default 1/60 s]
       │                  ├─ read input (inpututil deltas computed here)
       │                  ├─ advance simulation
       │                  └─ return err (non-nil quits)
       └─ every frame → Draw(screen)   [screen cleared each call]
                          └─ optional FinalScreenDrawer.DrawFinalScreen
   Layout(outsideW,outsideH) called as needed to compute logical size
```
Key rule: `Draw` clears the screen every frame, so redraw everything each time. `Update` is where all `inpututil` "just pressed" checks must run. Ticks and frames are decoupled — on a 144 Hz monitor `Draw` may run more often than `Update`.

**Screen management & scaling.** `Layout` returns your *logical* resolution (e.g. 320×240 for retro), and Ebitengine automatically scales it to the window with integer/nearest scaling if you set `screen.Fill` and draw at logical size. For hi-DPI multiply the layout by `ebiten.DeviceScaleFactor()`. `ebiten.SetWindowResizingMode`, `SetFullscreen`, and `SetVsyncEnabled` control the window.

**Rendering pipeline.** You draw `*ebiten.Image` onto `*ebiten.Image` via `screen.DrawImage(img, op)` where `op *ebiten.DrawImageOptions` holds a `GeoM` (geometry/affine matrix: Translate/Scale/Rotate) and `ColorM`/`ColorScale`. Ebitengine automatically batches successive draws that share source/target and uses an internal texture atlas, so drawing the same tile many times is cheap. There is **no automatic Z-buffer** — draw order is painter's-algorithm and you control it. (v2.9.0's headline feature was "Improved Rendering Quality for Vector Graphics.")

**Audio.** Create exactly one `audio.Context` (`audio.NewContext(44100)`). Decode with `vorbis`, `mp3`, or `wav` subpackages, wrap for looping with `audio.NewInfiniteLoop(stream, stream.Length())`, and create a `*audio.Player`. Note: for new code prefer the F32 (32-bit float) APIs (`NewPlayerF32`, `vorbis.DecodeF32`, `audio.NewInfiniteLoopF32`) added in v2.8 — Ebitengine is moving to float32 internally.

```go
ctx := audio.NewContext(44100)
s, _ := vorbis.DecodeF32(f)               // f is io.Reader
loop := audio.NewInfiniteLoopF32(s, s.Length())
p, _ := ctx.NewPlayerF32(loop)
p.SetVolume(0.5); p.Play()
```
For effects (delay, low-pass, pan, distortion) use `solarlune/resound`, which wraps streams as `io.ReadSeeker`.

**Kage shaders.** Ebitengine's shader language is **Kage** — Go syntax, compiled at runtime to GLSL/HLSL/MSL/Metal so one shader runs everywhere. Only fragment (pixel) shaders are supported. Entry point is `Fragment`:

```go
//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
    return imageSrc0UnsafeAt(srcPos) * color
}
```
Types: `float`, `vec2/3/4`, `ivec2/3/4`, `mat2/3/4` (column-major), arrays; no structs. Global variables must be `uniform`. Prefer **pixel mode** (`//kage:unit pixels`) for new code over legacy texel mode. Compile with `ebiten.NewShader([]byte(src))`, draw with `screen.DrawRectShader` / `DrawTrianglesShader`. Learning resources: `tinne26/kage-desk` (tutorials), `Zyko0/kage-shaders` (library), and quasilyte's shader article. Preview tools: `luluka`, `kageviewer`, `kagei`, and the kageland.com web playground.

**Camera systems.** A camera is just a `GeoM` translation (and optional scale/rotation) applied to every world-space draw. Libraries: `setanarut/kamera` (shake, lerp, zoom, rotation), `scarycoffee/ebiten-camera`, and `tinne26/mipix` (pixel-art-aware camera/layout). Manual pattern: `op.GeoM.Translate(-cam.X, -cam.Y)` before each world draw; keep UI drawn without the camera transform.

**Particle systems.** No built-in particle system; roll your own slice of structs (pos, vel, life, alpha) updated in `Update` and drawn as small images/rects in `Draw`, or use `gween` for tweening particle values. Vector primitives via `ebiten/v2/vector` (StrokeLine, DrawFilledCircle) or `quasilyte/ebitengine-graphics` / `erparts/go-shapes`.

**Tilemaps.** Two main routes (see §3 and §5): `lafriks/go-tiled` for Tiled `.tmx`, `solarlune/ldtkgo` for LDtk `.ldtk`. Ebitengine's own `examples/tiles` shows hand-rolled tilemap drawing batched per tile ID.

**Collision detection.** Options range from manual AABB (`image.Rectangle.Overlaps`) to `solarlune/resolv` (shapes + spatial grid), `jakecoffman/cp` (Chipmunk2D port, full rigid-body), and `oliverbestmann/box2d-go` (Box2D v3 port).

**Scene management.** `joelschutz/stagehand` (generic, typed state, fade/slide transitions, FSM director), `noppikinatta/bamenn` (simple), `mbrc12/prestige` (minimal). Pattern: each scene implements `Update/Draw/Layout(state)`, the manager delegates and handles transitions.

**ECS patterns.** `yottahmd/donburi` (archetype-based, feature-rich, math/transform/hierarchy/events helpers) is the most popular; alternatives `mlange-42/ark`, `unitoftime/ecs`, `marioolofo/go-gameengine-ecs`, `sedyh/mizu`, `x-hgg-x/goecs`. donburi query example:

```go
query := donburi.NewQuery(filter.Contains(Position, Velocity))
query.Each(world, func(e *donburi.Entry) {
    pos := Position.Get(e); vel := Velocity.Get(e)
    pos.X += vel.X; pos.Y += vel.Y
})
```
The `x-hgg-x` sokoban/arkanoid/space-invaders repos are complete ECS reference games.

### 2. Retro pixel asset creation (8-bit / 16-bit)

**Editors.**
- **Aseprite** ($19.99 one-time on Steam — App ID 431730, "Overwhelmingly Positive," 20,000+ reviews; the price rose from $14.99 to $19.99 on June 1 per Aseprite's own Steam news; also compilable free from source) — industry-standard animated sprite editor: layers+frames, indexed/RGBA/grayscale color modes (indexed palettes up to 256 colors), onion skinning, tilemaps, slices, reference layers, Pixel-Perfect freehand, Shading ink, CLI for pipeline automation, sprite-sheet + JSON export.
- **LibreSprite** (free, GPL fork of old Aseprite) — near-identical classic workflow; lacks post-fork features (notably tilemaps).
- **Piskel** (free, browser + desktop) — fastest zero-setup start; limited (web version lacks layers); volunteer maintenance mode.
- **Pixelorama** (free, open source, Godot-based) — layers, onion skinning, tilemaps (rectangular/isometric/hex), non-destructive effects, browser version; the strongest free option in 2026.
- **GraphicsGale** (free, Windows) — strong animation, popular in Japan.
- **GIMP** — general raster editor when you need more than pixel tools.
- **Lospec Pixel Editor** — community palette integration.

**Canvas sizing conventions:** 16×16 (NES-style), 32×32 (SNES-style), 64×64 (GBA-style). Use transparent background; use Indexed mode for strict palette discipline.

**Palette constraints (authenticity).** Hardcore retro = 4 colors (Game Boy palette); tight indie = 8–16/sprite; comfortable indie = 32–64 for the whole game. Download NES/SNES/Game Boy palettes from **Lospec** as `.hex`/`.gpl`, then in Aseprite `Window > Palettes > Load Palette`. Shading rule: never pure black shadows or pure white highlights; build a color ramp (dark → base → highlight → specular). Turn OFF anti-aliasing on pencil/line tools.

**Sprite sheets & animation.** In Aseprite, frames = time (horizontal), layers = depth (vertical). Export to `.png` + `.json` sprite sheet; load in Go with `setanarut/aseprite`, `SolarLune/goaseprite`, or `askeladdk/aseprite` (parse tags/slices/cels), and play back with `ganim8`, `setanarut/anim`, or `aseplayer`.

**Tile/level editors.** **Tiled** (TMX/JSON, free, orthogonal + isometric + hex) and **LDtk** ("Level Designer Toolkit", JSON, IntGrid, AutoLayers, Entities). Both load into Go (see §5).

**AI / procedural.** AI sprite generators exist (aispritegen, sprite-ai) — treat output as a starting point and clean up in a real editor. Procedural in-Go: `SolarLune/dngn` (random maps), `aldernero/sketchy` (generative art framework).

**Free asset sources (verify licenses).**
- **Kenney (kenney.nl / kenney.itch.io)** — 40,000+ CC0 assets (no attribution, commercial OK); the "All-in-1" bundle is confirmed on Kenney's itch.io page as "60,000+ game assets including 2D sprites, 3D models and more" for $19.95+. Best single starting point.
- **OpenGameArt.org** — decade-old archive; *filter by CC0*; home of the Liberated Pixel Cup (LPC) 32×32 top-down standard and the Universal LPC Spritesheet Generator (note LPC is dual CC-BY-SA 3.0 / GPLv3 — attribution + share-alike).
- **itch.io** — thousands of free packs; notable CC0 creators Pixel Frog, Ansimuz, 0x72 (e.g. "16×16 DungeonTileset II").
- License rule of thumb: CC0 = safe always; CC-BY = credit; CC-BY-SA = credit + share-alike derivatives.

### 3. Game graphical style quick references

**Top-down (Zelda clone).** Tile-based world with 16×16 or 32×32 tiles. Store the world as a 2D grid; camera follows the player by translating all world draws by `-player.X+screenW/2`. NPC interaction: check facing tile + `inpututil.IsKeyJustPressed` for an action key; inventory as a slice/struct rendered as UI on top (no camera transform). Movement uses grid or free pixel movement with AABB collision against a "solid" tile layer.

**Isometric (Shadowrun clone).** Store the map in plain "map coordinates" (map.x, map.y); convert only for rendering and input. Using tiles W×H (example 128×64, so half = 64×32):

- **Map → screen:** `screen.x = (map.x - map.y) * TILE_W_HALF`; `screen.y = (map.x + map.y) * TILE_H_HALF`
- **Screen → map:** `map.x = (screen.x/TILE_W_HALF + screen.y/TILE_H_HALF) / 2`; `map.y = (screen.y/TILE_H_HALF - screen.x/TILE_W_HALF) / 2`
- Simplified float-only screen→map: `map.x = screen.x/TILE_W + screen.y/TILE_H`; `map.y = screen.y/TILE_H - screen.x/TILE_W`

The iso tile's origin is its **top corner**; sprites usually draw from top-left, so subtract `TILE_W_HALF` from screen.x before drawing, and add `SCREEN_W_HALF` to center. **Depth sorting / tile stacking:** use the painter's algorithm — draw back-to-front, where `(map.x + map.y)` is the depth key (iterate the grid diagonally / in increasing sum). Within a stacked tile, draw bottom-to-top. For tall entities spanning tiles, assign each a Z = `(map.x+map.y)+height` and sort the draw list every frame (Ebitengine has no Z-buffer). Ebitengine ships an `examples/isometric` demo; `Flokey82/go_gens/gameisometric` is a working Go+Ebitengine iso experiment. Reference math: Clint Bellanger's "Isometric Tiles Math" (clintbellanger.net) — the same method Tiled uses; the formulas above are quoted from it and verified against its worked examples (tile (2,1) → screen (64,96) and back).

**Side-scroll (Mega Man clone).** Parallax: draw multiple background layers scrolled at fractions of the camera speed (`bg.X = -cam.X * 0.5`). Platformer physics: apply gravity to `vel.Y` each tick, integrate position, then resolve collisions axis-by-axis (horizontal pass, then vertical pass) so you can detect "on ground." Screen-based level transitions: when the player crosses a screen boundary, snap/lerp the camera to the next screen. Enemy patterns: simple FSMs or fixed movement scripts. Use `resolv` for collision (see below) or manual AABB.

**Terminal UI (The Oregon Trail).** Render a fixed-width monospace font onto the Ebitengine canvas with `text/v2` + a bitmap font (`hajimehoshi/bitmapfont`, `quasilyte/bitsweetfont`) or `tinne26/etxt`. Simulate a terminal by drawing a character grid, a status bar row, and choice menus (numbered options selected by keys). Add a CRT/scanline look with a Kage post-processing shader. Alternatively prototype purely in the terminal with `JoelOtter/termloop` (see §5) and port the render layer to Ebitengine later.

### 4. Tool graphical style quick references

**Terminal UI (map-navigation TUI).** Same canvas-text approach as above: a character grid model, cursor movement via arrow keys, panels drawn as boxes with box-drawing glyphs. `etk` (+ `messeji` text widgets, `kibodo` on-screen keyboard) provides a Go toolkit for GUI on Ebitengine. `liamg/darktile` is a real GPU-rendered terminal emulator built on Ebitengine — a strong reference for terminal rendering.

**Retro pixel GUI (Mario Paint-style / 16-bit filesystem browser).** Use `ebitenui` (retained-mode widgets: buttons, lists, windows, drag-and-drop, fully skinnable with your own 9-slice pixel art) or `furex` (flexbox layout — great if you think in CSS/React). To render a Unix filesystem as openable "boxes": model each folder/file as an entity with a rect; on click (`inpututil.IsMouseButtonJustPressed` + `CursorPosition`) open a child window/panel; use `ebiten-imgui` (Dear ImGui) for fast tool UIs. `ebitenui` requires Ebitengine ≥ v2.9.0 in current versions.

### 5. Useful Go packages for 2D game dev

- **go-sdl2 (`github.com/veandco/go-sdl2`)** — SDL2 bindings (window/render/audio/input/gamepad). *Not* an Ebitengine dependency — it's an alternative low-level path when you need raw SDL. Requires CGo + native SDL2 libs installed (`go get github.com/veandco/go-sdl2/sdl`, plus SDL2 dev packages). Use it only if Ebitengine's abstractions don't fit; most Ebitengine projects won't need it.
- **tile (`kelindar/tile`)** — fast grid/tile map with pathfinding and observers; quasilyte's benchmarks found it impressive on allocations for pathfinding grids. Good for large logical tile grids separate from rendering.
- **grid (`s0rg/grid`)** — generic 2D grid with pathfinding, ray/shadow casting, line-of-sight. `greenthepear/egriden` is a grid-based game framework.
- **termloop (`github.com/JoelOtter/termloop`)** — terminal game engine on Termbox (pure Go, easy cross-compile). Model: `Game` → `Level` (of `Cell`s) → `Entity` implementing `Draw(*Screen)` + `Tick(Event)`. Install `go get -u github.com/JoelOtter/termloop`. Great for TUI prototypes and Oregon-Trail-style tools; separate from Ebitengine.
```go
game := tl.NewGame()
level := tl.NewBaseLevel(tl.Cell{Bg: tl.ColorGreen, Fg: tl.ColorBlack, Ch: 'v'})
level.AddEntity(tl.NewRectangle(10, 10, 50, 20, tl.ColorBlue))
game.Screen().SetLevel(level)
game.Start()
```
- **pi (`github.com/elgopher/pi`)** — Pico-8-inspired retro engine powered by Ebitengine; deliberately constrained (low res like 128×128, 64 colors, CPU rendering) to help you finish games. Uses `piebiten` backend. Minimal:
```go
pi.SetScreenSize(47, 9)
pi.Draw = func() { picofont.Print("HELLO WORLD", 2, 2) }
piebiten.Run()
```
Related: `drpaneas/pigo8` (PIGO8) offers a PICO-8-style API (`spr()`, `btn()`, `map()`) in Go over Ebitengine, and can import `.p8` cartridge assets (you must own PICO-8).
- **nano** — a lightweight framework/utility in the Go game space (least-documented of this list; treat as experimental and verify maintenance before adopting).
- **ganim8 (`github.com/yohamta/ganim8`, current `/v3`; also mirrored as `yottahmd/ganim8`)** — sprite animation inspired by Love2D's anim8. Build a grid, slice frames, animate:
```go
g32 := ganim8.NewGrid(32, 32, 1024, 1024)
anim := ganim8.New(monsterImg, g32.Frames("1-5", 5), 100*time.Millisecond)
// Update: anim.Update()   Draw: anim.Draw(screen, ganim8.DrawOpts(x, y, 0, 1, 1, 0.5, 0.5))
```
- **furex (`github.com/yottahmd/furex`, published as furex-ui)** — flexbox UI framework; you supply rendering, it handles layout + button/touch events. Install `go get github.com/yottahmd/furex/v2`. Good for responsive HUDs.
- **ebitenui (`github.com/ebitenui/ebitenui`)** — full retained-mode widget library (buttons, lists, combo boxes, windows, tooltips, drag-and-drop), fully skinnable. Docs at ebitenui.github.io; demo `go run github.com/ebitenui/ebitenui/_examples/demo@latest`. Requires Ebitengine ≥ v2.9.0. The default choice for complex in-game menus and tools.
- **pathing (`github.com/quasilyte/pathing`)** — very fast, zero-allocation grid A* + greedy BFS, built for real-time RTS (used in the *Roboden* game). Restrictive (few tile kinds, bounded map) but extremely efficient. Alternatives: `beefsack/go-astar` (classic A*), `SolarLune/paths` (video-game-oriented), `s0rg/grid`.

**resolv collision (`github.com/solarlune/resolv`, current v0.8.0, Go 1.20+).** Note a **major recent API rework** (Object → Shape): older tutorials using `resolv.NewObject`/`object.Check()` are legacy. Current API is shape-based: `resolv.NewRectangle(x,y,w,h)` (returns a `*ConvexPolygon`), `resolv.NewCircle(x,y,r)`, added to a `resolv.NewSpace(w, h, cellW, cellH)` (a spatial grid of cells for broad-phase). Guidance: make cell size ≈ a normal object and move objects at most one cell per check to avoid tunneling. Test directly with `shape.Intersection(other) (intersection, ok)`, or against many via `shape.IntersectionTest(resolv.IntersectionTestSettings{TestAgainst: shape.SelectTouchingCells(1).FilterShapes(), OnIntersect: ...})`; the returned `IntersectionSet` carries the MTV (minimum translation vector) you use to push shapes apart. `LineTest` handles ray/ground probes. For platformers, do horizontal then vertical passes and resolve with the MTV; the repo's `examples` platformer is the in-depth reference.

```go
space := resolv.NewSpace(640, 480, 16, 16)
player := resolv.NewRectangle(200, 100, 32, 32)
wall   := resolv.NewRectangle(200, 200, 64, 16)
space.Add(player, wall)

player.IntersectionTest(resolv.IntersectionTestSettings{
    TestAgainst: player.SelectTouchingCells(1).FilterShapes(),
    OnIntersect: func(set resolv.IntersectionSet, i, max int) bool {
        // use set's MTV to push the player out of the wall
        return true
    },
})
```

### 6. Capturing keyboard & mouse input

- **Held state (call anytime):** `ebiten.IsKeyPressed(ebiten.KeySpace)`, `ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)`, `ebiten.CursorPosition() (x, y)`, `ebiten.Wheel() (dx, dy)`.
- **Edge detection (call in `Update`):** `inpututil.IsKeyJustPressed`, `IsKeyJustReleased`, `IsMouseButtonJustPressed/Released`, `KeyPressDuration`. `inpututil` computes per-tick deltas, so these must be called in `Update`, not `Draw`.
- **Enumerate:** `inpututil.AppendJustPressedKeys(nil)`, `AppendPressedKeys`.
- **Touch (mobile):** `ebiten.TouchPosition(id)`, `inpututil.AppendJustPressedTouchIDs(nil)`; multi-touch supported (one ID per press).
- **Gamepad:** `ebiten.IsStandardGamepadButtonPressed`, `StandardGamepadAxisValue`, `inpututil.AppendJustConnectedGamepadIDs`; standard layout maps Xbox/PlayStation controllers uniformly.
- **Action mapping:** `quasilyte/ebitengine-input` gives Godot-style action maps (bind multiple physical keys/buttons/axes to one action, auto keyboard/gamepad switching, input simulation for tests).

```go
func (g *Game) Update() error {
    if inpututil.IsKeyJustPressed(ebiten.KeySpace) { g.jump() }
    if ebiten.IsKeyPressed(ebiten.KeyRight)        { g.x += 2 }
    if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
        x, y := ebiten.CursorPosition(); g.click(x, y)
    }
    return nil
}
```

### 7. Saving game state

Go's stdlib covers serialization: `encoding/json` (human-readable, versionable, best default), `encoding/gob` (compact Go-native binary), or protobuf (`google.golang.org/protobuf`) for schema evolution/cross-language. Pattern: marshal a `SaveData` struct → write to disk.

**Cross-platform save locations** are the real problem. Use **`quasilyte/gdata/v2`**, a platform-agnostic key-value store (like localStorage) that picks the conventional app-data folder per OS and even works on Wasm:

```go
m, _ := gdata.Open(gdata.Config{AppName: "mygame"})
m.SaveObjectProp("core", "save.data", jsonBytes)
b, _ := m.LoadObjectProp("core", "save.data")
```
Otherwise resolve `os.UserConfigDir()` (→ `%AppData%` on Windows, `~/Library/Application Support` on macOS, `~/.config` on Linux). For browsers there's no filesystem — persist to `localStorage` (gdata handles this). **Save slots**: use distinct filenames/keys (`slot1.json`…). **Auto-save**: serialize on a timer or at checkpoints in `Update`; write to a temp file then rename to avoid corruption. On consoles (Switch/Xbox), plain JSON-to-folder won't work — use the platform save API.

### 8. Building for multiplatform

- **Requirements:** Go ≥ 1.23 (per current go.mod).
- **Desktop native:** `go build .` — on **Windows Ebitengine is pure Go (no C compiler needed)**; macOS/Linux/FreeBSD link native graphics/audio libs via CGo.
- **Cross-compilation reality:** Only **Windows** and **Wasm** targets cross-compile cleanly. All other targets need CGo, which makes cross-compiling "almost impossible" (Ebitengine FAQ, referencing Go issue #18296) — build each OS on its own machine/CI runner or in an OS-matched Docker container.
- **WebAssembly:** `env GOOS=js GOARCH=wasm go build -o game.wasm .`, copy `wasm_exec.js` from your Go install, host with an HTML loader (`WebAssembly.instantiateStreaming`). For local testing: `go run github.com/hajimehoshi/wasmserve@latest ./path` then open `localhost:8080`. Embed via `<iframe>` (recommended). Note browsers block audio until a user gesture.
- **Mobile (`ebitenmobile`):**
  - Android: `ebitenmobile bind -target android -androidapi 23 -javapkg com.you.pkg -o out.aar -v ./yourpkg` → import the AAR.
  - iOS: `ebitenmobile bind -target ios -o Out.xcframework -v ./yourpkg`.
- **Consoles:** Nintendo Switch supported (`-tags` builds, NDA'd SDK); limited Xbox (CGo) and PlayStation 5 (`-tags=playstation5`) support exist but are gated.
- **Docker builds:** use OS-matched images (e.g. a Linux image with X11/ALSA dev headers) to produce reproducible CGo builds in CI; a Windows or Wasm target can be produced from any host.

### 9. Building for Steam

Use **`github.com/hajimehoshi/go-steamworks`** (depends on `ebitengine/purego`; go.mod targets Go 1.23). It's a thin binding — **most of the Steamworks API is not yet implemented** (language, achievements/stats, restart-if-necessary, and a subset are available). Architecture must be **amd64**; **macOS arm64 (Apple Silicon) is unsupported** by the Steamworks SDK path.

Initialization (typically in `init()`):
```go
const appID = 480 // replace with your AppID

func init() {
    if steamworks.RestartAppIfNecessary(appID) { os.Exit(1) }
    if err := steamworks.Init(); err != nil {
        panic(fmt.Sprintf("steamworks.Init failed: %v", err))
    }
}
```
You must ship the Steamworks dynamic libraries alongside your binary (Windows `steam_api64.dll`; Linux `libsteam_api.so`; macOS `libsteam_api.dylib`) — these come from the SDK's `redistribution_bin` and are governed by Valve's Steamworks SDK Access Agreement. Read the user's language via `steamworks.SteamApps().GetCurrentGameLanguage()`.

**Achievements/overlay:** Achievements are configured in the Steamworks App Admin backend and unlocked in code via the Stats & Achievements API (`SetAchievement` → `StoreStats`). Because the Go binding is partial, verify the specific call exists in the current binding version or contribute it; Valve's own guide notes basic achievements take "under 10 lines of code" once the SDK is wired. The Steam overlay generally works when the SDK is initialized and the game runs through the Steam client.

**Packaging / SteamPipe:** enroll in the Steamworks Partner Program (one-time fee per app), create your app, configure **depots** (per-platform content), and upload builds with the **SteamPipe** `steamcmd`/`ContentBuilder` tooling using a depot build `.vdf` script, then set the build live on a branch. Odencat has shipped multiple Ebitengine games to Steam (*Bear's Restaurant*, *Meg's Monster*, *Mr. Saitou*, *Snowman Story*), confirming the pipeline is viable.

### 10. Cheatsheet (API quick reference)

```
LIFECYCLE
  ebiten.RunGame(game)                      // start loop
  ebiten.RunGameWithOptions(game, opts)     // with GraphicsLibrary etc.
  ebiten.SetWindowSize(w,h) / SetWindowTitle(s) / SetFullscreen(b)
  ebiten.SetTPS(n)  // change tick rate
  ebiten.ActualTPS() / ActualFPS()

DRAWING
  screen.Fill(color)                        // clear
  screen.DrawImage(img, op)                 // op *ebiten.DrawImageOptions
  op.GeoM.Translate(x,y) / Scale(sx,sy) / Rotate(theta)
  op.ColorScale.Scale(r,g,b,a) / op.Filter = ebiten.FilterNearest
  ebitenutil.DebugPrint(screen, "…")
  vector.DrawFilledRect / StrokeLine / DrawFilledCircle
  ebiten.NewImage(w,h) / ebiten.NewImageFromImage(img)

INPUT (held)      ebiten.IsKeyPressed(ebiten.KeyX)
INPUT (edge)      inpututil.IsKeyJustPressed(ebiten.KeyX)  // in Update()
MOUSE             ebiten.CursorPosition(); ebiten.Wheel()
                  ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
TOUCH             ebiten.TouchPosition(id)
GAMEPAD           ebiten.IsStandardGamepadButtonPressed(id, btn)

KEY CONSTANTS     KeyA…KeyZ, Key0…Key9, KeySpace, KeyEnter, KeyEscape,
                  KeyArrowUp/Down/Left/Right, KeyShift, KeyControl
MOUSE CONSTANTS   MouseButtonLeft / Right / Middle

SHADERS  s,_ := ebiten.NewShader(src); screen.DrawRectShader(w,h,s,op)

COORDINATES  logical origin (0,0) = top-left; +x right, +y down.
```

### 11. Real games & projects built with Ebitengine

**Commercial / shipped:**
- **Odencat** (Hajime Hoshi is CTO): *Fishing Paradiso* (1,000,000+ downloads, 4.6★/31.5K reviews on Google Play), *Bear's Restaurant* (Steam + Switch), *Meg's Monster* (Switch, praised as a top indie), *Mousebusters*, *Snowman Story* (Google Play Indie Games Festival Top 10, 2020), *Mr. Saitou*, *Rakuen* (Switch, w/ Morizora Studio).
- **quasilyte's *Roboden*** — indirect-control RTS about robot colonies (open source, Steam).
- **quasilyte's *Decipherism*** — cipher puzzle game (Game Off 2022 award).
- **aaaaxy (divVerent)** — non-Euclidean 2D puzzle platformer (on Steam).

**Open-source games & jam entries (great to read):**
- ECS reference set: `x-hgg-x/sokoban-go`, `arkanoid-go`, `space-invaders-go`.
- `TheTophatDemon/Feta-Feles-Remastered` (bullet hell), `ketMix/retromancer` (action-adventure), `quasilyte/sinecord` (music puzzle), `oddstream/gosol` (solitaire engine), `tslocum/monovania` (metroidvania), `tslocum/citylimits` (city-builder).
- Emulators: `pokemium/worldwide` (Game Boy Color), and a GBA emulator.
- The **Ebitengine Game Jam** (annual, June) and **Holiday Hack** produce dozens of entries yearly; browse the itch.io "Made with Ebitengine" collection and the official Showcase page.

### 12–13. Concept diagrams (described)

**Game loop** (see §1). **Rendering pipeline:** world objects → apply camera `GeoM` → `DrawImage` onto offscreen/screen → automatic batching+atlas → optional `FinalScreenDrawer`/post shader → present. **Coordinate systems:** screen space (top-left origin, +y down); world space (camera offset); grid/tile space (÷ tile size); isometric map space (formulas in §3). **Scene graph:** `SceneManager` → active `Scene` → entities (or ECS `World` → systems). **Input flow:** OS event → Ebitengine internal state → (in `Update`) `inpututil` computes just-pressed/released deltas → your action mapping → game logic.

## Recommendations

**Stage 1 — Prototype (week 1).** Start from the official examples (`go run github.com/hajimehoshi/ebiten/v2/examples/snake@latest`, etc.). Build one screen with the raw `Game` interface, `inpututil` input, and `ebitenutil.DebugPrint`. Do NOT add ECS or scene libs yet. Threshold to advance: you have a controllable sprite with collision.

**Stage 2 — Structure (weeks 2–4).** Add `stagehand` for scenes and, only if your entity count/interactions justify it, `donburi` for ECS. Add `ebitenui` (menus/tools) or `furex` (HUD). Pick your map tool now — **Tiled + go-tiled** for orthogonal top-down/side-scroll, **LDtk + ldtkgo** for richer level design; use the isometric formulas in §3 if going iso. Add `resolv` (v0.8.0; note its recent Object→Shape API rework — ignore pre-v0.8 tutorials) or manual AABB.

**Stage 3 — Content & polish.** Lock a palette from Lospec before drawing; create sprites in Aseprite (or free Pixelorama/LibreSprite), animate with `ganim8`. Add audio via the F32 audio APIs + `resound` effects. Add Kage shaders for CRT/transition/hit-flash effects (learn from kage-desk). Wire saves through `quasilyte/gdata` from the start so you never hardcode paths.

**Stage 4 — Ship.** Stand up **per-OS CI runners** (GitHub Actions matrix) because CGo blocks cross-compilation for macOS/Linux; produce Windows and Wasm from any host. For Steam: join the Partner Program early, integrate `go-steamworks` behind a build tag (so non-Steam builds still compile), target amd64, and script SteamPipe depots. Test the Wasm build in an iframe.

**Change-your-plan thresholds:** If you need heavy rigid-body physics (ragdolls, joints), jump from `resolv` to `jakecoffman/cp` or `box2d-go`. If pathfinding shows up in profiles with many units, switch to `quasilyte/pathing`. If you want a fantasy-console constraint to actually finish, adopt `pi`/`pigo8` instead of raw Ebitengine. If you need Apple Silicon Steam builds, know the Steamworks path currently blocks arm64 — plan an amd64 build (Rosetta) accordingly.

## Caveats
- **Version currency:** Current stable is v2.9.0 (released October 8, 2025), whose headline feature was improved vector-graphics rendering quality; v2.8 introduced float32 audio + Kage custom vertex attributes. Confirm exact minor version and any newer changes at `pkg.go.dev/github.com/hajimehoshi/ebiten/v2` before pinning.
- **The user's fork** (`github.com/scottdef/ebiten`) tracks upstream `hajimehoshi/ebiten`; all APIs here reference upstream. Rebase the fork on upstream tags to stay current, and import paths remain `github.com/hajimehoshi/ebiten/v2` unless you deliberately replace the module.
- **CGo/cross-compile** is the single biggest shipping constraint — verified against Ebitengine's own FAQ (only Windows/Wasm cross-compile).
- **go-steamworks is partial** — do not assume any given Steamworks call is bound; check the source or contribute it.
- **`nano`** is the least-documented package in the request; I could not confirm an authoritative, actively maintained "nano" retro-game package tied to Ebitengine — verify its identity/maintenance before adopting, and prefer `pi`/`pigo8` for the fantasy-console niche.
- **AI asset generators** vary in license and quality; confirm commercial-use terms of any generated art.
- **Isometric depth-sorting** beyond simple `(x+y)` ordering (overlapping tall sprites) may require topological sort or layer splitting; the simple key works for standard floor/wall tiles.
- **Fishing Paradiso download figure:** the store-verifiable number is 1,000,000+ (Google Play). Some secondary sources cite higher figures, but these are not corroborated by store listings.