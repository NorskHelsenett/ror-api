package projectscontroller

import (
	"fmt"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/customvalidators"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	projectService "github.com/NorskHelsenett/ror-api/internal/apiservices/projectsService"

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
	validate = validator.New()
	customvalidators.Setup(validate)
}

// @Summary	Create project
// @Schemes
// @Description	Create a project
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200				{object}	apicontracts.Project
// @Failure		403				{object}	rorerror.ErrorData
// @Failure		400				{object}	rorerror.ErrorData
// @Failure		401				{object}	rorerror.ErrorData
// @Failure		500				{object}	rorerror.ErrorData
// @Router			/v1/projects	[post]
// @Param			project			body	apicontracts.Project	true	"Project"
// @Security		ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: project
		// Access: create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectProject)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var project apicontracts.ProjectModel
		if err := c.BindJSON(&project); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if err := validate.Struct(&project); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not validate project object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		createdProject, err := projectService.Create(ctx, &project)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Unable to create project", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", createdProject)

		c.JSON(http.StatusOK, createdProject)
	}
}

// @Summary	Get projects by filter
// @Schemes
// @Description	Get projects by filter
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200					{object}	apicontracts.PaginatedResult[apicontracts.Project]
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		400					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v1/projects/filter	[get]
// @Param			filter				body	apicontracts.Filter	true	"Filter"
// @Security		ApiKey || AccessToken
func GetByFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var filter apicontracts.Filter
		if err := c.BindJSON(&filter); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if validationErr := validate.Struct(&filter); validationErr != nil {
			rlog.Errorc(ctx, "could validate input", validationErr)
			rerr := rorerror.NewRorError(http.StatusBadRequest, validationErr.Error())
			rerr.GinLogErrorAbort(c)
			return
		}

		result, err := projectService.GetByFilter(ctx, &filter)
		if err != nil {
			rlog.Errorc(ctx, "could not get projects", err)
			c.JSON(http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// @Summary	Get clusters by projectid
// @Schemes
// @Description	Get clusters by projectid
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200									{array}		apicontracts.ClusterInfo
// @Failure		403									{object}	rorerror.ErrorData
// @Failure		400									{object}	rorerror.ErrorData
// @Failure		401									{object}	rorerror.ErrorData
// @Failure		500									{object}	rorerror.ErrorData
// @Router			/v1/projects/{projectId}/clusters	[get]
// @Param			projectId							path	string	true	"projectId"
// @Security		ApiKey || AccessToken
func GetClustersByProjectId() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		projectId := c.Param("id")
		if projectId == "" || len(projectId) == 0 {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		clusters, err := projectService.GetClustersByProjectId(ctx, projectId)
		if err != nil {
			rlog.Errorc(ctx, "could not get projects", err)
			c.JSON(http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, clusters)
	}
}

// @Summary	Get projects by id
// @Schemes
// @Description	Get projects by id
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200							{object}	apicontracts.Project
// @Failure		403							{object}	rorerror.ErrorData
// @Failure		400							{object}	rorerror.ErrorData
// @Failure		401							{object}	rorerror.ErrorData
// @Failure		500							{object}	rorerror.ErrorData
// @Router			/v1/projects/{projectId}	[get]
// @Param			id							path	string	true	"id"
// @Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		projectId := c.Param("id")
		if projectId == "" || len(projectId) == 0 {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		object, err := projectService.GetById(ctx, projectId)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "could not get object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, object)
	}
}

// @Summary	Update project
// @Schemes
// @Description	Update a project by id
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200							{object}	apicontracts.PaginatedResult[apicontracts.Project]
// @Failure		403							{object}	rorerror.ErrorData
// @Failure		400							{object}	rorerror.ErrorData
// @Failure		401							{object}	rorerror.ErrorData
// @Failure		500							{object}	rorerror.ErrorData
// @Router			/v1/projects/{projectId}	[put]
// @Param			projectId					path	string					true	"projectId"
// @Param			project						body	apicontracts.Project	true	"Project"
// @Security		ApiKey || AccessToken
func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var input apicontracts.ProjectModel

		projectId := c.Param("id")
		if projectId == "" || len(projectId) == 0 {
			rlog.Errorc(ctx, "invalid id", fmt.Errorf("id is zero length"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: project
		// Subject: projectId
		// Access: update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeProject, projectId)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		//validate the request body
		err := c.BindJSON(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		err = validate.Struct(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		updatedObject, originalObject, err := projectService.Update(ctx, projectId, &input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "Could not update object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if updatedObject == nil {
			rlog.Errorc(ctx, "Could not update object", fmt.Errorf("object does not exist"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not update object, does it exist?!")
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", updatedObject)
		c.Set("oldObject", originalObject)
		c.JSON(http.StatusOK, updatedObject)
	}
}

// @Summary	Delete project
// @Schemes
// @Description	Delete a project by id
// @Tags			projects
// @Accept			application/json
// @Produce		application/json
// @Success		200							{bool}		bool
// @Failure		403							{object}	rorerror.ErrorData
// @Failure		400							{object}	rorerror.ErrorData
// @Failure		401							{object}	rorerror.ErrorData
// @Failure		500							{object}	rorerror.ErrorData
// @Router			/v1/projects/{projectId}	[delete]
// @Param			projectId					path	string	true	"projectId"
// @Security		ApiKey || AccessToken
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		projectId := c.Param("id")
		if projectId == "" || len(projectId) == 0 {
			rlog.Errorc(ctx, "invalid id", fmt.Errorf("id is zero length"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: project
		// Subject: projectId
		// Access: delete
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeProject, projectId)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, _, err := projectService.Delete(ctx, projectId)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not delete object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
