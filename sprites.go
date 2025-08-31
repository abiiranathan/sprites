// Package sprites generates a sprite image from individual images, along with corresponding CSS and HTML files.
// It supports resizing images to a uniform size and copying the generated sprite to a specified location.
package sprites

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// Config holds sprite generation configuration
type Config struct {
	IconSize     int      // size to which each icon will be resized (square)
	OutputDir    string   // directory to save generated files
	SpriteFile   string   // name of the generated sprite image file
	CSSFile      string   // name of the generated CSS file
	HTMLFile     string   // name of the generated HTML file
	SourcePrefix string   // optional prefix for source image paths
	Images       []string // list of image file paths to include in the sprite
	CopyTo       string   // optional destination to copy the sprite
	StaticPrefix string   // optional prefix for static assets in generated HTML/CSS
}

// Generate creates the sprite, CSS, and HTML files.
//
// It accepts a Config struct pointer with necessary parameters.
//
// Returns an error if any step fails.
//
// The default icon size is 64x64 pixels if not specified.
//
// The config.OutputDir must be specified and will be created if it doesn't exist.
//
// The config.Images slice must contain at least one image path.
//
// The generated sprite image, CSS, and HTML files will be
// saved in config.OutputDir.
// The default names for the generated files are "sprite.png", "sprite.css", and "index.html" if not specified.
func Generate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if cfg.IconSize <= 0 {
		return fmt.Errorf("icon size must be greater than zero")
	}

	if cfg.OutputDir == "" {
		return fmt.Errorf("output directory cannot be empty")
	}

	if cfg.SpriteFile == "" {
		cfg.SpriteFile = "sprite.png"
	}

	if cfg.CSSFile == "" {
		cfg.CSSFile = "sprite.css"
	}

	if cfg.HTMLFile == "" {
		cfg.HTMLFile = "index.html"
	}

	if len(cfg.Images) == 0 {
		return fmt.Errorf("no images specified")
	}

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	resizedImages, err := resizeImages(cfg)
	if err != nil {
		return fmt.Errorf("failed to resize images: %w", err)
	}

	if err := combineImages(cfg, resizedImages); err != nil {
		return fmt.Errorf("failed to combine images: %w", err)
	}

	if err := generateCSS(cfg); err != nil {
		return fmt.Errorf("failed to generate CSS: %w", err)
	}

	if err := generateHTML(cfg); err != nil {
		return fmt.Errorf("failed to generate HTML: %w", err)
	}

	if err := copySprite(cfg); err != nil {
		return fmt.Errorf("failed to copy sprite: %w", err)
	}
	return nil
}

func resizeImages(cfg *Config) ([]image.Image, error) {
	resized := make([]image.Image, 0, len(cfg.Images))

	for _, imgPath := range cfg.Images {
		img, err := loadAndResize(cfg, imgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load and resize image %s: %w", imgPath, err)
		}

		// Save individual resized image
		base := filepath.Base(imgPath)
		dest := filepath.Join(cfg.OutputDir, base)
		if err := saveImage(img, dest); err != nil {
			return nil, fmt.Errorf("failed to save resized image %s: %w", dest, err)
		}

		resized = append(resized, img)
	}
	return resized, nil
}

func loadAndResize(cfg *Config, path string) (image.Image, error) {
	fullPath := path
	if cfg.SourcePrefix != "" {
		fullPath = filepath.Join(cfg.SourcePrefix, path)
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", fullPath, err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image %s: %w", fullPath, err)
	}

	return resizeNearestNeighbor(cfg.IconSize, img), nil
}

func resizeNearestNeighbor(size int, src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	w, h := b.Dx(), b.Dy()

	for y := range size {
		for x := range size {
			sx := x * w / size
			sy := y * h / size
			if sx >= w {
				sx = w - 1 // prevent out-of-bounds
			}
			if sy >= h {
				sy = h - 1 // prevent out-of-bounds
			}
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// saveImage saves an image to the specified path in PNG format
func saveImage(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()
	return png.Encode(f, img)
}

// combineImages merges resized images into a single sprite image
func combineImages(cfg *Config, imgs []image.Image) error {
	totalWidth := len(imgs) * cfg.IconSize
	sprite := image.NewRGBA(image.Rect(0, 0, totalWidth, cfg.IconSize))

	for i, img := range imgs {
		x := i * cfg.IconSize
		draw.Draw(sprite, image.Rect(x, 0, x+cfg.IconSize, cfg.IconSize), img, image.Point{}, draw.Over)
	}

	return saveImage(sprite, filepath.Join(cfg.OutputDir, cfg.SpriteFile))
}

// generateCSS creates a CSS file mapping each icon to its position in the sprite
func generateCSS(cfg *Config) error {
	var sb strings.Builder

	// Use StaticPrefix if provided for the sprite URL
	staticURL := cfg.SpriteFile

	// Prepend StaticPrefix if provided
	if cfg.StaticPrefix != "" {
		staticURL = strings.TrimRight(cfg.StaticPrefix, "/") + "/" + cfg.SpriteFile
	}

	sb.WriteString(fmt.Sprintf(".sprite-icon { background-image: url('%s'); width: %dpx; height: %dpx; display: inline-block; }\n\n",
		staticURL, cfg.IconSize, cfg.IconSize))

	for i, imgPath := range cfg.Images {
		name := strings.TrimSuffix(filepath.Base(imgPath), filepath.Ext(imgPath))
		xOffset := i * cfg.IconSize
		sb.WriteString(fmt.Sprintf(".%s { background-position: -%dpx 0; }\n", name, xOffset))
	}

	return os.WriteFile(filepath.Join(cfg.OutputDir, cfg.CSSFile), []byte(sb.String()), 0644)
}

// generateHTML creates an HTML file demonstrating the use of the sprite icons
func generateHTML(cfg *Config) error {
	var sb strings.Builder
	// Use StaticPrefix if provided for the CSS URL
	cssURL := cfg.CSSFile

	// Prepend StaticPrefix if provided
	if cfg.StaticPrefix != "" {
		cssURL = strings.TrimRight(cfg.StaticPrefix, "/") + "/" + cfg.CSSFile
	}

	sb.WriteString(fmt.Sprintf("<!DOCTYPE html>\n<html>\n<head>\n<link rel='stylesheet' href='%s'>\n</head>\n<body>\n", cssURL))
	for _, imgPath := range cfg.Images {
		name := strings.TrimSuffix(filepath.Base(imgPath), filepath.Ext(imgPath))
		sb.WriteString(fmt.Sprintf("<div class='sprite-icon %s'></div>\n", name))
	}
	sb.WriteString("</body>\n</html>")

	return os.WriteFile(filepath.Join(cfg.OutputDir, cfg.HTMLFile), []byte(sb.String()), 0644)
}

// Check if copy destination is the same as output directory
func isSameDirectory(path1, path2 string) (bool, error) {
	if path1 == "" || path2 == "" {
		return false, nil
	}

	abs1, err := filepath.Abs(path1)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path for %s: %w", path1, err)
	}

	abs2, err := filepath.Abs(path2)
	if err != nil {
		return false, fmt.Errorf("failed to get absolute path for %s: %w", path2, err)
	}

	// Clean the paths to normalize . and .. components
	abs1 = filepath.Clean(abs1)
	abs2 = filepath.Clean(abs2)

	return abs1 == abs2, nil
}

// copySprite copies the generated sprite image to a specified location if configured
func copySprite(cfg *Config) error {
	if cfg.CopyTo == "" {
		return nil
	}

	// Avoid copying to the same location
	same, err := isSameDirectory(cfg.CopyTo, cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to compare directories: %w", err)
	}

	if same {
		fmt.Printf("Warning: Copy destination is the same as output directory; skipping copy.\n")
		return nil
	}

	srcPath := filepath.Join(cfg.OutputDir, cfg.SpriteFile)
	destPath := filepath.Join(cfg.CopyTo, cfg.SpriteFile)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source sprite: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination sprite: %w", err)
	}
	defer dest.Close()

	_, err = dest.ReadFrom(src)
	return err
}
