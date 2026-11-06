package aclsnapshot

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

type Publisher interface {
	PublishSnapshot(context.Context, snapshot.Limits) ([]byte, error)
}

type SignPayload func(payload []byte, mediaType string) (string, error)

// Handler returns a complete authoritative ACL policy signed by ror-api.
//
//	@Summary	Fetch signed ACL snapshot
//	@Description	Requires the configured dedicated service API-key identity. Disabled until reader and explicit limits are configured.
//	@Tags	acl
//	@Produce	application/jose
//	@Param	If-None-Match	header	string	false	"Previously verified strong snapshot ETag; weak tags and wildcard are rejected"
//	@Success	200	{string}	string	"Compact RS512 JWS"
//	@Success	304	"No change"
//	@Failure	400	{object}	rorerror.ErrorData
//	@Failure	401	{object}	rorerror.ErrorData
//	@Failure	403	{object}	rorerror.ErrorData
//	@Failure	500	{object}	rorerror.ErrorData
//	@Failure	503	{object}	rorerror.ErrorData
//	@Router	/v2/acl/snapshot	[get]
//	@Security	ApiKey
func Handler(cfg Config, publisher Publisher, sign SignPayload) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-cache")
		varyEncoding(c.Writer.Header())
		if cfg.ReaderServiceID == "" {
			abort(c, http.StatusServiceUnavailable, "ACL snapshot endpoint is disabled", nil)
			return
		}
		value, exists := c.Get("identity")
		identity, ok := value.(identitymodels.Identity)
		if !exists || !ok {
			abort(c, http.StatusUnauthorized, "authenticated identity required", nil)
			return
		}
		if identity.Validate() != nil ||
			identity.Type != identitymodels.IdentityTypeService ||
			identity.Auth.AuthProvider != identitymodels.IdentityProviderApiKey ||
			identity.GetId() != cfg.ReaderServiceID {
			rlog.Infoc(c.Request.Context(), "ACL snapshot read denied",
				rlog.String("service", identity.GetId()))
			abort(c, http.StatusForbidden, "dedicated snapshot service API key required", nil)
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
				rlog.Infoc(ctx, "ACL snapshot read", rlog.String("service", identity.GetId()), rlog.Int("status", http.StatusNotModified))
				c.Status(http.StatusNotModified)
				return
			}
		}
		rlog.Infoc(ctx, "ACL snapshot read", rlog.String("service", identity.GetId()), rlog.Int("status", http.StatusOK))
		c.Data(http.StatusOK, "application/jose", []byte(envelope))
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
	if len(envelope) > limits.MaxEnvelopeBytes {
		return fmt.Errorf("signed snapshot exceeds envelope byte bound")
	}
	parts := strings.Split(envelope, ".")
	if len(parts) != 3 || parts[1] != base64.RawURLEncoding.EncodeToString(payload) || parts[2] == "" {
		return fmt.Errorf("signer did not preserve payload bytes")
	}
	if len(parts[0]) > base64.RawURLEncoding.EncodedLen(limits.MaxHeaderBytes) {
		return fmt.Errorf("protected header exceeds byte bound")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(header) > limits.MaxHeaderBytes {
		return fmt.Errorf("invalid protected header encoding")
	}
	var protected struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(header, &protected); err != nil {
		return fmt.Errorf("decode signing header: %w", err)
	}
	if protected.Algorithm != "RS512" || protected.Type != snapshot.MediaType ||
		protected.KeyID == "" || len(protected.KeyID) > limits.MaxFieldBytes {
		return fmt.Errorf("signer header does not match snapshot contract")
	}
	return nil
}
