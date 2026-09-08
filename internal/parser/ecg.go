package parser

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/BRO3886/healthsync/internal/ecg"
	"github.com/BRO3886/healthsync/internal/storage"
)

// looksLikeECG sniffs a CSV head for the Apple ECG export header.
func looksLikeECG(head []byte) bool {
	return bytes.Contains(head, []byte("Sample Rate")) || bytes.Contains(head, []byte("Lead I"))
}

// isECGPath reports whether a file path looks like an ECG export
// (electrocardiograms/ecg_2026-08-26.csv). The directory name is localized on
// some devices, so the content sniff in looksLikeECG is the authority; this is
// only a cheap pre-filter on the extension.
func isECGPath(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".csv")
}

// parseECG reads an Apple Watch ECG CSV.
//
// The file is a short key/value header followed by one voltage per line:
//
//	Name,André Perdigão
//	Recorded Date,2026-08-26 18:15:35 +0100
//	Classification,Sinus Rhythm
//	Sample Rate,512 hertz
//	Lead,Lead I
//	Unit,µV
//
//	-265,868
//
// The decimal separator follows the device locale: "-265,868" (pt) is the
// same sample as "-265.868" (en). A line is a sample once it is purely
// numeric; everything before the first sample is header.
func parseECG(r io.Reader, fileName string) (*storage.ECGRow, error) {
	row := &storage.ECGRow{FileName: fileName}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	var samples []float32
	inSamples := false
	var sum float64

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if v, ok := parseSampleLine(line); ok {
			inSamples = true
			samples = append(samples, float32(v))
			sum += v
			continue
		}
		if inSamples {
			// Non-numeric trailing content after samples: ignore.
			continue
		}
		key, val := splitHeaderLine(line)
		switch strings.ToLower(key) {
		case "recorded date":
			row.RecordedDate = normalizeTimestamp(val)
		case "classification":
			row.Classification = val
		case "symptoms":
			row.Symptoms = val
		case "software version":
			row.SoftwareVersion = val
		case "device":
			row.Device = val
		case "sample rate":
			row.SampleRateHz = leadingFloat(val)
		case "lead":
			row.Lead = val
		case "unit":
			row.Unit = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading ecg: %w", err)
	}
	if row.RecordedDate == "" {
		return nil, fmt.Errorf("ecg %s: no Recorded Date header", fileName)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("ecg %s: no samples", fileName)
	}

	buf := make([]byte, 4*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(s))
	}
	row.Samples = buf
	row.SampleCount = len(samples)
	if row.SampleRateHz > 0 {
		if a := ecg.Analyze(samples, row.SampleRateHz); a.Beats >= 4 && a.Quality != "poor" {
			hr := math.Round(a.HRMean)
			row.AverageHR = &hr
		}
	}
	return row, nil
}

// splitHeaderLine parses `Key,Value` where Value may be quoted ("Watch7,11").
func splitHeaderLine(line string) (string, string) {
	rec, err := csv.NewReader(strings.NewReader(line)).Read()
	if err != nil || len(rec) == 0 {
		i := strings.IndexByte(line, ',')
		if i < 0 {
			return line, ""
		}
		return line[:i], strings.TrimSpace(line[i+1:])
	}
	if len(rec) == 1 {
		return rec[0], ""
	}
	return rec[0], strings.TrimSpace(strings.Join(rec[1:], ","))
}

// parseSampleLine recognises "-265,868", "-265.868", "-265" as a voltage.
func parseSampleLine(line string) (float64, bool) {
	if line == "" {
		return 0, false
	}
	s := line
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9', c == '.':
		case c == '-' && i == 0:
		default:
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func leadingFloat(s string) float64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.Replace(fields[0], ",", ".", 1), 64)
	return v
}

// DecodeECGSamples turns the stored little-endian float32 blob back into values.
func DecodeECGSamples(b []byte) []float32 {
	out := make([]float32, len(b)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}
