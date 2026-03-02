// TODO: Describe package
package datacenterscontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/datacentersservice"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	validate = validator.New()
}

// TODO: Describe function
//
//	@Summary	Get datacenters
//	@Schemes
//	@Description	Get datacenters
//	@Tags			datacenters
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{array}		apicontracts.Datacenter
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v1/datacenters	[get]
//	@Security		ApiKey || AccessToken
func GetAll() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		_, err := gincontext.GetUserFromGinContext(c)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "Could not get user", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		// importing apicontracts for swagger
		var _ apicontracts.Datacenter

		datacenters, err := datacentersservice.GetAllByUser(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "Could not get datacenters", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, datacenters)
	}
}

// TODO: Describe function
//
//	@Summary	Get datacenter by name
//	@Schemes
//	@Description	Get datacenter by name
//	@Tags			datacenters
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200									{object}	apicontracts.Datacenter
//	@Failure		403									{object}	rorerror.ErrorData
//	@Failure		401									{object}	rorerror.ErrorData
//	@Failure		500									{string}	Failure	message
//	@Router			/v1/datacenters/{datacenterName}	[get]
//	@Param			datacenterName						path	string	true	"datacenterName"
//	@Security		ApiKey || AccessToken
func GetByName() gin.HandlerFunc {
	// todo scheduled for deletion
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		datacenterName := c.Param("datacenterName")
		defer cancel()

		// importing apicontracts for swagger
		var _ apicontracts.Datacenter

		datacenter, err := datacentersservice.GetByName(ctx, datacenterName)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "Could not get datacenter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if datacenter == nil {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		c.JSON(http.StatusOK, datacenter)
	}
}

// @Summary	Get datacenter by id
// @Schemes
// @Description	Get datacenter by id
// @Tags			datacenters
// @Accept			application/json
// @Produce		application/json
// @Success		200						{object}	apicontracts.Datacenter
// @Failure		403						{string}	Forbidden
// @Failure		401						{string}	Unauthorized
// @Failure		500						{string}	Failure	message
// @Router			/v1/datacenters/id/{id}	[get]
// @Param			id						path	string	true	"id"
// @Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	// todo scheduled for deletion
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		datacenterId := c.Param("id")
		defer cancel()

		// importing apicontracts for swagger
		var _ apicontracts.Datacenter

		datacenter, err := datacentersservice.GetById(ctx, datacenterId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "Could not get datacenter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if datacenter == nil {
			c.JSON(http.StatusNotFound, nil)
			return
		}

		c.JSON(http.StatusOK, datacenter)
	}
}

// TODO: Describe function
//
//	@Summary	Create datacenter
//	@Schemes
//	@Description	Create datacenter
//	@Tags			datacenters
//	@Accept			application/json
//	@Produce		application/json
//	@Param			datacenter	body		apicontracts.Datacenter	true	"Datacenter"
//	@Success		200			{object}	apicontracts.Datacenter
//	@Failure		403			{string}	Forbidden
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v1/datacenters [post]
//	@Security		ApiKey || AccessToken
func Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		// Access check
		// Scope: ror
		// Subject: datacenter
		// Access: create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectDatacenter)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var datacenterInput apicontracts.DatacenterModel

		//validate the request body
		if err := c.BindJSON(&datacenterInput); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing body", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&datacenterInput); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		datacenter, err := datacentersservice.Create(ctx, &datacenterInput, identity.User)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not create datacenter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if datacenter == nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not create datacenter, does it already exists?! ", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, datacenter)
	}
}

// TODO: Describe function
//
//	@Summary	Update a datacenter
//	@Schemes
//	@Description	Update a datacenter by id
//	@Tags			datacenters
//	@Accept			application/json
//	@Produce		application/json
//	@Param			datacenterId	path		string					true	"datacenterId"
//	@Param			datacenter		body		apicontracts.Datacenter	true	"Datacenter"
//	@Success		200				{object}	apicontracts.Datacenter
//	@Failure		403				{string}	Forbidden
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{string}	Failure	message
//	@Router			/v1/datacenters/{datacenterId} [put]
//	@Security		ApiKey || AccessToken
func Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		datacenterId := c.Param("datacenterId")
		defer cancel()

		identity := rorcontext.GetIdentityFromRorContext(ctx)

		// Access check
		// Scope: ror
		// Subject: datacenter
		// Access: update
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectDatacenter)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Update {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var datacenterInput apicontracts.DatacenterModel

		//validate the request body
		if err := c.BindJSON(&datacenterInput); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing body", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&datacenterInput); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		datacenter, err := datacentersservice.Update(ctx, datacenterId, &datacenterInput, identity.User)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not update datacenter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if datacenter == nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Could not update datacenter, does it exists?!", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, datacenter)
	}
}
