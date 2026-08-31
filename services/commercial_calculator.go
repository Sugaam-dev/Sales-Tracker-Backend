package services

import (
	"errors"
	"math"
	"os"
	"strconv"
	"time"

	"crm-auth-service/models"
)

// Fixed exchange rate matrix relative to base USD.
var ExchangeRates = map[string]float64{
	models.CurrencyUSD: 1.00,
	models.CurrencyEUR: 0.92,
	models.CurrencyGBP: 0.79,
	models.CurrencyINR: 83.50,
}

// IsValidCurrency checks if the currency is supported.
func IsValidCurrency(c string) bool {
	_, ok := ExchangeRates[c]
	return ok
}

// Convert converts a base USD amount to target currency with 2 decimal precision.
func Convert(baseUSD float64, targetCurrency string) float64 {
	rate, ok := ExchangeRates[targetCurrency]
	if !ok {
		rate = 1.00
	}
	return Round2(baseUSD * rate)
}

// ConvertToBaseUSD converts an amount from source currency back to base USD.
func ConvertToBaseUSD(amount float64, sourceCurrency string) float64 {
	rate, ok := ExchangeRates[sourceCurrency]
	if !ok || rate == 0 {
		rate = 1.00
	}
	return Round2(amount / rate)
}

// Round2 rounds a float64 to two decimal places.
func Round2(val float64) float64 {
	return math.Round(val*100) / 100
}

// SolveDates calculates and validates the 3 date fields:
// - Case 1: Start + Duration -> End = Start + Duration months
// - Case 2: Start + End -> Duration = diff in months
// - Case 3: Duration + End -> Start = End - Duration months
// - All Three Provided: Start + Duration is authoritative, recalculate End Date.
func SolveDates(startStr *string, durationMonths *int, endStr *string, defaultStart time.Time) (time.Time, int, time.Time, error) {
	const dateFormat = "2006-01-02"

	var start *time.Time
	if startStr != nil && *startStr != "" {
		t, err := time.Parse(dateFormat, *startStr)
		if err != nil {
			return time.Time{}, 0, time.Time{}, errors.New("invalid start date format, expected YYYY-MM-DD")
		}
		start = &t
	}

	var end *time.Time
	if endStr != nil && *endStr != "" {
		t, err := time.Parse(dateFormat, *endStr)
		if err != nil {
			return time.Time{}, 0, time.Time{}, errors.New("invalid end date format, expected YYYY-MM-DD")
		}
		end = &t
	}

	var dur *int
	if durationMonths != nil && *durationMonths > 0 {
		dur = durationMonths
	}

	// Conflict resolution: If all three provided, Start + Duration wins
	if start != nil && dur != nil && end != nil {
		computedEnd := start.AddDate(0, *dur, 0)
		return *start, *dur, computedEnd, nil
	}

	// Case 1: Start + Duration -> End = Start + Duration
	if start != nil && dur != nil {
		computedEnd := start.AddDate(0, *dur, 0)
		return *start, *dur, computedEnd, nil
	}

	// Case 2: Start + End -> Duration
	if start != nil && end != nil {
		if end.Before(*start) {
			return time.Time{}, 0, time.Time{}, errors.New("end date cannot be before start date")
		}
		// Calculate whole months difference
		months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
		if months < 1 {
			months = 1
		}
		return *start, months, *end, nil
	}

	// Case 3: Duration + End -> Start = End - Duration
	if dur != nil && end != nil {
		computedStart := end.AddDate(0, -(*dur), 0)
		return computedStart, *dur, *end, nil
	}

	// Single or no fields provided: use defaults
	s := defaultStart
	if start != nil {
		s = *start
	}
	d := 1
	if dur != nil {
		d = *dur
	}
	e := s.AddDate(0, d, 0)
	if end != nil {
		e = *end
	}
	if e.Before(s) {
		e = s.AddDate(0, d, 0)
	}

	return s, d, e, nil
}

// CalculateFinancialSummary computes all authoritative project financial metrics in base USD.
func CalculateFinancialSummary(
	resources []models.CommercialResource,
	expenses []models.CommercialExpense,
	markupPercent float64,
	discountPercent float64,
	manualSellingPrice *float64,
	durationMonths int,
) models.FinancialSummaryDTO {
	var totalResourceCost float64
	for _, r := range resources {
		days := float64(r.OnsiteDays + r.OffshoreDays)
		totalResourceCost += days * r.DailyCost
	}

	var totalExpenses float64
	for _, e := range expenses {
		totalExpenses += e.Cost
	}

	totalProjectCost := Round2(totalResourceCost + totalExpenses)

	// Calculated Selling Price = C * (1 + Markup/100) * (1 - Discount/100)
	calcSellingPrice := Round2(totalProjectCost * (1.0 + markupPercent/100.0) * (1.0 - discountPercent/100.0))

	// Effective Selling Price
	effectiveSellingPrice := calcSellingPrice
	if manualSellingPrice != nil && *manualSellingPrice > 0 {
		effectiveSellingPrice = Round2(*manualSellingPrice)
	}

	grossProfit := Round2(effectiveSellingPrice - totalProjectCost)

	var marginPercent float64
	if effectiveSellingPrice > 0 {
		marginPercent = Round2((grossProfit / effectiveSellingPrice) * 100.0)
	}

	var roiPercent float64
	if totalProjectCost > 0 {
		roiPercent = Round2((grossProfit / totalProjectCost) * 100.0)
	}

	if durationMonths < 1 {
		durationMonths = 1
	}

	// Monthly cash flow projections for break-even, max cash out, and NPV
	monthlyCost := totalProjectCost / float64(durationMonths)
	monthlyRevenue := effectiveSellingPrice / float64(durationMonths)

	var breakEvenMonth *int
	var maxCashOut float64
	var npv float64

	// Annual discount rate (default 10% annual)
	annualDiscountRate := 0.10
	if envRate := os.Getenv("COMMERCIAL_NPV_ANNUAL_DISCOUNT_RATE"); envRate != "" {
		if r, err := strconv.ParseFloat(envRate, 64); err == nil && r > 0 {
			annualDiscountRate = r
		}
	}
	// Monthly discount rate r = (1 + annualRate)^(1/12) - 1
	monthlyDiscountRate := math.Pow(1.0+annualDiscountRate, 1.0/12.0) - 1.0

	var cumCost, cumRevenue float64
	for m := 1; m <= durationMonths; m++ {
		cumCost += monthlyCost
		cumRevenue += monthlyRevenue
		cumCashPos := cumRevenue - cumCost

		// First month where cumulative revenue >= cumulative cost
		if breakEvenMonth == nil && cumRevenue >= cumCost {
			monthVal := m
			breakEvenMonth = &monthVal
		}

		// Maximum cash out (peak negative cash position)
		if cumCashPos < 0 {
			deficit := -cumCashPos
			if deficit > maxCashOut {
				maxCashOut = deficit
			}
		}

		// Monthly net cash flow discounted to present value
		monthlyNetFlow := monthlyRevenue - monthlyCost
		discountFactor := math.Pow(1.0+monthlyDiscountRate, float64(m))
		npv += monthlyNetFlow / discountFactor
	}

	npvRounded := Round2(npv)

	return models.FinancialSummaryDTO{
		TotalResourceCost:      Round2(totalResourceCost),
		TotalExpenses:          Round2(totalExpenses),
		TotalProjectCost:       totalProjectCost,
		CalculatedSellingPrice: calcSellingPrice,
		EffectiveSellingPrice:  effectiveSellingPrice,
		GrossProfit:            grossProfit,
		MarginPercent:          marginPercent,
		ROIPercent:             roiPercent,
		BreakEvenMonth:         breakEvenMonth,
		MaximumCashOut:         Round2(maxCashOut),
		NPV:                    &npvRounded,
	}
}

// ConvertFinancialSummary converts all monetary summary fields to the target currency.
func ConvertFinancialSummary(summary models.FinancialSummaryDTO, targetCurrency string) models.FinancialSummaryDTO {
	converted := summary
	converted.TotalResourceCost = Convert(summary.TotalResourceCost, targetCurrency)
	converted.TotalExpenses = Convert(summary.TotalExpenses, targetCurrency)
	converted.TotalProjectCost = Convert(summary.TotalProjectCost, targetCurrency)
	converted.CalculatedSellingPrice = Convert(summary.CalculatedSellingPrice, targetCurrency)
	converted.EffectiveSellingPrice = Convert(summary.EffectiveSellingPrice, targetCurrency)
	converted.GrossProfit = Convert(summary.GrossProfit, targetCurrency)
	converted.MaximumCashOut = Convert(summary.MaximumCashOut, targetCurrency)
	if summary.NPV != nil {
		npvVal := Convert(*summary.NPV, targetCurrency)
		converted.NPV = &npvVal
	}
	return converted
}
