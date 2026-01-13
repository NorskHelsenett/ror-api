package oauthmiddleware

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
)

// Function extracts groups from user object
func ExtractGroups(user *identitymodels.User) ([]string, error) {
	if user == nil {
		msg := "user is nil"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	emailArray := strings.Split(user.Email, "@")
	if len(emailArray) > 2 {
		msg := "could not extract domain from email"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	domain := emailArray[1]
	groups := make([]string, 0)
	for i := 0; i < len(user.Groups); i++ {
		g := fmt.Sprintf("%s@%s", user.Groups[i], domain)
		groups = append(groups, g)
	}

	return groups, nil
}

func extractTokenFromAuthorizationHeader(auth string) (string, rorginerror.RorGinError) {
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth {
		return "", rorginerror.NewRorGinError(http.StatusUnauthorized, "Could not find bearer token in Authorization header")
	}
	return token, nil
}

func extractUnverifiedClaims(token string) (unverifiedToken, rorginerror.RorGinError) {
	var unverifiedClaims unverifiedToken

	// Parse token without verification to extract issuer
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return unverifiedClaims, rorginerror.NewRorGinError(http.StatusUnauthorized, "Invalid token format")
	}

	// Decode base64 payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return unverifiedClaims, rorginerror.NewRorGinError(http.StatusUnauthorized, "Could not decode token payload")
	}

	err = json.Unmarshal(payload, &unverifiedClaims)

	if err != nil {
		return unverifiedClaims, rorginerror.NewRorGinError(http.StatusUnauthorized, "Could not parse issuer from token claims")
	}

	if unverifiedClaims.Issuer == "" {
		return unverifiedClaims, rorginerror.NewRorGinError(http.StatusUnauthorized, "Issuer claim is missing in token")
	}

	if len(unverifiedClaims.Audience) == 0 {
		return unverifiedClaims, rorginerror.NewRorGinError(http.StatusUnauthorized, "Audience claim is missing in token")
	}

	return unverifiedClaims, nil
}
