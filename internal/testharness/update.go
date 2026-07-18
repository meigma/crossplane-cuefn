package testharness

import (
	"slices"

	"github.com/rogpeppe/go-internal/txtar"
)

// SeedGoldens returns the txtar file rewritten with a want.yaml section for
// every unit that needs one, placed after the unit's observed.yaml section
// when present (or appended). Hand-written sections are never touched. The
// second return reports whether anything was added.
func SeedGoldens(raw []byte, res *CaseResult) ([]byte, bool) {
	arch := txtar.Parse(raw)
	changed := false
	for _, u := range res.Units {
		if !u.NeedsSeed || u.Golden == nil {
			continue
		}
		insertSection(arch, txtar.File{Name: sectionName(u.Label, sectionWantYAML), Data: u.Golden},
			sectionName(u.Label, sectionObserved))
		changed = true
	}
	if !changed {
		return raw, false
	}
	return txtar.Format(arch), true
}

// UpdateGoldens returns the txtar file with every drifted want.yaml section
// replaced by the unit's rendered output. want.cue and error.txt are
// hand-written intent and are never rewritten. The second return reports
// whether anything changed.
func UpdateGoldens(raw []byte, res *CaseResult) ([]byte, bool) {
	arch := txtar.Parse(raw)
	changed := false
	for _, u := range res.Units {
		if u.Golden == nil {
			continue
		}
		name := sectionName(u.Label, sectionWantYAML)
		for i, file := range arch.Files {
			if file.Name != name {
				continue
			}
			if normalizeText(file.Data) != normalizeText(u.Golden) {
				arch.Files[i].Data = u.Golden
				changed = true
			}
		}
	}
	if !changed {
		return raw, false
	}
	return txtar.Format(arch), true
}

// sectionName joins a unit label and section into the txtar file name.
func sectionName(label, section string) string {
	if label == "" {
		return section
	}
	return label + "/" + section
}

// insertSection places file directly after the section named after when it
// exists, else appends it.
func insertSection(arch *txtar.Archive, file txtar.File, after string) {
	for i, existing := range arch.Files {
		if existing.Name == after {
			arch.Files = slices.Insert(arch.Files, i+1, file)
			return
		}
	}
	arch.Files = append(arch.Files, file)
}
