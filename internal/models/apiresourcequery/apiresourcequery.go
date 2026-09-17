package apiresourcequery

import (
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/gin-gonic/gin"
)

func NewResourceQueryFromClient(c *gin.Context) apiresourcecontracts.ResourceQuery {

	owner := apiresourcecontracts.ResourceOwnerReference{
		Scope:   aclscope.Scope(c.Query("ownerScope")),
		Subject: c.Query("ownerSubject"),
	}

	query := apiresourcecontracts.ResourceQuery{
		Owner:      owner,
		Kind:       c.Query("kind"),
		ApiVersion: c.Query("apiversion"),
	}

	if query.Owner.Scope == aclscope.ScopeRor {
		query.Global = true
	}

	return query
}
