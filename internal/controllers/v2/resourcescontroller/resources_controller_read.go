package resourcescontroller

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror/pkg/handlers/ginresourcequeryhandler"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"
	"github.com/NorskHelsenett/ror/pkg/rorresources"

	"github.com/gin-gonic/gin"
)

// Get a list of cluster resources og given group/version/kind.
//
//	@Summary	Get resources
//	@Schemes
//	@Description	Get a list of resources
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//
// @Param q query string false "A general query string (NOT IMPLEMENTED YET)"
// @Param apiversion query string false "The API version for the resource (e.g., 'v1' or 'apps/v1')"
// @Param kind query string false "The kind of resource"
// @Param ownerrefs query string false "JSON array of owner references [{'scope': '...', 'subject': '...'}]"
// @Param uids query string false "Comma-separated list of UIDs"
// @Param fields query string false "Comma-separated list of fields to include"
// @Param sort query string false "Comma-separated list of fields to sort by (+field for ascending, -field for descending)"
// @Param filters query string false "JSON array of filter objects [{'field':'field1','value':'value1','type':'string','operator':'eq'}]"
// @Param offset query int false "Starting offset for pagination"
// @Param limit query int false "Maximum number of results to return"
// @Success		200				{object}		rorresources.ResourceSet
// @Failure		403				{string}	Forbidden
// @Failure		400				{object}	rorerror.RorError
// @Failure		401				{object}	rorerror.RorError
// @Failure		500				{string}	Failure	message
// @Router			/v2/resources [get]
// @Security		ApiKey || AccessToken
func GetResources() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var rsQuery *rorresources.ResourceQuery

		testQuery := c.Query("query") == ""
		if testQuery {
			rsQuery = ginresourcequeryhandler.ParseResourceQuery(c)
		}

		if !testQuery {
			// Decode the base64 query
			base64Query, err := base64.StdEncoding.DecodeString(c.Query("query"))
			if err != nil {
				c.JSON(http.StatusBadRequest, "400: Invalid base64 query")
				return
			}

			rsQuery = rorresources.NewResourceQuery()
			err = json.Unmarshal(base64Query, rsQuery)
			if err != nil {
				c.JSON(http.StatusBadRequest, "400: Invalid query")
				return
			}
		}
		if rsQuery == nil {
			c.JSON(http.StatusBadRequest, "400: Invalid query")
			return
		}

		if validationErr := validate.Struct(rsQuery); validationErr != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, validationErr.Error())
			rerr.GinLogErrorAbort(c)
			return
		}

		rsSet := resourcesv2service.GetResourceByQuery(ctx, rsQuery)

		c.JSON(http.StatusOK, rsSet)
	}
}

// Get a cluster resources og given group/version/kind/uid.
//
//	@Summary	Get resource
//	@Schemes
//	@Description	Get a resource by uid
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid				path		string				true	"The uid of the resource"
//	@Success		200				{array}		rorresources.Resource
//	@Failure		403				{string}	Forbidden
//	@Failure		401				{object}	rorerror.RorError
//	@Failure		500				{string}	Failure	message
//	@Router			/v2/resource/uid/{uid} [get]
//	@Security		ApiKey || AccessToken
func GetResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		if c.Param("uid") == "" {
			c.JSON(http.StatusBadRequest, "400: Missing uid")
			return
		}

		resources := resourcesv2service.GetResourceByUID(ctx, c.Param("uid"))
		if resources == nil {
			c.JSON(http.StatusNotFound, "404: Resource not found")
			return
		}
		c.JSON(http.StatusOK, resources.GetAll())
	}
}
