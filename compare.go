// Package goregress provides visual regression testing utilities
// using OpenCV (gocv) for comparing UI screenshots.
//
// Usage típico en tests de Cicada:
//
//	result, err := goregress.Compare("baseline.png", "current.png", nil)
//	if err != nil { t.Fatal(err) }
//	if !result.Pass {
//	    t.Errorf("visual regression detected: similarity %.2f%% < threshold %.2f%%",
//	        result.Similarity*100, result.Threshold*100)
//	}
package goregress

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"gocv.io/x/gocv"
)

// CompareOptions configures the comparison behavior.
type CompareOptions struct {
	// Threshold is the minimum similarity ratio (0.0–1.0) to consider a pass.
	// Default: 0.995 (99.5% similar).
	Threshold float64

	// DiffOutputPath is where to save the diff image highlighting changes.
	// If empty, no diff image is generated.
	DiffOutputPath string

	// Region limits comparison to a specific rectangle within the images.
	// If zero-value, the entire image is compared.
	Region image.Rectangle

	// IgnoreRegions are areas to mask out before comparison (e.g., timestamps, dynamic content).
	IgnoreRegions []image.Rectangle

	// Method selects the comparison algorithm.
	// Default: MethodSSIM.
	Method CompareMethod
}

// CompareMethod defines which algorithm to use for image comparison.
type CompareMethod int

const (
	// MethodSSIM uses Structural Similarity Index (recommended for UI testing).
	MethodSSIM CompareMethod = iota
	// MethodPixel uses absolute pixel-by-pixel difference.
	MethodPixel
	// MethodHistogram uses histogram correlation.
	MethodHistogram
)

// Result holds the output of an image comparison.
type Result struct {
	// Pass indicates whether the images are similar enough.
	Pass bool
	// Similarity is the computed similarity ratio (0.0–1.0).
	Similarity float64
	// Threshold used for this comparison.
	Threshold float64
	// DiffPixels is the count of pixels that differ beyond tolerance (MethodPixel only).
	DiffPixels int
	// TotalPixels is the total number of pixels compared.
	TotalPixels int
	// DiffPath is the path to the generated diff image, if any.
	DiffPath string
	// Duration is how long the comparison took.
	Duration time.Duration
	// Method used for this comparison.
	Method CompareMethod
}

// DefaultOptions returns sensible defaults for UI regression testing.
func DefaultOptions() *CompareOptions {
	return &CompareOptions{
		Threshold: 0.995,
		Method:    MethodSSIM,
	}
}

// Compare loads two images from disk and compares them.
// Returns a Result indicating whether they match within the configured threshold.
func Compare(baselinePath, currentPath string, opts *CompareOptions) (*Result, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	if opts.Threshold == 0 {
		opts.Threshold = 0.995
	}

	start := time.Now()

	baseline := gocv.IMRead(baselinePath, gocv.IMReadColor)
	if baseline.Empty() {
		return nil, fmt.Errorf("goregress: cannot read baseline image: %s", baselinePath)
	}
	defer baseline.Close()

	current := gocv.IMRead(currentPath, gocv.IMReadColor)
	if current.Empty() {
		return nil, fmt.Errorf("goregress: cannot read current image: %s", currentPath)
	}
	defer current.Close()

	return CompareMats(baseline, current, opts, start)
}

// CompareMats compares two gocv.Mat images directly.
// Useful when you already have the images in memory (e.g., from a screenshot capture).
func CompareMats(baseline, current gocv.Mat, opts *CompareOptions, start time.Time) (*Result, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	if opts.Threshold == 0 {
		opts.Threshold = 0.995
	}
	if start.IsZero() {
		start = time.Now()
	}

	// Validate dimensions match
	if baseline.Rows() != current.Rows() || baseline.Cols() != current.Cols() {
		return nil, fmt.Errorf(
			"goregress: image dimensions differ: baseline=%dx%d current=%dx%d",
			baseline.Cols(), baseline.Rows(), current.Cols(), current.Rows(),
		)
	}

	// Crop to region if specified
	b := baseline
	c := current
	if !opts.Region.Empty() {
		r := opts.Region
		rect := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
		b = baseline.Region(rect)
		c = current.Region(rect)
	}

	// Apply ignore masks
	if len(opts.IgnoreRegions) > 0 {
		b = b.Clone()
		c = c.Clone()
		defer b.Close()
		defer c.Close()
		for _, region := range opts.IgnoreRegions {
			maskColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
			gocv.Rectangle(&b, region, maskColor, -1) // filled rectangle
			gocv.Rectangle(&c, region, maskColor, -1)
		}
	}

	var result *Result
	var err error

	switch opts.Method {
	case MethodSSIM:
		result, err = compareSSIM(b, c, opts)
	case MethodPixel:
		result, err = comparePixel(b, c, opts)
	case MethodHistogram:
		result, err = compareHistogram(b, c, opts)
	default:
		return nil, fmt.Errorf("goregress: unknown comparison method: %d", opts.Method)
	}

	if err != nil {
		return nil, err
	}

	result.Duration = time.Since(start)
	result.Pass = result.Similarity >= opts.Threshold
	result.Threshold = opts.Threshold
	result.Method = opts.Method

	// Generate diff image if requested
	if opts.DiffOutputPath != "" {
		diffPath, genErr := generateDiff(baseline, current, opts.DiffOutputPath)
		if genErr != nil {
			return result, fmt.Errorf("goregress: comparison ok but diff generation failed: %w", genErr)
		}
		result.DiffPath = diffPath
	}

	return result, nil
}

// compareSSIM computes the Structural Similarity Index between two images.
// Uses a simplified SSIM approach compatible with gocv's available API.
func compareSSIM(a, b gocv.Mat, opts *CompareOptions) (*Result, error) {
	// Convert to grayscale
	grayA := gocv.NewMat()
	defer grayA.Close()
	grayB := gocv.NewMat()
	defer grayB.Close()

	gocv.CvtColor(a, &grayA, gocv.ColorBGRToGray)
	gocv.CvtColor(b, &grayB, gocv.ColorBGRToGray)

	// Convert to float64 for precision
	fA := gocv.NewMat()
	defer fA.Close()
	fB := gocv.NewMat()
	defer fB.Close()

	grayA.ConvertTo(&fA, gocv.MatTypeCV64F)
	grayB.ConvertTo(&fB, gocv.MatTypeCV64F)

	// SSIM constants
	c1 := 6.5025  // (0.01 * 255)^2
	c2 := 58.5225 // (0.03 * 255)^2
	kSize := image.Pt(11, 11)

	// μ (means) via Gaussian blur
	muA := gocv.NewMat()
	defer muA.Close()
	muB := gocv.NewMat()
	defer muB.Close()
	gocv.GaussianBlur(fA, &muA, kSize, 1.5, 1.5, gocv.BorderDefault)
	gocv.GaussianBlur(fB, &muB, kSize, 1.5, 1.5, gocv.BorderDefault)

	// μ² and μA·μB
	muASq := gocv.NewMat()
	defer muASq.Close()
	muBSq := gocv.NewMat()
	defer muBSq.Close()
	muAB := gocv.NewMat()
	defer muAB.Close()

	gocv.Multiply(muA, muA, &muASq)
	gocv.Multiply(muB, muB, &muBSq)
	gocv.Multiply(muA, muB, &muAB)

	// σ² = E[X²] - μ²
	aaSq := gocv.NewMat()
	defer aaSq.Close()
	bbSq := gocv.NewMat()
	defer bbSq.Close()
	abMul := gocv.NewMat()
	defer abMul.Close()

	sigmaASq := gocv.NewMat()
	defer sigmaASq.Close()
	sigmaBSq := gocv.NewMat()
	defer sigmaBSq.Close()
	sigmaAB := gocv.NewMat()
	defer sigmaAB.Close()

	gocv.Multiply(fA, fA, &aaSq)
	gocv.GaussianBlur(aaSq, &sigmaASq, kSize, 1.5, 1.5, gocv.BorderDefault)
	gocv.Subtract(sigmaASq, muASq, &sigmaASq)

	gocv.Multiply(fB, fB, &bbSq)
	gocv.GaussianBlur(bbSq, &sigmaBSq, kSize, 1.5, 1.5, gocv.BorderDefault)
	gocv.Subtract(sigmaBSq, muBSq, &sigmaBSq)

	gocv.Multiply(fA, fB, &abMul)
	gocv.GaussianBlur(abMul, &sigmaAB, kSize, 1.5, 1.5, gocv.BorderDefault)
	gocv.Subtract(sigmaAB, muAB, &sigmaAB)

	// Build SSIM per-pixel using ConvertTo for scalar ops:
	//   numerator   = (2·μA·μB + C1)(2·σAB + C2)
	//   denominator = (μA² + μB² + C1)(σA² + σB² + C2)

	// num1 = 2·muAB + c1  (using ConvertTo with alpha=2, beta=c1)
	num1 := gocv.NewMat()
	defer num1.Close()
	convertScaleAdd(muAB, 2.0, c1, &num1)

	// num2 = 2·sigmaAB + c2
	num2 := gocv.NewMat()
	defer num2.Close()
	convertScaleAdd(sigmaAB, 2.0, c2, &num2)

	// den1 = muASq + muBSq + c1
	den1 := gocv.NewMat()
	defer den1.Close()
	gocv.Add(muASq, muBSq, &den1)
	addScalar(den1, c1, &den1)

	// den2 = sigmaASq + sigmaBSq + c2
	den2 := gocv.NewMat()
	defer den2.Close()
	gocv.Add(sigmaASq, sigmaBSq, &den2)
	addScalar(den2, c2, &den2)

	// numerator = num1 * num2
	numerator := gocv.NewMat()
	defer numerator.Close()
	gocv.Multiply(num1, num2, &numerator)

	// denominator = den1 * den2
	denominator := gocv.NewMat()
	defer denominator.Close()
	gocv.Multiply(den1, den2, &denominator)

	// ssimMap = numerator / denominator
	ssimMap := gocv.NewMat()
	defer ssimMap.Close()
	gocv.Divide(numerator, denominator, &ssimMap)

	// Mean SSIM across all pixels
	mean := ssimMap.Mean()
	similarity := mean.Val1

	return &Result{
		Similarity:  similarity,
		TotalPixels: a.Rows() * a.Cols(),
	}, nil
}

// convertScaleAdd computes: dst = src * alpha + beta (per-element).
// Uses Mat.ConvertTo which maps to OpenCV's convertTo(dst, type, alpha, beta).
func convertScaleAdd(src gocv.Mat, alpha, beta float64, dst *gocv.Mat) {
	src.ConvertToWithParams(dst, gocv.MatTypeCV64F, alpha, beta)
}

// addScalar adds a scalar value to every element: dst = src + val.
func addScalar(src gocv.Mat, val float64, dst *gocv.Mat) {
	src.ConvertToWithParams(dst, gocv.MatTypeCV64F, 1.0, val)
}

// comparePixel does absolute pixel-by-pixel comparison.
func comparePixel(a, b gocv.Mat, opts *CompareOptions) (*Result, error) {
	diff := gocv.NewMat()
	defer diff.Close()

	gocv.AbsDiff(a, b, &diff)

	// Convert to grayscale for threshold
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(diff, &gray, gocv.ColorBGRToGray)

	// Any pixel with diff > 0 is considered different
	thresh := gocv.NewMat()
	defer thresh.Close()
	gocv.Threshold(gray, &thresh, 1.0, 255.0, gocv.ThresholdBinary)

	diffPixels := gocv.CountNonZero(thresh)
	totalPixels := a.Rows() * a.Cols()
	similarity := 1.0 - float64(diffPixels)/float64(totalPixels)

	return &Result{
		Similarity:  similarity,
		DiffPixels:  diffPixels,
		TotalPixels: totalPixels,
	}, nil
}

// compareHistogram uses a normalized mean-error approach for tolerant comparison.
// More forgiving than pixel-exact — good for detecting large-scale color shifts.
func compareHistogram(a, b gocv.Mat, opts *CompareOptions) (*Result, error) {
	// Use AbsDiff + normalized mean as a simpler, reliable metric
	diff := gocv.NewMat()
	defer diff.Close()
	gocv.AbsDiff(a, b, &diff)

	// Convert to grayscale
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(diff, &gray, gocv.ColorBGRToGray)

	// Mean difference across all pixels (0–255 scale)
	mean := gray.Mean()
	avgDiff := mean.Val1

	// Normalize to 0–1 similarity (0 diff = 1.0 similarity)
	similarity := 1.0 - (avgDiff / 255.0)
	similarity = math.Max(0, math.Min(1, similarity))

	return &Result{
		Similarity:  similarity,
		TotalPixels: a.Rows() * a.Cols(),
	}, nil
}

// generateDiff creates a visual diff image highlighting the differences.
// Red pixels = areas that changed.
func generateDiff(baseline, current gocv.Mat, outputPath string) (string, error) {
	diff := gocv.NewMat()
	defer diff.Close()
	gocv.AbsDiff(baseline, current, &diff)

	// Amplify differences for visibility
	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(diff, &gray, gocv.ColorBGRToGray)

	// Threshold to get binary mask of differences
	mask := gocv.NewMat()
	defer mask.Close()
	gocv.Threshold(gray, &mask, 10.0, 255.0, gocv.ThresholdBinary)

	// Create colored overlay: baseline with red highlighting on diffs
	overlay := baseline.Clone()
	defer overlay.Close()

	// Paint diff areas red on the overlay
	red := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 0, 255, 0), baseline.Rows(), baseline.Cols(), gocv.MatTypeCV8UC3)
	defer red.Close()

	red.CopyToWithMask(&overlay, mask)

	// Blend: 60% original + 40% overlay for readability
	blended := gocv.NewMat()
	defer blended.Close()
	gocv.AddWeighted(baseline, 0.6, overlay, 0.4, 0, &blended)

	// Ensure directory exists and save
	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
	}

	ok := gocv.IMWrite(outputPath, blended)
	if !ok {
		return "", fmt.Errorf("failed to write diff image to: %s", outputPath)
	}

	return outputPath, nil
}
