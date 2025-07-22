package auditlog

import (
	"context"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/models"
	"github.com/NorskHelsenett/ror-api/internal/mongodbrepo/mongoTypes"
	auditlogrepo "github.com/NorskHelsenett/ror-api/internal/mongodbrepo/repositories/auditlogRepo"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

// Create creates a new auditlog entry in the database
func Create(ctx context.Context, msg string, category models.AuditCategory, action models.AuditAction, user *identitymodels.User, newObject any, oldObject any) error {
	auditLog := mongoTypes.MongoAuditLog{}
	auditLogMetadata := mongoTypes.MongoAuditLogMetadata{}
	auditLogMetadata.Msg = msg
	auditLogMetadata.Timestamp = time.Now()
	auditLogMetadata.Category = category
	auditLogMetadata.Action = action
	auditLogMetadata.User = *user
	auditLog.Metadata = auditLogMetadata
	data := make(map[string]any)
	data["new_object"] = newObject
	data["old_object"] = oldObject
	auditLog.Data = data
	err := auditlogrepo.Create(ctx, auditLog)
	if err != nil {
		return fmt.Errorf("could not create auditlog: %v", err)
	}
	return nil
}
