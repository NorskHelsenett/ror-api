package taskscontroller

import (
	"fmt"
	"net/http"
	"strings"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	tasksservice "github.com/NorskHelsenett/ror-api/internal/apiservices/tasksService"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init tasks controller")
	validate = validator.New()
}

// @Summary	Get a task
// @Schemes
// @Description	Get a task by id
// @Tags			tasks
// @Accept			application/json
// @Produce		application/json
// @Param			id		path		string				true	"id"
// @Param			task	body		apicontracts.Task	true	"Get a task"
// @Success		200		{object}	apicontracts.Task
// @Failure		403		{string}	Forbidden
// @Failure		400		{object}	rorerror.RorError
// @Failure		401		{object}	rorerror.RorError
// @Failure		500		{string}	Failure	message
// @Router			/v1/tasks/:id [get]
// @Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		_, err := gincontext.GetUserFromGinContext(c)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusForbidden, "Could not get user", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		taskId := c.Param("id")
		if taskId == "" || len(taskId) == 0 {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "invalid task id")
			rerr.GinLogErrorAbort(c)
			return
		}

		result, err := tasksservice.GetById(ctx, taskId)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "could not get task", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// @Summary	Get tasks
// @Schemes
// @Description	Get all tasks
// @Tags			tasks
// @Accept			application/json
// @Produce		application/json
// @Success		200			{array}		apicontracts.Task
// @Failure		403			{string}	Forbidden
// @Failure		400			{object}	rorerror.RorError
// @Failure		401			{string}	Unauthorized
// @Failure		500			{string}	Failure	message
// @Router			/v1/tasks	[get]
// @Security		ApiKey || AccessToken
func GetAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: acl
		// Access: read
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		tasks, err := tasksservice.GetAll(ctx)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "Could not find tasks ...", err)
			rerr.GinLogErrorAbort(c)
		}

		c.JSON(http.StatusOK, tasks)
	}
}

// @Summary	Create a task
// @Schemes
// @Description	Create a task
// @Tags			tasks
// @Accept			application/json
// @Produce		application/json
// @Param			task	body		apicontracts.Task	true	"Add a task"
// @Success		200		{array}		apicontracts.Task
// @Failure		403		{string}	Forbidden
// @Failure		400		{object}	rorerror.RorError
// @Failure		401		{object}	rorerror.RorError
// @Failure		500		{string}	Failure	message
// @Router			/v1/tasks [post]
// @Security		ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: acl
		// Access: create
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var task apicontracts.Task
		//validate the request body
		if err := c.BindJSON(&task); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not validate task object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&task); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, fmt.Sprintf("Required fields are missing: %s", err), err)
			rerr.GinLogErrorAbort(c)
			return
		}

		createdTask, err := tasksservice.Create(ctx, &task)
		if err != nil {
			rlog.Errorc(ctx, "could not create task", err)
			if strings.Contains(err.Error(), "exists") {
				rerr := rorerror.NewRorError(http.StatusBadRequest, "Already exists")
				rerr.GinLogErrorAbort(c)
				return
			}
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields are missing")
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", createdTask)

		c.JSON(http.StatusOK, createdTask)
	}
}

// @Summary	Update a task
// @Schemes
// @Description	Update a task by id
// @Tags			tasks
// @Accept			application/json
// @Produce		application/json
// @Param			id		path		string				true	"id"
// @Param			task	body		apicontracts.Task	true	"Update task"
// @Success		200		{object}	apicontracts.Task
// @Failure		403		{string}	Forbidden
// @Failure		400		{object}	rorerror.RorError
// @Failure		401		{object}	rorerror.RorError
// @Failure		500		{string}	Failure	message
// @Router			/v1/tasks/:id [put]
// @Security		ApiKey || AccessToken
func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var taskInput apicontracts.Task

		taskId := c.Param("id")
		if taskId == "" || len(taskId) == 0 {
			rlog.Errorc(ctx, "invalid task id", fmt.Errorf("id is zero length"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid task id")
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: ror
		// Subject: acl
		// Access: update
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		//validate the request body
		if err := c.BindJSON(&taskInput); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if validationErr := validate.Struct(&taskInput); validationErr != nil {
			rlog.Errorc(ctx, "could not validate reqired fields", validationErr)
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields missing")
			rerr.GinLogErrorAbort(c)
			return
		}

		updatedTask, originalTask, err := tasksservice.Update(ctx, taskId, &taskInput)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "Could not update task", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if updatedTask == nil {
			rlog.Errorc(ctx, "Could not update task", fmt.Errorf("task does not exist"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not update task, does it exist?!")
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", updatedTask)
		c.Set("oldObject", originalTask)

		c.JSON(http.StatusOK, updatedTask)
	}
}

// @Summary	Delete a task
// @Schemes
// @Description	Delete a task by id
// @Tags			tasks
// @Accept			application/json
// @Produce		application/json
// @Param			id	path		string	true	"id"
// @Success		200	{bool}		true
// @Failure		403	{string}	Forbidden
// @Failure		400	{object}	rorerror.RorError
// @Failure		401	{string}	Unauthorized
// @Failure		500	{string}	Failure	message
// @Router			/v1/tasks/:id [delete]
// @Security		ApiKey || AccessToken
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		taskId := c.Param("taskId")
		if taskId == "" || len(taskId) == 0 {
			rlog.Errorc(ctx, "invalid id", fmt.Errorf("id is zero lenght"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: ror
		// Subject: acl
		// Access: delete
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectAcl)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, deletedTask, err := tasksservice.Delete(ctx, taskId)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not delete task", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("oldObject", deletedTask)

		c.JSON(http.StatusOK, result)
	}
}
