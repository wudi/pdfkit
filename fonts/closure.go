package fonts

import (
	"bytes"
	"fmt"

	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// ComputeClosureGSUB computes the transitive closure of glyphs reachable via GSUB substitutions.
func ComputeClosureGSUB(fontData []byte, initialGIDs map[int]bool) (map[int]bool, error) {
	reader := bytes.NewReader(fontData)
	loader, err := opentype.NewLoader(reader)
	if err != nil {
		return nil, fmt.Errorf("create loader: %w", err)
	}

	// Check for GSUB table
	gsubTag := opentype.NewTag('G', 'S', 'U', 'B')
	if !loader.HasTable(gsubTag) {
		// No GSUB, return initial set
		closure := make(map[int]bool)
		for gid := range initialGIDs {
			closure[gid] = true
		}
		return closure, nil
	}

	gsubBytes, err := loader.RawTable(gsubTag)
	if err != nil {
		return nil, fmt.Errorf("read GSUB table: %w", err)
	}

	layout, _, err := tables.ParseLayout(gsubBytes)
	if err != nil {
		return nil, fmt.Errorf("parse GSUB layout: %w", err)
	}

	// Prepare GSUB lookup entries so we can reference them by index (needed for contextual lookups)
	lookupEntries := make([]gsubLookupEntry, len(layout.LookupList.Lookups))
	for i, lookup := range layout.LookupList.Lookups {
		gsubLookups, err := lookup.AsGSUBLookups()
		if err != nil {
			continue
		}
		lookupEntries[i] = gsubLookupEntry{subtables: gsubLookups}
	}

	closure := make(map[int]bool)
	for gid := range initialGIDs {
		closure[gid] = true
	}

	// Fixed-point iteration
	changed := true
	for changed {
		changed = false
		// Snapshot current GIDs to iterate safely
		currentGIDs := make([]uint16, 0, len(closure))
		for gid := range closure {
			currentGIDs = append(currentGIDs, uint16(gid))
		}
		visiting := make(map[int]bool)
		for idx := range lookupEntries {
			if processLookup(idx, currentGIDs, closure, lookupEntries, visiting) {
				changed = true
			}
		}
	}

	return closure, nil
}

type gsubLookupEntry struct {
	subtables []tables.GSUBLookup
}

func processLookup(idx int, currentGIDs []uint16, closure map[int]bool, entries []gsubLookupEntry, visiting map[int]bool) bool {
	if idx < 0 || idx >= len(entries) {
		return false
	}
	if visiting[idx] {
		return false
	}
	entry := entries[idx]
	if len(entry.subtables) == 0 {
		return false
	}
	visiting[idx] = true
	changed := false
	for _, subtable := range entry.subtables {
		if processSubtable(subtable, currentGIDs, closure, func(target int, glyphs []uint16) bool {
			return processLookup(target, glyphs, closure, entries, visiting)
		}) {
			changed = true
		}
	}
	visiting[idx] = false
	return changed
}

type lookupInvoker func(idx int, glyphs []uint16) bool

func processSubtable(subtable tables.GSUBLookup, currentGIDs []uint16, closure map[int]bool, invoke lookupInvoker) bool {
	changed := false
	cov := subtable.Cov()

	for _, gid := range currentGIDs {
		idx, ok := cov.Index(tables.GlyphID(gid))
		if !ok {
			continue
		}

		switch t := subtable.(type) {
		case tables.SingleSubs:
			switch d := t.Data.(type) {
			case tables.SingleSubstData1:
				// Output = Input + Delta
				out := int(gid) + int(d.DeltaGlyphID)
				if !closure[out] {
					closure[out] = true
					changed = true
				}
			case tables.SingleSubstData2:
				// Output = Substitutes[idx]
				if idx < len(d.SubstituteGlyphIDs) {
					out := d.SubstituteGlyphIDs[idx]
					if !closure[int(out)] {
						closure[int(out)] = true
						changed = true
					}
				}
			}

		case tables.MultipleSubs:
			if idx < len(t.Sequences) {
				seq := t.Sequences[idx]
				for _, out := range seq.SubstituteGlyphIDs {
					if !closure[int(out)] {
						closure[int(out)] = true
						changed = true
					}
				}
			}

		case tables.AlternateSubs:
			if idx < len(t.AlternateSets) {
				set := t.AlternateSets[idx]
				for _, out := range set.AlternateGlyphIDs {
					if !closure[int(out)] {
						closure[int(out)] = true
						changed = true
					}
				}
			}

		case tables.LigatureSubs:
			if idx < len(t.LigatureSets) {
				set := t.LigatureSets[idx]
				for _, lig := range set.Ligatures {
					// Check if all components are in closure
					allPresent := true
					for _, comp := range lig.ComponentGlyphIDs {
						if !closure[int(comp)] {
							allPresent = false
							break
						}
					}
					if allPresent {
						if !closure[int(lig.LigatureGlyph)] {
							closure[int(lig.LigatureGlyph)] = true
							changed = true
						}
					}
				}
			}

		case tables.ExtensionSubs:
			// Unwrap extension
			ext := tables.Extension(t)
			if int(ext.ExtensionOffset) < len(ext.RawData) {
				innerData := ext.RawData[ext.ExtensionOffset:]
				var innerSubtable tables.GSUBLookup
				var err error

				switch ext.ExtensionLookupType {
				case 1: // Single
					var s tables.SingleSubs
					s, _, err = tables.ParseSingleSubs(innerData)
					innerSubtable = s
				case 2: // Multiple
					var s tables.MultipleSubs
					s, _, err = tables.ParseMultipleSubs(innerData)
					innerSubtable = s
				case 3: // Alternate
					var s tables.AlternateSubs
					s, _, err = tables.ParseAlternateSubs(innerData)
					innerSubtable = s
				case 4: // Ligature
					var s tables.LigatureSubs
					s, _, err = tables.ParseLigatureSubs(innerData)
					innerSubtable = s
				// Contextual (5, 6) and ReverseChain (8) are skipped for now
				default:
					err = fmt.Errorf("unsupported extension type")
				}

				if err == nil && innerSubtable != nil {
					// Recurse
					if processSubtable(innerSubtable, []uint16{gid}, closure, invoke) {
						changed = true
					}
				}
			}

		case tables.ContextualSubs:
			if processContextualSubstitution(t.Data, idx, currentGIDs, invoke) {
				changed = true
			}

		case tables.ChainedContextualSubs:
			if processChainedContextualSubstitution(t.Data, idx, currentGIDs, invoke) {
				changed = true
			}

		case tables.ReverseChainSingleSubs:
			if idx < len(t.SubstituteGlyphIDs) {
				out := t.SubstituteGlyphIDs[idx]
				if !closure[int(out)] {
					closure[int(out)] = true
					changed = true
				}
			}
		}
	}
	return changed
}

func processContextualSubstitution(data tables.ContextualSubsITF, coverageIndex int, currentGIDs []uint16, invoke lookupInvoker) bool {
	switch t := data.(type) {
	case tables.ContextualSubs1:
		fmt1 := tables.SequenceContextFormat1(t)
		if coverageIndex >= 0 && coverageIndex < len(fmt1.SeqRuleSet) {
			return triggerSequenceRuleSet(fmt1.SeqRuleSet[coverageIndex], currentGIDs, invoke)
		}
	case tables.ContextualSubs2:
		fmt2 := tables.SequenceContextFormat2(t)
		return triggerAllSequenceRuleSets(fmt2.ClassSeqRuleSet, currentGIDs, invoke)
	case tables.ContextualSubs3:
		fmt3 := tables.SequenceContextFormat3(t)
		return triggerSequenceLookupRecords(fmt3.SeqLookupRecords, currentGIDs, invoke)
	}
	return false
}

func processChainedContextualSubstitution(data tables.ChainedContextualSubsITF, coverageIndex int, currentGIDs []uint16, invoke lookupInvoker) bool {
	switch t := data.(type) {
	case tables.ChainedContextualSubs1:
		fmt1 := tables.ChainedSequenceContextFormat1(t)
		if coverageIndex >= 0 && coverageIndex < len(fmt1.ChainedSeqRuleSet) {
			return triggerChainedSequenceRuleSet(fmt1.ChainedSeqRuleSet[coverageIndex], currentGIDs, invoke)
		}
	case tables.ChainedContextualSubs2:
		fmt2 := tables.ChainedSequenceContextFormat2(t)
		return triggerAllChainedRuleSets(fmt2.ChainedClassSeqRuleSet, currentGIDs, invoke)
	case tables.ChainedContextualSubs3:
		fmt3 := tables.ChainedSequenceContextFormat3(t)
		return triggerSequenceLookupRecords(fmt3.SeqLookupRecords, currentGIDs, invoke)
	}
	return false
}

func triggerSequenceRuleSet(set tables.SequenceRuleSet, currentGIDs []uint16, invoke lookupInvoker) bool {
	changed := false
	for _, rule := range set.SeqRule {
		if triggerSequenceLookupRecords(rule.SeqLookupRecords, currentGIDs, invoke) {
			changed = true
		}
	}
	return changed
}

func triggerAllSequenceRuleSets(sets []tables.SequenceRuleSet, currentGIDs []uint16, invoke lookupInvoker) bool {
	changed := false
	for _, set := range sets {
		if triggerSequenceRuleSet(set, currentGIDs, invoke) {
			changed = true
		}
	}
	return changed
}

func triggerChainedSequenceRuleSet(set tables.ChainedSequenceRuleSet, currentGIDs []uint16, invoke lookupInvoker) bool {
	changed := false
	for _, rule := range set.ChainedSeqRules {
		if triggerSequenceLookupRecords(rule.SeqLookupRecords, currentGIDs, invoke) {
			changed = true
		}
	}
	return changed
}

func triggerAllChainedRuleSets(sets []tables.ChainedSequenceRuleSet, currentGIDs []uint16, invoke lookupInvoker) bool {
	changed := false
	for _, set := range sets {
		if triggerChainedSequenceRuleSet(set, currentGIDs, invoke) {
			changed = true
		}
	}
	return changed
}

func triggerSequenceLookupRecords(records []tables.SequenceLookupRecord, currentGIDs []uint16, invoke lookupInvoker) bool {
	changed := false
	for _, record := range records {
		if invoke(int(record.LookupListIndex), currentGIDs) {
			changed = true
		}
	}
	return changed
}
