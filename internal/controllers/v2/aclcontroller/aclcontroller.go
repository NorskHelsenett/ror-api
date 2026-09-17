// The aclcontroller package provides controller functions for the /v2/acl endpoints.
// It resolves access through the V3 ACL backend (pkg/acl resolver).
package aclcontroller

import (
	"context"
	"net/http"
	"strings"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"
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
//	@Param			scope			path		string	true	"scope filter"
//	@Param			subject			path		string	true	"subject (uid) filter"
//	@Router			/v2/acl/lookup/{scope}/{subject}	[get]
//	@Security		ApiKey || AccessToken
func LookupAclByScopeSubject() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		scopeParam, err := aclscope.ParseScope(c.Param("scope"))
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "missing scope or wrong scope in path parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		subjectParam, err := aclscope.ParseSubject(scopeParam, c.Param("subject"))
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "missing subject or wrong subject in path parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		// Gate: the caller must be able to read the resource (incl. inherited access).
		allowed, err := aclservice.HasAccess(ctx, scopeParam, subjectParam, aclmodels.CapRor.WithVerb(aclmodels.VerbRead))
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not check access", err)
			rerr.GinLogErrorAbort(c)
			return
		}
		if !allowed {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "no read access to resource")
			rerr.GinLogErrorAbort(c)
			return
		}

		accessGroups, err := aclservice.GetAccessGroupsByScopeSubject(ctx, scopeParam, subjectParam)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not look up acl", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		resp := aclmodels.Acl3LookupByScopeSubjectResponse{
			Scope:      scopeParam,
			Subject:    subjectParam,
			AccesGroup: accessGroups,
		}
		c.JSON(http.StatusOK, resp)
	}
}

// CheckAccess checks if the caller has the specified access for the given scope and subject and accesstype using the V3 ACL backend.
//
//	@Summary	Check acl access by scope and subject and accesstype
//	@Schemes
//	@Description	CheckAccess checks if the caller has the specified access for the given scope and subject and accesstype using the V3 ACL backend.
//	@Tags			acl
//
// @Success      200            {string}  string  "Access Granted"
// @Header       200            {string}  Cache-Control   "Anti-caching directives"
// @Failure      401            {string}  string  "Unauthorized - Token missing or malformed"
// @Header       401            {string}  X-ROR-ERROR "Authentication scheme requirements"
// @Failure      403            {string}  string  "Forbidden - Insufficient permissions"
//
//	@Param			scope			path		string	true	"scope filter"
//	@Param			subject			path		string	true	"subject (uid) filter"
//	@Param			accesstype		path		string	true	"access type filter"
//	@Router			/v2/acl/lookup/{scope}/{subject}/{accesstype}	[head]
//	@Security		ApiKey || AccessToken
func CheckAccess() gin.HandlerFunc {
	return func(c *gin.Context) {

		// Implementation for checking access goes here.
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		scope := c.Param("scope")
		if scope == "" || len(scope) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid scope")
			rerr.GinLogErrorAbort(c)
			return
		}

		subject := c.Param("subject")
		if subject == "" || len(subject) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid subject")
			rerr.GinLogErrorAbort(c)
			return
		}

		access := c.Param("accesstype")
		if access == "" || len(access) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid accesstype")
			rerr.GinLogErrorAbort(c)
			return
		}

		v3access, err := aclmodels.ParseAccessTypeV3(access)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid accesstype")
			rerr.GinLogErrorAbort(c)
			return
		}

		v3scope, err := aclscope.ParseScope(scope)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid scope")
			rerr.GinLogErrorAbort(c)
			return
		}

		v3subject, err := aclscope.ParseSubject(v3scope, subject)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid subject or scope subject combination")
			rerr.GinLogErrorAbort(c)
			return
		}

		allowed, err := aclservice.HasAccess(ctx, v3scope, v3subject, v3access)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusUnauthorized, "failed to lookup acl")
			rerr.GinLogErrorAbort(c)
			return
		}
		if allowed {
			c.Status(http.StatusOK)
			return
		}

		c.Status(http.StatusForbidden)
	}
}

// aclManageAllowed gates ACL management on ScopeRor/SubjectAcl + the given verb.
// It writes the error response and returns false when not allowed.
func aclManageAllowed(c *gin.Context, ctx context.Context, verb aclmodels.Verb) bool {
	allowed, err := aclservice.HasAccess(ctx, aclscope.ScopeRor, aclscope.SubjectAcl, aclmodels.CapRor.WithVerb(verb))
	if err != nil {
		rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not check access", err)
		rerr.GinLogErrorAbort(c)
		return false
	}
	if !allowed {
		rerr := rorginerror.NewRorGinError(http.StatusForbidden, "no access")
		rerr.GinLogErrorAbort(c)
		return false
	}
	return true
}

// CreateAcl creates a V3 ACL entry (no V2 conversion; preserves all v3 capabilities).
//
//	@Summary	Create acl
//	@Schemes
//	@Description	Create a V3 ACL entry
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200		{object}	aclmodels.AclV3ListItem
//	@Failure		400		{object}	rorerror.ErrorData
//	@Failure		401		{object}	rorerror.ErrorData
//	@Failure		403		{object}	rorerror.ErrorData
//	@Failure		500		{object}	rorerror.ErrorData
//	@Param			acl		body		aclmodels.AclV3ListItem	true	"Acl"
//	@Router			/v2/acl	[post]
//	@Security		ApiKey || AccessToken
func CreateAcl() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.MustGetIdentityFromRorContext(ctx)
		if !aclManageAllowed(c, ctx, aclmodels.VerbCreate) {
			return
		}

		var item aclmodels.AclV3ListItem
		if err := c.BindJSON(&item); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not bind request body", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		created, err := aclservice.CreateV3(ctx, &item, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not create acl", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, messagebuscontracts.AclUpdateEvent{Action: "Create"}, messagebuscontracts.Route_Acl_Update, nil)
		c.JSON(http.StatusOK, created)
	}
}

// UpdateAcl updates a V3 ACL entry by id.
//
//	@Summary	Update acl
//	@Schemes
//	@Description	Update a V3 ACL entry by id
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200			{object}	aclmodels.AclV3ListItem
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		403			{object}	rorerror.ErrorData
//	@Failure		500			{object}	rorerror.ErrorData
//	@Param			id			path		string					true	"acl id"
//	@Param			acl			body		aclmodels.AclV3ListItem	true	"Acl"
//	@Router			/v2/acl/{id}	[put]
//	@Security		ApiKey || AccessToken
func UpdateAcl() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.MustGetIdentityFromRorContext(ctx)
		if !aclManageAllowed(c, ctx, aclmodels.VerbUpdate) {
			return
		}

		id := c.Param("id")
		if id == "" {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		var item aclmodels.AclV3ListItem
		if err := c.BindJSON(&item); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not bind request body", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		updated, err := aclservice.UpdateV3(ctx, id, &item, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not update acl", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, messagebuscontracts.AclUpdateEvent{Action: "Update"}, messagebuscontracts.Route_Acl_Update, nil)
		c.JSON(http.StatusOK, updated)
	}
}

// DeleteAcl deletes a V3 ACL entry by id.
//
//	@Summary	Delete acl
//	@Schemes
//	@Description	Delete a V3 ACL entry by id
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200			{boolean}	bool
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		403			{object}	rorerror.ErrorData
//	@Failure		500			{object}	rorerror.ErrorData
//	@Param			id			path		string	true	"acl id"
//	@Router			/v2/acl/{id}	[delete]
//	@Security		ApiKey || AccessToken
func DeleteAcl() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		identity := rorcontext.MustGetIdentityFromRorContext(ctx)
		if !aclManageAllowed(c, ctx, aclmodels.VerbDelete) {
			return
		}

		id := c.Param("id")
		if id == "" {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		ok, _, err := aclservice.DeleteV3(ctx, id, &identity)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not delete acl", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		_ = apiconnections.RabbitMQConnection.SendMessage(ctx, messagebuscontracts.AclUpdateEvent{Action: "Delete"}, messagebuscontracts.Route_Acl_Update, nil)
		c.JSON(http.StatusOK, ok)
	}
}

// GetAclById returns a V3 ACL entry by id.
//
//	@Summary	Get acl by id
//	@Schemes
//	@Description	Get a V3 ACL entry by id
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200			{object}	aclmodels.AclV3ListItem
//	@Failure		400			{object}	rorerror.ErrorData
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		403			{object}	rorerror.ErrorData
//	@Failure		404			{object}	rorerror.ErrorData
//	@Failure		500			{object}	rorerror.ErrorData
//	@Param			id			path		string	true	"acl id"
//	@Router			/v2/acl/{id}	[get]
//	@Security		ApiKey || AccessToken
func GetAclById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		if !aclManageAllowed(c, ctx, aclmodels.VerbRead) {
			return
		}

		id := c.Param("id")
		if id == "" {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid id")
			rerr.GinLogErrorAbort(c)
			return
		}

		item, err := aclservice.GetV3ById(ctx, id)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get acl", err)
			rerr.GinLogErrorAbort(c)
			return
		}
		if item == nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "acl not found")
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, item)
	}
}

// GetAclByFilter returns a page of V3 ACL entries matching the filter.
//
//	@Summary	Get acl by filter
//	@Schemes
//	@Description	Get a page of V3 ACL entries matching the filter
//	@Tags			acl
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200				{object}	apicontracts.PaginatedResult[aclmodels.AclV3ListItem]
//	@Failure		400				{object}	rorerror.ErrorData
//	@Failure		401				{object}	rorerror.ErrorData
//	@Failure		403				{object}	rorerror.ErrorData
//	@Failure		500				{object}	rorerror.ErrorData
//	@Param			filter			body		apicontracts.Filter	true	"Filter"
//	@Router			/v2/acl/filter	[post]
//	@Security		ApiKey || AccessToken
func GetAclByFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		if !aclManageAllowed(c, ctx, aclmodels.VerbRead) {
			return
		}

		var filter apicontracts.Filter
		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not bind filter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		result, err := aclservice.GetByFilterV3(ctx, &filter)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get acl by filter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
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
