package goregress_test

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	goregress "github.com/Pondigo/go-regress"

	"gocv.io/x/gocv"
)

// ---------------------------------------------------------------------------
// Core comparison tests
// ---------------------------------------------------------------------------

func TestIdenticalImages(t *testing.T) {
	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(100, 150, 200, 0), 200, 300, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	baseline := filepath.Join(t.TempDir(), "baseline.png")
	gocv.IMWrite(baseline, img)

	result, err := goregress.Compare(baseline, baseline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Pass {
		t.Errorf("identical images should pass, got similarity=%.4f", result.Similarity)
	}
	if result.Similarity < 0.999 {
		t.Errorf("expected similarity ~1.0, got %.4f", result.Similarity)
	}
}

func TestDifferentImages(t *testing.T) {
	dir := t.TempDir()

	baseline := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(255, 0, 0, 0), 200, 300, gocv.MatTypeCV8UC3,
	)
	defer baseline.Close()

	current := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(0, 0, 255, 0), 200, 300, gocv.MatTypeCV8UC3,
	)
	defer current.Close()

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	gocv.IMWrite(bPath, baseline)
	gocv.IMWrite(cPath, current)

	result, err := goregress.Compare(bPath, cPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pass {
		t.Error("completely different images should NOT pass")
	}
	t.Logf("similarity between blue and red: %.4f", result.Similarity)
}

func TestSizeMismatch(t *testing.T) {
	dir := t.TempDir()

	small := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 0, 0, 0), 100, 100, gocv.MatTypeCV8UC3)
	defer small.Close()
	big := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 0, 0, 0), 200, 200, gocv.MatTypeCV8UC3)
	defer big.Close()

	sPath := filepath.Join(dir, "small.png")
	bPath := filepath.Join(dir, "big.png")
	gocv.IMWrite(sPath, small)
	gocv.IMWrite(bPath, big)

	_, err := goregress.Compare(sPath, bPath, nil)
	if err == nil {
		t.Error("expected error for size mismatch, got nil")
	}
}

func TestMissingFile(t *testing.T) {
	dir := t.TempDir()
	img := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(0, 0, 0, 0), 50, 50, gocv.MatTypeCV8UC3)
	defer img.Close()
	validPath := filepath.Join(dir, "valid.png")
	gocv.IMWrite(validPath, img)

	_, err := goregress.Compare(filepath.Join(dir, "nope.png"), validPath, nil)
	if err == nil {
		t.Error("expected error for missing baseline")
	}

	_, err = goregress.Compare(validPath, filepath.Join(dir, "nope.png"), nil)
	if err == nil {
		t.Error("expected error for missing current")
	}
}

// ---------------------------------------------------------------------------
// Method-specific tests
// ---------------------------------------------------------------------------

func TestPixelMethod(t *testing.T) {
	dir := t.TempDir()

	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(128, 128, 128, 0), 100, 100, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	modified := img.Clone()
	defer modified.Close()
	roi := modified.Region(image.Rect(0, 0, 10, 10))
	white := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(255, 255, 255, 0), 10, 10, gocv.MatTypeCV8UC3,
	)
	defer white.Close()
	white.CopyTo(&roi)

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	gocv.IMWrite(bPath, img)
	gocv.IMWrite(cPath, modified)

	result, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.98,
		Method:    goregress.MethodPixel,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("pixel method: similarity=%.4f diffPixels=%d total=%d",
		result.Similarity, result.DiffPixels, result.TotalPixels)

	if result.DiffPixels == 0 {
		t.Error("expected some diff pixels")
	}
	if result.Method != goregress.MethodPixel {
		t.Errorf("expected MethodPixel, got %d", result.Method)
	}
}

func TestHistogramMethod(t *testing.T) {
	dir := t.TempDir()

	baseline := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(100, 100, 100, 0), 200, 200, gocv.MatTypeCV8UC3,
	)
	defer baseline.Close()

	// Slightly different shade — histogram should detect some difference but still be close
	current := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(110, 110, 110, 0), 200, 200, gocv.MatTypeCV8UC3,
	)
	defer current.Close()

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	gocv.IMWrite(bPath, baseline)
	gocv.IMWrite(cPath, current)

	result, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.90,
		Method:    goregress.MethodHistogram,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("histogram method: similarity=%.4f", result.Similarity)

	if result.Method != goregress.MethodHistogram {
		t.Errorf("expected MethodHistogram, got %d", result.Method)
	}
	if result.Similarity <= 0 || result.Similarity > 1.0 {
		t.Errorf("similarity out of range: %.4f", result.Similarity)
	}
}

func TestHistogramMethodIdentical(t *testing.T) {
	dir := t.TempDir()

	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(80, 120, 200, 0), 150, 150, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	p := filepath.Join(dir, "img.png")
	gocv.IMWrite(p, img)

	result, err := goregress.Compare(p, p, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodHistogram,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Pass {
		t.Errorf("identical images should pass with histogram, similarity=%.4f", result.Similarity)
	}
}

func TestHistogramMethodDrasticallyDifferent(t *testing.T) {
	dir := t.TempDir()

	black := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(0, 0, 0, 0), 100, 100, gocv.MatTypeCV8UC3,
	)
	defer black.Close()
	white := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(255, 255, 255, 0), 100, 100, gocv.MatTypeCV8UC3,
	)
	defer white.Close()

	bPath := filepath.Join(dir, "black.png")
	wPath := filepath.Join(dir, "white.png")
	gocv.IMWrite(bPath, black)
	gocv.IMWrite(wPath, white)

	result, err := goregress.Compare(bPath, wPath, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodHistogram,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pass {
		t.Error("black vs white should not pass at 99% threshold")
	}
	t.Logf("histogram black vs white: similarity=%.4f", result.Similarity)
}

// ---------------------------------------------------------------------------
// Options tests
// ---------------------------------------------------------------------------

func TestDiffImageGeneration(t *testing.T) {
	dir := t.TempDir()

	img1 := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(100, 100, 100, 0), 200, 300, gocv.MatTypeCV8UC3,
	)
	defer img1.Close()

	img2 := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(200, 200, 200, 0), 200, 300, gocv.MatTypeCV8UC3,
	)
	defer img2.Close()

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	diffPath := filepath.Join(dir, "diff.png")
	gocv.IMWrite(bPath, img1)
	gocv.IMWrite(cPath, img2)

	result, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold:      0.99,
		DiffOutputPath: diffPath,
		Method:         goregress.MethodPixel,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DiffPath == "" {
		t.Error("expected diff path to be set")
	}

	if _, err := os.Stat(diffPath); os.IsNotExist(err) {
		t.Errorf("diff image was not created at: %s", diffPath)
	}
}

func TestIgnoreRegions(t *testing.T) {
	dir := t.TempDir()

	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(128, 128, 128, 0), 100, 100, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	modified := img.Clone()
	defer modified.Close()
	roi := modified.Region(image.Rect(0, 0, 20, 20))
	white := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(255, 255, 255, 0), 20, 20, gocv.MatTypeCV8UC3,
	)
	defer white.Close()
	white.CopyTo(&roi)

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	gocv.IMWrite(bPath, img)
	gocv.IMWrite(cPath, modified)

	r1, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodPixel,
	})
	if err != nil {
		t.Fatal(err)
	}

	r2, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodPixel,
		IgnoreRegions: []image.Rectangle{
			image.Rect(0, 0, 20, 20),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("without ignore: similarity=%.4f | with ignore: similarity=%.4f",
		r1.Similarity, r2.Similarity)

	if r2.Similarity < r1.Similarity {
		t.Error("ignoring the changed region should increase similarity")
	}
}

func TestRegionComparison(t *testing.T) {
	dir := t.TempDir()

	// Baseline: gray 200x200
	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(128, 128, 128, 0), 200, 200, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	// Modified: change only bottom-right 50x50 to white
	modified := img.Clone()
	defer modified.Close()
	roi := modified.Region(image.Rect(150, 150, 200, 200))
	white := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(255, 255, 255, 0), 50, 50, gocv.MatTypeCV8UC3,
	)
	defer white.Close()
	white.CopyTo(&roi)

	bPath := filepath.Join(dir, "baseline.png")
	cPath := filepath.Join(dir, "current.png")
	gocv.IMWrite(bPath, img)
	gocv.IMWrite(cPath, modified)

	// Compare full image — should detect difference
	rFull, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodPixel,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Compare only top-left region (unmodified) — should pass
	rRegion, err := goregress.Compare(bPath, cPath, &goregress.CompareOptions{
		Threshold: 0.99,
		Method:    goregress.MethodPixel,
		Region:    image.Rect(0, 0, 100, 100),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("full image: similarity=%.4f | top-left region only: similarity=%.4f",
		rFull.Similarity, rRegion.Similarity)

	if !rRegion.Pass {
		t.Error("region comparison on unmodified area should pass")
	}
	if rFull.Similarity >= rRegion.Similarity {
		t.Error("region restricted to unmodified area should have higher similarity than full image")
	}
}

func TestDefaultOptionsApplied(t *testing.T) {
	dir := t.TempDir()

	img := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(128, 128, 128, 0), 50, 50, gocv.MatTypeCV8UC3,
	)
	defer img.Close()

	p := filepath.Join(dir, "img.png")
	gocv.IMWrite(p, img)

	// nil opts — should use defaults
	r, err := goregress.Compare(p, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Threshold != 0.995 {
		t.Errorf("expected default threshold 0.995, got %.3f", r.Threshold)
	}
	if r.Method != goregress.MethodSSIM {
		t.Errorf("expected default MethodSSIM, got %d", r.Method)
	}

	// Zero threshold in opts — should be filled with default
	r2, err := goregress.Compare(p, p, &goregress.CompareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Threshold != 0.995 {
		t.Errorf("zero threshold should default to 0.995, got %.3f", r2.Threshold)
	}
}

// ---------------------------------------------------------------------------
// CompareMats tests
// ---------------------------------------------------------------------------

func TestCompareMatsDirectly(t *testing.T) {
	a := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(50, 100, 150, 0), 100, 100, gocv.MatTypeCV8UC3,
	)
	defer a.Close()
	b := a.Clone()
	defer b.Close()

	result, err := goregress.CompareMats(a, b, nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Pass {
		t.Errorf("identical mats should pass, similarity=%.4f", result.Similarity)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
}

// ---------------------------------------------------------------------------
// Visual output — generates baseline, current, and diff to testdata/output
// so you can visually inspect the comparison results.
// Run: go test -v -run TestVisualOutput
// ---------------------------------------------------------------------------

func TestVisualOutput(t *testing.T) {
	outDir := "testdata/output"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Build a baseline: gray background with a blue rectangle (simulates a button)
	baseline := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(240, 240, 240, 0), 300, 400, gocv.MatTypeCV8UC3,
	)
	defer baseline.Close()
	blue := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(200, 120, 40, 0), 60, 140, gocv.MatTypeCV8UC3,
	)
	defer blue.Close()
	roiBtn := baseline.Region(image.Rect(130, 120, 270, 180))
	blue.CopyTo(&roiBtn)

	// Build current: same layout but the button shifted + color changed
	current := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(240, 240, 240, 0), 300, 400, gocv.MatTypeCV8UC3,
	)
	defer current.Close()
	green := gocv.NewMatWithSizeFromScalar(
		gocv.NewScalar(60, 180, 80, 0), 60, 140, gocv.MatTypeCV8UC3,
	)
	defer green.Close()
	roiBtn2 := current.Region(image.Rect(140, 125, 280, 185))
	green.CopyTo(&roiBtn2)

	bPath := filepath.Join(outDir, "baseline.png")
	cPath := filepath.Join(outDir, "current.png")
	diffPath := filepath.Join(outDir, "diff.png")
	gocv.IMWrite(bPath, baseline)
	gocv.IMWrite(cPath, current)

	// --- Run all three methods and log results ---
	methods := []struct {
		name   string
		method goregress.CompareMethod
	}{
		{"SSIM", goregress.MethodSSIM},
		{"Pixel", goregress.MethodPixel},
		{"Histogram", goregress.MethodHistogram},
	}

	for _, m := range methods {
		opts := &goregress.CompareOptions{
			Threshold: 0.99,
			Method:    m.method,
		}
		// Generate diff image only for the Pixel run
		if m.method == goregress.MethodPixel {
			opts.DiffOutputPath = diffPath
		}

		result, err := goregress.Compare(bPath, cPath, opts)
		if err != nil {
			t.Fatalf("[%s] error: %v", m.name, err)
		}
		t.Logf("[%s] pass=%v similarity=%.4f (%.1f%%) threshold=%.4f duration=%s",
			m.name, result.Pass, result.Similarity, result.Similarity*100,
			result.Threshold, result.Duration)
	}

	// --- Create side-by-side triptych: baseline | current | diff ---
	diffImg := gocv.IMRead(diffPath, gocv.IMReadColor)
	if diffImg.Empty() {
		t.Fatal("diff image was not generated")
	}
	defer diffImg.Close()

	triptych := createTriptych(t, baseline, current, diffImg)
	defer triptych.Close()

	triptychPath := filepath.Join(outDir, "triptych.png")
	if ok := gocv.IMWrite(triptychPath, triptych); !ok {
		t.Fatal("failed to write triptych")
	}

	t.Logf("output images written to %s/", outDir)
	t.Logf("  baseline.png  — original UI state")
	t.Logf("  current.png   — changed UI state")
	t.Logf("  diff.png      — differences highlighted in red")
	t.Logf("  triptych.png  — side-by-side: baseline | current | diff")
}

// createTriptych stitches three images horizontally into one canvas.
func createTriptych(t *testing.T, a, b, c gocv.Mat) gocv.Mat {
	t.Helper()
	h := a.Rows()
	w := a.Cols()
	canvas := gocv.NewMatWithSize(h, w*3, gocv.MatTypeCV8UC3)

	r1 := canvas.Region(image.Rect(0, 0, w, h))
	a.CopyTo(&r1)
	r2 := canvas.Region(image.Rect(w, 0, w*2, h))
	b.CopyTo(&r2)
	r3 := canvas.Region(image.Rect(w*2, 0, w*3, h))
	c.CopyTo(&r3)

	return canvas
}
