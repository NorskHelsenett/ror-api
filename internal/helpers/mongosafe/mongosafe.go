// Package mongosafe builds MongoDB filter values from untrusted input.
//
// Eq and FieldPath are registered as nosql-injection barriers for CodeQL in
// .github/codeql/extensions/ror-api-go-models.
package mongosafe

import (
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var fieldPathPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)

// Literal is the set of value types that can be safely matched with Eq.
type Literal interface {
	~string | bson.ObjectID
}

// Eq wraps v in an explicit $eq so it is always matched as a literal value and
// can never be interpreted as a query operator or aggregation expression.
//
//	bson.M{"uid": mongosafe.Eq(uid)}
func Eq[T Literal](v T) bson.M {
	return bson.M{"$eq": v}
}

// FieldPath validates an untrusted document field name such as "a.b_c" so it
// can be used as a filter key or as "$"+name in an aggregation expression.
func FieldPath(name string) (string, error) {
	if !fieldPathPattern.MatchString(name) {
		return "", fmt.Errorf("invalid field path %q", name)
	}
	return name, nil
}
