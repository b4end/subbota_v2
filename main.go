package main

import (
	"log"
	"subbotatest/cmd"

	"github.com/hajimehoshi/ebiten/v2"
)

const ()

func main() {
	ebiten.SetWindowTitle("Getting Started")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSize(cmd.ScreenW, cmd.ScreenH)
	ebiten.SetFullscreen(true)
	ebiten.SetVsyncEnabled(true)
	ebiten.SetTPS(60)

	err := ebiten.RunGame(&cmd.Game{})
	if err != nil {
		log.Fatal(err)
	}
}
