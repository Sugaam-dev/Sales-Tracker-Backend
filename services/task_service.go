package services

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"crm-auth-service/models"
	"crm-auth-service/repository"
)

type TaskService interface {
	CreateTask(ctx context.Context, userID string, req models.CreateTaskRequest) (*models.TaskResponse, error)
	GetTasks(ctx context.Context, userID string) ([]models.TaskResponse, error)
	UpdateTaskStatus(ctx context.Context, taskID, userID string, req models.UpdateTaskStatusRequest) error
	DeleteTask(ctx context.Context, taskID, userID string) error
}

type taskService struct {
	repo repository.TaskRepository
}

func NewTaskService(repo repository.TaskRepository) TaskService {
	return &taskService{repo: repo}
}

func (s *taskService) CreateTask(ctx context.Context, userID string, req models.CreateTaskRequest) (*models.TaskResponse, error) {
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return nil, ErrValidation
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrValidation
	}

	var parsedDate *time.Time
	if req.DueDate != nil && *req.DueDate != "" {
		t, err := time.Parse("2006-01-02", *req.DueDate)
		if err != nil {
			t2, err2 := time.Parse(time.RFC3339, *req.DueDate)
			if err2 == nil {
				parsedDate = &t2
			}
		} else {
			parsedDate = &t
		}
	}

	task := &models.Task{
		UserID:   userUUID,
		Text:     req.Text,
		DueDate:  parsedDate,
		Priority: req.Priority,
	}

	if err := s.repo.Create(ctx, task); err != nil {
		return nil, err
	}

	resp := models.ToTaskResponse(*task)
	return &resp, nil
}

func (s *taskService) GetTasks(ctx context.Context, userID string) ([]models.TaskResponse, error) {
	tasks, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	responses := make([]models.TaskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, models.ToTaskResponse(*t))
	}
	return responses, nil
}

func (s *taskService) UpdateTaskStatus(ctx context.Context, taskID, userID string, req models.UpdateTaskStatusRequest) error {
	if req.Completed == nil {
		return ErrValidation
	}

	rowsAffected, err := s.repo.UpdateStatus(ctx, taskID, userID, *req.Completed)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *taskService) DeleteTask(ctx context.Context, taskID, userID string) error {
	rowsAffected, err := s.repo.Delete(ctx, taskID, userID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
