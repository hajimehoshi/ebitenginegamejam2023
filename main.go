// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023 Hajime Hoshi

package main

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	screenWidth  = 1920
	screenHeight = 1080
)

//go:embed RobotoCondensed-Bold.ttf
var roboto []byte

var robotoTT *opentype.Font

func init() {
	tt, err := opentype.Parse(roboto)
	if err != nil {
		panic(err)
	}
	robotoTT = tt
}

var titleFace font.Face

func init() {
	f, err := opentype.NewFace(robotoTT, &opentype.FaceOptions{
		Size:    256,
		DPI:     72,
		Hinting: font.HintingVertical,
	})
	if err != nil {
		panic(err)
	}
	titleFace = text.FaceWithLineHeight(f, 256)
}

var uiFace font.Face

func init() {
	f, err := opentype.NewFace(robotoTT, &opentype.FaceOptions{
		Size:    64,
		DPI:     72,
		Hinting: font.HintingVertical,
	})
	if err != nil {
		panic(err)
	}
	uiFace = f
}

var itemFace font.Face

func init() {
	f, err := opentype.NewFace(robotoTT, &opentype.FaceOptions{
		Size:    96,
		DPI:     72,
		Hinting: font.HintingVertical,
	})
	if err != nil {
		panic(err)
	}
	itemFace = f
}

type Phase int

const (
	PhaseTitle Phase = iota
	PhaseGame
	PhaseGameOver
)

const (
	maxRecoveryTime = 30
	maxDamageTime   = 5
	initPlayerLife  = 3
	maxPlayerLife   = 6
)

type Game struct {
	phase Phase

	score        int
	items        [3][4]*Item
	coolTime     int
	bugID        int
	playerLife   int
	recoveryTime int
	damageTime   int
}

func (g *Game) Update() error {
	switch g.phase {
	case PhaseTitle:
		return g.updateTitle()
	case PhaseGame:
		return g.updateGame()
	case PhaseGameOver:
		return g.updateGameOver()
	}
	return nil
}

func (g *Game) updateTitle() error {
	if justPressed() {
		g.initGame()
		g.phase = PhaseGame
	}
	return nil
}

func (g *Game) initGame() {
	g.score = 0
	for j := range g.items {
		for i := range g.items[j] {
			g.items[j][i] = nil
		}
	}
	g.coolTime = 0
	g.bugID = 0
	g.playerLife = initPlayerLife
	g.recoveryTime = 0
	g.damageTime = 0
}

func (g *Game) updateGame() error {
	if g.recoveryTime > 0 {
		g.recoveryTime--
	}
	if g.damageTime > 0 {
		g.damageTime--
	}

	if g.coolTime > 0 {
		g.coolTime--
	}

	if g.coolTime == 0 {
		j := rand.Intn(len(g.items))
		i := rand.Intn(len(g.items[j]))
		if g.items[j][i] == nil {
			width := (screenWidth - 128) / len(g.items[j])
			height := (screenHeight - 128 - 64) / len(g.items)
			x := i*width + width/2 + 128/2
			y := j*height + height/2 + 128/2 + 64

			log10 := math.Log(10)

			var recovery bool
			if g.score >= 100 {
				switch {
				case g.playerLife < 2:
					recovery = rand.Intn(3) == 0
				case g.playerLife < 4:
					recovery = rand.Intn(5) == 0
				case g.playerLife < maxPlayerLife:
					recovery = rand.Intn(10) == 0
				}
			}
			if recovery {
				var label string
				switch rand.Intn(2) {
				case 0:
					label = "CONTRI-\nBUTION"
				case 1:
					label = "SPONSORING"
				}

				g.items[j][i] = NewItem(label, x, y, true, 400, 0)
			} else {
				g.bugID++
				id := g.bugID
				var label string
				switch rand.Intn(2) {
				case 0:
					label = fmt.Sprintf("BUG\n#%d", id)
				case 1:
					label = fmt.Sprintf("FEATURE\nREQUEST\n#%d", id)
				}

				// lifetime:
				//   star 0   -> 400 [ticks]
				//   star 100 -> 300 [ticks]
				//   star 1k  -> 200 [ticks]
				//   star 10k -> 100 [ticks]
				lifetime := 100 * (5 - (math.Log(float64(g.score)) / log10))
				lifetime = min(400, max(100, lifetime))

				// base score:
				//   star 0    -> 10
				//   star 1k   -> 10 * √10
				//   star 10k  -> 100
				//   star 100k -> 100 * √10
				//baseScore := 10 * (math.Log(float64(g.score))/log10 - 1)
				baseScore := math.Sqrt(float64(g.score))
				baseScore = max(10, baseScore)

				g.items[j][i] = NewItem(label, x, y, false, int(lifetime), int(baseScore))
			}

			// cool time:
			//   star 0    -> 60-120 [ticks]
			//   star 100  -> 45-90 [ticks]
			//   star 1k   -> 30-60 [ticks]
			//   star 10k  -> 15-30 [ticks]
			//   star 100k -> 5-10 [ticks]
			coolTime := 15 * (5 - (math.Log(float64(g.score)) / log10))
			coolTime = min(60, max(5, coolTime))
			g.coolTime = int(coolTime) + rand.Intn(int(coolTime))
		}
	}

	for j := range g.items {
		for i := range g.items[j] {
			it := g.items[j][i]
			if it == nil {
				continue
			}
			it.Update()
			if it.Resolved() {
				g.score += it.Score()
				if it.recovery {
					g.playerLife++
					g.recoveryTime = maxRecoveryTime
				}
				g.items[j][i] = nil
			} else if it.Alive() {
				if it.hovered {
					ebiten.SetCursorShape(ebiten.CursorShapePointer)
				} else {
					ebiten.SetCursorShape(ebiten.CursorShapeDefault)
				}
			} else {
				g.items[j][i] = nil
				g.damageTime = maxDamageTime
				g.playerLife--
				if g.playerLife <= 0 {
					g.phase = PhaseGameOver
				}
			}
		}
	}
	return nil
}

func (g *Game) updateGameOver() error {
	if g.damageTime > 0 {
		g.damageTime--
	}

	if justPressed() {
		g.phase = PhaseTitle
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	switch g.phase {
	case PhaseTitle:
		g.drawTitle(screen)
	case PhaseGame:
		g.drawGame(screen)
	case PhaseGameOver:
		g.drawGame(screen)
		g.drawGameOver(screen)
	}
}

func (g *Game) drawTitle(screen *ebiten.Image) {
	lines := []string{"GAME", "ENGINE", "DEVELOPMENT", "SIMULATOR"}
	m := titleFace.Metrics()
	for i, line := range lines {
		a := font.MeasureString(titleFace, line)
		x := (screenWidth - fixedToFloat64(a)) / 2
		y := (screenHeight - fixedToFloat64(m.Height)*float64(len(lines))) / 2
		y += fixedToFloat64(m.Height*fixed.Int26_6(i) + m.Ascent)

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		text.DrawWithOptions(screen, line, titleFace, op)
	}
}

func (g *Game) drawGame(screen *ebiten.Image) {
	// Draw the score.
	{
		txt := fmt.Sprintf("GitHub Stars: %d", g.score)

		x := 32.0
		y := 32 + fixedToFloat64(uiFace.Metrics().Ascent)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		text.DrawWithOptions(screen, txt, uiFace, op)
	}

	// Draw the life.
	{
		txt := fmt.Sprintf("Life: %d", g.playerLife)
		a := font.MeasureString(uiFace, txt)
		x := screenWidth - 32.0 - fixedToFloat64(a)
		y := 32 + fixedToFloat64(uiFace.Metrics().Ascent)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		text.DrawWithOptions(screen, txt, uiFace, op)
	}

	for j := range g.items {
		for i := range g.items[j] {
			it := g.items[j][i]
			if it == nil {
				continue
			}
			it.Draw(screen)
		}
	}

	if g.recoveryTime > 0 {
		a := byte(0xff * float64(g.recoveryTime) / maxRecoveryTime / 2)
		clr := color.RGBA{0, a, 0, a}
		vector.DrawFilledRect(screen, 0, 0, screenWidth, screenHeight, clr, false)
	} else if g.damageTime > 0 {
		a := byte(0xff * float64(g.damageTime) / maxDamageTime / 2)
		clr := color.RGBA{a, 0, 0, a}
		vector.DrawFilledRect(screen, 0, 0, screenWidth, screenHeight, clr, false)
	}
}

func (g *Game) drawGameOver(screen *ebiten.Image) {
	clr := color.RGBA{0, 0, 0, 0x80}
	vector.DrawFilledRect(screen, 0, 0, screenWidth, screenHeight, clr, false)

	stars := fmt.Sprintf("%d STARS", g.score)
	if g.score == 1 {
		stars = stars[:len(stars)-1] // Remove 's' for plural.
	}
	lines := []string{"GAME OVER", "YOU GOT", stars}
	m := titleFace.Metrics()
	for i, line := range lines {
		a := font.MeasureString(titleFace, line)
		x := (screenWidth - fixedToFloat64(a)) / 2
		y := (screenHeight - fixedToFloat64(m.Height)*float64(len(lines))) / 2
		y += fixedToFloat64(m.Height*fixed.Int26_6(i) + m.Ascent)

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		text.DrawWithOptions(screen, line, titleFace, op)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(960, 540)
	ebiten.SetWindowTitle("Game Engine Development Simulator")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeOnlyFullscreenEnabled)
	if err := ebiten.RunGame(&Game{}); err != nil {
		panic(err)
	}
}

func fixedToFloat64(x fixed.Int26_6) float64 {
	return float64(x>>6) + float64(x&(1<<6+1))/(1<<6)
}

type Item struct {
	label    string
	centerX  int
	centerY  int
	recovery bool

	hovered  bool
	resolved bool

	initLifetime int
	lifetime     int
	baseScore    int
}

func NewItem(label string, centerX, centerY int, recovery bool, lifetime int, baseScore int) *Item {
	return &Item{
		label:        label,
		centerX:      centerX,
		centerY:      centerY,
		recovery:     recovery,
		initLifetime: lifetime,
		lifetime:     lifetime,
		baseScore:    baseScore,
	}
}

func (i *Item) Bounds() image.Rectangle {
	lines := strings.Split(i.label, "\n")
	if len(lines) == 0 {
		return image.Rectangle{}
	}

	var width fixed.Int26_6
	for _, line := range lines {
		a := font.MeasureString(itemFace, line)
		if width < a {
			width = a
		}
	}

	m := itemFace.Metrics()
	height := m.Ascent + m.Descent + m.Height*fixed.Int26_6(len(lines)-1)

	minX := i.centerX - (width / 2).Ceil()
	maxX := i.centerX + (width / 2).Ceil()
	minY := i.centerY - (height / 2).Ceil()
	maxY := i.centerY + (height / 2).Ceil()
	return image.Rect(minX, minY, maxX, maxY)
}

func (i *Item) Update() {
	if i.lifetime > 0 {
		i.lifetime--
	}

	touchIDs := inpututil.AppendJustPressedTouchIDs(nil)
	for _, id := range touchIDs {
		i.hovered = image.Pt(ebiten.TouchPosition(id)).In(i.Bounds())
	}
	if len(touchIDs) == 0 {
		i.hovered = image.Pt(ebiten.CursorPosition()).In(i.Bounds())
	}
	if i.hovered && justPressed() {
		i.resolved = true
	}
}

func (i *Item) Resolved() bool {
	return i.resolved
}

func (i *Item) Alive() bool {
	return i.lifetime > 0
}

func (i *Item) Score() int {
	return int(float64(i.baseScore) * math.Sqrt(float64(i.lifetime)/float64(i.initLifetime)))
}

func (it *Item) Draw(screen *ebiten.Image) {
	minY := it.Bounds().Min.Y

	for i, line := range strings.Split(it.label, "\n") {
		width := font.MeasureString(itemFace, line)

		m := itemFace.Metrics()
		x := float64(it.centerX) - fixedToFloat64(width)/2
		y := float64(minY) + fixedToFloat64(m.Height)*float64(i) + fixedToFloat64(m.Ascent)

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		if it.hovered {
			op.ColorScale.ScaleWithColor(color.RGBA{0xff, 0xff, 0, 0xff})
		} else if it.recovery {
			op.ColorScale.ScaleWithColor(color.RGBA{0x80, 0xff, 0x80, 0xff})
			alpha := float32(it.lifetime) / float32(it.initLifetime)
			op.ColorScale.ScaleAlpha(alpha)
		} else {
			v := byte(0xff * float32(it.lifetime) / float32(it.initLifetime))
			op.ColorScale.ScaleWithColor(color.RGBA{0xff, v, v, 0xff})
		}
		text.DrawWithOptions(screen, line, itemFace, op)
	}
}

func max(x, y float64) float64 {
	if x > y {
		return x
	}
	return y
}

func min(x, y float64) float64 {
	if x < y {
		return x
	}
	return y
}

func justPressed() bool {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return true
	}
	if len(inpututil.AppendJustPressedTouchIDs(nil)) > 0 {
		return true
	}
	return false
}
