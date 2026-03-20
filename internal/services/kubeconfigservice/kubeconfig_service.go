package kubeconfigservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/kubernetes/providers/providermodels"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"
)

var httpClient = newHTTPClient()

func newHTTPClient() http.Client {
	var transport http.RoundTripper
	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {
		transport = otelhttp.NewTransport(http.DefaultTransport)
	}
	return http.Client{Timeout: 55 * time.Second, Transport: transport}
}

func GetKubeconfig(ctx context.Context, cluster *apicontracts.Cluster, credentials apicontracts.KubeconfigCredentials) (string, error) {
	switch cluster.Workspace.Datacenter.Provider {
	case providermodels.ProviderTypeTanzu:
		return getKubeconfigForTanzuCluster(ctx, cluster, credentials)
	default:
		return "", fmt.Errorf("provider %s is not supported", cluster.Workspace.Datacenter.Provider)
	}
}

func GetKubeconfigForWorkspace(ctx context.Context, workspace *apicontracts.Workspace, credentials apicontracts.KubeconfigCredentials) (string, error) {
	switch workspace.Datacenter.Provider {
	case providermodels.ProviderTypeTanzu:
		return getKubeconfigForTanzuWorkspace(ctx, workspace, credentials)
	default:
		return "", fmt.Errorf("provider %s is not supported", workspace.Datacenter.Provider)
	}
}

func getKubeconfigForTanzuCluster(ctx context.Context, cluster *apicontracts.Cluster, credentials apicontracts.KubeconfigCredentials) (string, error) {
	creds := apicontracts.TanzuKubeConfigPayload{
		User:          credentials.Username,
		Password:      credentials.Password,
		DatacenterUrl: cluster.Workspace.Datacenter.APIEndpoint,
		WorkspaceName: cluster.Workspace.Name,
		ClusterName:   cluster.ClusterName,
		ClusterId:     cluster.ClusterId,
		WorkspaceOnly: false,
	}

	return getKubeconfig(ctx, creds)
}

func getKubeconfigForTanzuWorkspace(ctx context.Context, workspace *apicontracts.Workspace, credentials apicontracts.KubeconfigCredentials) (string, error) {
	creds := apicontracts.TanzuKubeConfigPayload{
		User:          credentials.Username,
		Password:      credentials.Password,
		DatacenterUrl: workspace.Datacenter.APIEndpoint,
		WorkspaceName: workspace.Name,
		ClusterName:   "",
		ClusterId:     "",
		WorkspaceOnly: true,
	}

	return getKubeconfig(ctx, creds)
}

func getKubeconfig(ctx context.Context, configPayload apicontracts.TanzuKubeConfigPayload) (string, error) {
	var payload bytes.Buffer
	err := json.NewEncoder(&payload).Encode(configPayload)
	if err != nil {
		rlog.Error("failed to encode payload", err)
		return "", err
	}

	serviceUrl := rorconfig.GetString("TANZU_AUTH_BASE_URL")
	httpposturl := fmt.Sprintf("%s/v1/kubeconfig", serviceUrl)
	request, err := http.NewRequestWithContext(ctx, "POST", httpposturl, &payload)
	if err != nil {
		rlog.Error("failed to create request", err)
		return "", err
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := httpClient.Do(request)
	if err != nil {
		rlog.Error("failed to get kubeconfig", err)
		return "", err
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			rlog.Error("Failed to close response body", closeErr)
		}
	}()

	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("failed to get kubeconfig, status code: %d", response.StatusCode)
		rlog.Error("error", err)
		return "", err
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		rlog.Error("failed to read response body", err)
		return "", err
	}

	return string(body), nil
}
