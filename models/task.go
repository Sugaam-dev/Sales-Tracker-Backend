package models

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID        uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `json:"userId" gorm:"type:uuid;not null;index"`
	Text      string     `json:"text" gorm:"type:text;not null"`
	DueDate   *time.Time `json:"dueDate" gorm:"type:date"`
	Priority  string     `json:"priority" gorm:"type:varchar(20);not null"`
	Completed bool       `json:"completed" gorm:"not null;default:false"`
	CreatedAt time.Time  `json:"createdAt" gorm:"not null;autoCreateTime"`
	UpdatedAt time.Time  `json:"updatedAt" gorm:"not null;autoUpdateTime"`
}

func (Task) TableName() string { return "tasks" }

type CreateTaskRequest struct {
	Text     string  `json:"text" binding:"required"`
	DueDate  *string `json:"dueDate,omitempty"` // Expect YYYY-MM-DD
	Priority string  `json:"priority" binding:"required,oneof=Low Medium High"`
}

type UpdateTaskStatusRequest struct {
	Completed *bool `json:"completed" binding:"required"`
}

type TaskResponse struct {
	ID        string  `json:"id"`
	Text      string  `json:"text"`
	DueDate   *string `json:"dueDate,omitempty"`
	Priority  string  `json:"priority"`
	Completed bool    `json:"completed"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

func ToTaskResponse(t Task) TaskResponse {
	var dueDateStr *string
	if t.DueDate != nil {
		dateStr := t.DueDate.Format("2006-01-02")
		dueDateStr = &dateStr
	}

	return TaskResponse{
		ID:        t.ID.String(),
		Text:      t.Text,
		DueDate:   dueDateStr,
		Priority:  t.Priority,
		Completed: t.Completed,
		CreatedAt: t.CreatedAt.Format(time.RFC3339),
		UpdatedAt: t.UpdatedAt.Format(time.RFC3339),
	}
}
