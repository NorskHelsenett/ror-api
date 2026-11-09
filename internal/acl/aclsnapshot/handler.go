package aclsnapshot

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

type Publisher interface {
	PublishSnapshot(context.Context, snapshot.Limits) ([]byte, error)
}

type SignPayload func(payload []byte, mediaType string) (string, error)
type AccessCheck func(context.Context, aclscope.Scope, aclscope.Subject, aclmodels.AccessTypeV3) (bool, error)

// Handler returns a complete authoritative ACL policy signed by ror-api.
//
//	@Summary	Fetch signed ACL snapshot
//	@Description	Requires the ror:acl:snapshot:read ACL capability on scope ror, subject acl. Limits have defaults and optional overrides.
//	@Tags	acl
//	@Produce	application/jose
//	@Param	If-None-Match	header	string	false	"Previously verified strong snapshot ETag; weak tags and wildcard are rejected"
//	@Success	200	{string}	string	"Compact RS512 JWS"
//	@Success	304	"No change"
//	@Failure	400	{object}	rorerror.ErrorData
//	@Failure	401	{object}	rorerror.ErrorData
//	@Failure	403	{object}	rorerror.ErrorData
//	@Failure	500	{object}	rorerror.ErrorData
//	@Router	/v2/acl/snapshot	[get]
//	@Security	ApiKey || AccessToken
func Handler(cfg Config, publisher Publisher, sign SignPayload, hasAccess AccessCheck) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-cache")
		varyEncoding(c.Writer.Header())
		value, exists := c.Get("identity")
		identity, ok := value.(identitymodels.Identity)
		if !exists || !ok {
			abort(c, http.StatusUnauthorized, "authenticated identity required", nil)
			return
		}
		if err := identity.Validate(); err != nil {
			abort(c, http.StatusUnauthorized, "valid authenticated identity required", err)
			return
		}
		conditional := strings.Join(c.Request.Header.Values("If-None-Match"), ",")
		tags, err := parseConditional(conditional, cfg.Limits.MaxHeaderBytes)
		if err != nil {
			abort(c, http.StatusBadRequest, "invalid If-None-Match", err)
			return
		}
		ctx := context.WithValue(c.Request.Context(), identitymodels.ContexIdentity, identity)
		if timeout, ok := c.Get("timeout"); ok {
			duration, valid := timeout.(time.Duration)
			if !valid || duration <= 0 {
				abort(c, http.StatusInternalServerError, "invalid request timeout", nil)
				return
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
		allowed, err := hasAccess(ctx, aclscope.ScopeRor, aclscope.SubjectAcl,
			aclmodels.CapRorAclSnapshot.WithVerb(aclmodels.VerbRead))
		if err != nil {
			abort(c, http.StatusInternalServerError, "unable to resolve snapshot access", err)
			return
		}
		if !allowed {
			rlog.Infoc(ctx, "ACL snapshot read denied", rlog.String("identity", identity.GetId()))
			abort(c, http.StatusForbidden, "ror:acl:snapshot:read ACL capability required", nil)
			return
		}
		payload, err := publisher.PublishSnapshot(ctx, cfg.Limits)
		if err != nil {
			abort(c, http.StatusInternalServerError, "unable to publish ACL snapshot", err)
			return
		}
		envelope, err := sign(payload, snapshot.MediaType)
		if err != nil {
			abort(c, http.StatusInternalServerError, "unable to sign ACL snapshot", err)
			return
		}
		if err := validateEnvelope(envelope, payload, cfg.Limits); err != nil {
			abort(c, http.StatusInternalServerError, "invalid signed ACL snapshot", err)
			return
		}
		hash := sha256.New()
		if c.Writer.Header().Get("Content-Encoding") == "gzip" {
			// Match the global middleware's encoding for a strong representation
			// ETag. Signing and consumer verification still use decoded JWS bytes.
			writer := gzip.NewWriter(hash)
			if _, err := writer.Write([]byte(envelope)); err != nil {
				abort(c, http.StatusInternalServerError, "unable to hash compressed ACL snapshot", err)
				return
			}
			if err := writer.Close(); err != nil {
				abort(c, http.StatusInternalServerError, "unable to hash compressed ACL snapshot", err)
				return
			}
		} else {
			_, _ = hash.Write([]byte(envelope))
		}
		etag := fmt.Sprintf("\"%x\"", hash.Sum(nil))
		c.Header("ETag", etag)
		for _, tag := range tags {
			if tag == etag {
				rlog.Infoc(ctx, "ACL snapshot read", rlog.String("identity", identity.GetId()), rlog.Int("status", http.StatusNotModified))
				c.Status(http.StatusNotModified)
				return
			}
		}
		rlog.Infoc(ctx, "ACL snapshot read", rlog.String("identity", identity.GetId()), rlog.Int("status", http.StatusOK))
		c.Data(http.StatusOK, snapshot.ContentType, []byte(envelope))
	}
}

func abort(c *gin.Context, status int, message string, err error) {
	if err != nil {
		rorginerror.NewRorGinError(status, message, err).GinLogErrorAbort(c)
		return
	}
	rorginerror.NewRorGinError(status, message).GinLogErrorAbort(c)
}

// The snapshot protocol requires strong representation tags; it explicitly
// rejects wildcard/weak conditions rather than returning an ambiguous 304.
func parseConditional(value string, maxBytes int) ([]string, error) {
	if len(value) > maxBytes {
		return nil, fmt.Errorf("conditional header exceeds configured bound")
	}
	value = strings.Trim(value, " \t")
	if value == "" {
		return nil, nil
	}
	if value == "*" {
		return nil, fmt.Errorf("snapshot conditional requests require an exact representation tag")
	}
	var tags []string
	for value != "" {
		if strings.HasPrefix(value, "W/") {
			return nil, fmt.Errorf("snapshot conditional requests require strong entity tags")
		}
		if len(value) < 2 || value[0] != '"' {
			return nil, fmt.Errorf("expected quoted entity tag")
		}
		end := strings.IndexByte(value[1:], '"')
		if end < 0 {
			return nil, fmt.Errorf("unterminated entity tag")
		}
		end += 2
		tag := value[:end]
		for _, b := range []byte(tag[1 : len(tag)-1]) {
			if b < 0x21 || b == 0x7f {
				return nil, fmt.Errorf("invalid entity tag character")
			}
		}
		tags = append(tags, tag)
		value = strings.Trim(value[end:], " \t")
		if value == "" {
			break
		}
		if value[0] != ',' {
			return nil, fmt.Errorf("expected entity tag separator")
		}
		value = strings.Trim(value[1:], " \t")
		if value == "" {
			return nil, fmt.Errorf("missing entity tag after separator")
		}
	}
	return tags, nil
}

func validateEnvelope(envelope string, payload []byte, limits snapshot.Limits) error {
	if _, err := snapshot.ParseHeader([]byte(envelope), limits); err != nil {
		return err
	}
	if strings.Split(envelope, ".")[1] != base64.RawURLEncoding.EncodeToString(payload) {
		return fmt.Errorf("signer did not preserve payload bytes")
	}
	return nil
}
