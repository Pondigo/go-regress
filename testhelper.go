package goregress

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gocv.io/x/gocv"
)

// Suite manages baseline images and provides a convenient API for test files.
//
// Usage in Cicada tests:
//
//	func TestMain(m *testing.M) {
//	    goregress.BaselineDir = "testdata/baselines"
//	    goregress.DiffDir     = "testdata/diffs"
//	    os.Exit(m.Run())
//	}
//
//	func TestOrderScreen(t *testing.T) {
//	    suite := goregress.NewSuite(t.Name())
//	    screenshot := captureScreenshot() // your screenshot function
//	    suite.AssertMatch(t, "order_table", screenshot)
//	}
type Suite struct {
	Name        string
	BaselineDir string
	DiffDir     string
	Options     *CompareOptions
}

// Testing is a minimal interface matching *testing.T for error reporting.
type Testing interface {
	Helper()
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
	Name() string
}

// Global defaults — override in TestMain.
var (
	BaselineDir = "testdata/baselines"
	DiffDir     = "testdata/diffs"
)

// NewSuite creates a new regression suite for a test group.
func NewSuite(name string) *Suite {
	return &Suite{
		Name:        name,
		BaselineDir: BaselineDir,
		DiffDir:     DiffDir,
		Options:     DefaultOptions(),
	}
}

// AssertMatch compares a screenshot against its baseline.
// If no baseline exists, it saves the current image as the new baseline (first run).
// On mismatch, it saves a diff image and reports the failure.
func (s *Suite) AssertMatch(t Testing, label string, currentPath string) {
	t.Helper()

	baselinePath := s.baselinePath(label)
	diffPath := s.diffPath(label)

	// If baseline doesn't exist, save current as baseline
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		if err := s.saveBaseline(currentPath, baselinePath); err != nil {
			t.Fatalf("goregress: failed to save baseline: %v", err)
		}
		t.Logf("goregress: new baseline saved for %q at %s", label, baselinePath)
		return
	}

	opts := *s.Options // copy
	opts.DiffOutputPath = diffPath

	result, err := Compare(baselinePath, currentPath, &opts)
	if err != nil {
		t.Fatalf("goregress: comparison error for %q: %v", label, err)
	}

	if !result.Pass {
		t.Errorf(
			"goregress: visual regression in %q — similarity %.2f%% < threshold %.2f%% (diff: %s, took %s)",
			label,
			result.Similarity*100,
			result.Threshold*100,
			result.DiffPath,
			result.Duration.Round(time.Millisecond),
		)
	} else {
		t.Logf("goregress: %q passed — similarity %.2f%% (%s)",
			label, result.Similarity*100, result.Duration.Round(time.Millisecond))
		// Clean up diff from previous failures
		os.Remove(diffPath)
	}
}

// AssertMatchMat compares a gocv.Mat screenshot against its baseline.
// Saves the Mat to a temp file first, then delegates to AssertMatch.
func (s *Suite) AssertMatchMat(t Testing, label string, screenshot gocv.Mat) {
	t.Helper()

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("goregress_%s_%d.png", label, time.Now().UnixNano()))
	if ok := gocv.IMWrite(tmpPath, screenshot); !ok {
		t.Fatalf("goregress: failed to write screenshot to temp file")
	}
	defer os.Remove(tmpPath)

	s.AssertMatch(t, label, tmpPath)
}

// UpdateBaseline forces saving a new baseline for the given label.
// Useful when the UI has intentionally changed.
func (s *Suite) UpdateBaseline(label string, currentPath string) error {
	baselinePath := s.baselinePath(label)
	return s.saveBaseline(currentPath, baselinePath)
}

// UpdateAllBaselines updates baselines for all given labels from current screenshots.
func (s *Suite) UpdateAllBaselines(screenshots map[string]string) error {
	for label, path := range screenshots {
		if err := s.UpdateBaseline(label, path); err != nil {
			return fmt.Errorf("failed to update baseline %q: %w", label, err)
		}
	}
	return nil
}

func (s *Suite) baselinePath(label string) string {
	return filepath.Join(s.BaselineDir, s.Name, label+".png")
}

func (s *Suite) diffPath(label string) string {
	return filepath.Join(s.DiffDir, s.Name, label+"_diff.png")
}

func (s *Suite) saveBaseline(srcPath, dstPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create baseline dir: %w", err)
	}

	// Read and re-save to normalize format
	img := gocv.IMRead(srcPath, gocv.IMReadColor)
	if img.Empty() {
		return fmt.Errorf("cannot read source image: %s", srcPath)
	}
	defer img.Close()

	if ok := gocv.IMWrite(dstPath, img); !ok {
		return fmt.Errorf("failed to write baseline: %s", dstPath)
	}
	return nil
}
