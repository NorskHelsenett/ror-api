package mongosafe

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type customString string

func TestEqKeepsOperatorLikeInputLiteral(t *testing.T) {
	filter := bson.M{"uid": Eq(`{"$ne": ""}`)}

	raw, err := bson.Marshal(filter)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := bson.Raw(raw).LookupErr("uid", "$eq")
	if err != nil {
		t.Fatalf("lookup uid.$eq: %v", err)
	}
	if s, ok := got.StringValueOK(); !ok || s != `{"$ne": ""}` {
		t.Fatalf("uid.$eq = %v, want the raw string", got)
	}
}

func TestEqSupportedTypes(t *testing.T) {
	if got := Eq(customString("x"))["$eq"]; got != customString("x") {
		t.Fatalf("named string: got %v", got)
	}
	id := bson.NewObjectID()
	if got := Eq(id)["$eq"]; got != id {
		t.Fatalf("object id: got %v", got)
	}
}

func TestFieldPath(t *testing.T) {
	for _, ok := range []string{"clusterid", "workspace.datacenter.name", "versions.nhn_tooling"} {
		if _, err := FieldPath(ok); err != nil {
			t.Errorf("FieldPath(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "$ROOT", "$$ROOT", "a..b", ".a", "a.", "a b", "a$b", "a{b}"} {
		if _, err := FieldPath(bad); err == nil {
			t.Errorf("FieldPath(%q) = nil, want error", bad)
		}
	}
}
