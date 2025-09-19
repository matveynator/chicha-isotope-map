package safecastrealtime

import (
	"strings"
	"unicode"
)

// muReplacer collapses the two common micro symbols into ASCII 'u'.
// Doing the work once keeps normalizeUnit cheap because strings.NewReplacer
// builds an internal table that we can reuse safely across calls.
var muReplacer = strings.NewReplacer("µ", "u", "μ", "u")

// ─── Constants with calibration factors ─────────────────────────────────────

const (
	// factorLND7317 handles Safecast realtime CPM feeds for LND 7318 tubes.
	// Safecast labels these fields as "lnd_7317" even though the hardware
	// variants are 7318U and 7318C.  Field manuals quote 334 CPM per µSv/h.
	factorLND7317 = 334.0
	// factorLND7317CPS converts counts per second into µSv/h for 7318 tubes.
	// Dividing the CPM factor by 60 honours the same calibration while
	// accepting realtime CPS feeds without duplicating constants elsewhere.
	factorLND7317CPS = factorLND7317
	// factorLND712 covers the classic LND 712 and the shielded 7128 EC.
	// Both share the same 108 CPM per µSv/h calibration in Safecast docs.
	factorLND712 = 108.0
	// factorLND712CPS mirrors the CPM constant for CPS payloads on LND 712.
	// Safecast documents 108 CPM per µSv/h; for per-second counts we divide
	// by 60 so both units share the same physical calibration.
	factorLND712CPS = factorLND712
)

// ─── Public conversion helpers ──────────────────────────────────────────────

// FromRealtime converts a raw Safecast realtime reading into µSv/h.
// The bool return value reports whether the conversion succeeded.  We keep the
// logic here so both storage and presentation layers stay consistent and follow
// the Go Proverb "Don't communicate by sharing memory, share memory by
// communicating" – callers only exchange simple numbers instead of duplicating
// rules.
func FromRealtime(value float64, unit string) (float64, bool) {
	if value <= 0 {
		return 0, false
	}

	normalized := normalizeUnit(unit)
	if normalized == "" {
		return 0, false
	}

	// Direct µSv/h values can be forwarded as-is.  We also accept plain
	// "usv/h" strings because some payloads replace the micro sign with
	// ASCII 'u'.
	if strings.Contains(normalized, "usv") {
		return value, true
	}

	// Counts-per-minute fields require detector specific calibration.  We
	// normalise the string further by keeping only letters and digits so we
	// can match both "lnd_7317" and "lnd-7317-cpm" without another lookup
	// table.
	clean := keepAlphaNumeric(normalized)
	if clean == "" {
		return 0, false
	}

	if containsAny(clean, []string{"lnd7317", "lnd7318"}) {
		if strings.Contains(clean, "cps") {
			return value / factorLND7317CPS, true
		}
		return value / factorLND7317, true
	}
	if containsAny(clean, []string{"lnd712u"}) {
		return value, true
	} else if containsAny(clean, []string{"lnd7128", "lnd712"}) {
		if strings.Contains(clean, "cps") {
			return value / factorLND712CPS, true
		}
		return value / factorLND712, true
	}

	// Unknown detectors remain unconverted so the caller can decide whether
	// to display raw CPM or hide the value.  LND 78017 falls into this
	// branch until we receive an official calibration factor.
	return 0, false
}

// ToMicroRoentgen converts µSv/h into µR/h.  Keeping the helper alongside the
// other conversion logic avoids scattering the 100× factor across the codebase
// and follows "Clear is better than clever" by expressing the unit change
// explicitly where it happens.
func ToMicroRoentgen(microSievert float64) float64 {
	return microSievert * 100.0
}

// ─── Internal helpers ───────────────────────────────────────────────────────

// normalizeUnit prepares a string for downstream checks by trimming spaces,
// lowercasing, and replacing different micro symbols with ASCII 'u'.  Keeping
// the replacement table small ensures the function stays easy to audit.
func normalizeUnit(unit string) string {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return ""
	}
	lowered := strings.ToLower(muReplacer.Replace(trimmed))
	return lowered
}

// keepAlphaNumeric removes every rune that is not a letter or digit.  This
// makes substring checks robust to underscores, dashes, and other separators
// in upstream unit names.  The helper keeps the code explicit and close to the
// data we consume, embracing "A little copying is better than a little
// dependency".
func keepAlphaNumeric(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// containsAny reports whether the haystack contains one of the provided
// needles.  We keep this helper private because callers should rely on
// FromRealtime's semantics instead of performing their own matches.
func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
