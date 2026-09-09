package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/services"
)

type TaskController struct {
	service services.TaskService
	log     *slog.Logger
}

func NewTaskController(service services.TaskService, log *slog.Logger) *TaskController {
	return &TaskController{
		service: service,
		log:     log,
	}
}

func (ctrl *TaskController) getUserID(c *gin.Context) (string, error) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		return "", errors.New("unauthorized")
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		return "", errors.New("invalid user id type")
	}
	return userID.String(), nil
}

func (ctrl *TaskController) handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, services.ErrNotFound) {
		helpers.ErrorResponse(c, http.StatusNotFound, "Task not found")
	} else if errors.Is(err, services.ErrValidation) {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Validation failed")
	} else {
		ctrl.log.Error("Task Controller Error", "err", err)
		helpers.ErrorResponse(c, http.StatusInternalServerError, "Error: "+err.Error())
	}
}

func (ctrl *TaskController) CreateTask(c *gin.Context) {
	userID, err := ctrl.getUserID(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req models.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	resp, err := ctrl.service.CreateTask(c.Request.Context(), userID, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *TaskController) GetTasks(c *gin.Context) {
	userID, err := ctrl.getUserID(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	resp, err := ctrl.service.GetTasks(c.Request.Context(), userID)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	if resp == nil {
		resp = []models.TaskResponse{}
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (ctrl *TaskController) UpdateTaskStatus(c *gin.Context) {
	userID, err := ctrl.getUserID(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	taskID := c.Param("id")

	var req models.UpdateTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.ErrorResponse(c, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}

	err = ctrl.service.UpdateTaskStatus(c.Request.Context(), taskID, userID, req)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Task status updated successfully",
	})
}

func (ctrl *TaskController) DeleteTask(c *gin.Context) {
	userID, err := ctrl.getUserID(c)
	if err != nil {
		helpers.ErrorResponse(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	taskID := c.Param("id")

	err = ctrl.service.DeleteTask(c.Request.Context(), taskID, userID)
	if err != nil {
		ctrl.handleServiceError(c, err)
		return
	}

	helpers.SuccessResponse(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Task deleted successfully",
	})
}
