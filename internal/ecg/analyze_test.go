package ecg

import (
	"math"
	"math/rand"
	"testing"
)

// synth builds a 30 s ECG-like signal in µV: for each beat a narrow positive
// QRS spike, a small P bump before and a broader T bump after, plus noise and
// slow baseline wander. rrs are the RR intervals in seconds.
func synth(rate float64, rrs []float64, noiseUV float64, seed int64) []float32 {
	rng := rand.New(rand.NewSource(seed))
	n := int(30 * rate)
	x := make([]float32, n)
	t := 0.4
	var beats []float64
	for _, rr := range rrs {
		beats = append(beats, t)
		t += rr
	}
	gauss := func(dt, sigma, amp float64) float64 { return amp * math.Exp(-dt*dt/(2*sigma*sigma)) }
	for i := 0; i < n; i++ {
		ts := float64(i) / rate
		v := 0.0
		for _, b := range beats {
			dt := ts - b
			if dt < -0.4 || dt > 0.6 {
				continue
			}
			v += gauss(dt, 0.012, 1000)      // R
			v += gauss(dt+0.03, 0.012, -150) // Q
			v += gauss(dt-0.03, 0.012, -250) // S
			v += gauss(dt+0.16, 0.025, 120)  // P
			v += gauss(dt-0.30, 0.06, 250)   // T
		}
		v += 200 * math.Sin(2*math.Pi*0.25*ts) // baseline wander
		v += rng.NormFloat64() * noiseUV
		x[i] = float32(v)
	}
	return x
}

func regular(rrSec float64, seconds float64) []float64 {
	var out []float64
	for t := 0.4; t+rrSec < seconds-0.4; t += rrSec {
		out = append(out, rrSec)
	}
	return out
}

func TestRegularRhythm(t *testing.T) {
	rr := regular(1.0, 30) // 60 bpm
	a := Analyze(synth(512, rr, 15, 1), 512)
	if a.Beats < len(rr) || a.Beats > len(rr)+1 {
		t.Fatalf("beats %d want ~%d", a.Beats, len(rr))
	}
	if math.Abs(a.HRMean-60) > 1 {
		t.Errorf("hr %.1f", a.HRMean)
	}
	if a.RMSSD > 10 || a.RRCV > 2 || a.Irregularity != "regular" {
		t.Errorf("should be regular: rmssd %.1f cv %.1f %s", a.RMSSD, a.RRCV, a.Irregularity)
	}
	if a.PrematureBeats != 0 {
		t.Errorf("premature %d", a.PrematureBeats)
	}
	if a.Quality != "good" {
		t.Errorf("quality %s snr %.1f noise %.3f", a.Quality, a.SNR, a.NoiseMV)
	}
	if len(a.Template) == 0 || a.TemplateT0 > -290 {
		t.Errorf("template missing: %d %v", len(a.Template), a.TemplateT0)
	}
	// R at template time 0 should be the maximum.
	maxI := 0
	for i, v := range a.Template {
		if v > a.Template[maxI] {
			maxI = i
		}
	}
	if tAtMax := a.TemplateT0 + float64(maxI)*a.TemplateDT; math.Abs(tAtMax) > 6 {
		t.Errorf("template not aligned on R: peak at %.1f ms", tAtMax)
	}
	if math.Abs(a.RAmplitudeMV-1.0) > 0.15 {
		t.Errorf("R amplitude %.2f mV", a.RAmplitudeMV)
	}
}

func TestPrematureBeatsAndIrregularity(t *testing.T) {
	// 75 bpm with two premature beats (short RR then compensatory pause).
	var rr []float64
	for i := 0; i < 34; i++ {
		switch i {
		case 10, 22:
			rr = append(rr, 0.5, 1.1)
		default:
			rr = append(rr, 0.8)
		}
	}
	a := Analyze(synth(512, rr, 15, 2), 512)
	if a.PrematureBeats != 2 {
		t.Errorf("premature %d want 2 (rr=%v)", a.PrematureBeats, a.RRIntervals)
	}
	if a.HRMax < 100 || a.HRMin > 60 {
		t.Errorf("hr range %.0f–%.0f", a.HRMin, a.HRMax)
	}

	// Very irregular (AF-like): RR uniformly random 0.4–1.1 s.
	rng := rand.New(rand.NewSource(3))
	var irr []float64
	for s := 0.4; s < 28; {
		d := 0.4 + rng.Float64()*0.7
		irr = append(irr, d)
		s += d
	}
	b := Analyze(synth(512, irr, 15, 4), 512)
	if b.Irregularity != "very irregular" || b.RRCV < 20 {
		t.Errorf("irregular rhythm not flagged: %s cv %.1f", b.Irregularity, b.RRCV)
	}
}

func TestInvertedAndNoisy(t *testing.T) {
	x := synth(512, regular(0.75, 30), 15, 5)
	for i := range x {
		x[i] = -x[i]
	}
	a := Analyze(x, 512)
	if math.Abs(a.HRMean-80) > 1.5 {
		t.Errorf("inverted lead hr %.1f", a.HRMean)
	}
	noisy := Analyze(synth(512, regular(0.75, 30), 400, 6), 512)
	if noisy.Quality == "good" {
		t.Errorf("heavy noise should not be rated good (snr %.1f)", noisy.SNR)
	}
	empty := Analyze(nil, 512)
	if empty.Quality != "poor" || empty.Beats != 0 {
		t.Error("empty input")
	}
	flat := Analyze(make([]float32, 512*30), 512)
	if flat.Quality != "poor" {
		t.Error("flat line must be poor")
	}
}
