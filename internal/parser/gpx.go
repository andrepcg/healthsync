package parser

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/BRO3886/healthsync/internal/storage"
)

// gpxTrackPoint mirrors <trkpt> as written by "Apple Health Export".
type gpxTrackPoint struct {
	Lat  string `xml:"lat,attr"`
	Lon  string `xml:"lon,attr"`
	Ele  string `xml:"ele"`
	Time string `xml:"time"`
	Ext  struct {
		Speed  string `xml:"speed"`
		Course string `xml:"course"`
		HAcc   string `xml:"hAcc"`
		VAcc   string `xml:"vAcc"`
	} `xml:"extensions"`
}

// RouteSummary is what parseGPX derives from a track.
type RouteSummary struct {
	Points         []storage.RoutePoint
	DistanceM      float64
	ElevationGainM float64
}

// maxHorizontalAccuracy is the GPS accuracy (metres) beyond which a point is
// too noisy to contribute to the distance total.
const maxHorizontalAccuracy = 50.0

// parseGPX streams the track points out of a GPX document and computes total
// distance (haversine, skipping low-accuracy points) and elevation gain
// (positive deltas of a 3-point smoothed altitude, because raw watch altitude
// jitters by a metre or two between samples).
func parseGPX(r io.Reader) (*RouteSummary, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	out := &RouteSummary{}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decoding gpx: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "trkpt" {
			continue
		}
		var tp gpxTrackPoint
		if err := dec.DecodeElement(&tp, &se); err != nil {
			return nil, fmt.Errorf("decoding trkpt: %w", err)
		}
		lat, err1 := strconv.ParseFloat(tp.Lat, 64)
		lon, err2 := strconv.ParseFloat(tp.Lon, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out.Points = append(out.Points, storage.RoutePoint{
			Seq:    len(out.Points),
			Time:   tp.Time,
			Lat:    lat,
			Lon:    lon,
			Ele:    parseFloat(tp.Ele),
			Speed:  parseFloat(tp.Ext.Speed),
			Course: parseFloat(tp.Ext.Course),
			HAcc:   parseFloat(tp.Ext.HAcc),
			VAcc:   parseFloat(tp.Ext.VAcc),
		})
	}

	out.DistanceM = routeDistance(out.Points)
	out.ElevationGainM = elevationGain(out.Points)
	return out, nil
}

func routeDistance(pts []storage.RoutePoint) float64 {
	var total float64
	var prev *storage.RoutePoint
	for i := range pts {
		p := &pts[i]
		if acc, ok := p.HAcc.(float64); ok && acc > maxHorizontalAccuracy {
			continue
		}
		if prev != nil {
			total += haversine(prev.Lat, prev.Lon, p.Lat, p.Lon)
		}
		prev = p
	}
	return total
}

func elevationGain(pts []storage.RoutePoint) float64 {
	var eles []float64
	for _, p := range pts {
		if e, ok := p.Ele.(float64); ok {
			eles = append(eles, e)
		}
	}
	if len(eles) < 2 {
		return 0
	}
	smooth := make([]float64, len(eles))
	for i := range eles {
		lo, hi := i-1, i+1
		if lo < 0 {
			lo = 0
		}
		if hi >= len(eles) {
			hi = len(eles) - 1
		}
		sum := 0.0
		for j := lo; j <= hi; j++ {
			sum += eles[j]
		}
		smooth[i] = sum / float64(hi-lo+1)
	}
	var gain float64
	for i := 1; i < len(smooth); i++ {
		if d := smooth[i] - smooth[i-1]; d > 0 {
			gain += d
		}
	}
	return gain
}

// haversine returns the great-circle distance in metres.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
