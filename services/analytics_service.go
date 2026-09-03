package services

import (
	"context"
	"time"

	"crm-auth-service/models"
	"crm-auth-service/repository"
)

type AnalyticsService interface {
	GetDashboardSummary(ctx context.Context, userRole, userEmail, ownerFilter, regionFilter string) (*models.DashboardSummaryData, error)
	GetReportsAnalytics(ctx context.Context, userRole, userEmail string, dateFrom, dateTo *time.Time, ownerFilter, regionFilter string) (*models.ReportsAnalyticsData, error)
}

type analyticsService struct {
	analyticsRepo repository.AnalyticsRepository
	leadRepo      repository.LeadRepository
}

func NewAnalyticsService(analyticsRepo repository.AnalyticsRepository, leadRepo repository.LeadRepository) AnalyticsService {
	return &analyticsService{
		analyticsRepo: analyticsRepo,
		leadRepo:      leadRepo,
	}
}

func (s *analyticsService) GetDashboardSummary(ctx context.Context, userRole, userEmail, ownerFilter, regionFilter string) (*models.DashboardSummaryData, error) {
	// Role-based authorization enforcement
	effectiveOwner := ownerFilter
	if userRole != models.RoleAdmin && userRole != models.RoleSalesManager && userRole != models.RoleLeader {
		userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
		if err == nil && userName != "" {
			effectiveOwner = userName
		}
	}

	return s.analyticsRepo.GetDashboardSummary(ctx, effectiveOwner, regionFilter)
}

func (s *analyticsService) GetReportsAnalytics(ctx context.Context, userRole, userEmail string, dateFrom, dateTo *time.Time, ownerFilter, regionFilter string) (*models.ReportsAnalyticsData, error) {
	// Role-based authorization enforcement
	effectiveOwner := ownerFilter
	if userRole != models.RoleAdmin && userRole != models.RoleSalesManager && userRole != models.RoleLeader {
		userName, err := s.leadRepo.GetUserNameByEmail(userEmail)
		if err == nil && userName != "" {
			effectiveOwner = userName
		}
	}

	return s.analyticsRepo.GetReportsAnalytics(ctx, dateFrom, dateTo, effectiveOwner, regionFilter)
}
