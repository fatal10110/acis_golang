//go:build race

package probe

// raceDetector reports a -race build, which samples the real-geodata
// equivalence test instead of walking every region.
const raceDetector = true
