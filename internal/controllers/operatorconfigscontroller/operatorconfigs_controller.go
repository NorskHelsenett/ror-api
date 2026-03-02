// TODO: Describe package
package operatorconfigscontroller

import (
	"fmt"
	"net/http"
	"strings"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/operatorconfigservice"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init operator config controller")
	validate = validator.New()
}

// TODO: Describe function
//
//	@Summary	Get an operator config
//	@Schemes
//	@Description	Get an operator config by id
//	@Tags			operatorconfigs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id	path		string	true	"id"
//	@Success		200	{object}	apicontracts.OperatorConfig
//	@Failure		403	{string}	Forbidden
//	@Failure		400	{object}	rorerror.ErrorData
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v1/operatorconfigs/{id} [get]
//	@Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		result, err := operatorconfigservice.GetById(ctx, id)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get operator config", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// TODO: Describe function
//
//	@Summary	Get all operator configs
//	@Schemes
//	@Description	Get all operator configs
//	@Tags			operatorconfigs
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200					{array}		apicontracts.OperatorConfig
//	@Failure		403					{string}	Forbidden
//	@Failure		401					{object}	rorerror.ErrorData
//	@Failure		500					{string}	Failure	message
//	@Router			/v1/operatorconfigs	[get]
//	@Security		ApiKey || AccessToken
func GetAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		elements, err := operatorconfigservice.GetAll(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not find operator configs ...", err)
			rerr.GinLogErrorAbort(c)
		}

		c.JSON(http.StatusOK, elements)
	}
}

// TODO: Describe function
//
//	@Summary	Create an operator config
//	@Schemes
//	@Description	Create an operator config
//	@Tags			operatorconfigs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			operatorconfig	body		apicontracts.OperatorConfig	true	"Add an operator config"
//	@Success		200				{object}	apicontracts.OperatorConfig
//	@Failure		403				{string}	Forbidden
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v1/operatorconfigs [post]
//	@Security		ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		// Access check
		// Scope: ror
		// Subject: global
		// Access: create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var config apicontracts.OperatorConfig
		//validate the request body
		if err := c.BindJSON(&config); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not validate operator config input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&config); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		created, err := operatorconfigservice.Create(ctx, &config)
		if err != nil {
			rlog.Errorc(ctx, "could not create operator config", err)
			if strings.Contains(err.Error(), "exists") {
				rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Already exists", err)
				rerr.GinLogErrorAbort(c)
				return
			}
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields are missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", created)

		c.JSON(http.StatusOK, created)
	}
}

// TODO: Describe function
//
//	@Summary	Update an operator config
//	@Schemes
//	@Description	Update an operator config by id
//	@Tags			operatorconfigs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id				path		string						true	"id"
//	@Param			operatorconfig	body		apicontracts.OperatorConfig	true	"Update operator config"
//	@Success		200				{object}	apicontracts.OperatorConfig
//	@Failure		403				{string}	Forbidden
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v1/operatorconfigs/{id} [put]
//	@Security		ApiKey || AccessToken
func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid operator config id", fmt.Errorf("id is zero length"))
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: ror
		// Subject: global
		// Access: update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var input apicontracts.OperatorConfig

		//validate the request body
		if err := c.BindJSON(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Input is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&input); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Required fields missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		updated, original, err := operatorconfigservice.Update(ctx, id, &input)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not update operator config", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if updated == nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not update operator config, does it exist?!", fmt.Errorf("object does not exist"))
			rerr.GinLogErrorAbort(c)
			return
		}

		c.Set("newObject", updated)
		c.Set("oldObject", original)

		c.JSON(http.StatusOK, updated)
	}
}

// TODO: Describe function
//
//	@Summary	Delete an operator config
//	@Schemes
//	@Description	Delete an operator config by id
//	@Tags			operatorconfigs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id	path		string	true	"id"
//	@Success		200	{boolean}	true
//	@Failure		403	{string}	Forbidden
//	@Failure		400	{object}	rorerror.ErrorData
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v1/operatorconfigs/{id} [delete]
//	@Security		ApiKey || AccessToken
func Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Invalid id", fmt.Errorf("id is zero length"))
			rerr.GinLogErrorAbort(c)
			return
		}
		// Access check
		// Scope: ror
		// Subject: global
		// Access: delete
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, err := operatorconfigservice.Delete(ctx, id)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not delete operator config", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
