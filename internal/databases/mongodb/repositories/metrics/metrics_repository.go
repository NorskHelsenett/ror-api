// TODO: This library is imported from ror, should determine if its a public library or not
package metrics

import (
	"context"
	"errors"
	"fmt"

	aclrepo "github.com/NorskHelsenett/ror-api/internal/acl/repositories"
	"github.com/NorskHelsenett/ror-api/internal/helpers/mapping"
	mongoHelper "github.com/NorskHelsenett/ror-api/internal/helpers/mongoHelper"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"go.mongodb.org/mongo-driver/mongo"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	CollectionName = "clusters"
)

func getGroupBaseQuery() bson.M {
	return bson.M{
		"totalCpu":              bson.M{"$sum": "$metrics.cpu"},
		"totalMemory":           bson.M{"$sum": "$metrics.memory"},
		"totalCpuConsumed":      bson.M{"$sum": "$metrics.cpuconsumed"},
		"totalMemoryConsumed":   bson.M{"$sum": "$metrics.memoryconsumed"},
		"totalNodePoolCount":    bson.M{"$sum": "$metrics.nodepoolcount"},
		"totalNodeCount":        bson.M{"$sum": "$metrics.nodecount"},
		"totalClusterCount":     bson.M{"$sum": "$metrics.clustercount"},
		"totalWorkspaceCount":   bson.M{"$sum": "$metrics.workspacecount"},
		"totalPriceMonth":       bson.M{"$sum": "$metrics.pricemonth"},
		"totalPriceYear":        bson.M{"$sum": "$metrics.priceyear"},
		"totalCpuPercentage":    bson.M{"$avg": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$metrics.cpu", 0}}, 0, bson.M{"$divide": bson.A{"$metrics.cpuconsumed", bson.M{"$multiply": bson.A{"$metrics.cpu", 10}}}}}}},
		"totalMemoryPercentage": bson.M{"$avg": bson.M{"$multiply": bson.A{bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$metrics.memory", 0}}, 0, bson.M{"$divide": bson.A{"$metrics.memoryconsumed", "$metrics.memory"}}}}, 100}}},
	}
}

func GetTotal(ctx context.Context) (*apicontracts.MetricsTotal, error) {
	db := mongodb.GetMongoDb()
	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = "$id"

	query := []bson.M{
		{"$group": groupQuery},
	}
	resultsTotal, err := db.Collection(CollectionName).Aggregate(ctx, query)
	if err != nil {
		return nil, errors.New("could get metrics")
	}

	if resultsTotal.RemainingBatchLength() == 0 {
		return nil, nil
	}

	var acc []bson.M
	if err = resultsTotal.All(ctx, &acc); err != nil {
		return nil, errors.New("could not extract metrics")
	}
	metrics := GetMetricFromPrimitivM(acc[0])
	total := apicontracts.MetricsTotal{
		Cpu:              metrics.Cpu,
		Memory:           metrics.Memory,
		CpuConsumed:      metrics.CpuConsumed,
		MemoryConsumed:   metrics.MemoryConsumed,
		CpuPercentage:    metrics.CpuPercentage,
		MemoryPercentage: metrics.MemoryPercentage,
		NodePoolCount:    metrics.NodePoolCount,
		NodeCount:        metrics.NodeCount,
		ClusterCount:     metrics.ClusterCount,
		WorkspaceCount:   0,
		DatacenterCount:  0,
	}

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(resultsTotal, ctx)

	return &total, nil
}

func GetTotalByUser(ctx context.Context) (*apicontracts.MetricsTotal, error) {
	db := mongodb.GetMongoDb()
	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = "$id"

	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)

	queryFiltered := []bson.M{
		accessQuery,
		{"$group": groupQuery},
	}

	resultsFiltered, err := db.Collection(CollectionName).Aggregate(ctx, queryFiltered)
	if err != nil {
		return nil, errors.New("could not get metrics by user")
	}

	var accFiltered []bson.M
	if err = resultsFiltered.All(ctx, &accFiltered); err != nil {
		return nil, errors.New("could not get metrics by user")
	}

	filtered := apicontracts.Metrics{}

	if len(accFiltered) > 0 {
		filtered = GetMetricFromPrimitivM(accFiltered[0])
	}

	totalByUser := apicontracts.MetricsTotal{
		Cpu:              filtered.Cpu,
		Memory:           filtered.Memory,
		CpuConsumed:      filtered.CpuConsumed,
		MemoryConsumed:   filtered.MemoryConsumed,
		CpuPercentage:    filtered.CpuPercentage,
		MemoryPercentage: filtered.MemoryPercentage,
		NodePoolCount:    filtered.NodePoolCount,
		NodeCount:        filtered.NodeCount,
		ClusterCount:     filtered.ClusterCount,
		WorkspaceCount:   0,
		DatacenterCount:  0,
	}

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(resultsFiltered, ctx)

	return &totalByUser, nil
}

func GetForDatacenters(ctx context.Context) (*apicontracts.MetricList, error) {
	db := mongodb.GetMongoDb()
	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = bson.M{"$toString": "$workspace.datacenter._id"}
	groupQuery["_name"] = bson.M{"$first": "$workspace.datacenter.name"}

	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)

	queryCount := []bson.M{
		accessQuery,
		{
			"$lookup": bson.M{
				"from":         "workspaces",
				"localField":   "workspaceid",
				"foreignField": "_id",
				"as":           "workspaces",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"$first": "$workspaces",
				},
			},
		},
		{
			"$lookup": bson.M{
				"from":         "datacenters",
				"localField":   "workspaces.datacenterid",
				"foreignField": "_id",
				"as":           "datacenters",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"datacenter": bson.M{
						"$first": "$datacenters",
					},
				},
			},
		},
		{"$group": groupQuery},
		{"$sort": bson.M{"_id": 1}},
	}

	results, err := db.Collection(CollectionName).Aggregate(ctx, queryCount)
	if err != nil {
		return nil, errors.New("Could not get metrics for datacenters")
	}

	if results.RemainingBatchLength() == 0 {
		return nil, nil
	}

	response := apicontracts.MetricList{}

	var acc []bson.M
	if err = results.All(ctx, &acc); err != nil {
		return nil, errors.New("Could not get metrics for datacenters")
	}

	if len(acc) == 0 {
		return nil, errors.New("Could not get metrics for datacenters")
	}

	for i := 0; i < len(acc); i++ {
		data := acc[i]
		metrics := GetMetricFromPrimitivM(data)
		item := apicontracts.MetricItem{
			Id:      fmt.Sprint(data["_id"]),
			Metrics: metrics,
		}

		response.Items = append(response.Items, item)
	}

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(results, ctx)

	return &response, nil
}

func GetForDatacenterId(ctx context.Context, datacenterId string) (*apicontracts.MetricItem, error) {
	dcId, err := primitive.ObjectIDFromHex(datacenterId)
	if err != nil {
		return nil, errors.New("Could not get metrics for datacenterid")
	}
	db := mongodb.GetMongoDb()
	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = bson.M{"$toString": "$workspace.datacenter._id"}
	groupQuery["_name"] = bson.M{"$first": "$workspace.datacenter.name"}

	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)
	queryCount := []bson.M{
		accessQuery,
		{
			"$lookup": bson.M{
				"from":         "workspaces",
				"localField":   "workspaceid",
				"foreignField": "_id",
				"as":           "workspaces",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"$first": "$workspaces",
				},
			},
		},
		{
			"$lookup": bson.M{
				"from":         "datacenters",
				"localField":   "workspaces.datacenterid",
				"foreignField": "_id",
				"as":           "datacenters",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"datacenter": bson.M{
						"$first": "$datacenters",
					},
				},
			},
		},
		{"$match": bson.M{"workspace.datacenter._id": dcId}},
		{"$group": groupQuery},
		{"$sort": bson.M{"id": 1}},
	}

	results, err := db.Collection(CollectionName).Aggregate(ctx, queryCount)
	if err != nil {
		return nil, errors.New("Could not get metrics for datacentername")
	}

	if results.RemainingBatchLength() == 0 {
		return nil, nil
	}

	response := apicontracts.MetricItem{}

	var acc []bson.M
	if err = results.All(ctx, &acc); err != nil {
		return nil, errors.New("Could not get metrics for datacentername")
	}

	if len(acc) > 1 {
		return nil, errors.New("Could not get metrics for datacentername")
	}

	data := acc[0]
	metrics := GetMetricFromPrimitivM(data)
	response.Id = fmt.Sprint(data["_id"])
	response.Name = fmt.Sprint(data["_name"])
	response.Metrics = metrics

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(results, ctx)

	return &response, nil
}

func GetForWorkspaces(ctx context.Context, filter *apicontracts.Filter) (*apicontracts.PaginatedResult[apicontracts.Metric], error) {
	db := mongodb.GetMongoDb()
	bsonSort := bson.M{}
	for i := 0; i < len(filter.Sort); i++ {
		sort := filter.Sort[i]
		if sort.SortField == "" {
			continue
		}

		bsonSort[sort.SortField] = sort.SortOrder
	}

	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = bson.M{"$toString": "$workspace._id"}
	groupQuery["_name"] = bson.M{"$first": "$workspace.name"}
	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)

	queryCount := []bson.M{
		accessQuery,
		{
			"$lookup": bson.M{
				"from":         "workspaces",
				"localField":   "workspaceid",
				"foreignField": "_id",
				"as":           "workspaces",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"$first": "$workspaces",
				},
			},
		}, {
			"$lookup": bson.M{
				"from":         "datacenters",
				"localField":   "workspaces.datacenterid",
				"foreignField": "_id",
				"as":           "datacenters",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"datacenter": bson.M{
						"$first": "$datacenters",
					},
				},
			},
		},
		{"$group": groupQuery},
		{"$sort": bsonSort},
		{"$skip": filter.Skip},
		{"$limit": filter.Limit},
	}

	totalQuery := []bson.M{
		accessQuery,
		{"$group": groupQuery},
	}

	clusterCollection := db.Collection(CollectionName)
	totalQueryCountCursor, err := clusterCollection.Aggregate(ctx, totalQuery)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspaces")
	}

	totalQueryCount := totalQueryCountCursor.RemainingBatchLength()
	results, err := clusterCollection.Aggregate(ctx, queryCount)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspaces")
	}

	if results.RemainingBatchLength() == 0 {
		return nil, nil
	}

	var metrics []apicontracts.Metric

	var agg []bson.M
	if err = results.All(ctx, &agg); err != nil {
		return nil, errors.New("Could not get metrics for workspaces")
	}

	lengthAgg := len(agg)
	if lengthAgg == 0 {
		return nil, errors.New("Could not get metrics for workspaces")
	}

	for i := 0; i < lengthAgg; i++ {
		data := agg[i]
		metric := GetMetricItemFromPrimitivM(data)
		metric.Id = fmt.Sprint(data["_id"])
		metric.Name = fmt.Sprint(data["_name"])
		metrics = append(metrics, metric)
	}

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(totalQueryCountCursor, ctx)
	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(results, ctx)

	paginatedResult := apicontracts.PaginatedResult[apicontracts.Metric]{}
	paginatedResult.Data = metrics
	paginatedResult.DataCount = int64(len(metrics))
	paginatedResult.TotalCount = int64(totalQueryCount)
	paginatedResult.Offset = int64(filter.Skip)

	return &paginatedResult, nil
}

func GetForWorkspacesByDatacenterId(ctx context.Context, filter *apicontracts.Filter, datacenterId string) (*apicontracts.PaginatedResult[apicontracts.Metric], error) {
	dcId, err := primitive.ObjectIDFromHex(datacenterId)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspaces by datacenter")
	}
	db := mongodb.GetMongoDb()
	bsonSort := bson.M{}
	for i := 0; i < len(filter.Sort); i++ {
		sort := filter.Sort[i]
		if sort.SortField == "" {
			continue
		}
		bsonSort[sort.SortField] = sort.SortOrder
	}

	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = bson.M{"$toString": "$workspace._id"}
	groupQuery["_name"] = bson.M{"$first": "$workspace.name"}

	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)

	queryCount := []bson.M{
		accessQuery,
		{
			"$lookup": bson.M{
				"from":         "workspaces",
				"localField":   "workspaceid",
				"foreignField": "_id",
				"as":           "workspaces",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"$first": "$workspaces",
				},
			},
		},
		{
			"$lookup": bson.M{
				"from":         "datacenters",
				"localField":   "workspaces.datacenterid",
				"foreignField": "_id",
				"as":           "datacenters",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"datacenter": bson.M{
						"$first": "$datacenters",
					},
				},
			},
		},
		{"$match": bson.M{"workspace.datacenter._id": dcId}},
		{"$group": groupQuery},
		{"$sort": bsonSort},
		{"$skip": filter.Skip},
		{"$limit": filter.Limit},
	}

	totalQuery := []bson.M{
		{"$match": bson.M{"workspace.datacenter._id": dcId}},
		{"$group": groupQuery},
	}

	clusterCollection := db.Collection(CollectionName)
	totalQueryCountCursor, err := clusterCollection.Aggregate(ctx, totalQuery)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspaces by datacenter")
	}

	totalQueryCount := totalQueryCountCursor.RemainingBatchLength()

	results, err := clusterCollection.Aggregate(ctx, queryCount)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspaces by datacenter")
	}

	if results.RemainingBatchLength() == 0 {
		return nil, nil
	}

	var metrics []apicontracts.Metric

	var agg []bson.M
	if err = results.All(ctx, &agg); err != nil {
		return nil, errors.New("Could not get metrics for workspaces by datacenter")
	}

	lengthAgg := len(agg)
	if lengthAgg == 0 {
		return nil, errors.New("Could not get metrics for workspaces by datacenter")
	}

	for i := 0; i < lengthAgg; i++ {
		data := agg[i]
		metric := GetMetricItemFromPrimitivM(data)
		metric.Id = fmt.Sprint(data["_id"])
		metric.Name = fmt.Sprint(data["_name"])
		metrics = append(metrics, metric)
	}

	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(totalQueryCountCursor, ctx)
	defer func(cursor *mongo.Cursor, databaseCtx context.Context) {
		_ = cursor.Close(databaseCtx)
	}(results, ctx)

	paginatedResult := apicontracts.PaginatedResult[apicontracts.Metric]{}
	paginatedResult.Data = metrics
	paginatedResult.DataCount = int64(len(metrics))
	paginatedResult.TotalCount = int64(totalQueryCount)
	paginatedResult.Offset = int64(filter.Skip)

	return &paginatedResult, nil
}

func GetForWorkspaceId(ctx context.Context, workspaceName string) (*apicontracts.MetricItem, error) {
	wId, err := primitive.ObjectIDFromHex(workspaceName)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspace")
	}
	db := mongodb.GetMongoDb()
	groupQuery := getGroupBaseQuery()
	groupQuery["_id"] = bson.M{"$toString": "$workspace._id"}
	groupQuery["_name"] = bson.M{"$first": "$workspace.name"}

	accessLists := aclrepo.GetACL2ByIdentityQuery(ctx, aclmodels.AclV2QueryAccessScope{Scope: aclmodels.Acl2ScopeCluster})
	accessQuery := mongoHelper.CreateClusterACLFilter(accessLists)

	queryCount := []bson.M{
		accessQuery,
		{
			"$lookup": bson.M{
				"from":         "workspaces",
				"localField":   "workspaceid",
				"foreignField": "_id",
				"as":           "workspaces",
			},
		},
		{
			"$set": bson.M{
				"workspace": bson.M{
					"$first": "$workspaces",
				},
			},
		},
		{"$match": bson.M{"workspace._id": wId}},
		{"$group": groupQuery},
		{"$sort": bson.M{"id": 1}},
	}

	results, err := db.Collection(CollectionName).Aggregate(ctx, queryCount)
	if err != nil {
		return nil, errors.New("Could not get metrics for workspace")
	}

	if results.RemainingBatchLength() == 0 {
		return nil, nil
	}

	response := apicontracts.MetricItem{}

	var acc []bson.M
	if err = results.All(ctx, &acc); err != nil {
		return nil, errors.New("Could not get metrics for workspace")
	}

	if len(acc) > 1 {
		return nil, errors.New("Could not get metrics for workspace")
	}

	data := acc[0]
	metric := GetMetricFromPrimitivM(data)
	response.Id = fmt.Sprint(data["_id"])
	response.Name = fmt.Sprint(data["_name"])
	response.Metrics = metric

	defer func(metricsCursor *mongo.Cursor, databaseCtx context.Context) {
		_ = metricsCursor.Close(databaseCtx)
	}(results, ctx)

	return &response, nil
}

func GetMetricFromPrimitivM(data primitive.M) apicontracts.Metrics {
	totalCpu, err := mapping.InterfaceToInt64(data["totalCpu"])
	if err != nil {
		totalCpu = 0
	}

	totalClusters, err := mapping.InterfaceToInt64(data["totalClusterCount"])
	if err != nil {
		totalClusters = 0
	}

	totalCpuConsumed, err := mapping.InterfaceToInt64(data["totalCpuConsumed"])
	if err != nil {
		totalCpuConsumed = 0
	}

	totalMemory, err := mapping.InterfaceToInt64(data["totalMemory"])
	if err != nil {
		totalMemory = 0
	}

	totalMemoryConsumed, err := mapping.InterfaceToInt64(data["totalMemoryConsumed"])
	if err != nil {
		totalMemoryConsumed = 0
	}

	totalNodeCount, err := mapping.InterfaceToInt64(data["totalNodeCount"])
	if err != nil {
		totalNodeCount = 0
	}

	totalNodePoolCount, err := mapping.InterfaceToInt64(data["totalNodePoolCount"])
	if err != nil {
		totalNodePoolCount = 0
	}

	totalCpuPercentage, err := mapping.InterfaceToInt64(data["totalCpuPercentage"])
	if err != nil {
		totalCpuPercentage = 0
	}

	totalMemoryPercentage, err := mapping.InterfaceToInt64(data["totalMemoryPercentage"])
	if err != nil {
		totalMemoryPercentage = 0
	}

	totalPriceMonth, err := mapping.InterfaceToInt64(data["totalPriceMonth"])
	if err != nil {
		totalPriceMonth = 0
	}

	totalPriceYear, err := mapping.InterfaceToInt64(data["totalPriceYear"])
	if err != nil {
		totalPriceYear = 0
	}

	return apicontracts.Metrics{
		Cpu:              totalCpu,
		Memory:           totalMemory,
		CpuConsumed:      totalCpuConsumed,
		MemoryConsumed:   totalMemoryConsumed,
		NodePoolCount:    totalNodePoolCount,
		NodeCount:        totalNodeCount,
		ClusterCount:     totalClusters,
		CpuPercentage:    totalCpuPercentage,
		MemoryPercentage: totalMemoryPercentage,
		PriceMonth:       totalPriceMonth,
		PriceYear:        totalPriceYear,
	}
}

func GetMetricItemFromPrimitivM(data primitive.M) apicontracts.Metric {
	totalCpu, err := mapping.InterfaceToInt64(data["totalCpu"])
	if err != nil {
		totalCpu = 0
	}

	totalClusters, err := mapping.InterfaceToInt64(data["totalClusterCount"])
	if err != nil {
		totalClusters = 0
	}

	totalCpuConsumed, err := mapping.InterfaceToInt64(data["totalCpuConsumed"])
	if err != nil {
		totalCpuConsumed = 0
	}

	totalMemory, err := mapping.InterfaceToInt64(data["totalMemory"])
	if err != nil {
		totalMemory = 0
	}

	totalMemoryConsumed, err := mapping.InterfaceToInt64(data["totalMemoryConsumed"])
	if err != nil {
		totalMemoryConsumed = 0
	}

	totalNodeCount, err := mapping.InterfaceToInt64(data["totalNodeCount"])
	if err != nil {
		totalNodeCount = 0
	}

	totalNodePoolCount, err := mapping.InterfaceToInt64(data["totalNodePoolCount"])
	if err != nil {
		totalNodePoolCount = 0
	}

	totalCpuPercentage, err := mapping.InterfaceToInt64(data["totalCpuPercentage"])
	if err != nil {
		totalCpuPercentage = 0
	}

	totalMemoryPercentage, err := mapping.InterfaceToInt64(data["totalMemoryPercentage"])
	if err != nil {
		totalMemoryPercentage = 0
	}

	totalPriceMonth, err := mapping.InterfaceToInt64(data["totalPriceMonth"])
	if err != nil {
		totalPriceMonth = 0
	}

	totalPriceYear, err := mapping.InterfaceToInt64(data["totalPriceYear"])
	if err != nil {
		totalPriceYear = 0
	}

	return apicontracts.Metric{
		Cpu:              totalCpu,
		Memory:           totalMemory,
		CpuConsumed:      totalCpuConsumed,
		MemoryConsumed:   totalMemoryConsumed,
		NodePoolCount:    totalNodePoolCount,
		NodeCount:        totalNodeCount,
		ClusterCount:     totalClusters,
		CpuPercentage:    totalCpuPercentage,
		MemoryPercentage: totalMemoryPercentage,
		PriceMonth:       totalPriceMonth,
		PriceYear:        totalPriceYear,
	}
}
