package resourcesv2service

import (
	"context"
	"strconv"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apiresourcecontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
)

const (
	RESOURCECOLLECTION = "resourcesv2"
)

type ResourceDBProvider interface {
	Set(ctx context.Context, resource *rorresources.Resource) error
	Get(ctx context.Context, rorResourceQuery *rorresources.ResourceQuery) (*rorresources.ResourceSet, error)
	Del(ctx context.Context, resource *rorresources.Resource) error
	GetHashlistByQuery(ctx context.Context, rorResourceQuery *rorresources.ResourceQuery) (apiresourcecontracts.HashList, error)
}

// Mongodb implementation of ResourceDBProvider

type ResourceMongoDB struct {
	db *mongodb.MongodbCon
}

func NewResourceMongoDB(db *mongodb.MongodbCon) ResourceDBProvider {
	return &ResourceMongoDB{db: db}
}

func (r *ResourceMongoDB) Set(ctx context.Context, resource *rorresources.Resource) error {
	filter := bson.M{"uid": resource.GetUID()}
	update := bson.M{"$set": resource}
	_, err := r.db.UpsertOne(ctx, RESOURCECOLLECTION, filter, update)
	if err != nil {
		rlog.Errorc(ctx, "Failed to upsert resource", err)
		return err
	}
	return nil
}

func (r *ResourceMongoDB) Get(ctx context.Context, rorResourceQuery *rorresources.ResourceQuery) (*rorresources.ResourceSet, error) {
	query := GenerateAggregateQuery(ctx, rorResourceQuery)
	var resources = make([]rorresources.Resource, 0)
	err := r.db.Aggregate(ctx, RESOURCECOLLECTION, query, &resources)
	if err != nil {
		return nil, err
	}
	resourceSet := rorresources.NewResourceSet()
	if len(resources) > 0 {
		for _, resource := range resources {
			resourceSet.Add(rorresources.NewResourceFromStruct(resource))
		}
		return resourceSet, nil
	}
	return nil, nil
}

func (r *ResourceMongoDB) Del(ctx context.Context, resource *rorresources.Resource) error {
	filter := bson.M{"uid": resource.GetUID()}
	_, err := r.db.DeleteOne(ctx, RESOURCECOLLECTION, filter)
	if err != nil {
		return err
	}
	return nil
}

func (r *ResourceMongoDB) GetHashlistByQuery(ctx context.Context, rorResourceQuery *rorresources.ResourceQuery) (apiresourcecontracts.HashList, error) {
	hashList := apiresourcecontracts.HashList{}
	query := GenerateAggregateQuery(ctx, rorResourceQuery)

	project := bson.M{}
	project["hash"] = "$rormeta.hash"
	project["uid"] = "$metadata.uid"
	query = append(query, bson.M{"$project": project})

	//mongodb.NewMongodbQuery(query).PrettyPrint()
	mongodb.NewMongodbQuery(query).MongoshPrint(RESOURCECOLLECTION)

	hashItems := []apiresourcecontracts.HashItem{}
	err := r.db.Aggregate(ctx, RESOURCECOLLECTION, query, &hashItems)
	if err != nil {
		return hashList, err
	}
	hashList.Items = hashItems
	return hashList, nil
}

func GenerateAggregateQuery(ctx context.Context, rorResourceQuery *rorresources.ResourceQuery) []bson.M {
	query := make([]bson.M, 0)
	match := bson.M{}
	authorizedOwnerRefs := aclservice.GetOwnerrefByContextAccess(ctx, aclmodels.AccessTypeRead)
	match["rormeta.ownerref"] = bson.M{"$in": authorizedOwnerRefs}

	if rorResourceQuery == nil {
		return query
	}

	// Add filters
	if !rorResourceQuery.VersionKind.Empty() {
		apiversion, kind := rorResourceQuery.VersionKind.ToAPIVersionAndKind()
		if apiversion != "" {
			match["typemeta.apiversion"] = apiversion
		}
		if kind != "" {
			match["typemeta.kind"] = kind
		}
	}

	if len(rorResourceQuery.Uids) > 0 {
		match["uid"] = bson.M{"$in": rorResourceQuery.Uids}
	}

	if len(rorResourceQuery.OwnerRefs) > 0 {
		match["rormeta.ownerref"] = bson.M{"$in": rorResourceQuery.OwnerRefs}
	}

	if len(rorResourceQuery.Filters) == 1 {
		addMatchFilter(rorResourceQuery.Filters[0], match)
	} else if len(rorResourceQuery.Filters) > 1 {
		filterCount := map[string]int{}
		for _, filter := range rorResourceQuery.Filters {
			filterCount[filter.Field]++
		}

		for key, value := range filterCount {
			if value == 1 {
				for _, filter := range rorResourceQuery.Filters {
					if filter.Field == key {
						addMatchFilter(filter, match)
					}
				}
			} else {
				var filterList []string
				for _, filter := range rorResourceQuery.Filters {
					if filter.Field == key {
						filterList = append(filterList, filter.Value)
					}
				}
				match[key] = bson.M{"$in": filterList}
			}
		}
	}
	query = append(query, bson.M{"$match": match})

	// Add sorting
	sortaggregate := bson.M{}
	if len(rorResourceQuery.Order) != 0 {
		for _, orderline := range rorResourceQuery.GetOrderSorted() {
			if orderline.Descending {
				sortaggregate[orderline.Field] = -1
			} else {
				sortaggregate[orderline.Field] = 1
			}
		}

	} else {
		sortaggregate["metadata.name"] = 1
	}
	query = append(query, bson.M{"$sort": sortaggregate})
	// Add projection
	if len(rorResourceQuery.Fields) != 0 {
		project := bson.M{}
		project["metadata"] = 1
		project["rormeta"] = 1
		project["typemeta"] = 1
		for _, field := range rorResourceQuery.Fields {
			project[field] = 1
		}
		query = append(query, bson.M{"$project": project})
	}

	// Add offset and limit
	if rorResourceQuery.Offset != 0 {
		query = append(query, bson.M{"$skip": rorResourceQuery.Offset})
	}

	if rorResourceQuery.Limit > 1000 {
		rorResourceQuery.Limit = 1000
	}

	if rorResourceQuery.Limit == 0 {
		query = append(query, bson.M{"$limit": 100})
	}

	if rorResourceQuery.Limit != -1 {
		query = append(query, bson.M{"$limit": rorResourceQuery.Limit})
	}
	return query
}

func addMatchFilter(filter rorresources.ResourceQueryFilter, match bson.M) {
	switch filter.Type {
	case rorresources.FilterTypeString:
		if filter.Operator == "eq" {
			match[filter.Field] = bson.M{"$eq": filter.Value}
		}
		if filter.Operator == "ne" {
			match[filter.Field] = bson.M{"$ne": filter.Value}
		}
		if filter.Operator == "regexp" {
			match[filter.Field] = bson.M{"$regex": filter.Value, "$options": "i"}
		}
	case rorresources.FilterTypeInt:
		if filter.Operator == "eq" {
			match[filter.Field] = bson.M{"$eq": filter.Value}
		}
		if filter.Operator == "gt" {
			match[filter.Field] = bson.M{"$gt": filter.Value}
		}
		if filter.Operator == "lt" {
			match[filter.Field] = bson.M{"$lt": filter.Value}
		}
		if filter.Operator == "ge" {
			match[filter.Field] = bson.M{"$gte": filter.Value}
		}
		if filter.Operator == "le" {
			match[filter.Field] = bson.M{"$lte": filter.Value}
		}
	case rorresources.FilterTypeBool:
		if filter.Operator == "eq" {
			boolfilter, err := strconv.ParseBool(filter.Value)
			if err == nil {
				match[filter.Field] = bson.M{"$eq": boolfilter}
			}
		}
		// case rorresources.FilterTypeTime:
		// 	format := "2006-01-02 15:04:05.999999999 -0700 MST"
		// 	timevalue, err := time.Parse(format, filter.Value)
		// 	if err == nil {
		// 		//timevalue := time.ParseTimestamps(filter.Value)
		// 		if filter.Operator == "eq" {
		// 			match[filter.Field] = bson.M{"$eq": timevalue}
		// 		}
		// 		if filter.Operator == "gt" {
		// 			match[filter.Field] = bson.M{"$gt": timevalue}
		// 		}
		// 		if filter.Operator == "lt" {
		// 			match[filter.Field] = bson.M{"$lt": timevalue}
		// 		}
		// 		if filter.Operator == "ge" {
		// 			match[filter.Field] = bson.M{"$gte": timevalue}
		// 		}
		// 		if filter.Operator == "le" {
		// 			match[filter.Field] = bson.M{"$lte": timevalue}
		// 		}
		// 	}
	}
}
