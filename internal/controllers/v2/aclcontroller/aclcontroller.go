// The aclcontroller package provides controller functions for the /v2/acl endpoints.
// It resolves access through the V3 ACL backend (pkg/acl resolver).
package aclcontroller

import (
	"net/http"
	"strings"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/gin-gonic/gin"
)

// LookupAcl resolves the scope+subject pairs the caller has the given access
// type for, using the V3 ACL backend.
//
//	@Summary	Lookup acl access
//	@Schemes
//	@Description	Lookup the scope+subject pairs the caller has the given access type for
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	aclmodels.AclV3LookupResponse
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Param			access			query		string	true	"Access type, e.g. kubernetes:logon"
//	@Param			scope			query		[]string	false	"Optional scope filter; repeat or comma-separate to narrow results, e.g. KubernetesCluster"
//	@Param			subject			query		[]string	false	"Optional subject (uid) filter; repeat or comma-separate to narrow results"
//	@Router			/v2/acl/lookup	[get]
//	@Security		ApiKey || AccessToken
func LookupAcl() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		accessParam := c.Query("access")
		if accessParam == "" {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "missing access query parameter")
			rerr.GinLogErrorAbort(c)
			return
		}

		access := aclmodels.AccessTypeV3(accessParam)
		if err := aclmodels.ValidateAccess(access); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid access type", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		filter := acl.OwnerrefFilter{}
		for _, s := range splitCSVParams(c.QueryArray("scope")) {
			filter.Scopes = append(filter.Scopes, aclscope.Scope(s))
		}
		for _, s := range splitCSVParams(c.QueryArray("subject")) {
			filter.Subjects = append(filter.Subjects, aclscope.Subject(s))
		}

		refs, unrestricted, err := aclservice.ResolveOwnerrefs(ctx, access, filter)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not resolve access", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		resp := aclmodels.AclV3LookupResponse{
			Access:       access,
			Unrestricted: unrestricted,
			Ownerrefs:    make([]aclmodels.AclV3LookupOwnerref, 0, len(refs)),
		}
		for _, ref := range refs {
			resp.Ownerrefs = append(resp.Ownerrefs, aclmodels.AclV3LookupOwnerref{
				Scope:   ref.Scope,
				Subject: ref.Subject,
			})
		}

		c.JSON(http.StatusOK, resp)
	}
}

// LookupAcl resolves the scope+subject pairs the caller has the given access
// type for, using the V3 ACL backend.
//
//	@Summary	Lookup acl access by scope and subject
//	@Schemes
//	@Description	Lookup the access group pairs the caller has access for, filtered by scope and subject.
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	aclmodels.Acl3LookupByScopeSubjectResponse
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Param			scope			path		[]string	true	"scope filter"
//	@Param			subject			path		[]string	true	"subject (uid) filter"
//	@Router			/v2/acl/lookup/{scope}/{subject}	[get]
//	@Security		ApiKey || AccessToken
func LookupAclByScopeSubject() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		scopeParam, ok := aclscope.ScopeFromString(c.Param("scope"))
		if !ok {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "missing scope or wrong scope in path parameter")
			rerr.GinLogErrorAbort(c)
			return
		}

		subjectParam, ok := aclscope.GetSubjectFromString(scopeParam, c.Param("subject"))
		if !ok {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "missing subject or wrong subject in path parameter")
			rerr.GinLogErrorAbort(c)
			return
		}

		_ = scopeParam
		_ = subjectParam
		_ = ctx

		resp := aclmodels.Acl3LookupByScopeSubjectResponse{
			Scope:      scopeParam,
			Subject:    subjectParam,
			AccesGroup: make([]aclmodels.Acl3LookupByScopeSubjectAccessGroup, 0),
		}
		c.JSON(http.StatusOK, resp)
	}
}

// splitCSVParams flattens query parameter values that may be supplied either as
// repeated keys (?scope=a&scope=b) or comma-separated (?scope=a,b), trimming
// whitespace and dropping empty entries.
func splitCSVParams(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
