package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"crm-auth-service/helpers"
	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

type CommercialService interface {
	GetCommercial(ctx context.Context, leadID string, userRole, userEmail string, currencyParam string) (*models.GetCommercialResponse, error)
	UpdateCommercial(ctx context.Context, leadID string, userRole, userEmail string, currencyParam string, req models.UpdateCommercialRequest) (*models.GetCommercialResponse, error)
	GetAnalytics(ctx context.Context, leadID string, userRole, userEmail string, currencyParam string) (*models.CommercialAnalyticsResponse, error)
}

type commercialService struct {
	commRepo repository.CommercialRepository
	leadRepo repository.LeadRepository
	userRepo repository.UserRepository
}

func NewCommercialService(
	commRepo repository.CommercialRepository,
	leadRepo repository.LeadRepository,
	userRepo repository.UserRepository,
) CommercialService {
	return &commercialService{
		commRepo: commRepo,
		leadRepo: leadRepo,
		userRepo: userRepo,
	}
}

func (s *commercialService) getLeadAndCheckAccess(ctx context.Context, leadID string, userRole, userEmail string) (*models.LeadContextDTO, error) {
	lead, err := s.leadRepo.FindByID(ctx, leadID)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	user, err := s.userRepo.FindByEmail(ctx, userEmail)
	if err != nil {
		return nil, ErrUnauthorized
	}

	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, user.ID, userRole)
	if err != nil {
		return nil, ErrUnauthorized
	}

	if !scope.CanAccessLead(lead) {
		return nil, ErrUnauthorized
	}

	var estReqDateStr *string
	if lead.EstimatedRequirementDate != nil {
		d := lead.EstimatedRequirementDate.Format("2006-01-02")
		estReqDateStr = &d
	}

	leadContext := &models.LeadContextDTO{
		LeadID:                   lead.LeadID,
		Company:                  lead.Company,
		ProjectName:              lead.ProjectName,
		Owner:                    lead.Owner,
		EstimatedRequirementDate: estReqDateStr,
	}

	return leadContext, nil
}

func (s *commercialService) resolveTargetCurrency(param string, estimateCurrency string) (string, error) {
	target := strings.ToUpper(strings.TrimSpace(param))
	if target == "" {
		target = strings.ToUpper(strings.TrimSpace(estimateCurrency))
	}
	if target == "" {
		target = models.CurrencyUSD
	}
	if !IsValidCurrency(target) {
		return "", errors.New("invalid currency, must be one of USD, EUR, GBP, INR")
	}
	return target, nil
}

func (s *commercialService) mapToDetailsDTO(est *models.CommercialEstimation, targetCurrency string) models.CommercialEstimationDetailsDTO {
	resourcesDTO := make([]models.CommercialResourceDTO, len(est.Resources))
	for i, r := range est.Resources {
		days := r.OnsiteDays + r.OffshoreDays
		baseCost := float64(days) * r.DailyCost
		baseRev := float64(days) * r.BillingRate

		resID := r.ID
		resourcesDTO[i] = models.CommercialResourceDTO{
			ID:           &resID,
			Role:         r.Role,
			Grade:        r.Grade,
			OnsiteDays:   r.OnsiteDays,
			OffshoreDays: r.OffshoreDays,
			DailyCost:    Convert(r.DailyCost, targetCurrency),
			BillingRate:  Convert(r.BillingRate, targetCurrency),
			TotalDays:    days,
			TotalCost:    Convert(baseCost, targetCurrency),
			TotalRevenue: Convert(baseRev, targetCurrency),
		}
	}

	expensesDTO := make([]models.CommercialExpenseDTO, len(est.Expenses))
	for i, e := range est.Expenses {
		expID := e.ID
		expensesDTO[i] = models.CommercialExpenseDTO{
			ID:          &expID,
			ExpenseType: e.ExpenseType,
			Cost:        Convert(e.Cost, targetCurrency),
			Remarks:     e.Remarks,
		}
	}

	var totalSDLCManDays int
	for _, sdlc := range est.SDLCAllocations {
		totalSDLCManDays += sdlc.ManDays
	}

	sdlcDTO := make([]models.SDLCAllocationDTO, len(est.SDLCAllocations))
	for i, item := range est.SDLCAllocations {
		var pct float64
		if totalSDLCManDays > 0 {
			pct = Round2((float64(item.ManDays) / float64(totalSDLCManDays)) * 100.0)
		}
		sdlcID := item.ID
		sdlcDTO[i] = models.SDLCAllocationDTO{
			ID:         &sdlcID,
			Phase:      item.Phase,
			ManDays:    item.ManDays,
			Percentage: pct,
		}
	}

	// Calculate financial summary in base USD, then convert to target currency
	baseSummary := CalculateFinancialSummary(
		est.Resources,
		est.Expenses,
		est.MarkupPercent,
		est.DiscountPercent,
		est.ManualSellingPrice,
		est.EstimatedDurationMonths,
	)
	convertedSummary := ConvertFinancialSummary(baseSummary, targetCurrency)

	var manualPriceConverted *float64
	if est.ManualSellingPrice != nil {
		val := Convert(*est.ManualSellingPrice, targetCurrency)
		manualPriceConverted = &val
	}

	return models.CommercialEstimationDetailsDTO{
		ID:                      est.ID,
		LeadID:                  est.LeadID,
		Currency:                targetCurrency,
		BillingType:             est.BillingType,
		StartDate:               est.StartDate.Format("2006-01-02"),
		EstimatedDurationMonths: est.EstimatedDurationMonths,
		EstimatedEndDate:        est.EstimatedEndDate.Format("2006-01-02"),
		MarkupPercent:           est.MarkupPercent,
		DiscountPercent:         est.DiscountPercent,
		ManualSellingPrice:      manualPriceConverted,
		Status:                  est.Status,
		Resources:               resourcesDTO,
		Expenses:                expensesDTO,
		SDLCAllocations:         sdlcDTO,
		FinancialSummary:        convertedSummary,
		CreatedAt:               est.CreatedAt.Format(time.RFC3339),
		UpdatedAt:               est.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *commercialService) GetCommercial(ctx context.Context, leadID string, userRole, userEmail string, currencyParam string) (*models.GetCommercialResponse, error) {
	leadContext, err := s.getLeadAndCheckAccess(ctx, leadID, userRole, userEmail)
	if err != nil {
		return nil, err
	}

	est, err := s.commRepo.GetByLeadID(ctx, leadID)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			// In-memory read-only blank draft without writing to DB
			now := time.Now()
			startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			durationMonths := 1
			endDate := startDate.AddDate(0, 1, 0)
			if leadContext.EstimatedRequirementDate != nil {
				if parsedReqDate, pErr := time.Parse("2006-01-02", *leadContext.EstimatedRequirementDate); pErr == nil {
					reqDate := time.Date(parsedReqDate.Year(), parsedReqDate.Month(), parsedReqDate.Day(), 0, 0, 0, 0, time.UTC)
					if reqDate.After(startDate) {
						endDate = reqDate
						months := (endDate.Year()-startDate.Year())*12 + int(endDate.Month()-startDate.Month())
						if endDate.Day() > startDate.Day() {
							months++
						}
						if months < 1 {
							months = 1
						}
						durationMonths = months
					}
				}
			}

			est = &models.CommercialEstimation{
				LeadID:                  leadID,
				Currency:                models.CurrencyUSD,
				BillingType:             "T&M",
				StartDate:               startDate,
				EstimatedDurationMonths: durationMonths,
				EstimatedEndDate:        endDate,
				MarkupPercent:           0.00,
				DiscountPercent:         0.00,
				Status:                  models.CommercialStatusDraft,
				Resources:               []models.CommercialResource{},
				Expenses:                []models.CommercialExpense{},
				SDLCAllocations: []models.SDLCAllocation{
					{Phase: "Discovery & Architecture", ManDays: 0},
					{Phase: "UI/UX Design", ManDays: 0},
					{Phase: "Core Development", ManDays: 0},
					{Phase: "QA & Testing", ManDays: 0},
					{Phase: "Deployment & UAT", ManDays: 0},
				},
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			return nil, err
		}
	}

	targetCurrency, err := s.resolveTargetCurrency(currencyParam, est.Currency)
	if err != nil {
		return nil, ErrValidation
	}

	details := s.mapToDetailsDTO(est, targetCurrency)

	return &models.GetCommercialResponse{
		LeadContext:          *leadContext,
		CommercialEstimation: details,
	}, nil
}

func (s *commercialService) UpdateCommercial(
	ctx context.Context,
	leadID string,
	userRole, userEmail string,
	currencyParam string,
	req models.UpdateCommercialRequest,
) (*models.GetCommercialResponse, error) {
	leadContext, err := s.getLeadAndCheckAccess(ctx, leadID, userRole, userEmail)
	if err != nil {
		return nil, err
	}

	existing, err := s.commRepo.GetByLeadID(ctx, leadID)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			now := time.Now()
			startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			endDate := startDate.AddDate(0, 1, 0)
			if leadContext.EstimatedRequirementDate != nil {
				if parsedReqDate, pErr := time.Parse("2006-01-02", *leadContext.EstimatedRequirementDate); pErr == nil {
					reqDate := time.Date(parsedReqDate.Year(), parsedReqDate.Month(), parsedReqDate.Day(), 0, 0, 0, 0, time.UTC)
					if reqDate.After(startDate) {
						endDate = reqDate
					}
				}
			}
			existing = &models.CommercialEstimation{
				LeadID:                  leadID,
				Currency:                models.CurrencyUSD,
				BillingType:             "T&M",
				StartDate:               startDate,
				EstimatedDurationMonths: 1,
				EstimatedEndDate:        endDate,
				MarkupPercent:           0.00,
				DiscountPercent:         0.00,
				Status:                  models.CommercialStatusDraft,
				Resources:               []models.CommercialResource{},
				Expenses:                []models.CommercialExpense{},
				SDLCAllocations:         []models.SDLCAllocation{},
			}
		} else {
			return nil, err
		}
	}

	// 1. Solve Currency
	currency := existing.Currency
	if req.Currency != nil && *req.Currency != "" {
		c := strings.ToUpper(strings.TrimSpace(*req.Currency))
		if !IsValidCurrency(c) {
			return nil, ErrValidation
		}
		currency = c
	}

	// 2. Solve Billing Type
	billingType := existing.BillingType
	if req.BillingType != nil && strings.TrimSpace(*req.BillingType) != "" {
		billingType = strings.TrimSpace(*req.BillingType)
	}

	// 3. Solve Dates
	defaultStart := existing.StartDate
	startDate, durationMonths, endDate, err := SolveDates(req.StartDate, req.EstimatedDurationMonths, req.EstimatedEndDate, defaultStart)
	if err != nil {
		return nil, ErrValidation
	}

	// 4. Solve Markup & Discount & Manual Selling Price
	markup := existing.MarkupPercent
	if req.MarkupPercent != nil {
		if *req.MarkupPercent < 0 {
			return nil, ErrValidation
		}
		markup = *req.MarkupPercent
	}

	discount := existing.DiscountPercent
	if req.DiscountPercent != nil {
		if *req.DiscountPercent < 0 || *req.DiscountPercent > 100 {
			return nil, ErrValidation
		}
		discount = *req.DiscountPercent
	}

	var manualSellingPrice *float64
	if req.ManualSellingPrice != nil {
		if *req.ManualSellingPrice < 0 {
			return nil, ErrValidation
		}
		// If sent in requested or header currency, store in Base USD
		baseVal := ConvertToBaseUSD(*req.ManualSellingPrice, currency)
		manualSellingPrice = &baseVal
	}

	status := existing.Status
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		status = strings.TrimSpace(*req.Status)
	}

	// 5. Solve Resources
	var resourcesToSave *[]models.CommercialResource
	if req.Resources != nil {
		resList := make([]models.CommercialResource, len(*req.Resources))
		for i, r := range *req.Resources {
			if strings.TrimSpace(r.Role) == "" || strings.TrimSpace(r.Grade) == "" {
				return nil, ErrValidation
			}
			if r.OnsiteDays < 0 || r.OffshoreDays < 0 || r.DailyCost < 0 || r.BillingRate < 0 {
				return nil, ErrValidation
			}

			// Convert input cost/rate from active currency to Base USD for storage
			dailyCostUSD := ConvertToBaseUSD(r.DailyCost, currency)
			billingRateUSD := ConvertToBaseUSD(r.BillingRate, currency)
			days := float64(r.OnsiteDays + r.OffshoreDays)
			totalCostUSD := Round2(days * dailyCostUSD)
			totalRevenueUSD := Round2(days * billingRateUSD)

			resList[i] = models.CommercialResource{
				Role:         strings.TrimSpace(r.Role),
				Grade:        strings.TrimSpace(r.Grade),
				OnsiteDays:   r.OnsiteDays,
				OffshoreDays: r.OffshoreDays,
				DailyCost:    dailyCostUSD,
				BillingRate:  billingRateUSD,
				TotalCost:    totalCostUSD,
				TotalRevenue: totalRevenueUSD,
			}
		}
		resourcesToSave = &resList
	}

	// 6. Solve Expenses
	var expensesToSave *[]models.CommercialExpense
	if req.Expenses != nil {
		expList := make([]models.CommercialExpense, len(*req.Expenses))
		for i, e := range *req.Expenses {
			if strings.TrimSpace(e.ExpenseType) == "" || e.Cost < 0 {
				return nil, ErrValidation
			}
			costUSD := ConvertToBaseUSD(e.Cost, currency)
			expList[i] = models.CommercialExpense{
				ExpenseType: strings.TrimSpace(e.ExpenseType),
				Cost:        costUSD,
				Remarks:     e.Remarks,
			}
		}
		expensesToSave = &expList
	}

	// 7. Solve SDLC Allocations
	var sdlcToSave *[]models.SDLCAllocation
	if req.SDLCAllocations != nil {
		sdlcList := make([]models.SDLCAllocation, len(*req.SDLCAllocations))
		for i, sItem := range *req.SDLCAllocations {
			if strings.TrimSpace(sItem.Phase) == "" || sItem.ManDays < 0 {
				return nil, ErrValidation
			}
			sdlcList[i] = models.SDLCAllocation{
				Phase:   strings.TrimSpace(sItem.Phase),
				ManDays: sItem.ManDays,
			}
		}
		sdlcToSave = &sdlcList
	}

	estimationHeader := &models.CommercialEstimation{
		ID:                      existing.ID,
		LeadID:                  leadID,
		Currency:                currency,
		BillingType:             billingType,
		StartDate:               startDate,
		EstimatedDurationMonths: durationMonths,
		EstimatedEndDate:        endDate,
		MarkupPercent:           markup,
		DiscountPercent:         discount,
		ManualSellingPrice:      manualSellingPrice,
		Status:                  status,
	}

	updated, err := s.commRepo.UpdateAggregate(ctx, leadID, estimationHeader, resourcesToSave, expensesToSave, sdlcToSave)
	if err != nil {
		return nil, err
	}

	// Calculate authoritative financial summary in Base USD to sync Deal Value (leads.value)
	baseSummary := CalculateFinancialSummary(
		updated.Resources,
		updated.Expenses,
		updated.MarkupPercent,
		updated.DiscountPercent,
		updated.ManualSellingPrice,
		updated.EstimatedDurationMonths,
	)

	// Set lead Deal Value from the evaluated commercial selling price
	dealValue := baseSummary.EffectiveSellingPrice
	_ = s.leadRepo.UpdateLead(leadID, map[string]interface{}{
		"value": dealValue,
	})

	targetCurrency, err := s.resolveTargetCurrency(currencyParam, updated.Currency)
	if err != nil {
		targetCurrency = updated.Currency
	}

	details := s.mapToDetailsDTO(updated, targetCurrency)

	return &models.GetCommercialResponse{
		LeadContext:          *leadContext,
		CommercialEstimation: details,
	}, nil
}

func (s *commercialService) GetAnalytics(ctx context.Context, leadID string, userRole, userEmail string, currencyParam string) (*models.CommercialAnalyticsResponse, error) {
	_, err := s.getLeadAndCheckAccess(ctx, leadID, userRole, userEmail)
	if err != nil {
		return nil, err
	}

	est, err := s.commRepo.GetByLeadID(ctx, leadID)
	if err != nil {
		if errors.Is(err, helpers.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	targetCurrency, err := s.resolveTargetCurrency(currencyParam, est.Currency)
	if err != nil {
		return nil, ErrValidation
	}

	// 1. Financial Summary
	baseSummary := CalculateFinancialSummary(
		est.Resources,
		est.Expenses,
		est.MarkupPercent,
		est.DiscountPercent,
		est.ManualSellingPrice,
		est.EstimatedDurationMonths,
	)
	financialSummary := ConvertFinancialSummary(baseSummary, targetCurrency)

	// 2. Grade-wise Allocation
	gradeManDaysMap := make(map[string]int)
	for _, r := range est.Resources {
		days := r.OnsiteDays + r.OffshoreDays
		gradeManDaysMap[r.Grade] += days
	}
	var gradeLabels []string
	for g := range gradeManDaysMap {
		gradeLabels = append(gradeLabels, g)
	}
	sort.Strings(gradeLabels)
	var gradeData []float64
	for _, g := range gradeLabels {
		gradeData = append(gradeData, float64(gradeManDaysMap[g]))
	}
	gradeWise := models.ChartDataDTO{
		Labels: gradeLabels,
		Datasets: []models.ChartDatasetDTO{
			{Label: "Man-days", Data: gradeData},
		},
	}

	// 3. Phase-wise Allocation & SDLC Effort Distribution
	var phaseLabels []string
	var phaseData []float64
	for _, sItem := range est.SDLCAllocations {
		phaseLabels = append(phaseLabels, sItem.Phase)
		phaseData = append(phaseData, float64(sItem.ManDays))
	}
	phaseWise := models.ChartDataDTO{
		Labels: phaseLabels,
		Datasets: []models.ChartDatasetDTO{
			{Label: "Man-days", Data: phaseData},
		},
	}
	sdlcEffort := models.ChartDataDTO{
		Labels: phaseLabels,
		Datasets: []models.ChartDatasetDTO{
			{Data: phaseData},
		},
	}

	// 4. Cost Breakdown
	costBreakdownLabels := []string{"Resource Cost"}
	costBreakdownData := []float64{financialSummary.TotalResourceCost}

	expenseTypeCostMap := make(map[string]float64)
	for _, e := range est.Expenses {
		expenseTypeCostMap[e.ExpenseType] += e.Cost
	}
	var expenseTypes []string
	for t := range expenseTypeCostMap {
		expenseTypes = append(expenseTypes, t)
	}
	sort.Strings(expenseTypes)
	for _, t := range expenseTypes {
		costBreakdownLabels = append(costBreakdownLabels, t)
		costBreakdownData = append(costBreakdownData, Convert(expenseTypeCostMap[t], targetCurrency))
	}
	costBreakdown := models.ChartDataDTO{
		Labels: costBreakdownLabels,
		Datasets: []models.ChartDatasetDTO{
			{Label: "Cost", Data: costBreakdownData},
		},
	}

	// 5. Revenue vs Cost (Onsite, Offshore, Expenses)
	var onsiteCostUSD, onsiteRevUSD float64
	var offshoreCostUSD, offshoreRevUSD float64
	for _, r := range est.Resources {
		onsiteCostUSD += float64(r.OnsiteDays) * r.DailyCost
		onsiteRevUSD += float64(r.OnsiteDays) * r.BillingRate
		offshoreCostUSD += float64(r.OffshoreDays) * r.DailyCost
		offshoreRevUSD += float64(r.OffshoreDays) * r.BillingRate
	}
	revenueVsCost := models.ChartDataDTO{
		Labels: []string{"Onsite", "Offshore", "Expenses"},
		Datasets: []models.ChartDatasetDTO{
			{
				Label: "Cost",
				Data: []float64{
					Convert(onsiteCostUSD, targetCurrency),
					Convert(offshoreCostUSD, targetCurrency),
					financialSummary.TotalExpenses,
				},
			},
			{
				Label: "Revenue",
				Data: []float64{
					Convert(onsiteRevUSD, targetCurrency),
					Convert(offshoreRevUSD, targetCurrency),
					0.00,
				},
			},
		},
	}

	// 6. Cumulative Cash Flow Projection
	duration := est.EstimatedDurationMonths
	if duration < 1 {
		duration = 1
	}
	var monthLabels []string
	var cumCostData []float64
	var cumRevData []float64
	var cumCashPosData []float64

	monthlyCost := financialSummary.TotalProjectCost / float64(duration)
	monthlyRevenue := financialSummary.EffectiveSellingPrice / float64(duration)

	var runningCost, runningRev float64
	for m := 1; m <= duration; m++ {
		monthLabels = append(monthLabels, fmt.Sprintf("Month %d", m))
		runningCost += monthlyCost
		runningRev += monthlyRevenue
		runningPos := runningRev - runningCost

		cumCostData = append(cumCostData, Round2(runningCost))
		cumRevData = append(cumRevData, Round2(runningRev))
		cumCashPosData = append(cumCashPosData, Round2(runningPos))
	}

	cumulativeCashFlow := models.ChartDataDTO{
		Labels: monthLabels,
		Datasets: []models.ChartDatasetDTO{
			{Label: "Cumulative Cost", Data: cumCostData},
			{Label: "Cumulative Revenue", Data: cumRevData},
			{Label: "Cumulative Cash Position", Data: cumCashPosData},
		},
	}

	return &models.CommercialAnalyticsResponse{
		Currency:               targetCurrency,
		FinancialSummary:       financialSummary,
		GradeWiseAllocation:    gradeWise,
		PhaseWiseAllocation:    phaseWise,
		CostBreakdown:          costBreakdown,
		RevenueVsCost:          revenueVsCost,
		SDLCEffortDistribution: sdlcEffort,
		CumulativeCashFlow:     cumulativeCashFlow,
	}, nil
}
