package handlerv2selfcontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/v2/apicontractsv2self"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init user controller")
	validate = validator.New()
}

// @Summary	Get self
// @Schemes
// @Description	Get user details
// @Tags			self
// @Accept			application/json
// @Produce		application/json
// @Success		200	{object}	apicontractsv2self.SelfData
// @Failure		403	{string}	Forbidden
// @Failure		401	{string}	Unauthorized
// @Failure		500	{string}	Failure	message
// @Router			/v2/self [get]
// @Security		ApiKey || AccessToken
func GetSelf() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := gincontext.GetRorContextFromGinContext(c)

		identity := rorcontext.MustGetIdentityFromRorContext(ctx)

		fail := func(err error) {
			rlog.Errorc(ctx, "could not resolve identity", err)
			c.JSON(http.StatusInternalServerError, "could not resolve identity")
		}

		// Every identity type exposes the same human readable name: a user's
		// display name, a cluster's cluster id or a service's id.
		name, err := identity.GetName()
		if err != nil {
			fail(err)
			return
		}

		result := apicontractsv2self.SelfData{
			Auth: identity.GetAuthInfo(),
			Type: identity.Type,
			User: apicontractsv2self.SelfUser{Name: name},
		}

		switch {
		case identity.IsUser():
			email, err := identity.GetEmail()
			if err != nil {
				fail(err)
				return
			}
			result.User.Email = email

			groups, err := identity.GetGroups()
			if err != nil {
				fail(err)
				return
			}
			if c.Query("filteredgroups") == "true" {
				groupsInUse, err := aclservice.GetGroupsInUse(ctx, groups)
				if err != nil {
					rlog.Errorc(ctx, "could not filter groups in use, falling back to unfiltered groups", err)
				} else {
					groups = groupsInUse
				}
			}
			result.User.Groups = groups
		case identity.IsCluster():
			uid, err := identity.GetSubject()
			if err != nil {
				fail(err)
				return
			}
			result.User.Uid = uid
		}

		c.JSON(http.StatusOK, result)
	}
}
