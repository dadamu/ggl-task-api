package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	app "github.com/dadamu/ggl-task-api/internal/application/task"
	"github.com/dadamu/ggl-task-api/internal/domain/task"
	"github.com/dadamu/ggl-task-api/internal/transport/http/apierror"
	"github.com/dadamu/ggl-task-api/internal/transport/http/dto"
)

// Task serves the /tasks endpoints.
type Task struct {
	svc *app.Service
}

// NewTask returns a Task handler.
func NewTask(svc *app.Service) *Task {
	apierror.UseJSONFieldNames()
	return &Task{svc: svc}
}

// List godoc
//
//	@Summary	List tasks
//	@Tags		tasks
//	@Produce	json
//	@Success	200	{object}	dto.TaskList	"Tasks ordered by id ascending"
//	@Failure	500	{object}	apierror.Response
//	@Router		/tasks [get]
func (h *Task) List(c *gin.Context) {
	tasks, err := h.svc.List(c.Request.Context())
	if err != nil {
		apierror.FromError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromTasks(tasks))
}

// Create godoc
//
//	@Summary	Create a task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.CreateTaskRequest	true	"Task to create; status defaults to 0"
//	@Success	201		{object}	dto.Task
//	@Header		201		{string}	Location			"/tasks/{id}"
//	@Failure	400		{object}	apierror.Response	"INVALID_JSON or VALIDATION_FAILED"
//	@Failure	415		{object}	apierror.Response	"UNSUPPORTED_MEDIA_TYPE"
//	@Failure	413		{object}	apierror.Response	"PAYLOAD_TOO_LARGE"
//	@Failure	500		{object}	apierror.Response
//	@Router		/tasks [post]
func (h *Task) Create(c *gin.Context) {
	var req dto.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierror.FromBindError(c, err)
		return
	}
	status := task.StatusIncomplete
	if req.Status != nil {
		status = task.Status(*req.Status)
	}

	created, err := h.svc.Create(c.Request.Context(), app.CreateInput{Name: *req.Name, Status: status})
	if err != nil {
		apierror.FromError(c, err)
		return
	}
	c.Header("Location", "/tasks/"+strconv.FormatInt(created.ID, 10))
	c.JSON(http.StatusCreated, dto.FromTask(created))
}

// Update godoc
//
//	@Summary		Update a task
//	@Description	Full replacement. `version` must equal the current version (optimistic locking).
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int						true	"Task ID"	minimum(1)
//	@Param			body	body		dto.UpdateTaskRequest	true	"New name, status and the version last read"
//	@Success		200		{object}	dto.Task
//	@Failure		400		{object}	apierror.Response	"INVALID_ID, INVALID_JSON or VALIDATION_FAILED"
//	@Failure		404		{object}	apierror.Response	"TASK_NOT_FOUND"
//	@Failure		409		{object}	apierror.Response	"VERSION_CONFLICT, with the current task"
//	@Failure		415		{object}	apierror.Response	"UNSUPPORTED_MEDIA_TYPE"
//	@Failure		413		{object}	apierror.Response	"PAYLOAD_TOO_LARGE"
//	@Failure		500		{object}	apierror.Response
//	@Router			/tasks/{id} [put]
func (h *Task) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req dto.UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierror.FromBindError(c, err)
		return
	}

	updated, err := h.svc.Update(c.Request.Context(), app.UpdateInput{
		ID:      id,
		Name:    *req.Name,
		Status:  task.Status(*req.Status),
		Version: *req.Version,
	})
	if err != nil {
		apierror.FromError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromTask(updated))
}

// Delete godoc
//
//	@Summary	Delete a task
//	@Tags		tasks
//	@Param		id	path	int	true	"Task ID"	minimum(1)
//	@Success	204
//	@Failure	400	{object}	apierror.Response	"INVALID_ID"
//	@Failure	404	{object}	apierror.Response	"TASK_NOT_FOUND"
//	@Failure	500	{object}	apierror.Response
//	@Router		/tasks/{id} [delete]
func (h *Task) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		apierror.FromError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// parseID reads the :id path parameter. It writes a 400 and returns false if
// the parameter is not a positive integer.
func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		apierror.Abort(c, http.StatusBadRequest, apierror.Body{
			Code:    apierror.CodeInvalidID,
			Message: "id must be a positive integer",
		})
		return 0, false
	}
	return id, true
}
