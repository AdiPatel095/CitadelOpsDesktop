package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

func summarizeRuns(runs []RunResult) Summary {
	summary := Summary{
		Startup: StartupSummary{
			AppReadyMs:          distribution(startupValues(runs, func(value StartupMetrics) float64 { return value.AppReadyMs })),
			CLSTotal:            distribution(startupValues(runs, func(value StartupMetrics) float64 { return value.CLSTotal })),
			FirstContentfulMs:   distribution(startupValues(runs, func(value StartupMetrics) float64 { return value.FirstContentfulMs })),
			LargestContentfulMs: distribution(startupValues(runs, func(value StartupMetrics) float64 { return value.LargestContentfulMs })),
			TotalBlockingMs:     distribution(startupValues(runs, func(value StartupMetrics) float64 { return value.TotalBlockingMs })),
			TransferBytes:       distribution(startupValues(runs, func(value StartupMetrics) float64 { return float64(value.TransferBytes) })),
		},
		Diagnostics: DiagnosticsSummary{
			ConsoleErrors:   distribution(diagnosticValues(runs, func(value Diagnostics) float64 { return float64(value.ConsoleErrorCount) })),
			HTTPFailures:    distribution(diagnosticValues(runs, func(value Diagnostics) float64 { return float64(value.HTTPFailureCount) })),
			NetworkFailures: distribution(diagnosticValues(runs, func(value Diagnostics) float64 { return float64(value.NetworkFailureCount) })),
			UnhandledExceptions: distribution(diagnosticValues(runs, func(value Diagnostics) float64 {
				return float64(value.UnhandledExceptionCount)
			})),
		},
	}

	for _, definition := range measuredScenarios {
		values := scenarioValues(runs, definition.Name)
		summary.Scenarios = append(summary.Scenarios, ScenarioSummary{
			EventDurationMs: distribution(scenarioMetricValues(values, func(value ScenarioMetrics) float64 { return value.EventDurationMs })),
			LayoutShift:     distribution(scenarioMetricValues(values, func(value ScenarioMetrics) float64 { return value.LayoutShift })),
			LongTaskMs:      distribution(scenarioMetricValues(values, func(value ScenarioMetrics) float64 { return value.LongTaskMs })),
			Name:            definition.Name,
			PaintMs:         distribution(scenarioMetricValues(values, func(value ScenarioMetrics) float64 { return value.PaintMs })),
			ReadyMs:         distribution(scenarioMetricValues(values, func(value ScenarioMetrics) float64 { return value.ReadyMs })),
		})
	}
	return summary
}

func startupValues(runs []RunResult, pick func(StartupMetrics) float64) []float64 {
	values := make([]float64, 0, len(runs))
	for _, run := range runs {
		values = append(values, pick(run.Startup))
	}
	return values
}

func diagnosticValues(runs []RunResult, pick func(Diagnostics) float64) []float64 {
	values := make([]float64, 0, len(runs))
	for _, run := range runs {
		values = append(values, pick(run.Diagnostics))
	}
	return values
}

func scenarioValues(runs []RunResult, name string) []ScenarioMetrics {
	values := make([]ScenarioMetrics, 0, len(runs))
	for _, run := range runs {
		for _, scenario := range run.Scenarios {
			if scenario.Name == name {
				values = append(values, scenario)
				break
			}
		}
	}
	return values
}

func scenarioMetricValues(values []ScenarioMetrics, pick func(ScenarioMetrics) float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		result = append(result, pick(value))
	}
	return result
}

func distribution(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return Distribution{
		Maximum: sorted[len(sorted)-1],
		Median:  percentile(sorted, 0.5),
		Minimum: sorted[0],
		P95:     percentile(sorted, 0.95),
	}
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := float64(len(sorted)-1) * quantile
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func evaluateBudgets(summary Summary) []BudgetResult {
	budgets := []BudgetResult{
		budget("Startup / app ready p95", summary.Startup.AppReadyMs.P95, 2500, "ms"),
		budget("Startup / FCP p95", summary.Startup.FirstContentfulMs.P95, 1800, "ms"),
		budget("Startup / LCP p95", summary.Startup.LargestContentfulMs.P95, 2500, "ms"),
		budget("Startup / CLS p95", summary.Startup.CLSTotal.P95, 0.1, "score"),
		budget("Startup / blocking time p95", summary.Startup.TotalBlockingMs.P95, 300, "ms"),
		budget("Startup / transfer p95", summary.Startup.TransferBytes.P95, 5*1024*1024, "bytes"),
		budget("Diagnostics / console errors p95", summary.Diagnostics.ConsoleErrors.P95, 0, "count"),
		budget("Diagnostics / HTTP failures p95", summary.Diagnostics.HTTPFailures.P95, 0, "count"),
		budget("Diagnostics / network failures p95", summary.Diagnostics.NetworkFailures.P95, 0, "count"),
		budget("Diagnostics / unhandled exceptions p95", summary.Diagnostics.UnhandledExceptions.P95, 0, "count"),
	}

	for _, scenario := range summary.Scenarios {
		readyBudget := 750.0
		switch scenario.Name {
		case "Load Battle Stats":
			readyBudget = 2500
		case "Load My Stats":
			readyBudget = 1200
		}
		budgets = append(budgets,
			budget(scenario.Name+" / paint p95", scenario.PaintMs.P95, 250, "ms"),
			budget(scenario.Name+" / ready p95", scenario.ReadyMs.P95, readyBudget, "ms"),
			budget(scenario.Name+" / interaction p95", scenario.EventDurationMs.P95, 200, "ms"),
			budget(scenario.Name+" / long tasks p95", scenario.LongTaskMs.P95, 200, "ms"),
		)
	}
	return budgets
}

func budget(metric string, value float64, limit float64, unit string) BudgetResult {
	return BudgetResult{
		Budget: limit,
		Metric: metric,
		Passed: value <= limit,
		Unit:   unit,
		Value:  value,
	}
}

func compareReports(current Summary, baseline Summary) []ComparisonItem {
	currentValues := flattenedSummary(current)
	baselineValues := flattenedSummary(baseline)
	keys := make([]string, 0, len(currentValues))
	for key := range currentValues {
		if _, ok := baselineValues[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	comparisons := make([]ComparisonItem, 0, len(keys))
	for _, key := range keys {
		currentValue := currentValues[key]
		baselineValue := baselineValues[key]
		deltaPercent := 0.0
		if baselineValue != 0 {
			deltaPercent = (currentValue - baselineValue) / baselineValue * 100
		} else if currentValue > 0 {
			deltaPercent = 100
		}
		comparisons = append(comparisons, ComparisonItem{
			Baseline:     baselineValue,
			Current:      currentValue,
			Delta:        currentValue - baselineValue,
			DeltaPercent: deltaPercent,
			Metric:       key,
		})
	}
	return comparisons
}

func flattenedSummary(summary Summary) map[string]float64 {
	values := map[string]float64{
		"Startup / app ready p95":                summary.Startup.AppReadyMs.P95,
		"Startup / FCP p95":                      summary.Startup.FirstContentfulMs.P95,
		"Startup / LCP p95":                      summary.Startup.LargestContentfulMs.P95,
		"Startup / CLS p95":                      summary.Startup.CLSTotal.P95,
		"Startup / blocking time p95":            summary.Startup.TotalBlockingMs.P95,
		"Startup / transfer p95":                 summary.Startup.TransferBytes.P95,
		"Diagnostics / console errors p95":       summary.Diagnostics.ConsoleErrors.P95,
		"Diagnostics / HTTP failures p95":        summary.Diagnostics.HTTPFailures.P95,
		"Diagnostics / network failures p95":     summary.Diagnostics.NetworkFailures.P95,
		"Diagnostics / unhandled exceptions p95": summary.Diagnostics.UnhandledExceptions.P95,
	}
	for _, scenario := range summary.Scenarios {
		values[scenario.Name+" / paint p95"] = scenario.PaintMs.P95
		values[scenario.Name+" / ready p95"] = scenario.ReadyMs.P95
		values[scenario.Name+" / interaction p95"] = scenario.EventDurationMs.P95
		values[scenario.Name+" / long tasks p95"] = scenario.LongTaskMs.P95
	}
	return values
}

func rankOpportunities(budgets []BudgetResult, comparisons []ComparisonItem) []Opportunity {
	comparisonByMetric := make(map[string]ComparisonItem, len(comparisons))
	for _, comparison := range comparisons {
		comparisonByMetric[comparison.Metric] = comparison
	}

	opportunities := make([]Opportunity, 0, len(budgets))
	for _, result := range budgets {
		priority := 0.0
		if result.Budget > 0 {
			priority = result.Value / result.Budget
		} else if result.Value > 0 {
			priority = 10 + result.Value
		}
		var baselineDelta *float64
		if comparison, ok := comparisonByMetric[result.Metric]; ok {
			delta := comparison.DeltaPercent
			baselineDelta = &delta
			if delta > 0 {
				priority *= 1 + delta/100
			}
		}
		opportunities = append(opportunities, Opportunity{
			BaselineDeltaPercent: baselineDelta,
			Budget:               result.Budget,
			Metric:               result.Metric,
			Priority:             priority,
			Value:                result.Value,
		})
	}
	sort.SliceStable(opportunities, func(left, right int) bool {
		return opportunities[left].Priority > opportunities[right].Priority
	})
	return opportunities
}

func loadReport(path string) (*Report, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func writeReport(outputDir string, report Report) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(outputDir, "Report.json")
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func printReport(report Report, path string) {
	fmt.Printf("\nCitadel UI performance: %d measured run(s)\n", len(report.Runs))
	fmt.Printf("Startup p95: ready %.0f ms, FCP %.0f ms, LCP %.0f ms, blocking %.0f ms, transfer %s\n",
		report.Summary.Startup.AppReadyMs.P95,
		report.Summary.Startup.FirstContentfulMs.P95,
		report.Summary.Startup.LargestContentfulMs.P95,
		report.Summary.Startup.TotalBlockingMs.P95,
		formatBytes(report.Summary.Startup.TransferBytes.P95),
	)
	fmt.Println("View journeys (p95):")
	for _, scenario := range report.Summary.Scenarios {
		fmt.Printf("  %-20s paint %6.0f ms  ready %6.0f ms  long tasks %6.0f ms\n",
			scenario.Name,
			scenario.PaintMs.P95,
			scenario.ReadyMs.P95,
			scenario.LongTaskMs.P95,
		)
	}

	passed := 0
	for _, result := range report.Budgets {
		if result.Passed {
			passed++
		}
	}
	fmt.Printf("Budgets: %d/%d passed\n", passed, len(report.Budgets))
	fmt.Println("Highest-leverage opportunities:")
	for index, opportunity := range report.Opportunities {
		if index == 5 {
			break
		}
		result := budgetForMetric(report.Budgets, opportunity.Metric)
		baseline := ""
		if opportunity.BaselineDeltaPercent != nil {
			baseline = fmt.Sprintf(", %+.1f%% vs baseline", *opportunity.BaselineDeltaPercent)
		}
		fmt.Printf("  %d. %s: %s / %s%s\n",
			index+1,
			opportunity.Metric,
			formatBudgetValue(opportunity.Value, result.Unit),
			formatBudgetValue(opportunity.Budget, result.Unit),
			baseline,
		)
	}
	fmt.Printf("Report: %s\n", path)
}

func budgetForMetric(budgets []BudgetResult, metric string) BudgetResult {
	for _, result := range budgets {
		if result.Metric == metric {
			return result
		}
	}
	return BudgetResult{}
}

func formatBudgetValue(value float64, unit string) string {
	switch unit {
	case "bytes":
		return formatBytes(value)
	case "count":
		return fmt.Sprintf("%.0f", value)
	case "score":
		return fmt.Sprintf("%.3f", value)
	default:
		return fmt.Sprintf("%.0f ms", value)
	}
}

func formatBytes(value float64) string {
	const mebibyte = 1024 * 1024
	const kibibyte = 1024
	if value >= mebibyte {
		return fmt.Sprintf("%.2f MiB", value/mebibyte)
	}
	if value >= kibibyte {
		return fmt.Sprintf("%.1f KiB", value/kibibyte)
	}
	return fmt.Sprintf("%.0f B", value)
}

func budgetsPassed(budgets []BudgetResult) bool {
	for _, result := range budgets {
		if !result.Passed {
			return false
		}
	}
	return true
}
