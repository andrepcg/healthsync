// Package ecg derives rhythm metrics from a single-lead recording such as the
// Apple Watch's 30-second Lead I ECG: R-peak detection, heart rate, RR
// intervals, short-term heart-rate variability, irregularity, premature beats,
// signal quality and an averaged beat for display.
//
// It deliberately does not report PR, QRS or QT intervals. Those need a clean,
// calibrated multi-lead recording to measure to the tens of milliseconds that
// make them clinically meaningful; a wrist lead with movement noise cannot, and
// a wrong number is worse than none. Interpretation of rhythm stays with the
// device classification and a clinician.
package ecg

import (
	"math"
	"sort"
)

// Analysis is the result of Analyze.
type Analysis struct {
	SampleRate float64 `json:"sample_rate"`
	DurationS  float64 `json:"duration_s"`

	// Beats detected and their positions (seconds from start).
	Beats       int       `json:"beats"`
	RPeaksS     []float64 `json:"r_peaks_s"`
	RPeaksMV    []float64 `json:"r_peaks_mv"`
	RRIntervals []float64 `json:"rr_ms"` // successive RR intervals in ms

	// Heart rate from RR intervals (bpm).
	HRMean float64 `json:"hr_mean"`
	HRMin  float64 `json:"hr_min"`
	HRMax  float64 `json:"hr_max"`

	// Short-term HRV over the recording (ms / %). Ultra-short-term values are
	// only comparable with other 30-second recordings of the same person.
	SDNN  float64 `json:"sdnn_ms"`
	RMSSD float64 `json:"rmssd_ms"`
	PNN50 float64 `json:"pnn50_pct"`

	// Irregularity: coefficient of variation of RR (%), and a label.
	RRCV           float64 `json:"rr_cv_pct"`
	Irregularity   string  `json:"irregularity"` // regular|mildly irregular|irregular|very irregular
	PrematureBeats int     `json:"premature_beats"`

	// Signal quality.
	Quality      string   `json:"quality"` // good|fair|poor
	NoiseMV      float64  `json:"noise_mv"`
	RAmplitudeMV float64  `json:"r_amplitude_mv"`
	SNR          float64  `json:"snr"`
	Notes        []string `json:"notes"`

	// Averaged beat aligned on the R peak: -300 ms … +500 ms, in mV.
	Template   []float64 `json:"template_mv"`
	TemplateT0 float64   `json:"template_t0_ms"` // time of first template sample relative to R (negative)
	TemplateDT float64   `json:"template_dt_ms"` // ms between template samples
}

// Analyze runs the pipeline on raw samples in microvolts.
func Analyze(samplesUV []float32, rate float64) *Analysis {
	a := &Analysis{SampleRate: rate, Notes: []string{}, RPeaksS: []float64{}, RPeaksMV: []float64{}, RRIntervals: []float64{}, Template: []float64{}}
	n := len(samplesUV)
	if n == 0 || rate <= 0 {
		a.Quality = "poor"
		a.Notes = append(a.Notes, "no samples")
		return a
	}
	a.DurationS = float64(n) / rate

	// mV, baseline removed with a 0.6 s moving median-ish (mean) high-pass.
	x := make([]float64, n)
	for i, v := range samplesUV {
		x[i] = float64(v) / 1000
	}
	x = removeBaseline(x, int(0.6*rate))

	// Saturation / flat-line check.
	flat := 0
	for i := 1; i < n; i++ {
		if x[i] == x[i-1] {
			flat++
		}
	}
	flatLine := float64(flat)/float64(n) > 0.5

	peaks := detectRPeaks(x, rate)
	a.Beats = len(peaks)
	if flatLine && len(peaks) < 4 {
		a.Quality = "poor"
		a.Notes = append(a.Notes, "more than half of the signal is flat: poor skin contact or the recording was interrupted")
	}

	// Noise: high-frequency residual outside QRS windows.
	a.NoiseMV = noiseEstimate(x, peaks, rate)
	if len(peaks) > 0 {
		amp := make([]float64, len(peaks))
		for i, p := range peaks {
			amp[i] = math.Abs(x[p])
		}
		a.RAmplitudeMV = median(amp)
		if a.NoiseMV > 0 {
			a.SNR = a.RAmplitudeMV / a.NoiseMV
		} else if a.RAmplitudeMV > 0 {
			a.SNR = 999 // no measurable noise at all (finite so it serialises)
		}
	}

	if len(peaks) < 4 {
		a.Quality = "poor"
		a.Notes = append(a.Notes, "too few beats detected to describe the rhythm; the trace is probably noisy or the strap was loose")
		return a
	}

	for _, p := range peaks {
		a.RPeaksS = append(a.RPeaksS, float64(p)/rate)
		a.RPeaksMV = append(a.RPeaksMV, x[p])
	}
	rr := make([]float64, 0, len(peaks)-1)
	for i := 1; i < len(peaks); i++ {
		rr = append(rr, float64(peaks[i]-peaks[i-1])/rate*1000)
	}
	a.RRIntervals = rr

	// Heart rate.
	meanRR := mean(rr)
	a.HRMean = 60000 / meanRR
	minRR, maxRR := rr[0], rr[0]
	for _, v := range rr {
		if v < minRR {
			minRR = v
		}
		if v > maxRR {
			maxRR = v
		}
	}
	a.HRMax = 60000 / minRR
	a.HRMin = 60000 / maxRR

	// HRV.
	a.SDNN = stddev(rr)
	var sq float64
	nn50 := 0
	for i := 1; i < len(rr); i++ {
		d := rr[i] - rr[i-1]
		sq += d * d
		if math.Abs(d) > 50 {
			nn50++
		}
	}
	if len(rr) > 1 {
		a.RMSSD = math.Sqrt(sq / float64(len(rr)-1))
		a.PNN50 = float64(nn50) / float64(len(rr)-1) * 100
	}
	a.RRCV = a.SDNN / meanRR * 100

	// Premature beats: an RR much shorter than its neighbours' median,
	// followed by a longer one (compensatory pause).
	a.PrematureBeats = countPremature(rr)

	// Irregularity label from the CV after removing the premature beats'
	// influence is overkill; the raw CV with premature count is clearer.
	// Breathing alone moves RR by a few percent at rest (respiratory sinus
	// arrhythmia), so "regular" is generous; atrial fibrillation typically
	// sits well above 20 %.
	switch {
	case a.RRCV < 7:
		a.Irregularity = "regular"
	case a.RRCV < 12:
		a.Irregularity = "mildly irregular"
	case a.RRCV < 20:
		a.Irregularity = "irregular"
	default:
		a.Irregularity = "very irregular"
	}

	// Quality label.
	if a.Quality == "" {
		switch {
		case a.SNR >= 8 && a.Beats >= 15:
			a.Quality = "good"
		case a.SNR >= 4:
			a.Quality = "fair"
		default:
			a.Quality = "poor"
		}
	}
	if a.Quality != "good" {
		a.Notes = append(a.Notes, "noise is high relative to the beats; small waves (P, T) may not be visible in the averaged beat")
	}
	if a.DurationS < 25 {
		a.Notes = append(a.Notes, "recording shorter than 30 s; variability figures are less stable")
	}
	if a.PrematureBeats > 0 {
		a.Notes = append(a.Notes, "premature beats are counted from RR timing alone; the watch's classification and a clinician decide whether they matter")
	}

	a.Template, a.TemplateT0, a.TemplateDT = averageBeat(x, peaks, rr, rate)
	return a
}

// removeBaseline subtracts a centred moving average (window samples), which
// removes slow wander (breathing, arm movement) but leaves the beats intact.
func removeBaseline(x []float64, window int) []float64 {
	n := len(x)
	if window < 3 || window >= n {
		return x
	}
	out := make([]float64, n)
	half := window / 2
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += x[i]
		if i >= window {
			sum -= x[i-window]
		}
		c := i - half
		if c >= 0 {
			cnt := window
			if i < window {
				cnt = i + 1
			}
			out[c] = x[c] - sum/float64(cnt)
		}
	}
	for c := n - half; c < n; c++ {
		if c >= 0 {
			out[c] = x[c] - sum/float64(window)
		}
	}
	return out
}

// detectRPeaks is a Pan–Tompkins style detector: band-limit, differentiate,
// square, integrate over 120 ms, adaptive threshold with a 250 ms refractory
// period, then snap each detection to the largest absolute deflection nearby.
func detectRPeaks(x []float64, rate float64) []int {
	n := len(x)
	if n < int(rate) {
		return nil
	}
	// Low-pass (5-sample moving average ≈ 100 Hz at 512 Hz) then derivative.
	lp := movingAverage(x, max(3, int(rate/100)))
	d := make([]float64, n)
	for i := 2; i < n-2; i++ {
		d[i] = (2*lp[i+2] + lp[i+1] - lp[i-1] - 2*lp[i-2]) / 8
	}
	for i := range d {
		d[i] *= d[i]
	}
	feat := movingAverage(d, int(0.12*rate))

	// Adaptive threshold: start from a robust level, learn from peaks found.
	sorted := append([]float64(nil), feat...)
	sort.Float64s(sorted)
	noiseLvl := sorted[len(sorted)*50/100]
	sigLvl := sorted[len(sorted)*98/100]
	thr := noiseLvl + 0.35*(sigLvl-noiseLvl)

	refractory := int(0.25 * rate)
	search := int(0.06 * rate)
	var peaks []int
	i := 0
	for i < n {
		if feat[i] > thr {
			// Find the maximum of the feature over the next 150 ms window.
			end := min(n, i+int(0.15*rate))
			pk := i
			for j := i; j < end; j++ {
				if feat[j] > feat[pk] {
					pk = j
				}
			}
			// The integration window delays the feature; look back for the R.
			lo, hi := max(0, pk-int(0.12*rate)-search), min(n-1, pk+search)
			r := lo
			for j := lo; j <= hi; j++ {
				if math.Abs(x[j]) > math.Abs(x[r]) {
					r = j
				}
			}
			if len(peaks) == 0 || r-peaks[len(peaks)-1] > refractory {
				peaks = append(peaks, r)
				sigLvl = 0.875*sigLvl + 0.125*feat[pk]
			} else if math.Abs(x[r]) > math.Abs(x[peaks[len(peaks)-1]]) {
				peaks[len(peaks)-1] = r
			}
			thr = noiseLvl + 0.35*(sigLvl-noiseLvl)
			i = pk + refractory
			continue
		}
		noiseLvl = 0.98*noiseLvl + 0.02*feat[i]
		i++
	}

	// Enforce a consistent polarity: the dominant sign of detected peaks wins;
	// a detection with the opposite sign that sits next to a same-sign
	// deflection is re-snapped to it.
	pos := 0
	for _, p := range peaks {
		if x[p] > 0 {
			pos++
		}
	}
	wantPos := pos*2 >= len(peaks)
	for k, p := range peaks {
		if (x[p] > 0) == wantPos {
			continue
		}
		lo, hi := max(0, p-search), min(n-1, p+search)
		best := -1
		for j := lo; j <= hi; j++ {
			if (x[j] > 0) == wantPos && (best < 0 || math.Abs(x[j]) > math.Abs(x[best])) {
				best = j
			}
		}
		if best >= 0 {
			peaks[k] = best
		}
	}
	return peaks
}

func movingAverage(x []float64, w int) []float64 {
	n := len(x)
	if w < 2 {
		return append([]float64(nil), x...)
	}
	out := make([]float64, n)
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += x[i]
		if i >= w {
			sum -= x[i-w]
		}
		cnt := w
		if i < w {
			cnt = i + 1
		}
		out[i] = sum / float64(cnt)
	}
	return out
}

// noiseEstimate measures the high-frequency residual (signal minus a 20 ms
// smoothed copy) away from QRS complexes.
func noiseEstimate(x []float64, peaks []int, rate float64) float64 {
	sm := movingAverage(x, max(3, int(0.02*rate)))
	guard := int(0.1 * rate)
	inQRS := make([]bool, len(x))
	for _, p := range peaks {
		for j := max(0, p-guard); j < min(len(x), p+guard); j++ {
			inQRS[j] = true
		}
	}
	var res []float64
	for i := range x {
		if !inQRS[i] {
			res = append(res, math.Abs(x[i]-sm[i]))
		}
	}
	if len(res) == 0 {
		return 0
	}
	// Robust: median absolute residual × 1.4826 ≈ σ; ×3 gives a peak-noise feel.
	return 1.4826 * median(res) * 3
}

func countPremature(rr []float64) int {
	if len(rr) < 4 {
		return 0
	}
	count := 0
	for i := 1; i < len(rr)-1; i++ {
		lo, hi := max(0, i-4), min(len(rr), i+5)
		var local []float64
		for j := lo; j < hi; j++ {
			if j != i {
				local = append(local, rr[j])
			}
		}
		m := median(local)
		if rr[i] < 0.8*m && rr[i+1] > 1.05*m {
			count++
		}
	}
	return count
}

// averageBeat aligns beats on the R peak and takes the per-sample median so
// noise and the odd premature beat drop out. Beats whose preceding RR is far
// from the median are excluded.
func averageBeat(x []float64, peaks []int, rr []float64, rate float64) ([]float64, float64, float64) {
	pre := int(0.3 * rate)
	post := int(0.5 * rate)
	medRR := median(rr)
	var used [][]float64
	for i, p := range peaks {
		if p-pre < 0 || p+post >= len(x) {
			continue
		}
		if i > 0 && math.Abs(rr[i-1]-medRR) > 0.2*medRR {
			continue
		}
		used = append(used, x[p-pre:p+post])
	}
	if len(used) < 3 {
		return []float64{}, 0, 0
	}
	// Downsample the template to ~2 ms resolution for a compact payload.
	step := max(1, int(rate/500))
	length := (pre + post) / step
	tpl := make([]float64, length)
	col := make([]float64, len(used))
	for k := 0; k < length; k++ {
		for b, beat := range used {
			col[b] = beat[k*step]
		}
		tpl[k] = median(col)
	}
	return tpl, -float64(pre) / rate * 1000, float64(step) / rate * 1000
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range xs {
		s += v
	}
	return s / float64(len(xs))
}

func stddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := mean(xs)
	s := 0.0
	for _, v := range xs {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(xs)-1))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2
}
