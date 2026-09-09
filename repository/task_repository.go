package repository

import (
	"context"

	"gorm.io/gorm"

	"crm-auth-service/models"
)

type TaskRepository interface {
	Create(ctx context.Context, task *models.Task) error
	FindByUserID(ctx context.Context, userID string) ([]*models.Task, error)
	UpdateStatus(ctx context.Context, taskID, userID string, completed bool) (int64, error)
	Delete(ctx context.Context, taskID, userID string) (int64, error)
}

type taskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Create(ctx context.Context, task *models.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *taskRepository) FindByUserID(ctx context.Context, userID string) ([]*models.Task, error) {
	var tasks []*models.Task
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("due_date ASC NULLS LAST, priority ASC"). // Using NULLS LAST for due_date
		Find(&tasks).Error
	return tasks, err
}

func (r *taskRepository) UpdateStatus(ctx context.Context, taskID, userID string, completed bool) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ? AND user_id = ?", taskID, userID).
		Update("completed", completed)

	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *taskRepository) Delete(ctx context.Context, taskID, userID string) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", taskID, userID).
		Delete(&models.Task{})

	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
