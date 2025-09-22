package listviewcontroller

import (
	"net/http"
	"strings"

	"github.com/NorskHelsenett/ror-api/pkg/services/listviewservice"
	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/gin-gonic/gin"
)

// GetListView handles the HTTP GET request to retrieve a listview.

// @Summary	Get listview
// @Schemes
// @Description	Get listview
// @Tags			listview
// @Accept			application/json
// @Produce		application/json
// @Success		200	{object}	apilistview.Listview
// @Failure		403	{string}	Forbidden
// @Failure		401	{string}	Unauthorized
// @Failure		500	{string}	Failure	message
// @Router			/v2/listview [get]
// @Param			list				query	ListViewType	true	"The list to generate"
// @Param			metadataOnly		query	bool			false	"Set to true to only get metadata (no items)"
// @Param			extraFields			query	string			false	"Comma separated list of extra fields to include in the response (e.g. workorder,branch,testfield1)"
// @Security		ApiKey || AccessToken
func GetListView() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := gincontext.GetRorContextFromGinContext(c)

		list := listviewservice.ListViewType(c.Query("list"))
		metadataOnly := c.Query("metadataOnly") == "true"
		extraFields := strings.Split(c.Query("extraFields"), ",")

		generator, exists := listviewservice.Generators[list]
		if !exists || generator == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unsupported list type"})
			return
		}

		apilistview, err := generator.GenerateListView(ctx, metadataOnly, extraFields)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Return the generated list view
		c.JSON(http.StatusOK, apilistview)
	}
}
