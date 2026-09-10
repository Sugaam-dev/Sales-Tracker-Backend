package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	"crm-auth-service/middleware"
	"crm-auth-service/models"
	"crm-auth-service/repository"
)

type AnalyticsService interface {
	GetDashboardSummary(ctx context.Context, userID uuid.UUID, userRole, userEmail, ownerFilter, regionFilter string) (*models.DashboardSummaryData, error)
	GetReportsAnalytics(ctx context.Context, userID uuid.UUID, userRole, userEmail string, dateFrom, dateTo *time.Time, ownerFilter, regionFilter string) (*models.ReportsAnalyticsData, error)
}

type analyticsService struct {
	analyticsRepo repository.AnalyticsRepository
	leadRepo      repository.LeadRepository
	userRepo      repository.UserRepository
}

func NewAnalyticsService(analyticsRepo repository.AnalyticsRepository, leadRepo repository.LeadRepository, userRepo repository.UserRepository) AnalyticsService {
	return &analyticsService{
		analyticsRepo: analyticsRepo,
		leadRepo:      leadRepo,
		userRepo:      userRepo,
	}
}

func (s *analyticsService) GetDashboardSummary(ctx context.Context, userID uuid.UUID, userRole, userEmail, ownerFilter, regionFilter string) (*models.DashboardSummaryData, error) {
	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, userID, userRole)
	if err != nil {
		return nil, err
	}

	return s.analyticsRepo.GetDashboardSummary(ctx, scope, ownerFilter, regionFilter)
}

func (s *analyticsService) GetReportsAnalytics(ctx context.Context, userID uuid.UUID, userRole, userEmail string, dateFrom, dateTo *time.Time, ownerFilter, regionFilter string) (*models.ReportsAnalyticsData, error) {
	scope, err := middleware.ResolveDataScope(ctx, s.userRepo, userID, userRole)
	if err != nil {
		return nil, err
	}

	return s.analyticsRepo.GetReportsAnalytics(ctx, scope, dateFrom, dateTo, ownerFilter, regionFilter)
}

