package viewcontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/services/viewservice"
	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/gin-gonic/gin"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apiview"
)

// Getview handles the HTTP GET request to retrieve a view.

// @Summary	Get view
// @Schemes
// @Description	Get view
// @Tags			view
// @Accept			application/json
// @Produce		application/json
// @Success		200	{object}	apiview.View
// @Failure		403	{object}	rorerror.RorError
// @Failure		401	{object}	rorerror.RorError
// @Failure		500	{object}	rorerror.RorError
// @Router			/v2/views/{viewid} [get]
// @Param			viewid		path	string							true	"The ID of the view to retrieve"
// @Param			limit	query	int							false	"Number of items to return, if set to -1, only metadata is returned"
// @Param			offset		query	int							false	"Number of items to skip before starting to collect the result set"
// @Param			sort		query	string							false	"Comma separated list of fields to sort by (e.g. name,-date)"
// @Param			filter		query	string							false	"Filter expression (e.g. name==example*,date>2020-01-01)"
// @Param			fields		query	string							false	"Comma separated list of extra fields to include in the response (e.g. workorder,branch,testfield1)"
// @Security		ApiKey || AccessToken
func GetView() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := gincontext.GetRorContextFromGinContext(c)
		_ = apiview.View{} // Ensure apiview is imported
		generator, err := viewservice.Generators.GetGenerator(c.Param("viewid"))
		if err == viewservice.ErrViewNotRegistered {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Invalid or unsupported view", err)
			rerr.GinLogErrorAbort(c)
		}
		options := viewservice.ParseOptionsFromGinContext(c)

		apiview, err := generator.GenerateView(ctx, options...)
		if err != nil {
			rerr := rorerror.NewRorErrorFromError(http.StatusInternalServerError, err)
			rerr.GinLogErrorAbort(c)
		}

		c.JSON(http.StatusOK, apiview)
	}
}

// @Summary	Get view
// @Schemes
// @Description	Get view
// @Tags			view
// @Accept			application/json
// @Produce		application/json
// @Success		200	{object}	[]apiview.ViewMetadata
// @Failure		403	{object}	rorerror.RorError
// @Failure		401	{object}	rorerror.RorError
// @Failure		500	{object}	rorerror.RorError
// @Router			/v2/views [get]
// @Security		ApiKey || AccessToken
func GetViews() gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = apiview.ViewMetadata{} // Ensure apiview is imported
		ret := make([]apiview.ViewMetadata, 0, len(viewservice.Generators))
		for _, generator := range viewservice.Generators {
			ret = append(ret, generator.GetMetadata())
		}
		c.JSON(http.StatusOK, ret)
	}
}
