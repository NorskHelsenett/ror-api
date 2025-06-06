package workspacescontroller

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	workspacesservice "github.com/NorskHelsenett/ror-api/internal/apiservices/workspacesService"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	validate = validator.New()
}

// @Summary	Get workspaces
// @Schemes
// @Description	Get workspaces
// @Tags			workspaces
// @Accept			application/json
// @Produce		application/json
// @Success		200				{array}		apicontracts.Workspace
// @Failure		401				{object}	rorerror.RorError
// @Failure		403				{object}	rorerror.RorError
// @Success		404				{array}		apicontracts.Workspace
// @Router			/v1/workspaces	[get]
// @Security		ApiKey || AccessToken
func GetAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// importing apicontracts for swagger
		var _ apicontracts.Workspace

		workspaces, err := workspacesservice.GetAll(ctx)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusForbidden, "Could not get workspaces", err)
			rerr.GinLogErrorAbort(c)
		}

		if workspaces == nil {
			empty := make([]apicontracts.Workspace, 0)
			c.JSON(http.StatusNotFound, empty)
			return
		}

		c.JSON(http.StatusOK, workspaces)
	}
}

// @Summary	Get a workspace
// @Schemes
// @Description	Get a workspace its name
// @Tags			workspaces
// @Accept			application/json
// @Produce		application/json
// @Param			name	path		string	true	"name"
// @Success		200		{object}	apicontracts.Workspace
// @Failure		403		{string}	Forbidden
// @Failure		400		{object}	rorerror.RorError
// @Failure		401		{object}	rorerror.RorError
// @Failure		500		{object}	rorerror.RorError
// @Router			/v1/workspaces/{workspaceName} [get]
// @Security		ApiKey || AccessToken
func GetByName() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		workspaceName := c.Param("workspaceName")
		defer cancel()

		workspace, err := workspacesservice.GetByName(ctx, workspaceName)
		if err != nil {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		if workspace == nil {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		c.JSON(http.StatusOK, workspace)
	}
}

func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rlog.Errorc(ctx, "invalid id", fmt.Errorf("id is zero length"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: ror
		// Subject: global
		// Access: update
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var input apicontracts.Workspace
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

		updatedObject, originalObject, err := workspacesservice.Update(ctx, &input, id)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "Could not update object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if updatedObject == nil {
			rlog.Errorc(ctx, "could not update object", fmt.Errorf("object does not exist"))
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not update object, does it exist?!")
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", updatedObject)
		c.Set("oldObject", originalObject)

		c.JSON(http.StatusOK, updatedObject)
	}
}

// @Summary	Get a workspace by id
// @Schemes
// @Description	Get a workspace its id
// @Tags			workspaces
// @Accept			application/json
// @Produce		application/json
// @Param			id	path		string	true	"id"
// @Success		200	{object}	apicontracts.Workspace
// @Failure		403	{string}	Forbidden
// @Failure		400	{object}	rorerror.RorError
// @Failure		401	{object}	rorerror.RorError
// @Failure		500	{object}	rorerror.RorError
// @Router			/v1/workspaces/id/{workspaceName} [get]
// @Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		object, err := workspacesservice.GetById(ctx, id)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "could not get object", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, object)
	}
}

// Get a kubeconfig by workspace name.
// Identity must be authorized to view the requested cluster
//
//	@Summary	Get kubeconfig for workspace
//	@Schemes
//	@Description	Get a kubeconfig by workspace name.
//	@Tags			workspaces
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id			path		string								true	"id"
//	@Param			credentials	body		apicontracts.KubeconfigCredentials	true	"Credentials"
//	@Success		200			{object}	apicontracts.ClusterKubeconfig
//	@Failure		403			{string}	Forbidden
//	@Failure		401			{object}	rorerror.RorError
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/workspaces/{workspaceName}/login [post]
//	@Security		ApiKey || AccessToken
func GetKubeconfig() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		workspaceName := c.Param("workspaceName")
		if workspaceName == "" {
			rlog.Errorc(ctx, "workspace name must be provided", nil)
			c.JSON(http.StatusBadRequest, "workspace name must be provided")
			return
		}
		defer cancel()
		// Access check
		// Scope: ror
		// Subject: global
		// Access: owner
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal))
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Owner {
			rlog.Errorc(ctx, "403: No access", nil)
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var credentialPayload apicontracts.KubeconfigCredentials
		defer cancel()

		//validate the request body
		if err := c.BindJSON(&credentialPayload); err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if validationErr := validate.Struct(&credentialPayload); validationErr != nil {
			rlog.Errorc(ctx, "error when validating kubeconfig credentials", validationErr)
			rerr := rorerror.NewRorError(http.StatusBadRequest, validationErr.Error())
			rerr.GinLogErrorAbort(c)
			return
		}

		var result apicontracts.ClusterKubeconfig
		kubeconfigString, err := workspacesservice.GetKubeconfig(ctx, workspaceName, credentialPayload)
		if err != nil {
			rlog.Errorc(ctx, "error when fetching kubeconfig", err)
			if strings.Contains(err.Error(), "is not supported") {
				rlog.Debugc(ctx, "provider not supported")
				result.Status = "error"
				result.Message = "provider not supported"
				c.JSON(http.StatusBadRequest, result)
			} else if strings.Contains(err.Error(), "could not find workspace") {
				rlog.Debugc(ctx, "workspace not found")
				result.Status = "error"
				result.Message = "workspace not found"
				c.JSON(http.StatusNotFound, result)
			} else {
				rlog.Errorc(ctx, "error when fetching kubeconfig", err)
				result.Status = "error"
				result.Message = "error when fetching kubeconfig"
				c.JSON(http.StatusInternalServerError, result)
			}
			return
		}

		if len(kubeconfigString) == 0 {
			rlog.Errorc(ctx, "error, since kubeconfig is empty", nil)
			result.Status = "error"
			result.Message = "error, since kubeconfig is empty"
			c.JSON(http.StatusNotFound, result)
			return
		}

		kubeConfigEncoded := base64.StdEncoding.EncodeToString([]byte(kubeconfigString))
		result.Status = "success"
		result.Message = ""
		result.Data = kubeConfigEncoded
		result.DataType = "base64"

		c.JSON(http.StatusOK, result)
	}
}
