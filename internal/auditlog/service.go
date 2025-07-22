package auditlog

import (
	"context"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/models"
	"github.com/NorskHelsenett/ror-api/internal/mongodbrepo/mongoTypes"
	auditlogrepo "github.com/NorskHelsenett/ror-api/internal/mongodbrepo/repositories/auditlogRepo"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
)

// Create creates a new auditlog entry in the database
func Create(ctx context.Context, msg string, category models.AuditCategory, action models.AuditAction, user *identitymodels.User, newObject any, oldObject any) (any, error) {
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

	insertedID, err := auditlogrepo.Create(ctx, auditLog)
	if err != nil {
		rlog.Error("failed to create auditlog", err, rlog.String("msg", msg), rlog.Any("category", category), rlog.Any("action", action))
	}

	return insertedID, nil
}
