package main

import (
	"fmt"
	"image" // for color.Alpha{a}
	"image/draw"
	"image/png"
	"os"
)

func init() {
	// without this register .. At(), Bounds() functions will
	// caused memory pointer error!!
	image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)
}

func main() {
	args := os.Args
	fmt.Printf("args=%v\n", args)
	imgPath := args[1]

	imgfile, err := os.Open(imgPath)
	if err != nil {
		fmt.Printf("%q file not found!\n", imgPath)
		os.Exit(1)
	}
	defer imgfile.Close()

	img, _, err := image.Decode(imgfile)
	bounds := img.Bounds()
	fmt.Printf("bounds=%#v\n", bounds)
	w, h := bounds.Max.X, bounds.Max.Y
	fmt.Printf("w=%d h=%d\n", w, h)

	// canvas := image.NewAlpha(bounds)
	// canvas.Set(w/2, h/2, image.Transparent)
	// // http://golang.org/pkg/image/color/#Alpha
	// // Alpha represents an 8-bit alpha color.
	// x := 10
	// y := 10
	// a := uint8((23*x + 29*y) % 0x100)
	// canvas.SetAlpha(x, y, color.Alpha{a})

	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			rgba.Set(x, y, image.Transparent)
		}
	}

	outPath := "out.png"
	err = save(outPath, rgba)
	if err != nil {
		panic(err)
	}
	fmt.Printf("saved to %q\n", outPath)
}

func drawPixels(img *image.Alpha, px, py, pw, ph uint, fill bool) {
	var x, y uint
	for y = 0; y < ph; y++ {
		for x = 0; x < pw; x++ {
			if fill {
				img.Set(int(px*pw+x), int(py*ph+y), image.White)
			} else {
				img.Set(int(px*pw+x), int(py*ph+y), image.Transparent)
			}
		}
	}
}

func save(filename string, img image.Image) error {
	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	return png.Encode(out, img)
}
