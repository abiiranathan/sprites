package sprites

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
)

// ResizeNearestNeighbor resizes the source image to the specified dimensions
// using nearest neighbor interpolation.
//
// This method is extremely fast but may produce pixelated results, especially for
// significant size reductions. It works by mapping each destination pixel to the
// nearest source pixel without any smoothing or blending.
//
// Parameters:
//   - width: The width of the output image
//   - height: The height of the output image
//   - src: The source image to resize
//
// Returns:
//   - *image.RGBA: The resized image
func ResizeNearestNeighbor(width, height int, src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	w, h := b.Dx(), b.Dy()

	scaleX := float64(w) / float64(width)
	scaleY := float64(h) / float64(height)

	for y := range height {
		for x := range width {
			// Map to source coordinates with centered rounding
			sx := b.Min.X + int(float64(x)*scaleX+0.5*scaleX)
			sy := b.Min.Y + int(float64(y)*scaleY+0.5*scaleY)

			// Ensure we stay within upper bounds
			sx = min(sx, b.Max.X-1)
			sy = min(sy, b.Max.Y-1)

			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// A samplerFunc defines a function that can calculate a color for a given
// point in a source image using a specific interpolation algorithm.
type samplerFunc func(src image.Image, x, y, scaleX, scaleY float64) color.Color

// workerJob represents a row of pixels to process.
type workerJob struct {
	row    int
	width  int
	bounds image.Rectangle
	scaleX float64
	scaleY float64
}

// jobResult contains the processed row data.
type jobResult struct {
	row    int
	pixels []color.Color
}

// worker function processes jobs from the jobs channel and sends results to the results channel.
func worker(jobs <-chan workerJob, results chan<- jobResult, src image.Image, sampler samplerFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs {
		pixels := make([]color.Color, job.width)
		for x := 0; x < job.width; x++ {
			// Map destination coordinates to source coordinates (center-to-center)
			srcX := (float64(x)+0.5)*job.scaleX - 0.5 + float64(job.bounds.Min.X)
			srcY := (float64(job.row)+0.5)*job.scaleY - 0.5 + float64(job.bounds.Min.Y)

			// Sample using the provided interpolation algorithm
			pixels[x] = sampler(src, srcX, srcY, job.scaleX, job.scaleY)
		}
		results <- jobResult{row: job.row, pixels: pixels}
	}
}

// resizeWithSampler provides a generic, parallelized resizing framework for
// high-quality interpolation algorithms.
func resizeWithSampler(width, height int, src image.Image, sampler samplerFunc) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	srcW := float64(bounds.Dx())
	srcH := float64(bounds.Dy())

	scaleX := srcW / float64(width)
	scaleY := srcH / float64(height)

	numWorkers := min(height, runtime.NumCPU())
	jobs := make(chan workerJob, height)
	results := make(chan jobResult, height)

	var wg sync.WaitGroup
	for range numWorkers {
		wg.Add(1)
		go worker(jobs, results, src, sampler, &wg)
	}

	go func() {
		defer close(jobs)
		for y := range height {
			jobs <- workerJob{
				row:    y,
				width:  width,
				bounds: bounds,
				scaleX: scaleX,
				scaleY: scaleY,
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		for x, pixel := range result.pixels {
			dst.Set(x, result.row, pixel)
		}
	}

	return dst
}

// Lanczos-3 kernel function (sinc-based)
func lanczos3(x float64) float64 {
	if x == 0 {
		return 1.0
	}
	if math.Abs(x) >= 3.0 {
		return 0.0
	}
	pix := math.Pi * x
	return 3.0 * math.Sin(pix) * math.Sin(pix/3.0) / (pix * pix)
}

// sampleLanczos3 samples the source image at (x, y) using Lanczos-3 interpolation.
// This improved version stretches the kernel when downscaling to properly filter the
// image and prevent aliasing, resulting in much smoother and higher-quality results.
// x and y are in source image coordinates.
//
// Returns the interpolated color.
func sampleLanczos3(src image.Image, x, y, scaleX, scaleY float64) color.Color {
	bounds := src.Bounds()
	// The kernel support is 3. When downscaling, we must stretch the kernel
	// to act as a low-pass filter and prevent aliasing artifacts.
	supportX := 3.0 * math.Max(1.0, scaleX)
	supportY := 3.0 * math.Max(1.0, scaleY)

	// Calculate the bounding box of pixels to sample in the source image.
	xMin := int(math.Ceil(x - supportX))
	xMax := int(math.Floor(x + supportX))
	yMin := int(math.Ceil(y - supportY))
	yMax := int(math.Floor(y + supportY))

	var r, g, b, a float64
	var totalWeight float64

	// The scale for the kernel input, only applies when downscaling.
	sX := math.Max(1.0, scaleX)
	sY := math.Max(1.0, scaleY)

	for sy := yMin; sy <= yMax; sy++ {
		for sx := xMin; sx <= xMax; sx++ {
			// Skip if outside source image bounds
			if sx < bounds.Min.X || sx >= bounds.Max.X || sy < bounds.Min.Y || sy >= bounds.Max.Y {
				continue
			}

			// Calculate distance from sample point to the pixel center
			distX := x - float64(sx)
			distY := y - float64(sy)

			// Calculate Lanczos weights with scaling for antialiasing.
			weightX := lanczos3(distX / sX)
			weightY := lanczos3(distY / sY)
			weight := weightX * weightY

			if weight == 0 {
				continue
			}

			// Get source pixel and apply weight
			srcColor := src.At(sx, sy)
			sr, sg, sb, sa := srcColor.RGBA()

			r += float64(sr) * weight
			g += float64(sg) * weight
			b += float64(sb) * weight
			a += float64(sa) * weight
			totalWeight += weight
		}
	}

	// Normalize by total weight
	if totalWeight > 0 {
		r /= totalWeight
		g /= totalWeight
		b /= totalWeight
		a /= totalWeight
	}

	// Clamp to valid range and return
	return color.RGBA64{
		R: uint16(math.Max(0, math.Min(65535, r))),
		G: uint16(math.Max(0, math.Min(65535, g))),
		B: uint16(math.Max(0, math.Min(65535, b))),
		A: uint16(math.Max(0, math.Min(65535, a))),
	}
}

// ResizeLanczos3 resizes the source image to the specified dimensions using Lanczos-3 interpolation.
//
// Lanczos-3 interpolation uses a 6x6 sampling window with a sinc-based kernel that provides
// excellent detail preservation while minimizing artifacts. It's ideal for high-quality resizing
// of photographic images and detailed graphics. This implementation is optimized to prevent
// aliasing when downscaling, producing very smooth and clean results.
//
// Parameters:
//   - width: The width of the output image
//   - height: The height of the output image
//   - src: The source image to resize
//
// Returns:
//   - *image.RGBA: The resized image
func ResizeLanczos3(width, height int, src image.Image) image.Image {
	return resizeWithSampler(width, height, src, sampleLanczos3)
}
