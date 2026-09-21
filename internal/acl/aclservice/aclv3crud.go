// V3-native ACL CRUD, used by the /v2/acl endpoints. Unlike the V1 path in
// aclservice.go these functions operate directly on aclmodels.AclV3ListItem with
// no V2 conversion, so V3-only capabilities (resource:*, ror:config:*, ...) are
// preserved end to end.
package aclservice

import (
	"context"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/auditlog"
	"github.com/NorskHelsenett/ror-api/internal/models"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

// validateV3Entry enforces the required fields plus scope/access validity.
func validateV3Entry(item *aclmodels.AclV3ListItem) error {
	if item.Group == "" {
		return fmt.Errorf("group is required")
	}
	if item.Scope == "" || item.Subject == "" {
		return fmt.Errorf("scope and subject are required")
	}
	if len(item.Access) == 0 {
		return fmt.Errorf("at least one access entry is required")
	}
	return aclmodels.ValidateACLEntry(*item)
}

// CreateV3 validates and persists a V3 ACL entry. Version, Created and IssuedBy
// are set server-side.
func CreateV3(ctx context.Context, item *aclmodels.AclV3ListItem, identity *identitymodels.Identity) (*aclmodels.AclV3ListItem, error) {
	if err := validateV3Entry(item); err != nil {
		return nil, fmt.Errorf("invalid acl entry: %w", err)
	}
	item.Version = 3
	item.Created = time.Now()
	if identity != nil && identity.User != nil {
		item.IssuedBy = identity.User.Email
	}

	created, err := Store().Create(ctx, *item)
	if err != nil {
		return nil, fmt.Errorf("could not create acl: %w", err)
	}

	if _, err := auditlog.Create(ctx, "ACL created", models.AuditCategoryAcl, models.AuditActionCreate, auditUser(identity), created, nil); err != nil {
		return nil, fmt.Errorf("could not audit log create action: %w", err)
	}
	return created, nil
}

// UpdateV3 validates and updates a V3 ACL entry by id.
func UpdateV3(ctx context.Context, aclId string, item *aclmodels.AclV3ListItem, identity *identitymodels.Identity) (*aclmodels.AclV3ListItem, error) {
	if err := validateV3Entry(item); err != nil {
		return nil, fmt.Errorf("invalid acl entry: %w", err)
	}
	item.Version = 3
	if identity != nil && identity.User != nil {
		item.IssuedBy = identity.User.Email
	}

	updated, previous, err := Store().Update(ctx, aclId, *item)
	if err != nil {
		return nil, fmt.Errorf("could not update acl: %w", err)
	}

	if _, err := auditlog.Create(ctx, "ACL updated", models.AuditCategoryAcl, models.AuditActionUpdate, auditUser(identity), updated, previous); err != nil {
		return nil, fmt.Errorf("could not audit log update action: %w", err)
	}
	return updated, nil
}

// DeleteV3 deletes a V3 ACL entry by id.
func DeleteV3(ctx context.Context, aclId string, identity *identitymodels.Identity) (bool, *aclmodels.AclV3ListItem, error) {
	// NOTE: mirrors the V1 delete guard; revisit under uniform-identity (roadmap 2e).
	if identity == nil || !identity.IsUser() {
		return false, nil, fmt.Errorf("could not delete object, must be deleted by a user")
	}

	deleted, err := Store().Delete(ctx, aclId)
	if err != nil {
		return false, nil, fmt.Errorf("could not delete acl: %w", err)
	}

	if _, err := auditlog.Create(ctx, "Acl deleted", models.AuditCategoryAcl, models.AuditActionDelete, identity.User, deleted, nil); err != nil {
		return false, nil, fmt.Errorf("could not audit log delete action: %w", err)
	}
	return true, deleted, nil
}

// GetV3ById returns a V3 ACL entry by id (nil when not found).
func GetV3ById(ctx context.Context, id string) (*aclmodels.AclV3ListItem, error) {
	entry, err := aclStore.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not get acl by id: %w", err)
	}
	return entry, nil
}

// GetByFilterV3 returns a page of V3 ACL entries matching the filter, plus the
// total match count before pagination.
func GetByFilterV3(ctx context.Context, filter *apicontracts.Filter) (*apicontracts.PaginatedResult[aclmodels.AclV3ListItem], error) {
	page, totalCount, err := getByFilterV3(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("error when getting acl by filter: %w", err)
	}

	offset := 0
	if filter != nil {
		offset = filter.Skip
	}

	return &apicontracts.PaginatedResult[aclmodels.AclV3ListItem]{
		Data:       page,
		DataCount:  int64(len(page)),
		Offset:     int64(offset),
		TotalCount: int64(totalCount),
	}, nil
}

// auditUser returns the user to attribute an audit entry to, or nil for
// non-user identities.
func auditUser(identity *identitymodels.Identity) *identitymodels.User {
	if identity == nil {
		return nil
	}
	return identity.User
}
