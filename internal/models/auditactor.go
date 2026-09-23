package models

import identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

// AuditActor identifies who performed an audited action. It is the audit
// record's own type, so stored entries stay independent of the authentication
// identity model.
//
// The field names match the subset of the previously stored user document that
// carried meaning, so existing audit records still decode.
type AuditActor struct {
	Type    string   `json:"type,omitempty"`
	Subject string   `json:"subject,omitempty"`
	Name    string   `json:"name,omitempty"`
	Email   string   `json:"email,omitempty"`
	Groups  []string `json:"groups,omitempty"`
}

// ActorFromIdentity builds the audit actor for an identity. Fields that cannot
// be resolved are left empty: an audit record is still written when the actor is
// only partially known, and non-user principals simply carry no email.
func ActorFromIdentity(identity identitymodels.Identity) AuditActor {
	actor := AuditActor{Type: string(identity.Type)}
	if subject, err := identity.GetSubject(); err == nil {
		actor.Subject = subject
	}
	if name, err := identity.GetName(); err == nil {
		actor.Name = name
	}
	if email, err := identity.GetEmail(); err == nil {
		actor.Email = email
	}
	if groups, err := identity.GetGroups(); err == nil {
		actor.Groups = groups
	}
	return actor
}

// ActorFor is the nilable variant of ActorFromIdentity, for the call sites that
// hold a *Identity. A nil identity yields the zero actor, so the audit entry is
// still written for an unattributable action.
func ActorFor(identity *identitymodels.Identity) AuditActor {
	if identity == nil {
		return AuditActor{}
	}

	return ActorFromIdentity(*identity)
}
