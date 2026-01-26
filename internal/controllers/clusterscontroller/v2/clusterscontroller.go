package clusterscontroller

import "github.com/gin-gonic/gin"

type RegisterClusterRequest struct {
	ClusterId string `json:"clusterid"`
}

type RegisterClusterResponse struct {
	ClusterId string `json:"clusterid"`
	ApiKey    string `json:"apikey"`
}

// Register a cluster.
// Identity must be authorized to register a cluster
//
//	@Summary	Register a cluster
//	@Schemes
//	@Description	Register a cluster.
//	@Tags			clusters
//	@Accept			application/json
//	@Produce		application/json
//	@Param			data	body		RegisterClusterRequest	true	"data"
//	@Success		200	{object}	RegisterClusterResponse
//	@Failure		403	{string}	rorerror.ErrorData
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v2/clusters/register [post]
//	@Security		ApiKey || AccessToken
func RegisterCluster() gin.HandlerFunc {
	return func(c *gin.Context) {
	}
}
