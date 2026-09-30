package matcheval

// Score evaluates matching outcomes against ground-truth corpus cases.
func Score(cases []Case, outcomes []Outcome) Report {
	outcomeMap := make(map[string]Outcome, len(outcomes))
	for _, o := range outcomes {
		outcomeMap[o.CaseID] = o
	}

	report := Report{
		Overall:         computeCohortStats(cases, outcomeMap),
		ByWorkflow:      make(map[string]CohortStats),
		ByLanguage:      make(map[string]CohortStats),
		ByDifficulty:    make(map[string]CohortStats),
		ByIdentifier:    make(map[string]CohortStats),
		ByPackSensitive: make(map[string]CohortStats),
		Gates:           make(map[string]GateResult),
	}

	// Group cases by cohort dimensions
	workflowGroups := make(map[string][]Case)
	langGroups := make(map[string][]Case)
	diffGroups := make(map[string][]Case)
	identGroups := make(map[string][]Case)
	packGroups := make(map[string][]Case)

	for _, c := range cases {
		for _, w := range c.Workflow {
			workflowGroups[w] = append(workflowGroups[w], c)
		}
		if c.Language != "" {
			langGroups[c.Language] = append(langGroups[c.Language], c)
		}
		for _, d := range c.Label.Difficulty {
			diffGroups[d] = append(diffGroups[d], c)
		}

		if hasIdentifier(c) {
			identGroups["with_identifier"] = append(identGroups["with_identifier"], c)
		} else {
			identGroups["without_identifier"] = append(identGroups["without_identifier"], c)
		}

		if isPackSensitive(c) {
			packGroups["pack_sensitive"] = append(packGroups["pack_sensitive"], c)
		} else {
			packGroups["pack_insensitive"] = append(packGroups["pack_insensitive"], c)
		}
	}

	for k, grp := range workflowGroups {
		report.ByWorkflow[k] = computeCohortStats(grp, outcomeMap)
	}
	for k, grp := range langGroups {
		report.ByLanguage[k] = computeCohortStats(grp, outcomeMap)
	}
	for k, grp := range diffGroups {
		report.ByDifficulty[k] = computeCohortStats(grp, outcomeMap)
	}
	for k, grp := range identGroups {
		report.ByIdentifier[k] = computeCohortStats(grp, outcomeMap)
	}
	for k, grp := range packGroups {
		report.ByPackSensitive[k] = computeCohortStats(grp, outcomeMap)
	}

	// Evaluate Quality Gates on Overall cohort
	// SC-001: Precision >= 99.5% with 95% lower bound >= 99.0%
	if report.Overall.Accepted < 1000 {
		report.Gates["SC-001"] = GateInsufficientSample
	} else if report.Overall.Precision >= 0.995 && report.Overall.PrecisionLB95 >= 0.990 {
		report.Gates["SC-001"] = GatePass
	} else {
		report.Gates["SC-001"] = GateFail
	}

	// SC-002: Hard conflict protection (100% of cases avoid incorrect automatic acceptance)
	if len(report.Overall.HardConflictViolations) == 0 {
		report.Gates["SC-002"] = GatePass
	} else {
		report.Gates["SC-002"] = GateFail
	}

	// SC-003: Correct automatic coverage >= 80% of resolvable rows
	if report.Overall.NResolvable < 100 {
		report.Gates["SC-003"] = GateInsufficientSample
	} else if report.Overall.CoverageResolvable >= 0.80 {
		report.Gates["SC-003"] = GatePass
	} else {
		report.Gates["SC-003"] = GateFail
	}

	// SC-004: Candidate recall at 20 >= 98%
	if report.Overall.NResolvable < 100 {
		report.Gates["SC-004"] = GateInsufficientSample
	} else if report.Overall.RecallAt20 >= 0.98 {
		report.Gates["SC-004"] = GatePass
	} else {
		report.Gates["SC-004"] = GateFail
	}

	return report
}

func computeCohortStats(cases []Case, outcomes map[string]Outcome) CohortStats {
	var stats CohortStats
	targetInTop20 := 0

	for _, c := range cases {
		if c.Label.Disputed {
			stats.NDisputedExcluded++
			continue
		}
		stats.NCases++
		if c.Label.Resolvable {
			stats.NResolvable++
		}

		o, ok := outcomes[c.ID]
		if !ok {
			continue
		}

		if len(o.HardConflictViolations) > 0 {
			stats.HardConflictViolations = append(stats.HardConflictViolations, HardConflictViolation{
				CaseID:    c.ID,
				Path:      o.Path,
				Conflicts: o.HardConflictViolations,
			})
		}

		// Top 20 candidate recall check
		if c.Label.Resolvable && len(c.Label.Targets) > 0 {
			limit := len(o.TopCandidates)
			if limit > 20 {
				limit = 20
			}
			found := false
			for _, cand := range o.TopCandidates[:limit] {
				for _, target := range c.Label.Targets {
					if cand == target {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				targetInTop20++
			}
		}

		isAccepted := (o.State == "accepted" || o.State == "confirmed") && o.MatchedProductID != nil
		if isAccepted {
			stats.Accepted++
			if c.Label.Resolvable {
				isCorrect := false
				for _, target := range c.Label.Targets {
					if *o.MatchedProductID == target {
						isCorrect = true
						break
					}
				}
				if isCorrect {
					stats.AcceptedCorrect++
				}
			} else {
				stats.AbsentOrAmbiguousAccepted++
			}
		}

		// Confusions between unavailable and none
		if (c.Label.Expect == "unavailable" && o.State == "no_candidate") ||
			(c.Label.Expect == "abstain" && o.State == "unavailable") {
			stats.UnavailableVsNoneConfusions++
		}
	}

	if stats.Accepted > 0 {
		stats.Precision = float64(stats.AcceptedCorrect) / float64(stats.Accepted)
		stats.PrecisionLB95 = ClopperPearsonLower(stats.AcceptedCorrect, stats.Accepted, 0.95)
	}
	if stats.NResolvable > 0 {
		stats.CoverageResolvable = float64(stats.AcceptedCorrect) / float64(stats.NResolvable)
		stats.RecallAt20 = float64(targetInTop20) / float64(stats.NResolvable)
	}
	if stats.NCases > 0 {
		stats.CoverageAllRows = float64(stats.AcceptedCorrect) / float64(stats.NCases)
		stats.ReviewWorkload = float64(stats.NCases-stats.Accepted) / float64(stats.NCases)
	}

	return stats
}

func hasIdentifier(c Case) bool {
	if len(c.Row.Identifiers) > 0 {
		return true
	}
	for col := range c.Row.Cells {
		if col == "barcode" || col == "code" || col == "sku" {
			if c.Row.Cells[col] != "" {
				return true
			}
		}
	}
	return false
}

func isPackSensitive(c Case) bool {
	if c.RequiredGranularity == "pack" {
		return true
	}
	for _, d := range c.Label.Difficulty {
		if d == "pack_sensitive" {
			return true
		}
	}
	return false
}
