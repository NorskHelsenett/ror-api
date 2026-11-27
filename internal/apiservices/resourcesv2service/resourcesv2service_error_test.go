package resourcesv2service

import (
	"context"
	"errors"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubResourceDB struct {
	getFn func(ctx context.Context, query *rorresources.ResourceQuery) (*rorresources.ResourceSet, error)
}

func (s stubResourceDB) Set(ctx context.Context, resource *rorresources.Resource) error {
	return nil
}

func (s stubResourceDB) Get(ctx context.Context, query *rorresources.ResourceQuery) (*rorresources.ResourceSet, error) {
	return s.getFn(ctx, query)
}

func (s stubResourceDB) Del(ctx context.Context, resource *rorresources.Resource) error {
	return nil
}

func (s stubResourceDB) GetHashlistByQuery(ctx context.Context, query *rorresources.ResourceQuery) (apiresourcecontracts.HashList, error) {
	return apiresourcecontracts.HashList{}, nil
}

func TestGetResourceByUID_ReturnsErrorWhenDatabaseLookupFails(t *testing.T) {
	origNewResourceDB := newResourceDB
	newResourceDB = func(_ *mongodb.MongodbCon) ResourceDBProvider {
		return stubResourceDB{
			getFn: func(ctx context.Context, query *rorresources.ResourceQuery) (*rorresources.ResourceSet, error) {
				return nil, errors.New("boom")
			},
		}
	}
	t.Cleanup(func() { newResourceDB = origNewResourceDB })

	result, err := GetResourceByUID(testCtx(), "uid-123")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "boom")
}
