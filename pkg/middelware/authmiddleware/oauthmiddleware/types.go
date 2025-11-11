package oauthmiddleware

import "encoding/json"

type unverifiedToken struct {
	Issuer   string   `json:"iss"`
	Audience Audience `json:"aud"`
}

type Audience []string

func (a *Audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = Audience{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return err
	}
	*a = multi
	return nil
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
