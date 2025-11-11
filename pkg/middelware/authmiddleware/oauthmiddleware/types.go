package oauthmiddleware

type unverifiedToken struct {
	Issuer   string   `json:"iss"`
	Audience []string `json:"aud"`
}

func (u *unverifiedToken) MatchAudience(issuers ...string) (string, bool) {
	if len(issuers) == 0 {
		return "", false
	}
	for _, issuer := range issuers {
		for _, aud := range u.Audience {
			if aud == issuer {
				return aud, true
			}
		}
	}
	return "", false
}
