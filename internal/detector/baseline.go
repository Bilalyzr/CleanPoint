package detector

import "sort"

// Median returns the 50th percentile (average of the middle two for even n).
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	n := len(ys)
	if n%2 == 1 {
		return ys[n/2]
	}
	return (ys[n/2-1] + ys[n/2]) / 2
}

// Mad is the median absolute deviation from the median.
func Mad(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := Median(xs)
	devs := make([]float64, len(xs))
	for i, x := range xs {
		d := x - m
		if d < 0 {
			d = -d
		}
		devs[i] = d
	}
	return Median(devs)
}

// Baseline holds the robust center and scale per feature, trained only on the
// designated normal (training-period) windows.
type Baseline struct {
	Center map[string]float64
	Scale  map[string]float64
	Count  int
}

// TrainBaseline computes center = median(normal) and
// scale = max(1.4826*MAD(normal), epsilon) for each of the six scored
// features, per the source scoring formula.
func TrainBaseline(vectors []FeatureVector) Baseline {
	b := Baseline{Center: map[string]float64{}, Scale: map[string]float64{}, Count: len(vectors)}
	feats := []string{FModifiedRatio, FRenameRate, FEntropyShift, FExtNovelty, FDeleteRate, FSizeChange}
	for _, f := range feats {
		vals := []float64{}
		for _, v := range vectors {
			if !v.Missing[f] {
				vals = append(vals, v.Values[f])
			}
		}
		if len(vals) == 0 {
			// No evidence at all in baseline: treat as zero-centered with
			// epsilon scale; scoring marks coverage accordingly.
			b.Center[f] = 0
			b.Scale[f] = Epsilon[f]
			continue
		}
		b.Center[f] = Median(vals)
		scale := 1.4826 * Mad(vals)
		if scale < Epsilon[f] {
			scale = Epsilon[f]
		}
		b.Scale[f] = scale
	}
	return b
}
