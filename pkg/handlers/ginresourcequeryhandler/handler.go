package ginresourcequeryhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ParseGinResourceQuery parses a Gin context and returns a *rorresources.ResourceQuery object
// It handles the following query parameters:
// - q: A general query string
// - apiversion: The API version for the resource
// - kind: The kind of resource
// - ownerrefs: JSON array of owner references [{"scope": "...", "subject": "..."}]
// - uids: Comma-separated list of UIDs
// - fields: Comma-separated list of fields to include
// - sort: Comma-separated list of fields to sort by (+field for ascending, -field for descending)
// - filters: JSON array of filter objects [{"field":"field1","value":"value1","type":"string","operator":"eq"}]
// - offset: Starting offset for pagination
// - limit: Maximum number of results to return
func ParseGinResourceQuery(c *gin.Context) (*rorresources.ResourceQuery, error) {
	// Initialize a new resource query
	rq := rorresources.NewResourceQuery()

	// Parse APIVersion and Kind
	apiVersion := c.Query("apiversion")
	kind := c.Query("kind")

	gv := schema.GroupVersion{}
	if strings.Contains(apiVersion, "/") {
		parts := strings.Split(apiVersion, "/")
		gv.Group = parts[0]
		if len(parts) > 1 {
			gv.Version = parts[1]
		}
	} else {
		gv.Version = apiVersion
	}

	rq.VersionKind = schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    kind,
	}

	// Parse UIDs
	if uids := c.Query("uids"); uids != "" {
		rq.Uids = strings.Split(uids, ",")
	}

	// Parse OwnerRefs
	if ownerRefs := c.Query("ownerrefs"); ownerRefs != "" {
		var refs []rorresourceowner.RorResourceOwnerReference
		err := json.Unmarshal([]byte(ownerRefs), &refs)
		if err != nil {
			return nil, fmt.Errorf("could not parse ownerRefs from query: %w", err)

		}

		rq.OwnerRefs = refs
	}

	// Parse Fields
	if fields := c.Query("fields"); fields != "" {
		rq.Fields = strings.Split(fields, ",")
	}

	// Parse Sort/Order
	// The sort parameter can be a comma-separated list of fields with optional prefixes
	if sort := c.Query("sort"); sort != "" {
		fieldsList := strings.Split(sort, ",")
		orders := make([]rorresources.ResourceQueryOrder, 0, len(fieldsList))

		for i, field := range fieldsList {
			order := rorresources.ResourceQueryOrder{
				Index: i,
			}

			if strings.HasPrefix(field, " ") || strings.HasPrefix(field, "+") {
				order.Field = field[1:]
				order.Descending = false
			} else if strings.HasPrefix(field, "-") {
				order.Field = field[1:]
				order.Descending = true
			} else {
				order.Field = field
				order.Descending = false
			}

			orders = append(orders, order)
		}

		rq.Order = orders
	}

	// Parse Filters
	if filters := c.Query("filters"); filters != "" {
		var filterList []rorresources.ResourceQueryFilter
		err := json.Unmarshal([]byte(filters), &filterList)
		if err != nil {
			return nil, fmt.Errorf("could not unmarshal filters: %w", err)
		}

		rq.Filters = filterList
	}

	// Parse Offset
	if offset := c.Query("offset"); offset != "" {
		off, err := strconv.Atoi(offset)
		if err != nil {
			return nil, fmt.Errorf("could not parse offset from query: %w", err)
		}

		rq.Offset = off
	}

	// Parse Limit
	if limit := c.Query("limit"); limit != "" {
		lim, err := strconv.Atoi(limit)
		if err != nil {
			return nil, fmt.Errorf("could not parse limit from query: %w", err)
		}

		rq.Limit = lim
	}

	// Parse general query parameter
	if q := c.Query("q"); q != "" {
		// Handle general query parameter if needed
		// This could be used for full-text search or other purposes
	}

	return rq, nil
}

// ParseResourceQueryFromURL parses URL query parameters and returns a *rorresources.ResourceQuery object
// This is useful when you have a URL string instead of a Gin context
func ParseResourceQueryFromURL(urlStr string) (*rorresources.ResourceQuery, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	// Create a mock Gin context with the query parameters
	mockContext := &gin.Context{}
	mockContext.Request = &http.Request{
		URL: parsedURL,
	}

	query, err := ParseGinResourceQuery(mockContext)

	return query, err
}
