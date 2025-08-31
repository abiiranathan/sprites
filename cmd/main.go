package main

import (
	"bytes"
	"image"
	"image/png"
	"os"

	"github.com/abiiranathan/sprites"
)

const AVATAR_SIZE = 64

func main() {
	// Resize an input image and save to output image.
	if len(os.Args) != 3 {
		println("Usage: go run cmd/main.go <input file> <output file>")
		return
	}

	infile := os.Args[1]
	outfile := os.Args[2]

	f, err := os.Open(infile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		panic(err)
	}

	// Create an png decoder with the file size as buffer size
	buf := make([]byte, stat.Size())
	_, err = f.Read(buf)
	if err != nil {
		panic(err)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		panic(err)
	}
	resized := sprites.ResizeLanczos3(AVATAR_SIZE, AVATAR_SIZE, img)

	out, err := os.Create(outfile)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	err = png.Encode(out, resized)
	if err != nil {
		panic(err)
	}

	println("Resized image saved to", outfile)
}
