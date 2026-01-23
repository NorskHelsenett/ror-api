package mongodbseeding

import (
	"context"
	"errors"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/databases/mongodb/mongoTypes"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/kubernetes/providers/providermodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ErrCouldNotFindIdentifier = errors.New("could not find identifier")
	ErrCouldNotInsertSeed     = errors.New("could not insert seed")
	ErrCouldNotFindSeed       = errors.New("could find seed")
	ErrUnknownError           = errors.New("error getting existing entry, unknown error")
)

func CheckAndSeed(ctx context.Context) {
	seedPrices(ctx)

	if rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		seedDatacenters(ctx)
		seedAclv2Items(ctx)
		seedDevelopmentRulesets(ctx)
		seedApiKeys(ctx)
		seedProjects(ctx)
	}

	//seedInternalRuleset(ctx)
	seedTasks(ctx)
	seedOperatorConfigs(ctx)
}

// verifySeed will take a seed and a indentifier of the seed and attempt to find the object in the collection with the indentifer,
// if it fails to get a match with the identifier it will attempt to add the seed.
//
// There's no validating that the seed and identifier is connected to eacher.
//
// I want this function to generically handle any seed resource type and find it in the collection, if it couldn't it
// creates the resource, or returns any of the known error states.
func verifySeed[T any](ctx context.Context, collection *mongo.Collection, seed *T, identifier bson.M) error {

	var result *T
	err := collection.FindOne(ctx, identifier).Decode(&result)

	if err != nil {
		rlog.Infoc(ctx, "could not find entry, attempting to seed", rlog.String("collection_name", collection.Name()), rlog.Any("identifier", identifier))
	}

	if result != nil {
		rlog.Debugc(ctx, "found existing entry with identifier", rlog.String("collection_name", collection.Name()), rlog.Any("identifier", identifier))
		return nil
	}

	if err != mongo.ErrNoDocuments {
		rlog.Errorc(ctx, "unkown error, could not find entry", err, rlog.String("collection_name", collection.Name()), rlog.Any("identifier", identifier))
		return ErrUnknownError
	}

	_, err = collection.InsertOne(ctx, seed)
	if err != nil {
		rlog.Errorc(ctx, "could not insert seed", err, rlog.String("collection_name", collection.Name()))
		return ErrCouldNotInsertSeed
	}
	return nil
}

func seedDevelopmentRulesets(ctx context.Context) {
	db := mongodb.GetMongoDb()
	collection := db.Collection("messagerulesets")
	switchboardCount, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		rlog.Errorc(ctx, "could not check switchboard doc count", err)
		return
	}

	if switchboardCount != 0 {
		return
	}
}

func seedPrices(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("prices")

	mongoprices := []mongoTypes.MongoPrice{
		mongoTypes.NewMongoPrice(
			providermodels.ProviderTypeTanzu,
			"best-effort-medium",
			2,
			int64(8),
			int64(8238813184),
			900,
			time.Date(2022, 01, 01, 0, 0, 0, 0, time.UTC),
		),
		mongoTypes.NewMongoPrice(
			providermodels.ProviderTypeTanzu,
			"best-effort-large",
			4,
			int64(16),
			int64(16681451520),
			1800,
			time.Date(2022, 01, 01, 0, 0, 0, 0, time.UTC),
		),
		mongoTypes.NewMongoPrice(
			providermodels.ProviderTypeTanzu,
			"best-effort-xlarge",
			4,
			int64(32),
			int64(33567711232),
			2232,
			time.Date(2022, 01, 01, 0, 0, 0, 0, time.UTC),
		),
	}

	for _, mongoprice := range mongoprices {
		identifier := bson.M{"machineclass": mongoprice.MachineClass}
		err := verifySeed(ctx, collection, &mongoprice, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedDatacenters(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("datacenters")

	datacenters := []mongoTypes.MongoDatacenter{
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeUnknown,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "local",
			APIEndpoint: "localhost",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeK3d,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "local-k3d",
			APIEndpoint: "localhost",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeKind,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "local-kind",
			APIEndpoint: "localhost",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeTalos,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "local-talos",
			APIEndpoint: "localhost",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeTanzu,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "trd1",
			APIEndpoint: "ptr1-w02-cl01-api.sdi.nhn.no",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeTanzu,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "trd1-cl02",
			APIEndpoint: "ptr1-w02-cl02-api.sdi.nhn.no",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeTanzu,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Trøndelag",
			},
			Name:        "trd1cl02",
			APIEndpoint: "ptr1-w02-cl02-api.sdi.nhn.no",
		},
		{
			ID:       primitive.NewObjectID(),
			Provider: providermodels.ProviderTypeTanzu,
			Location: mongoTypes.MongoDatacenterLocation{
				Country: "Norway",
				Region:  "Oslo",
			},
			Name:        "osl1",
			APIEndpoint: "pos1-w02-cl01-api.sdi.nhn.no",
		},
	}

	for _, datacenter := range datacenters {
		identifier := bson.M{"name": datacenter.Name}
		err := verifySeed(ctx, collection, &datacenter, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedTasks(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("tasks")

	script1 := `#!/bin/bash
echo "task1"
helm repo add argo https://argoproj.github.io/argo-helm
helm install argocd argo/argo-cd --version $ARGOCD_VERSION --create-namespace -n argocd -f values.yaml
kubectl apply -f /app/rolebinding.yaml
`
	tasks := []apicontracts.Task{
		{
			Id:   primitive.NewObjectID(),
			Name: "argocd-installer",
			Config: apicontracts.TaskSpec{
				ImageName: "devops-base",
				Cmd:       "/app/entrypoint.sh",
				EnvVars: []apicontracts.KeyValue{
					{
						Key:   "ARGOCD_VERSION",
						Value: "5.55.0",
					},
				},
				BackOffLimit:     3,
				TimeOutInSeconds: 180,
				Version:          "1.0.0",
				Scripts: &apicontracts.TaskScripts{
					ScriptDirectory: "/scripts",
					FileNameAndData: []apicontracts.FileNameAndData{
						{
							FileName: "task1.sh",
							Data:     script1,
						},
						{
							FileName: "task2.sh",
							Data:     "echo 'task2'",
						},
					},
				},
				Secret: &apicontracts.TaskSecret{
					Path: "/data/",
					FileNameAndData: []apicontracts.FileNameAndData{
						{
							FileName: "values.yaml",
							Data:     "",
						},
					},
					GitSource: &apicontracts.TaskGitSource{
						Type:        apicontracts.Git,
						ContentPath: "config/config.yaml",
						GitConfig: apicontracts.GitConfig{
							Token:      "",
							User:       "",
							Repository: "https://helsegitlab.nhn.no/sdi/SDI-Infrastruktur/ror-jobs/argocd.git",
							Branch:     "feature/argocd-install",
							ProjectId:  413,
						},
					},
				},
			},
		},
		{
			Id:   primitive.NewObjectID(),
			Name: "cluster-agent-installer",
			Config: apicontracts.TaskSpec{
				ImageName:        "devops-base",
				Cmd:              "/app/entrypoint.sh",
				EnvVars:          make([]apicontracts.KeyValue, 0),
				BackOffLimit:     3,
				TimeOutInSeconds: 180,
				Version:          "1.0.0",
				Secret:           nil,
			},
		},
		{
			Id:   primitive.NewObjectID(),
			Name: "nhn-tooling-installer",
			Config: apicontracts.TaskSpec{
				ImageName:        "devops-base",
				Cmd:              "/app/entrypoint.sh",
				EnvVars:          make([]apicontracts.KeyValue, 0),
				BackOffLimit:     3,
				TimeOutInSeconds: 180,
				Version:          "1.0.0",
				Secret:           nil,
			},
		},
	}

	for _, task := range tasks {
		identifier := bson.M{"name": task.Name}
		err := verifySeed(ctx, collection, &task, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedOperatorConfigs(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("operatorconfigs")

	operatorConfigs := []mongoTypes.MongoOperatorConfig{
		{
			Id:         primitive.NewObjectID(),
			ApiVersion: "github.com/NorskHelsenett/ror/v1/config",
			Kind:       "ror-operator",
			Spec: &mongoTypes.MongoOperatorSpec{
				ImagePostfix: "ror-operator:0.0.1",
				Tasks: []mongoTypes.MongoOperatorTask{
					{
						Index:   0,
						Name:    "argocd-installer",
						Version: "0.0.1",
						RunOnce: true,
					},
					{
						Index:   1,
						Name:    "cluster-agent-installer",
						Version: "1.0.0",
						RunOnce: false,
					},
					{
						Index:   1,
						Name:    "nhn-tooling-installer",
						Version: "1.0.2",
						RunOnce: false,
					},
				},
			},
		},
	}

	for _, operatorConfig := range operatorConfigs {
		identifier := bson.M{"ApiVersion": "github.com/NorskHelsenett/ror/v1/config"}
		err := verifySeed(ctx, collection, &operatorConfig, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedAclv2Items(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("acl")

	aclv2items := []aclmodels.AclV2ListItem{
		*aclmodels.NewAclV2ListItem("A-T1-SDI-DevOps-Operators@ror.dev",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessAll(),
			true,
			"system@ror.dev",
		),

		*aclmodels.NewAclV2ListItem(
			"service-nhn@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2ScopeCluster),
			aclmodels.NewAclV2ListItemAccessEditor(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-audit@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessReadOnly(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-msswitchboard@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessContributor(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-mstanzu@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessOperator(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-tanzu-agent@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessOperator(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-mskind@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessOperator(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-msvulnerability@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessContributor(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-msslack@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessContributor(),
			false,
			"system@ror.dev",
		),
		*aclmodels.NewAclV2ListItem(
			"service-mstalos@ror.system",
			aclmodels.Acl2ScopeRor,
			aclmodels.Acl2Subject(aclmodels.Acl2RorSubjectGlobal),
			aclmodels.NewAclV2ListItemAccessOperator(),
			false,
			"system@ror.dev",
		),
	}

	for _, aclv2item := range aclv2items {
		identifier := bson.M{"group": aclv2item.Group}
		err := verifySeed(ctx, collection, &aclv2item, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedApiKeys(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("apikeys")

	apiKeys := []apicontracts.ApiKey{
		{
			Identifier:  "mstanzu",
			DisplayName: "mstanzu",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "246bd9a1958f8a52e5c31f0b832d2243a72d210472e3daa19449d21bed25664cbd4076864b5ea1c8732a4aeaf81d82c24653eb4cb593d6e8e876f6d2a996c629",
		},
		{
			Identifier:  "tanzu-agent",
			DisplayName: "Tanzu Agent",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "b410f9228c062580de16657e7ce715aa2b843ee46b0bc38e81c7bb3d21b7cc2b29527a4bb4c571b9f21aaedd4ece293bd1d245dc0fd05736af38dcc55785bebb",
		},
		{
			Identifier:  "mstalos",
			DisplayName: "mstalos",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "14f9a160e00b172f8da8ad487dac537b8399f3e5f7696d6d9e4d10a96d1cc9992fcbcf96579df0fa5323c0bb89f6dab875956899d822ddf695b6a6b592a21874",
		},
		{
			Identifier:  "mskind",
			DisplayName: "mskind",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "5b52ea5f512b1630efa24b9a86dbb23a6b97174220c262b0c6d6af11120149f06c1aae8afaeaedc4e1cf4a20cbf1ab81031df33a9af35573c1e5795b01b5f9d2",
		},
		{
			Identifier:  "msvulnerability",
			DisplayName: "msvulnerability",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "af0342b0a470675ab5a526b7a3db18faf3781cacafa82474bed940d9e35c3aa1f99fcff21da3b0fee7010962bb2722d5c6a65ace0eca871acbd61c586da6bb47",
		},
		{
			Identifier:  "msslack",
			DisplayName: "msslack",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "93e1613a8c9cbff6724a0935b81d2611a1afd8ebf42ffe0a4c529923baff5186d2a91d5ece0f348936e471181ca8f7228872c1014bf24623e53de66a53d040b1",
		},
		{
			Identifier:  "msswitchboard",
			DisplayName: "msswitchboard",
			Type:        "Service",
			ReadOnly:    false,
			Expires:     time.Time{},
			Created:     time.Now(),
			LastUsed:    time.Time{},
			Hash:        "dc9874d499431e92eb30f607b87e19efa3806c57344358f9bd392ba72ef5ffde80f4a942c3398bf8379ac0364bffba0ce24b9344ce183b4fd33807be9046d2fa",
		},
	}

	for _, apiKey := range apiKeys {
		identifier := bson.M{"identifier": apiKey.Identifier}
		err := verifySeed(ctx, collection, &apiKey, identifier)
		if err != nil {
			panic(err)
		}
	}
}

func seedProjects(ctx context.Context) {
	db := mongodb.GetMongoDb()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	collection := db.Collection("projects")

	roles := make([]mongoTypes.MongoProjectRole, 0, 2)
	roles = append(roles, mongoTypes.MongoProjectRole{
		ContactInfo: mongoTypes.MongoProjectContactInfo{
			UPN:   "p1@p1.no",
			Email: "p1@p1.no",
			Phone: "12345678",
		},
		RoleDefinition: apicontracts.ProjectRoleOwner,
	})
	roles = append(roles, mongoTypes.MongoProjectRole{
		ContactInfo: mongoTypes.MongoProjectContactInfo{
			UPN:   "p1@p1.no",
			Email: "p1@p1.no",
			Phone: "12345678",
		},
		RoleDefinition: apicontracts.ProjectRoleResponsible,
	})
	tags := map[string]string{}

	mongoProjects := []mongoTypes.MongoProject{
		{
			ID:          primitive.NewObjectID(),
			Name:        "Project 1",
			Description: "Project 1 description",
			Created:     time.Now(),
			Updated:     time.Now(),
			Active:      true,
			ProjectMetadata: mongoTypes.MongoProjectMetadata{
				Roles: roles,
				Billing: mongoTypes.MongoBilling{
					Workorder: "w-p1-123456",
				},
				ServiceTags: tags,
			},
		},
	}

	for _, mongoProject := range mongoProjects {
		identifier := bson.M{"name": mongoProject.Name}
		err := verifySeed(ctx, collection, &mongoProject, identifier)
		if err != nil {
			panic(err)
		}
	}
}
